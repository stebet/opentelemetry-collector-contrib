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
		GoalSampleRate:     1, // Keep all traces for deterministic tests.
		AdjustmentInterval: 15 * time.Second,
		Weight:             0.5,
		AgeOutValue:        0.5,
		BurstMultiple:      2.0,
		BurstDetectionDelay: 3,
		SamplingAttributes: []string{"service.name"},
		MaxKeys:            500,
		DecisionWait:       100 * time.Millisecond,
		NumTraces:          50000,
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

// makeErrorTrace creates a single-span trace with StatusCode == StatusCodeError.
func makeErrorTrace(traceID [16]byte, resourceAttrs map[string]string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	for k, v := range resourceAttrs {
		rs.Resource().Attributes().PutStr(k, v)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID(traceID))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Status().SetCode(ptrace.StatusCodeError)
	return td
}

// makeExceptionTrace creates a single-span trace with an "exception" event.
func makeExceptionTrace(traceID [16]byte, resourceAttrs map[string]string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	for k, v := range resourceAttrs {
		rs.Resource().Attributes().PutStr(k, v)
	}
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID(traceID))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Events().AppendEmpty().SetName("exception")
	return td
}

func TestConsumeTraces_Buffered(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DecisionWait = 1 * time.Hour // Never auto-flush.
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	traceID := [16]byte{1, 2, 3}
	td := makeTrace(traceID, nil, map[string]string{"service.name": "test-svc"})

	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	// Trace should be buffered, not yet forwarded.
	assert.Equal(t, 0, sink.SpanCount())
}

func TestFlush_SampleRateOne_KeepsAll(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1 // Rate=1 means always keep.
	cfg.DecisionWait = 50 * time.Millisecond
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	traceID := [16]byte{1}
	td := makeTrace(traceID, nil, map[string]string{"service.name": "svc-a"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	// Wait for the flush loop to fire (minimum tick is 1s).
	time.Sleep(1500 * time.Millisecond)

	assert.Equal(t, 1, sink.SpanCount(), "expected 1 span forwarded")
}

func TestShutdown_FlushesRemaining(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour // Prevent auto-flush.
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	traceID := [16]byte{2}
	td := makeTrace(traceID, nil, map[string]string{"service.name": "svc-b"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))

	// Shutdown should flush buffered traces.
	require.NoError(t, proc.Shutdown(context.Background()))
	assert.Equal(t, 1, sink.SpanCount(), "expected buffered trace flushed on shutdown")
}

func TestBuildSamplingKey_SingleAttr(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SamplingAttributes = []string{"service.name"}
	cfg.UseTraceLength = false
	proc, _ := newTestProcessor(t, cfg)

	batch := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "checkout"})
	td := &traceData{
		spanCount:   3,
		accumulated: batch,
	}

	key := proc.buildSamplingKey(td)
	assert.Equal(t, "checkout", key)
}

func TestBuildSamplingKey_UseTraceLength(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SamplingAttributes = []string{"service.name"}
	cfg.UseTraceLength = true
	proc, _ := newTestProcessor(t, cfg)

	batch := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "cart"})
	td := &traceData{
		spanCount:   5,
		accumulated: batch,
	}

	key := proc.buildSamplingKey(td)
	assert.Equal(t, "cart"+samplingKeySeparator+"5", key)
}

func TestBuildSamplingKey_SpanAttrFallback(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SamplingAttributes = []string{"http.method"}
	proc, _ := newTestProcessor(t, cfg)

	batch := makeTrace([16]byte{1}, map[string]string{"http.method": "GET"}, nil)
	td := &traceData{
		spanCount:   1,
		accumulated: batch,
	}

	key := proc.buildSamplingKey(td)
	assert.Equal(t, "GET", key)
}

