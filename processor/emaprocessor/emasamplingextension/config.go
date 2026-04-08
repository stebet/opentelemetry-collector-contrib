// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingextension"

import (
	"errors"
	"time"
)

// Config holds configuration for the EMA sampling policy extension.
type Config struct {
	AdjustmentInterval  time.Duration `mapstructure:"adjustment_interval"`
	Weight              float64       `mapstructure:"weight"`
	AgeOutValue         float64       `mapstructure:"age_out_value"`
	BurstMultiple       float64       `mapstructure:"burst_multiple"`
	BurstDetectionDelay uint          `mapstructure:"burst_detection_delay"`
	SamplingAttributes  []string      `mapstructure:"sampling_attributes"`
	MaxKeys             int           `mapstructure:"max_keys"`
	UseTraceLength      bool          `mapstructure:"use_trace_length"`
	GoalSampleRate      int           `mapstructure:"goal_sample_rate"`
}

// Validate checks the Config for invalid values.
func (c *Config) Validate() error {
	if c.GoalSampleRate < 1 {
		return errors.New("goal_sample_rate must be >= 1")
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
