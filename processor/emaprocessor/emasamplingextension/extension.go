// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingextension"

import (
	"context"
	"fmt"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaSamplingExtension holds a running EMASampleRate sampler and spawns evaluators.
type emaSamplingExtension struct {
	cfg     *Config
	logger  *zap.Logger
	sampler *dynsampler.EMASampleRate
}

var _ component.Component = (*emaSamplingExtension)(nil)
var _ samplingpolicy.Extension = (*emaSamplingExtension)(nil)

func newEmaSamplingExtension(cfg *Config, logger *zap.Logger) *emaSamplingExtension {
	maxKeys := cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}
	return &emaSamplingExtension{
		cfg:    cfg,
		logger: logger,
		sampler: &dynsampler.EMASampleRate{
			GoalSampleRate:             cfg.GoalSampleRate,
			AdjustmentIntervalDuration: cfg.AdjustmentInterval,
			Weight:                     cfg.Weight,
			AgeOutValue:                cfg.AgeOutValue,
			BurstMultiple:              cfg.BurstMultiple,
			BurstDetectionDelay:        cfg.BurstDetectionDelay,
			MaxKeys:                    maxKeys,
		},
	}
}

func (e *emaSamplingExtension) Start(_ context.Context, _ component.Host) error {
	if err := e.sampler.Start(); err != nil {
		return fmt.Errorf("failed to start EMA sample rate sampler: %w", err)
	}
	return nil
}

func (e *emaSamplingExtension) Shutdown(_ context.Context) error {
	return e.sampler.Stop()
}

// NewEvaluator satisfies samplingpolicy.Extension. cfg is ignored; all tuning
// is controlled by the extension's own Config.
func (e *emaSamplingExtension) NewEvaluator(_ string, _ map[string]any) (samplingpolicy.Evaluator, error) {
	return &emaSamplingEvaluator{
		sampler:    e.sampler,
		attrs:      e.cfg.SamplingAttributes,
		traceLen:   e.cfg.UseTraceLength,
	}, nil
}
