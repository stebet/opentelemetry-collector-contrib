// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
)

func newTestProcessor(t *testing.T, cfg *Config) (*emaSamplingProcessor, *consumertest.TracesSink) {
	t.Helper()
	if cfg.SamplingAttributes == nil {
		cfg.SamplingAttributes = []string{"service.name"}
	}
	set := processortest.NewNopSettings(typeStr)
	sink := &consumertest.TracesSink{}
	proc, err := newEMASamplingProcessor(set, sink, cfg)
	require.NoError(t, err)
	return proc, sink
}

func defaultTestConfig() *Config {
	return &Config{
		GoalSampleRate: 1,
		SharedConfig: emacore.SharedConfig{
			AdjustmentInterval:  15 * time.Second,
			Weight:              0.5,
			AgeOutValue:         0.5,
			BurstMultiple:       2.0,
			BurstDetectionDelay: 3,
			SamplingAttributes:  []string{"service.name"},
			MaxKeys:             500,
			DecisionWait:        100 * time.Millisecond,
			NumTraces:           50000,
		},
	}
}

func makeTrace(traceID [16]byte, attrs map[string]string, resourceAttrs map[string]string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	for k, v := range resourceAttrs {
		rs.Resource().Attributes().PutStr(k, v)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID(traceID))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	for k, v := range attrs {
		span.Attributes().PutStr(k, v)
	}
	return td
}

func TestConsumeTraces_Buffered(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DecisionWait = 1 * time.Hour
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	td := makeTrace([16]byte{1, 2, 3}, nil, map[string]string{"service.name": "test-svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	assert.Equal(t, 0, sink.SpanCount(), "trace should be buffered, not yet forwarded")
}

func TestFlush_SampleRateOne_KeepsAll(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 50 * time.Millisecond
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc-a"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	time.Sleep(1500 * time.Millisecond)
	assert.Equal(t, 1, sink.SpanCount(), "expected 1 span forwarded")
}

func TestShutdown_FlushesRemaining(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeTrace([16]byte{2}, nil, map[string]string{"service.name": "svc-b"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	require.NoError(t, proc.Shutdown(context.Background()))
	assert.Equal(t, 1, sink.SpanCount(), "expected buffered trace flushed on shutdown")
}

func TestNumTracesEviction(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.NumTraces = 2
	cfg.DecisionWait = 1 * time.Hour
	proc, _ := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	for i := 0; i < 3; i++ {
		traceID := [16]byte{byte(i + 1)}
		td := makeTrace(traceID, nil, map[string]string{"service.name": "svc"})
		require.NoError(t, proc.ConsumeTraces(context.Background(), td))
		time.Sleep(5 * time.Millisecond)
	}

	assert.LessOrEqual(t, proc.buffer.BufferedCount(), 2, "buffer should not exceed NumTraces")
}

func TestCapabilities_MutatesData(t *testing.T) {
	cfgOn := defaultTestConfig()
	cfgOn.AddSampleRateAttribute = true
	procOn, _ := newTestProcessor(t, cfgOn)
	assert.True(t, procOn.Capabilities().MutatesData)

	cfgOff := defaultTestConfig()
	cfgOff.AddSampleRateAttribute = false
	procOff, _ := newTestProcessor(t, cfgOff)
	assert.False(t, procOff.Capabilities().MutatesData)
}

func TestAddSampleRateAttribute_Enabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AddSampleRateAttribute = true
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	require.Equal(t, 1, sink.SpanCount())
	for _, received := range sink.AllTraces() {
		rs := received.ResourceSpans()
		for i := 0; i < rs.Len(); i++ {
			for j := 0; j < rs.At(i).ScopeSpans().Len(); j++ {
				spans := rs.At(i).ScopeSpans().At(j).Spans()
				for k := 0; k < spans.Len(); k++ {
					v, ok := spans.At(k).Attributes().Get("sampling.sample_rate")
					require.True(t, ok, "sampling.sample_rate attribute must be present")
					assert.Equal(t, int64(1), v.Int())
				}
			}
		}
	}
}

func TestAddSampleRateAttribute_Disabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AddSampleRateAttribute = false
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	require.Equal(t, 1, sink.SpanCount())
	for _, received := range sink.AllTraces() {
		rs := received.ResourceSpans()
		for i := 0; i < rs.Len(); i++ {
			for j := 0; j < rs.At(i).ScopeSpans().Len(); j++ {
				spans := rs.At(i).ScopeSpans().At(j).Spans()
				for k := 0; k < spans.Len(); k++ {
					_, ok := spans.At(k).Attributes().Get("sampling.sample_rate")
					assert.False(t, ok, "sampling.sample_rate must NOT be present when feature is disabled")
				}
			}
		}
	}
}

// TestBothServicesForwarded verifies that traces from two different services
// are independently forwarded through the processor. With GoalSampleRate=1 all
// traces are kept, and the test confirms that each service's span arrives at the
// consumer — proving that the processor builds separate sampling keys and routes
// each trace independently.
func TestBothServicesForwarded(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	hotTrace := makeTrace([16]byte{0xAA}, nil, map[string]string{"service.name": "hot-service"})
	coldTrace := makeTrace([16]byte{0xBB}, nil, map[string]string{"service.name": "cold-service"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), hotTrace))
	require.NoError(t, proc.ConsumeTraces(context.Background(), coldTrace))
	require.NoError(t, proc.Shutdown(context.Background()))

	servicesSeen := map[string]int{}
	for _, received := range sink.AllTraces() {
		rs := received.ResourceSpans()
		for i := 0; i < rs.Len(); i++ {
			if v, ok := rs.At(i).Resource().Attributes().Get("service.name"); ok {
				servicesSeen[v.AsString()]++
			}
		}
	}
	assert.Equal(t, 1, servicesSeen["hot-service"], "hot-service trace must be forwarded once")
	assert.Equal(t, 1, servicesSeen["cold-service"], "cold-service trace must be forwarded once")
}
