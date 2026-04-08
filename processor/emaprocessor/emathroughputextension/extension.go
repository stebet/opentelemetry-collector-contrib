// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputextension"

import (
	"context"
	"fmt"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaThroughputExtension holds a running EMAThroughput sampler and spawns evaluators.
type emaThroughputExtension struct {
	cfg     *Config
	logger  *zap.Logger
	sampler *dynsampler.EMAThroughput
}

var _ component.Component = (*emaThroughputExtension)(nil)
var _ samplingpolicy.Extension = (*emaThroughputExtension)(nil)

func newEmaThroughputExtension(cfg *Config, logger *zap.Logger) *emaThroughputExtension {
	maxKeys := cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}
	return &emaThroughputExtension{
		cfg:    cfg,
		logger: logger,
		sampler: &dynsampler.EMAThroughput{
			GoalThroughputPerSec: cfg.GoalThroughputPerSec,
			InitialSampleRate:    cfg.InitialSampleRate,
			AdjustmentInterval:   cfg.AdjustmentInterval,
			Weight:               cfg.Weight,
			AgeOutValue:          cfg.AgeOutValue,
			BurstMultiple:        cfg.BurstMultiple,
			BurstDetectionDelay:  cfg.BurstDetectionDelay,
			MaxKeys:              maxKeys,
		},
	}
}

func (e *emaThroughputExtension) Start(_ context.Context, _ component.Host) error {
	if err := e.sampler.Start(); err != nil {
		return fmt.Errorf("failed to start EMA throughput sampler: %w", err)
	}
	return nil
}

func (e *emaThroughputExtension) Shutdown(_ context.Context) error {
	return e.sampler.Stop()
}

// NewEvaluator satisfies samplingpolicy.Extension.
func (e *emaThroughputExtension) NewEvaluator(_ string, _ map[string]any) (samplingpolicy.Evaluator, error) {
	return &emaThroughputEvaluator{
		sampler:  e.sampler,
		attrs:    e.cfg.SamplingAttributes,
		traceLen: e.cfg.UseTraceLength,
	}, nil
}
