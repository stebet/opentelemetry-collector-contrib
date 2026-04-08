// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emacore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// mockSampler is a simple Sampler that always returns rate 1 (keep all).
type mockSampler struct {
	rate int
}

func (m *mockSampler) Start() error                              { return nil }
func (m *mockSampler) Stop() error                               { return nil }
func (m *mockSampler) GetSampleRateMulti(_ string, _ int) int   { return m.rate }

func makeBuffer(t *testing.T, cfg *SharedConfig, sink *consumertest.TracesSink) *TraceBuffer {
	t.Helper()
	sampler := &mockSampler{rate: 1}
	buf := NewTraceBuffer(zap.NewNop(), cfg, sampler, sink)
	require.NoError(t, buf.Start())
	return buf
}

func defaultConfig() *SharedConfig {
	return &SharedConfig{
		SamplingAttributes: []string{"service.name"},
		Weight:             0.5,
		AgeOutValue:        0.5,
		BurstMultiple:      2.0,
		BurstDetectionDelay: 3,
		MaxKeys:            500,
		DecisionWait:       100 * time.Millisecond,
		NumTraces:          50000,
	}
}

func makeTrace(traceID [16]byte, spanAttrs map[string]string, resourceAttrs map[string]string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	for k, v := range resourceAttrs {
		rs.Resource().Attributes().PutStr(k, v)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID(traceID))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	for k, v := range spanAttrs {
		span.Attributes().PutStr(k, v)
	}
	return td
}

func makeMultiTracesBatch(traceID1, traceID2 [16]byte) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "mixed-svc")
	ss := rs.ScopeSpans().AppendEmpty()

	span1 := ss.Spans().AppendEmpty()
	span1.SetTraceID(pcommon.TraceID(traceID1))
	span1.SetSpanID(pcommon.SpanID([8]byte{1}))

	span2 := ss.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID(traceID2))
	span2.SetSpanID(pcommon.SpanID([8]byte{2}))

	return td
}

func TestConsumeTraces_Buffered(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)
	defer func() { _ = buf.Shutdown(context.Background()) }()

	td := makeTrace([16]byte{1, 2, 3}, nil, map[string]string{"service.name": "test-svc"})
	require.NoError(t, buf.ConsumeTraces(context.Background(), td))

	assert.Equal(t, 0, sink.SpanCount(), "trace should be buffered, not yet forwarded")
}

func TestFlush_KeepsAll(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 50 * time.Millisecond
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)
	defer func() { _ = buf.Shutdown(context.Background()) }()

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc-a"})
	require.NoError(t, buf.ConsumeTraces(context.Background(), td))

	time.Sleep(1500 * time.Millisecond)
	assert.Equal(t, 1, sink.SpanCount(), "expected 1 span forwarded")
}

func TestShutdown_FlushesRemaining(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)

	td := makeTrace([16]byte{2}, nil, map[string]string{"service.name": "svc-b"})
	require.NoError(t, buf.ConsumeTraces(context.Background(), td))

	require.NoError(t, buf.Shutdown(context.Background()))
	assert.Equal(t, 1, sink.SpanCount(), "expected buffered trace flushed on shutdown")
}

func TestConsumeTraces_SplitsByTraceID(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)

	traceID1 := [16]byte{0xAA}
	traceID2 := [16]byte{0xBB}
	batch := makeMultiTracesBatch(traceID1, traceID2)

	require.NoError(t, buf.ConsumeTraces(context.Background(), batch))

	assert.Equal(t, 2, buf.BufferedCount(), "both traces should be buffered separately")

	require.NoError(t, buf.Shutdown(context.Background()))
	// Both traces should have been forwarded with 1 span each.
	assert.Equal(t, 2, sink.SpanCount(), "both spans should be forwarded")
}

// TestConsumeTraces_NoTraceMixing verifies the critical invariant that when
// two trace IDs arrive in the same ConsumeTraces call, the trace buffer stores
// each trace ID's spans independently — no span from one trace ID leaks into
// the accumulated batch of another trace ID.
func TestConsumeTraces_NoTraceMixing(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)

	traceID1 := pcommon.TraceID([16]byte{0x11})
	traceID2 := pcommon.TraceID([16]byte{0x22})

	// Both trace IDs in a single ConsumeTraces call.
	batch := ptrace.NewTraces()
	rs := batch.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "mixed-svc")
	ss := rs.ScopeSpans().AppendEmpty()

	span1 := ss.Spans().AppendEmpty()
	span1.SetTraceID(traceID1)
	span1.SetSpanID(pcommon.SpanID([8]byte{0x01}))
	span1.SetName("span-for-trace-1")

	span2 := ss.Spans().AppendEmpty()
	span2.SetTraceID(traceID2)
	span2.SetSpanID(pcommon.SpanID([8]byte{0x02}))
	span2.SetName("span-for-trace-2")

	require.NoError(t, buf.ConsumeTraces(context.Background(), batch))
	require.Equal(t, 2, buf.BufferedCount())

	// Flush everything.
	require.NoError(t, buf.Shutdown(context.Background()))
	require.Equal(t, 2, sink.SpanCount(), "one span per trace ID must be forwarded")

	// Verify that each forwarded ptrace.Traces batch contains ONLY spans
	// belonging to a single trace ID (no cross-contamination).
	for _, received := range sink.AllTraces() {
		seenTraceIDs := map[pcommon.TraceID]struct{}{}
		rs := received.ResourceSpans()
		for i := 0; i < rs.Len(); i++ {
			for j := 0; j < rs.At(i).ScopeSpans().Len(); j++ {
				spans := rs.At(i).ScopeSpans().At(j).Spans()
				for k := 0; k < spans.Len(); k++ {
					seenTraceIDs[spans.At(k).TraceID()] = struct{}{}
				}
			}
		}
		assert.Lenf(t, seenTraceIDs, 1,
			"each forwarded batch must contain spans from exactly one trace ID, got: %v", seenTraceIDs)
	}
}

