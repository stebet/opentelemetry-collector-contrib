// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputextension"

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var typeStr = component.MustNewType("ema_throughput_policy")

// NewFactory returns a new factory for the EMA throughput policy extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		typeStr,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelDevelopment,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		AdjustmentInterval:   15 * time.Second,
		Weight:               0.5,
		AgeOutValue:          0.5,
		BurstMultiple:        2.0,
		BurstDetectionDelay:  3,
		MaxKeys:              500,
		GoalThroughputPerSec: 100,
		InitialSampleRate:    10,
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	c := cfg.(*Config)
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ema_throughput_policy config: %w", err)
	}
	return newEmaThroughputExtension(c, set.Logger), nil
}
