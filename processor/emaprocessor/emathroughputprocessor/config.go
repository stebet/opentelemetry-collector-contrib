// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputprocessor"

import (
	"errors"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
)

// Config holds the configuration for the EMA Throughput tail sampling processor.
type Config struct {
	emacore.SharedConfig `mapstructure:",squash"`
	// GoalThroughputPerSec is the target number of traces to forward per second.
	// Must be >= 1. Default: 100.
	GoalThroughputPerSec int `mapstructure:"goal_throughput_per_sec"`
	// InitialSampleRate is the sample rate used during startup before the EMA converges.
	// Must be >= 1. Default: 10.
	InitialSampleRate int `mapstructure:"initial_sample_rate"`
}

// Validate checks the Config for invalid values.
func (c *Config) Validate() error {
	if c.GoalThroughputPerSec < 1 {
		return errors.New("goal_throughput_per_sec must be >= 1")
	}
	if c.InitialSampleRate < 1 {
		return errors.New("initial_sample_rate must be >= 1")
	}
	return c.SharedConfig.ValidateShared()
}
