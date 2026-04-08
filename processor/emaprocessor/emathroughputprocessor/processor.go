// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputprocessor"

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

type emaThroughputProcessor struct {
	buffer *emacore.TraceBuffer
}

var _ processor.Traces = (*emaThroughputProcessor)(nil)

func newEMAThroughputProcessor(set processor.Settings, next consumer.Traces, cfg *Config) (*emaThroughputProcessor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ema_throughput config: %w", err)
	}

	maxKeys := cfg.MaxKeys
	if maxKeys == 0 {
		maxKeys = 500
	}

	sampler := &dynsampler.EMAThroughput{
		GoalThroughputPerSec: cfg.GoalThroughputPerSec,
		InitialSampleRate:    cfg.InitialSampleRate,
		AdjustmentInterval:   cfg.AdjustmentInterval,
		Weight:               cfg.Weight,
		AgeOutValue:          cfg.AgeOutValue,
		BurstMultiple:        cfg.BurstMultiple,
		BurstDetectionDelay:  cfg.BurstDetectionDelay,
		MaxKeys:              maxKeys,
	}

	return &emaThroughputProcessor{
		buffer: emacore.NewTraceBuffer(set.Logger, &cfg.SharedConfig, sampler, next),
	}, nil
}

func (p *emaThroughputProcessor) Start(ctx context.Context, _ component.Host) error {
	return p.buffer.Start()
}

func (p *emaThroughputProcessor) Shutdown(ctx context.Context) error {
	return p.buffer.Shutdown(ctx)
}

func (p *emaThroughputProcessor) Capabilities() consumer.Capabilities {
	return p.buffer.Capabilities()
}

func (p *emaThroughputProcessor) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	return p.buffer.ConsumeTraces(ctx, traces)
}
