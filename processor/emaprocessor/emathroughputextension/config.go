// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputextension"

import (
	"errors"
	"time"
)

// Config holds configuration for the EMA throughput policy extension.
type Config struct {
	AdjustmentInterval  time.Duration `mapstructure:"adjustment_interval"`
	Weight              float64       `mapstructure:"weight"`
	AgeOutValue         float64       `mapstructure:"age_out_value"`
	BurstMultiple       float64       `mapstructure:"burst_multiple"`
	BurstDetectionDelay uint          `mapstructure:"burst_detection_delay"`
	SamplingAttributes  []string      `mapstructure:"sampling_attributes"`
	MaxKeys             int           `mapstructure:"max_keys"`
	UseTraceLength      bool          `mapstructure:"use_trace_length"`
	GoalThroughputPerSec int          `mapstructure:"goal_throughput_per_sec"`
	InitialSampleRate   int           `mapstructure:"initial_sample_rate"`
	// AddSampleRateAttribute stamps sampling.sample_rate on every kept span.
	AddSampleRateAttribute bool `mapstructure:"add_sample_rate_attribute"`
}

// Validate checks the Config for invalid values.
func (c *Config) Validate() error {
	if c.GoalThroughputPerSec < 1 {
		return errors.New("goal_throughput_per_sec must be >= 1")
	}
	if c.InitialSampleRate < 1 {
		return errors.New("initial_sample_rate must be >= 1")
	}
	if c.Weight <= 0 || c.Weight >= 1 {
		return errors.New("weight must be in the range (0, 1) exclusive")
	}
	if c.AgeOutValue <= 0 || c.AgeOutValue >= 1 {
		return errors.New("age_out_value must be in the range (0, 1) exclusive")
	}
	if len(c.SamplingAttributes) == 0 {
		return errors.New("sampling_attributes must contain at least one attribute key")
	}
	return nil
}
