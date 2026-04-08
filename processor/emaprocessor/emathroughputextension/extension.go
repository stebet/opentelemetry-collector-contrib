// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputextension"

import (
	"context"
	"fmt"
	"sync"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaThroughputExtension is a factory for per-policy EMA throughput evaluators.
// Each call to NewEvaluator creates its own independent dynsampler instance.
type emaThroughputExtension struct {
	cfg    *Config
	logger *zap.Logger

	mu       sync.Mutex
	samplers []*dynsampler.EMAThroughput // all active evaluator samplers, stopped on Shutdown
}

var _ component.Component = (*emaThroughputExtension)(nil)
var _ samplingpolicy.Extension = (*emaThroughputExtension)(nil)

func newEmaThroughputExtension(cfg *Config, logger *zap.Logger) *emaThroughputExtension {
	return &emaThroughputExtension{cfg: cfg, logger: logger}
}

// Start is a no-op; each sampler is started lazily in NewEvaluator.
func (e *emaThroughputExtension) Start(_ context.Context, _ component.Host) error { return nil }

// Shutdown stops all samplers that were created via NewEvaluator.
func (e *emaThroughputExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.samplers {
		s.Stop() //nolint:errcheck
	}
	return nil
}

// NewEvaluator creates an independent EMA throughput sampler for the given policy.
// Each evaluator has its own rate table so policies do not share state.
func (e *emaThroughputExtension) NewEvaluator(policyName string, _ map[string]any) (samplingpolicy.Evaluator, error) {
	maxKeys := e.cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}
	sampler := &dynsampler.EMAThroughput{
		GoalThroughputPerSec: e.cfg.GoalThroughputPerSec,
		InitialSampleRate:    e.cfg.InitialSampleRate,
		AdjustmentInterval:   e.cfg.AdjustmentInterval,
		Weight:               e.cfg.Weight,
		AgeOutValue:          e.cfg.AgeOutValue,
		BurstMultiple:        e.cfg.BurstMultiple,
		BurstDetectionDelay:  e.cfg.BurstDetectionDelay,
		MaxKeys:              maxKeys,
	}
	if err := sampler.Start(); err != nil {
		return nil, fmt.Errorf("failed to start EMA throughput sampler for policy %q: %w", policyName, err)
	}

	e.mu.Lock()
	e.samplers = append(e.samplers, sampler)
	e.mu.Unlock()

	return &emaThroughputEvaluator{
		sampler:           sampler,
		attrs:             e.cfg.SamplingAttributes,
		traceLen:          e.cfg.UseTraceLength,
		addSampleRateAttr: e.cfg.AddSampleRateAttribute,
	}, nil
}