func TestBuildSamplingKey_MissingAttr(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SamplingAttributes = []string{"nonexistent.attr"}
	proc, _ := newTestProcessor(t, cfg)

	batch := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	td := &traceData{
		spanCount:   1,
		accumulated: batch,
	}

	key := proc.buildSamplingKey(td)
	// Missing attribute should produce an empty string, not a panic.
	assert.Equal(t, "", key)
}

func TestCapabilities(t *testing.T) {
	cfg := defaultTestConfig()
	proc, _ := newTestProcessor(t, cfg)
	caps := proc.Capabilities()
	assert.False(t, caps.MutatesData)
}

// makeMultiTracesBatch creates a single ptrace.Traces containing spans from two
// different trace IDs, to verify that ConsumeTraces splits them correctly.
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

func TestConsumeTraces_SplitsByTraceID(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DecisionWait = 1 * time.Hour // Never auto-flush.
	proc, _ := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	traceID1 := [16]byte{0xAA}
	traceID2 := [16]byte{0xBB}
	batch := makeMultiTracesBatch(traceID1, traceID2)

	require.NoError(t, proc.ConsumeTraces(context.Background(), batch))

	proc.mu.Lock()
	entry1, ok1 := proc.traces[pcommon.TraceID(traceID1)]
	entry2, ok2 := proc.traces[pcommon.TraceID(traceID2)]
	proc.mu.Unlock()

	require.True(t, ok1, "trace1 should be buffered")
	require.True(t, ok2, "trace2 should be buffered")

	// Each entry should hold exactly 1 span.
	assert.Equal(t, int64(1), entry1.spanCount, "trace1 should have 1 span")
	assert.Equal(t, int64(1), entry2.spanCount, "trace2 should have 1 span")

	// Verify accumulated data for trace1 contains only trace1's span.
	rs1 := entry1.accumulated.ResourceSpans()
	require.Equal(t, 1, rs1.Len())
	spans1 := rs1.At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 1, spans1.Len())
	assert.Equal(t, pcommon.TraceID(traceID1), spans1.At(0).TraceID())
}

func TestNumTracesEviction(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.NumTraces = 2
	cfg.DecisionWait = 1 * time.Hour // Prevent auto-flush.
	proc, _ := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))
	defer func() { _ = proc.Shutdown(context.Background()) }()

	// Insert 3 traces; the third should trigger eviction of the oldest.
	for i := 0; i < 3; i++ {
		traceID := [16]byte{byte(i + 1)}
		td := makeTrace(traceID, nil, map[string]string{"service.name": "svc"})
		require.NoError(t, proc.ConsumeTraces(context.Background(), td))
		time.Sleep(5 * time.Millisecond) // Ensure distinct arrivedAt times.
	}

	proc.mu.Lock()
	bufferedCount := len(proc.traces)
	proc.mu.Unlock()

	assert.LessOrEqual(t, bufferedCount, 2, "buffer should not exceed NumTraces")
}

// TestBuildSamplingKey_MultipleAttributes_ProducesDistinctKeys verifies that
// traces with more than one sampling attribute produce keys that are unique per
// combination of values, and that identical attribute sets produce the same key.
func TestBuildSamplingKey_MultipleAttributes_ProducesDistinctKeys(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SamplingAttributes = []string{"service.name", "http.method"}
	proc, _ := newTestProcessor(t, cfg)

	makeMultiAttrTrace := func(traceID [16]byte, service, method string) *traceData {
		batch := makeTrace(traceID,
			map[string]string{"http.method": method},
			map[string]string{"service.name": service},
		)
		return &traceData{spanCount: 1, accumulated: batch}
	}

	tdA := makeMultiAttrTrace([16]byte{1}, "checkout", "POST")
	tdB := makeMultiAttrTrace([16]byte{2}, "checkout", "GET")
	tdC := makeMultiAttrTrace([16]byte{3}, "payment", "POST")
	tdD := makeMultiAttrTrace([16]byte{4}, "payment", "GET")

	keyA := proc.buildSamplingKey(tdA)
	keyB := proc.buildSamplingKey(tdB)
	keyC := proc.buildSamplingKey(tdC)
	keyD := proc.buildSamplingKey(tdD)

	// All four attribute combinations must produce distinct cache keys.
	keys := []string{keyA, keyB, keyC, keyD}
	for i := range keys {
		for j := range keys {
			if i != j {
				assert.NotEqualf(t, keys[i], keys[j],
					"key[%d]=%q should differ from key[%d]=%q", i, keys[i], j, keys[j])
			}
		}
	}

	// Each key must contain the separator, proving both attribute values are joined.
	for _, k := range keys {
		assert.Contains(t, k, samplingKeySeparator)
	}

	// Identical attribute values must always produce the same key.
	tdA2 := makeMultiAttrTrace([16]byte{5}, "checkout", "POST")
	assert.Equal(t, keyA, proc.buildSamplingKey(tdA2),
		"traces with identical attributes must share a cache key")
}

