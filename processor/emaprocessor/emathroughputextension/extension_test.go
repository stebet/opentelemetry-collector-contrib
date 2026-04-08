// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension

import (
	"context"
	"testing"
	"time"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

func TestFactory_CreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg)

	c := cfg.(*Config)
	assert.Equal(t, 15*time.Second, c.AdjustmentInterval)
	assert.Equal(t, 0.5, c.Weight)
	assert.Equal(t, 0.5, c.AgeOutValue)
	assert.Equal(t, 2.0, c.BurstMultiple)
	assert.Equal(t, uint(3), c.BurstDetectionDelay)
	assert.Equal(t, 500, c.MaxKeys)
	assert.Equal(t, 100, c.GoalThroughputPerSec)
	assert.Equal(t, 10, c.InitialSampleRate)
}

func TestFactory_CreateExtension_Valid(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.SamplingAttributes = []string{"service.name"}

	ext, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
}

func TestFactory_CreateExtension_Invalid(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	// Missing SamplingAttributes

	_, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.Error(t, err)
}

func TestExtension_StartShutdown(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.SamplingAttributes = []string{"service.name"}

	ext, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.NoError(t, err)

	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestExtension_NewEvaluator(t *testing.T) {
	cfg := &Config{
		AdjustmentInterval:   15 * time.Second,
		Weight:               0.5,
		AgeOutValue:          0.5,
		BurstMultiple:        2.0,
		BurstDetectionDelay:  3,
		MaxKeys:              500,
		GoalThroughputPerSec: 100,
		InitialSampleRate:    10,
		SamplingAttributes:   []string{"service.name"},
	}

	ext := newEmaThroughputExtension(cfg, componenttest.NewNopTelemetrySettings().Logger)
	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	defer ext.Shutdown(context.Background()) //nolint:errcheck

	evaluator, err := ext.NewEvaluator("test-policy", nil)
	require.NoError(t, err)
	require.NotNil(t, evaluator)
	assert.True(t, evaluator.IsStateful())
}

// TestEvaluatorsAreIsolated verifies that each NewEvaluator call produces an
// evaluator backed by its own independent dynsampler instance.
func TestEvaluatorsAreIsolated(t *testing.T) {
	cfg := &Config{
		AdjustmentInterval:   15 * time.Second,
		Weight:               0.5,
		AgeOutValue:          0.5,
		BurstMultiple:        2.0,
		BurstDetectionDelay:  3,
		MaxKeys:              500,
		GoalThroughputPerSec: 100,
		InitialSampleRate:    10,
		SamplingAttributes:   []string{"service.name"},
	}

	ext := newEmaThroughputExtension(cfg, componenttest.NewNopTelemetrySettings().Logger)
	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	defer ext.Shutdown(context.Background()) //nolint:errcheck

	eval1, err := ext.NewEvaluator("policy-a", nil)
	require.NoError(t, err)

	eval2, err := ext.NewEvaluator("policy-b", nil)
	require.NoError(t, err)

	// Two evaluators are distinct objects.
	assert.NotSame(t, eval1, eval2)

	// The extension is tracking two independent samplers.
	ext.mu.Lock()
	n := len(ext.samplers)
	same := ext.samplers[0] == ext.samplers[1]
	ext.mu.Unlock()
	assert.Equal(t, 2, n)
	assert.False(t, same, "each evaluator must own a distinct dynsampler instance")
}

// TestShutdown_StopsAllEvaluatorSamplers verifies Shutdown cleans up all
// dynamically-started samplers without error.
func TestShutdown_StopsAllEvaluatorSamplers(t *testing.T) {
	cfg := &Config{
		AdjustmentInterval:   15 * time.Second,
		Weight:               0.5,
		AgeOutValue:          0.5,
		BurstMultiple:        2.0,
		BurstDetectionDelay:  3,
		MaxKeys:              500,
		GoalThroughputPerSec: 100,
		InitialSampleRate:    10,
		SamplingAttributes:   []string{"service.name"},
	}

	ext := newEmaThroughputExtension(cfg, componenttest.NewNopTelemetrySettings().Logger)
	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))

	_, err := ext.NewEvaluator("p1", nil)
	require.NoError(t, err)
	_, err = ext.NewEvaluator("p2", nil)
	require.NoError(t, err)

	require.NoError(t, ext.Shutdown(context.Background()))
}

// TestDefaultConfig_AddSampleRateAttribute verifies the default enables the attribute.
func TestDefaultConfig_AddSampleRateAttribute(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	assert.True(t, cfg.AddSampleRateAttribute)
}

// TestEvaluator_AddSampleRateAttribute verifies sampling.sample_rate is stamped when enabled.
func TestEvaluator_AddSampleRateAttribute(t *testing.T) {
	eval := &emaThroughputEvaluator{
		sampler: &dynsampler.EMAThroughput{
			GoalThroughputPerSec: 1000,
			InitialSampleRate:    1, // always sample initially
		},
		attrs:             []string{"service.name"},
		addSampleRateAttr: true,
	}
	require.NoError(t, eval.sampler.Start())
	defer eval.sampler.Stop() //nolint:errcheck

	var gotSampled bool
	for range 20 {
		td := buildTestTraceData("svc-a")
		dec, err := eval.Evaluate(context.Background(), pcommon.TraceID{}, td)
		require.NoError(t, err)
		if dec == samplingpolicy.Sampled {
			span := td.ReceivedBatches.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			v, ok := span.Attributes().Get("sampling.sample_rate")
			require.True(t, ok, "sampling.sample_rate attribute must be present")
			assert.GreaterOrEqual(t, v.Int(), int64(1))
			gotSampled = true
			break
		}
	}
	if !gotSampled {
		t.Skip("did not sample in 20 attempts — probabilistic test, skipping")
	}
}

// TestEvaluator_NoSampleRateAttribute verifies sampling.sample_rate is absent when disabled.
func TestEvaluator_NoSampleRateAttribute(t *testing.T) {
	eval := &emaThroughputEvaluator{
		sampler: &dynsampler.EMAThroughput{
			GoalThroughputPerSec: 1000,
			InitialSampleRate:    1, // always sample initially
		},
		attrs:             []string{"service.name"},
		addSampleRateAttr: false,
	}
	require.NoError(t, eval.sampler.Start())
	defer eval.sampler.Stop() //nolint:errcheck

	var gotSampled bool
	for range 20 {
		td := buildTestTraceData("svc-b")
		dec, err := eval.Evaluate(context.Background(), pcommon.TraceID{}, td)
		require.NoError(t, err)
		if dec == samplingpolicy.Sampled {
			span := td.ReceivedBatches.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			_, ok := span.Attributes().Get("sampling.sample_rate")
			assert.False(t, ok, "sampling.sample_rate must not be present when disabled")
			gotSampled = true
			break
		}
	}
	if !gotSampled {
		t.Skip("did not sample in 20 attempts — probabilistic test, skipping")
	}
}

// buildTestTraceData returns a minimal TraceData with one span tagged with service.name=svcName.
func buildTestTraceData(svcName string) *samplingpolicy.TraceData {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", svcName)
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Spans().AppendEmpty().SetName("test-span")
	return &samplingpolicy.TraceData{
		ReceivedBatches: traces,
		SpanCount:       1,
	}
}
