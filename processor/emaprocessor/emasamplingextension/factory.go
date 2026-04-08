// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingextension"

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var typeStr = component.MustNewType("ema_sampling_policy")

// NewFactory returns a new factory for the EMA sampling policy extension.
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
		AdjustmentInterval:     15 * time.Second,
		Weight:                 0.5,
		AgeOutValue:            0.5,
		BurstMultiple:          2.0,
		BurstDetectionDelay:    3,
		MaxKeys:                500,
		GoalSampleRate:         10,
		AddSampleRateAttribute: true,
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	c := cfg.(*Config)
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ema_sampling_policy config: %w", err)
	}
	return newEmaSamplingExtension(c, set.Logger), nil
}
