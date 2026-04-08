// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputprocessor"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
)

var typeStr = component.MustNewType("ema_throughput")

// NewFactory returns a new factory for the EMA Throughput Tail Sampling processor.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		SharedConfig: emacore.SharedConfig{
			AdjustmentInterval:     15 * time.Second,
			Weight:                 0.5,
			AgeOutValue:            0.5,
			BurstMultiple:          2.0,
			BurstDetectionDelay:    3,
			MaxKeys:                500,
			UseTraceLength:         false,
			DecisionWait:           30 * time.Second,
			NumTraces:              50000,
			AddSampleRateAttribute: true,
		},
		GoalThroughputPerSec: 100,
		InitialSampleRate:    10,
	}
}

func createTracesProcessor(
	_ context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	return newEMAThroughputProcessor(set, next, cfg.(*Config))
}