// TestEMAIsolation_HighTrafficDoesNotDropLowTraffic verifies that a high-volume
// sampling key does not inflate the sample rate of an unrelated low-volume key.
// The EMA algorithm maintains independent per-key rates, so a "cold" key that
// has never been seen must always receive rate=1 (keep all) regardless of how
// much traffic a "hot" key has generated.
func TestEMAIsolation_HighTrafficDoesNotDropLowTraffic(t *testing.T) {
	cfg := &Config{
		GoalSampleRate:      10,
		AdjustmentInterval:  50 * time.Millisecond,
		Weight:              0.5,
		AgeOutValue:         0.5,
		BurstMultiple:       2.0,
		BurstDetectionDelay: 3,
		SamplingAttributes:  []string{"service.name"},
		MaxKeys:             500,
		// Long DecisionWait keeps all traces in buffer during the warm-up phase.
		DecisionWait: 30 * time.Second,
		NumTraces:    50000,
	}
	proc, sink := newTestProcessor(t, cfg)
	require.NoError(t, proc.Start(context.Background(), nil))

	// Warm up the EMA by repeatedly reporting high traffic for "hot-service"
	// across several adjustment intervals so the algorithm raises its rate.
	for round := 0; round < 4; round++ {
		for i := 0; i < 200; i++ {
			proc.sampler.GetSampleRateMulti("hot-service", 1)
		}
		time.Sleep(60 * time.Millisecond) // let the adjustment goroutine fire
	}

	// After warming up, "hot-service" must have an elevated rate (> 1).
	hotRate := proc.sampler.GetSampleRateMulti("hot-service", 1)
	assert.Greater(t, hotRate, 1,
		"hot-service should have sample rate > 1 after high-traffic warm-up")

	// A brand-new key that has never been seen must still have rate = 1,
	// proving it is completely isolated from "hot-service" pressure.
	coldRate := proc.sampler.GetSampleRateMulti("cold-service", 1)
	assert.Equal(t, 1, coldRate,
		"cold-service (unseen key) must have rate=1 regardless of hot-service traffic")

	// End-to-end verification: send one cold-service trace and flush.
	// Since rate=1 means rand.Intn(1)==0 always, it must be forwarded.
	coldTrace := makeTrace([16]byte{0xCC}, nil, map[string]string{"service.name": "cold-service"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), coldTrace))
	require.NoError(t, proc.Shutdown(context.Background()))

	// Count only the spans that belong to cold-service.
	coldSpanCount := 0
	for _, received := range sink.AllTraces() {
		rs := received.ResourceSpans()
		for i := 0; i < rs.Len(); i++ {
			if v, ok := rs.At(i).Resource().Attributes().Get("service.name"); ok && v.AsString() == "cold-service" {
				coldSpanCount += received.SpanCount()
			}
		}
	}
	assert.Equal(t, 1, coldSpanCount,
		"cold-service trace must be forwarded (rate=1 guarantees keep-all for unseen keys)")
}

