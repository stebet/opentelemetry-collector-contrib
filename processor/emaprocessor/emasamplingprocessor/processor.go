// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingprocessor"

import (
	"context"
	"fmt"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
)

type emaSamplingProcessor struct {
	buffer *emacore.TraceBuffer
}

var _ processor.Traces = (*emaSamplingProcessor)(nil)

func newEMASamplingProcessor(set processor.Settings, next consumer.Traces, cfg *Config) (*emaSamplingProcessor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ema_sampling config: %w", err)
	}

	maxKeys := cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}

	sampler := &dynsampler.EMASampleRate{
		GoalSampleRate:             cfg.GoalSampleRate,
		AdjustmentIntervalDuration: cfg.AdjustmentInterval,
		Weight:                     cfg.Weight,
		AgeOutValue:                cfg.AgeOutValue,
		BurstMultiple:              cfg.BurstMultiple,
		BurstDetectionDelay:        cfg.BurstDetectionDelay,
		MaxKeys:                    maxKeys,
	}

	return &emaSamplingProcessor{
		buffer: emacore.NewTraceBuffer(set.Logger, &cfg.SharedConfig, sampler, next),
	}, nil
}

func (p *emaSamplingProcessor) Start(ctx context.Context, _ component.Host) error {
	return p.buffer.Start()
}

func (p *emaSamplingProcessor) Shutdown(ctx context.Context) error {
	return p.buffer.Shutdown(ctx)
}

func (p *emaSamplingProcessor) Capabilities() consumer.Capabilities {
	return p.buffer.Capabilities()
}

func (p *emaSamplingProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	return p.buffer.ConsumeTraces(ctx, traces)
}
