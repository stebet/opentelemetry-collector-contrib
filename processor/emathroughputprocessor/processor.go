// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emathroughputprocessor"

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

// typeStr is the component type identifier registered with the OTel collector.
var typeStr = component.MustNewType("ema_throughput")

// samplingKeySeparator is the delimiter used when joining sampling attribute values.
const samplingKeySeparator = "\u2022" // bullet character •

// traceData holds the buffered spans for a single trace.
type traceData struct {
	arrivedAt   time.Time
	accumulated ptrace.Traces
	spanCount   int64
}

// emaThroughputProcessor is a tail sampling processor that uses the EMA
// Throughput algorithm to dynamically adjust sampling rates so the total
// number of forwarded traces per second stays near a configured goal.
type emaThroughputProcessor struct {
	logger       *zap.Logger
	cfg          *Config
	nextConsumer consumer.Traces

	sampler *dynsampler.EMAThroughput

	mu     sync.Mutex
	traces map[pcommon.TraceID]*traceData

	stopCh chan struct{}
	wg     sync.WaitGroup
}

var _ processor.Traces = (*emaThroughputProcessor)(nil)

func newEMAThroughputProcessor(
	set processor.Settings,
	nextConsumer consumer.Traces,
	cfg *Config,
) (*emaThroughputProcessor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ema_throughput config: %w", err)
	}

	maxKeys := cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}

	sampler := &dynsampler.EMAThroughput{
		GoalThroughputPerSec: cfg.GoalThroughputPerSec,
		InitialSampleRate:    cfg.InitialSampleRate,
		AdjustmentInterval:   cfg.AdjustmentInterval,
		Weight:               cfg.Weight,
		AgeOutValue:          cfg.AgeOutValue,
		BurstMultiple:        cfg.BurstMultiple,
		BurstDetectionDelay:  cfg.BurstDetectionDelay,
		MaxKeys:              maxKeys,
	}

	return &emaThroughputProcessor{
		logger:       set.Logger,
		cfg:          cfg,
		nextConsumer: nextConsumer,
		sampler:      sampler,
		traces:       make(map[pcommon.TraceID]*traceData),
		stopCh:       make(chan struct{}),
	}, nil
}

// Start initialises the EMA sampler and begins the background flush loop.
func (p *emaThroughputProcessor) Start(_ context.Context, _ component.Host) error {
	if err := p.sampler.Start(); err != nil {
		return fmt.Errorf("failed to start EMA throughput sampler: %w", err)
	}

	p.wg.Add(1)
	go p.flushLoop()
	return nil
}

// Shutdown stops the background loop and flushes any remaining buffered traces.
func (p *emaThroughputProcessor) Shutdown(ctx context.Context) error {
	close(p.stopCh)
	p.wg.Wait()

	p.mu.Lock()
	remaining := p.traces
	p.traces = make(map[pcommon.TraceID]*traceData)
	p.mu.Unlock()

	for id, td := range remaining {
		p.decide(ctx, id, td)
	}

	p.sampler.Stop()
	return nil
}

// Capabilities returns the capabilities of this processor.
func (p *emaThroughputProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: p.cfg.AddSampleRateAttribute}
}

// ConsumeTraces buffers incoming spans grouped by trace ID.
func (p *emaThroughputProcessor) ConsumeTraces(ctx context.Context, incoming ptrace.Traces) error {
	now := time.Now()

	p.mu.Lock()

	inRS := incoming.ResourceSpans()
	for i := 0; i < inRS.Len(); i++ {
		inResourceSpan := inRS.At(i)
		inSS := inResourceSpan.ScopeSpans()

		for j := 0; j < inSS.Len(); j++ {
			inScopeSpan := inSS.At(j)
			inSpans := inScopeSpan.Spans()

			byTrace := make(map[pcommon.TraceID][]int)
			for k := 0; k < inSpans.Len(); k++ {
				tid := inSpans.At(k).TraceID()
				byTrace[tid] = append(byTrace[tid], k)
			}

			for tid, indices := range byTrace {
				entry, exists := p.traces[tid]
				if !exists {
					entry = &traceData{
						arrivedAt:   now,
						accumulated: ptrace.NewTraces(),
					}
					p.traces[tid] = entry
				}

				outRS := entry.accumulated.ResourceSpans().AppendEmpty()
				inResourceSpan.Resource().CopyTo(outRS.Resource())
				outRS.SetSchemaUrl(inResourceSpan.SchemaUrl())
				outSS := outRS.ScopeSpans().AppendEmpty()
				inScopeSpan.Scope().CopyTo(outSS.Scope())
				outSS.SetSchemaUrl(inScopeSpan.SchemaUrl())

				for _, k := range indices {
					inSpans.At(k).CopyTo(outSS.Spans().AppendEmpty())
					entry.spanCount++
				}
			}
		}
	}

	evicted := p.drainOldest()
	p.mu.Unlock()

	for id, td := range evicted {
		p.decide(ctx, id, td)
	}

	return nil
}

