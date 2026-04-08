// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingprocessor"

import (
	"errors"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
)

// Config holds the configuration for the EMA tail sampling processor.
type Config struct {
	emacore.SharedConfig `mapstructure:",squash"`
	// GoalSampleRate is the target sample rate expressed as 1-in-N traces kept.
	// Must be >= 1. Default: 10.
	GoalSampleRate int `mapstructure:"goal_sample_rate"`
}

// Validate checks the Config for invalid values.
func (c *Config) Validate() error {
	if c.GoalSampleRate < 1 {
		return errors.New("goal_sample_rate must be >= 1")
	}
	return c.SharedConfig.ValidateShared()
}