func TestNumTracesEviction(t *testing.T) {
	cfg := defaultConfig()
	cfg.NumTraces = 2
	cfg.DecisionWait = 1 * time.Hour
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)
	defer func() { _ = buf.Shutdown(context.Background()) }()

	for i := 0; i < 3; i++ {
		traceID := [16]byte{byte(i + 1)}
		td := makeTrace(traceID, nil, map[string]string{"service.name": "svc"})
		require.NoError(t, buf.ConsumeTraces(context.Background(), td))
		time.Sleep(5 * time.Millisecond)
	}

	assert.LessOrEqual(t, buf.BufferedCount(), 2, "buffer should not exceed NumTraces")
}

func TestBuildKey_SingleAttr(t *testing.T) {
	traces := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "checkout"})
	key := BuildKey(traces, 3, []string{"service.name"}, false)
	assert.Equal(t, "checkout", key)
}

func TestBuildKey_UseTraceLength(t *testing.T) {
	traces := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "cart"})
	key := BuildKey(traces, 5, []string{"service.name"}, true)
	assert.Equal(t, "cart"+samplingKeySeparator+"5", key)
}

func TestBuildKey_SpanAttrFallback(t *testing.T) {
	traces := makeTrace([16]byte{1}, map[string]string{"http.method": "GET"}, nil)
	key := BuildKey(traces, 1, []string{"http.method"}, false)
	assert.Equal(t, "GET", key)
}

func TestBuildKey_MissingAttr(t *testing.T) {
	traces := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	key := BuildKey(traces, 1, []string{"nonexistent.attr"}, false)
	assert.Equal(t, "", key)
}

func TestBuildKey_MultipleAttributes_ProducesDistinctKeys(t *testing.T) {
	makeMultiAttrTrace := func(traceID [16]byte, service, method string) ptrace.Traces {
		return makeTrace(traceID,
			map[string]string{"http.method": method},
			map[string]string{"service.name": service},
		)
	}

	tracesA := makeMultiAttrTrace([16]byte{1}, "checkout", "POST")
	tracesB := makeMultiAttrTrace([16]byte{2}, "checkout", "GET")
	tracesC := makeMultiAttrTrace([16]byte{3}, "payment", "POST")
	tracesD := makeMultiAttrTrace([16]byte{4}, "payment", "GET")

	attrs := []string{"service.name", "http.method"}
	keyA := BuildKey(tracesA, 1, attrs, false)
	keyB := BuildKey(tracesB, 1, attrs, false)
	keyC := BuildKey(tracesC, 1, attrs, false)
	keyD := BuildKey(tracesD, 1, attrs, false)

	keys := []string{keyA, keyB, keyC, keyD}
	for i := range keys {
		for j := range keys {
			if i != j {
				assert.NotEqualf(t, keys[i], keys[j],
					"key[%d]=%q should differ from key[%d]=%q", i, keys[i], j, keys[j])
			}
		}
	}

	for _, k := range keys {
		assert.Contains(t, k, samplingKeySeparator)
	}
}

func TestCapabilities_MutatesData(t *testing.T) {
	sink := &consumertest.TracesSink{}

	cfgOn := defaultConfig()
	cfgOn.AddSampleRateAttribute = true
	bufOn := NewTraceBuffer(zap.NewNop(), cfgOn, &mockSampler{rate: 1}, sink)
	assert.True(t, bufOn.Capabilities().MutatesData)

	cfgOff := defaultConfig()
	cfgOff.AddSampleRateAttribute = false
	bufOff := NewTraceBuffer(zap.NewNop(), cfgOff, &mockSampler{rate: 1}, sink)
	assert.False(t, bufOff.Capabilities().MutatesData)
}

func TestAddSampleRateAttribute_Enabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	cfg.AddSampleRateAttribute = true
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	require.NoError(t, buf.ConsumeTraces(context.Background(), td))
	require.NoError(t, buf.Shutdown(context.Background()))

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
	cfg := defaultConfig()
	cfg.DecisionWait = 1 * time.Hour
	cfg.AddSampleRateAttribute = false
	sink := &consumertest.TracesSink{}
	buf := makeBuffer(t, cfg, sink)

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	require.NoError(t, buf.ConsumeTraces(context.Background(), td))
	require.NoError(t, buf.Shutdown(context.Background()))

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