// drainOldest removes trace entries until len(p.traces) <= NumTraces.
// Must be called with p.mu held.
func (p *emaThroughputProcessor) drainOldest() map[pcommon.TraceID]*traceData {
	if uint64(len(p.traces)) <= p.cfg.NumTraces {
		return nil
	}
	evicted := make(map[pcommon.TraceID]*traceData)
	for uint64(len(p.traces)) > p.cfg.NumTraces {
		var oldestID pcommon.TraceID
		var oldestTime time.Time
		first := true
		for id, td := range p.traces {
			if first || td.arrivedAt.Before(oldestTime) {
				oldestID = id
				oldestTime = td.arrivedAt
				first = false
			}
		}
		evicted[oldestID] = p.traces[oldestID]
		delete(p.traces, oldestID)
	}
	return evicted
}

// flushLoop runs in a goroutine, periodically flushing traces that have
// waited longer than DecisionWait.
func (p *emaThroughputProcessor) flushLoop() {
	defer p.wg.Done()

	tickInterval := p.cfg.DecisionWait / 4
	if tickInterval < time.Second {
		tickInterval = time.Second
	}
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.flush(context.Background())
		case <-p.stopCh:
			return
		}
	}
}

// flush evaluates all traces that have waited longer than DecisionWait.
func (p *emaThroughputProcessor) flush(ctx context.Context) {
	cutoff := time.Now().Add(-p.cfg.DecisionWait)

	p.mu.Lock()
	var ready []struct {
		id pcommon.TraceID
		td *traceData
	}
	for id, td := range p.traces {
		if td.arrivedAt.Before(cutoff) {
			ready = append(ready, struct {
				id pcommon.TraceID
				td *traceData
			}{id, td})
			delete(p.traces, id)
		}
	}
	p.mu.Unlock()

	for _, item := range ready {
		p.decide(ctx, item.id, item.td)
	}
}

// decide makes a sampling decision for a single trace and, if kept, forwards
// its accumulated spans to the next consumer.
func (p *emaThroughputProcessor) decide(ctx context.Context, _ pcommon.TraceID, td *traceData) {
	key := p.buildSamplingKey(td)
	count := int(td.spanCount)
	rate := p.sampler.GetSampleRateMulti(key, count)
	if rate < 1 {
		rate = 1
	}

	keep := rand.Intn(rate) == 0 //nolint:gosec // non-cryptographic sampling

	if !keep && p.cfg.AlwaysSampleErrors && p.traceHasErrors(td) {
		keep = true
		rate = 1
	}

	if !keep {
		p.logger.Debug("dropping trace",
			zap.String("key", key),
			zap.Int("sample_rate", rate),
			zap.Int64("span_count", td.spanCount),
		)
		return
	}

	p.logger.Debug("keeping trace",
		zap.String("key", key),
		zap.Int("sample_rate", rate),
		zap.Int64("span_count", td.spanCount),
	)

	if p.cfg.AddSampleRateAttribute {
		p.addSampleRateAttr(td, rate)
	}

	if err := p.nextConsumer.ConsumeTraces(ctx, td.accumulated); err != nil {
		p.logger.Error("failed to forward sampled trace", zap.Error(err))
	}
}

// traceHasErrors returns true if any span in the trace has StatusCode ==
// StatusCodeError or contains an event named "exception".
func (p *emaThroughputProcessor) traceHasErrors(td *traceData) bool {
	rs := td.accumulated.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if span.Status().Code() == ptrace.StatusCodeError {
					return true
				}
				events := span.Events()
				for e := 0; e < events.Len(); e++ {
					if events.At(e).Name() == "exception" {
						return true
					}
				}
			}
		}
	}
	return false
}

// addSampleRateAttr stamps every span in the trace with the integer attribute
// "sampling.sample_rate" set to the given rate value.
func (p *emaThroughputProcessor) addSampleRateAttr(td *traceData, rate int) {
	rs := td.accumulated.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		scopeSpans := rs.At(i).ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				spans.At(k).Attributes().PutInt("sampling.sample_rate", int64(rate))
			}
		}
	}
}

// buildSamplingKey constructs a sampling key for a trace by joining the first
// value found for each configured sampling attribute across all spans and
// resource attributes in the trace.
func (p *emaThroughputProcessor) buildSamplingKey(td *traceData) string {
	attrs := p.cfg.SamplingAttributes
	values := make([]string, len(attrs))

	for i, attr := range attrs {
		values[i] = p.findAttributeValue(td, attr)
	}

	key := strings.Join(values, samplingKeySeparator)
	if p.cfg.UseTraceLength {
		key = fmt.Sprintf("%s%s%d", key, samplingKeySeparator, td.spanCount)
	}
	return key
}

// findAttributeValue returns the first value found for the given attribute key
// across all resource and span attributes in the trace's accumulated data.
func (p *emaThroughputProcessor) findAttributeValue(td *traceData, attr string) string {
	rs := td.accumulated.ResourceSpans()
	for i := 0; i < rs.Len(); i++ {
		resourceSpan := rs.At(i)

		if v, ok := resourceSpan.Resource().Attributes().Get(attr); ok {
			return v.AsString()
		}

		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				if v, ok := spans.At(k).Attributes().Get(attr); ok {
					return v.AsString()
				}
			}
		}
	}
	return ""
}
