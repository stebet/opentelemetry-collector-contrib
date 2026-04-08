// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emacore // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const samplingKeySeparator = "\u2022"

// traceData holds the buffered spans for a single trace.
type traceData struct {
	arrivedAt   time.Time
	accumulated ptrace.Traces
	spanCount   int64
}

// TraceBuffer handles buffering, flushing, and sampling decisions for incoming traces.
type TraceBuffer struct {
	logger       *zap.Logger
	cfg          *SharedConfig
	sampler      Sampler
	nextConsumer consumer.Traces

	mu     sync.Mutex
	traces map[pcommon.TraceID]*traceData

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewTraceBuffer creates a new TraceBuffer. Call Start() before using it.
func NewTraceBuffer(logger *zap.Logger, cfg *SharedConfig, sampler Sampler, next consumer.Traces) *TraceBuffer {
	return &TraceBuffer{
		logger:       logger,
		cfg:          cfg,
		sampler:      sampler,
		nextConsumer: next,
		traces:       make(map[pcommon.TraceID]*traceData),
		stopCh:       make(chan struct{}),
	}
}

// Start initialises the sampler and begins the background flush loop.
func (b *TraceBuffer) Start() error {
	if err := b.sampler.Start(); err != nil {
		return fmt.Errorf("failed to start EMA sampler: %w", err)
	}
	b.wg.Add(1)
	go b.flushLoop()
	return nil
}

// Shutdown stops the background loop, flushes remaining traces, and stops the sampler.
func (b *TraceBuffer) Shutdown(ctx context.Context) error {
	close(b.stopCh)
	b.wg.Wait()

	b.mu.Lock()
	remaining := b.traces
	b.traces = make(map[pcommon.TraceID]*traceData)
	b.mu.Unlock()

	for id, td := range remaining {
		b.decide(ctx, id, td)
	}
	b.sampler.Stop() //nolint:errcheck
	return nil
}

// Capabilities returns processor capabilities based on config.
func (b *TraceBuffer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: b.cfg.AddSampleRateAttribute}
}

// Sampler returns the underlying Sampler for test inspection.
func (b *TraceBuffer) Sampler() Sampler {
	return b.sampler
}

// BufferedCount returns the number of traces currently held in the buffer.
func (b *TraceBuffer) BufferedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.traces)
}

// ConsumeTraces buffers incoming spans grouped by trace ID.
func (b *TraceBuffer) ConsumeTraces(ctx context.Context, incoming ptrace.Traces) error {
	now := time.Now()

	b.mu.Lock()

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
				entry, exists := b.traces[tid]
				if !exists {
					entry = &traceData{
						arrivedAt:   now,
						accumulated: ptrace.NewTraces(),
					}
					b.traces[tid] = entry
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

	evicted := b.drainOldest()
	b.mu.Unlock()

	for id, td := range evicted {
		b.decide(ctx, id, td)
	}
	return nil
}

func (b *TraceBuffer) drainOldest() map[pcommon.TraceID]*traceData {
	if uint64(len(b.traces)) <= b.cfg.NumTraces {
		return nil
	}
	evicted := make(map[pcommon.TraceID]*traceData)
	for uint64(len(b.traces)) > b.cfg.NumTraces {
		var oldestID pcommon.TraceID
		var oldestTime time.Time
		first := true
		for id, td := range b.traces {
			if first || td.arrivedAt.Before(oldestTime) {
				oldestID = id
				oldestTime = td.arrivedAt
				first = false
			}
		}
		evicted[oldestID] = b.traces[oldestID]
		delete(b.traces, oldestID)
	}
	return evicted
}

func (b *TraceBuffer) flushLoop() {
	defer b.wg.Done()
	tickInterval := b.cfg.DecisionWait / 4
	if tickInterval < time.Second {
		tickInterval = time.Second
	}
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.flush(context.Background())
		case <-b.stopCh:
			return
		}
	}
}

func (b *TraceBuffer) flush(ctx context.Context) {
	cutoff := time.Now().Add(-b.cfg.DecisionWait)
	b.mu.Lock()
	var ready []struct {
		id pcommon.TraceID
		td *traceData
	}
	for id, td := range b.traces {
		if td.arrivedAt.Before(cutoff) {
			ready = append(ready, struct {
				id pcommon.TraceID
				td *traceData
			}{id, td})
			delete(b.traces, id)
		}
	}
	b.mu.Unlock()
	for _, item := range ready {
		b.decide(ctx, item.id, item.td)
	}
}

func (b *TraceBuffer) decide(ctx context.Context, _ pcommon.TraceID, td *traceData) {
	key := b.buildSamplingKey(td)
	count := int(td.spanCount)
	rate := b.sampler.GetSampleRateMulti(key, count)
	if rate < 1 {
		rate = 1
	}
	keep := rand.Intn(rate) == 0 //nolint:gosec

	if !keep {
		b.logger.Debug("dropping trace",
			zap.String("key", key),
			zap.Int("sample_rate", rate),
			zap.Int64("span_count", td.spanCount),
		)
		return
	}

	b.logger.Debug("keeping trace",
		zap.String("key", key),
		zap.Int("sample_rate", rate),
		zap.Int64("span_count", td.spanCount),
	)

	if b.cfg.AddSampleRateAttribute {
		addSampleRateAttr(td.accumulated, rate)
	}

	if err := b.nextConsumer.ConsumeTraces(ctx, td.accumulated); err != nil {
		b.logger.Error("failed to forward sampled trace", zap.Error(err))
	}
}

func (b *TraceBuffer) buildSamplingKey(td *traceData) string {
	return BuildKey(td.accumulated, td.spanCount, b.cfg.SamplingAttributes, b.cfg.UseTraceLength)
}

// BuildKey constructs a sampling key from a ptrace.Traces value and config parameters.
// Exported so extension evaluators can share the same key-building logic.
func BuildKey(traces ptrace.Traces, spanCount int64, attrs []string, useTraceLength bool) string {
	values := make([]string, len(attrs))
	for i, attr := range attrs {
		values[i] = findAttributeValue(traces, attr)
	}
	key := strings.Join(values, samplingKeySeparator)
	if useTraceLength {
		key = fmt.Sprintf("%s%s%d", key, samplingKeySeparator, spanCount)
	}
	return key
}

func findAttributeValue(traces ptrace.Traces, attr string) string {
	rs := traces.ResourceSpans()
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

func addSampleRateAttr(traces ptrace.Traces, rate int) {
	rs := traces.ResourceSpans()
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
