// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingextension"

import (
	"context"
	"fmt"
	"sync"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaSamplingExtension is a factory for per-policy EMA sampling evaluators.
// Each call to NewEvaluator creates its own independent dynsampler instance.
type emaSamplingExtension struct {
	cfg    *Config
	logger *zap.Logger

	mu       sync.Mutex
	samplers []*dynsampler.EMASampleRate // all active evaluator samplers, stopped on Shutdown
}

var _ component.Component = (*emaSamplingExtension)(nil)
var _ samplingpolicy.Extension = (*emaSamplingExtension)(nil)

func newEmaSamplingExtension(cfg *Config, logger *zap.Logger) *emaSamplingExtension {
	return &emaSamplingExtension{cfg: cfg, logger: logger}
}

// Start is a no-op; each sampler is started lazily in NewEvaluator.
func (e *emaSamplingExtension) Start(_ context.Context, _ component.Host) error { return nil }

// Shutdown stops all samplers that were created via NewEvaluator.
func (e *emaSamplingExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.samplers {
		s.Stop() //nolint:errcheck
	}
	return nil
}

// NewEvaluator creates an independent EMA sampler for the given policy.
// Each evaluator has its own rate table so policies do not share state.
func (e *emaSamplingExtension) NewEvaluator(policyName string, _ map[string]any) (samplingpolicy.Evaluator, error) {
	maxKeys := e.cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}
	sampler := &dynsampler.EMASampleRate{
		GoalSampleRate:             e.cfg.GoalSampleRate,
		AdjustmentIntervalDuration: e.cfg.AdjustmentInterval,
		Weight:                     e.cfg.Weight,
		AgeOutValue:                e.cfg.AgeOutValue,
		BurstMultiple:              e.cfg.BurstMultiple,
		BurstDetectionDelay:        e.cfg.BurstDetectionDelay,
		MaxKeys:                    maxKeys,
	}
	if err := sampler.Start(); err != nil {
		return nil, fmt.Errorf("failed to start EMA sampler for policy %q: %w", policyName, err)
	}

	e.mu.Lock()
	e.samplers = append(e.samplers, sampler)
	e.mu.Unlock()

	return &emaSamplingEvaluator{
		sampler:  sampler,
		attrs:    e.cfg.SamplingAttributes,
		traceLen: e.cfg.UseTraceLength,
	}, nil
}