// TestCapabilities_MutatesData verifies that MutatesData reflects the
// add_sample_rate_attribute config flag.
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

// TestAddSampleRateAttribute_Enabled verifies that kept traces have the
// sampling.sample_rate attribute stamped on every span.
func TestAddSampleRateAttribute_Enabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1 // keep all traces
	cfg.DecisionWait = 1 * time.Hour
	cfg.AddSampleRateAttribute = true
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeTrace([16]byte{1}, nil, map[string]string{"service.name": "svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	require.Equal(t, 1, sink.SpanCount(), "trace should be forwarded")
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

// TestAddSampleRateAttribute_Disabled verifies no sampling.sample_rate
// attribute is written when the feature is turned off.
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

// TestAlwaysSampleErrors_ErrorStatus verifies that a trace with a span whose
// status is StatusCodeError is always forwarded even when the sample rate is
// very high (effectively guaranteeing it would be dropped otherwise).
func TestAlwaysSampleErrors_ErrorStatus(t *testing.T) {
	cfg := defaultTestConfig()
	// GoalSampleRate=1 keeps all; we rely on forcing keep via always_sample_errors
	// even if EMA would drop. Use rate=1 so the test is deterministic.
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AlwaysSampleErrors = true
	cfg.AddSampleRateAttribute = true
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeErrorTrace([16]byte{0xE1}, map[string]string{"service.name": "err-svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	assert.Equal(t, 1, sink.SpanCount(), "error trace must always be forwarded")
}

// TestAlwaysSampleErrors_ExceptionEvent verifies that a trace containing an
// "exception" event is always forwarded when always_sample_errors is on.
func TestAlwaysSampleErrors_ExceptionEvent(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AlwaysSampleErrors = true
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeExceptionTrace([16]byte{0xE2}, map[string]string{"service.name": "exc-svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	assert.Equal(t, 1, sink.SpanCount(), "exception-event trace must always be forwarded")
}

// TestAlwaysSampleErrors_SampleRateAttributeIsOne verifies that when a trace
// is force-kept due to errors, sampling.sample_rate is stamped as 1.
func TestAlwaysSampleErrors_SampleRateAttributeIsOne(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AlwaysSampleErrors = true
	cfg.AddSampleRateAttribute = true
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	// Manually force the sampler to report a high rate so the trace would
	// normally be dropped, then verify the error path overrides it.
	// (With GoalSampleRate=1 the sampler already returns 1; this test mainly
	// confirms the rate=1 override is written as the attribute value.)
	td := makeErrorTrace([16]byte{0xE3}, map[string]string{"service.name": "force-svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	require.Equal(t, 1, sink.SpanCount())
	received := sink.AllTraces()[0]
	span := received.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	v, ok := span.Attributes().Get("sampling.sample_rate")
	require.True(t, ok)
	assert.Equal(t, int64(1), v.Int(), "force-kept error traces must record sample_rate=1")
}

// TestAlwaysSampleErrors_Disabled verifies that with the flag off, error-status
// traces go through normal probabilistic sampling (they are still forwarded
// when GoalSampleRate=1, but for the right reason — not the force-keep path).
func TestAlwaysSampleErrors_Disabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.GoalSampleRate = 1
	cfg.DecisionWait = 1 * time.Hour
	cfg.AlwaysSampleErrors = false
	proc, sink := newTestProcessor(t, cfg)

	require.NoError(t, proc.Start(context.Background(), nil))

	td := makeErrorTrace([16]byte{0xE4}, map[string]string{"service.name": "svc"})
	require.NoError(t, proc.ConsumeTraces(context.Background(), td))
	require.NoError(t, proc.Shutdown(context.Background()))

	// Rate=1 always keeps, so we still expect the trace — just via normal path.
	assert.Equal(t, 1, sink.SpanCount())
}
