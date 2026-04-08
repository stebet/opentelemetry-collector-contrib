// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emacore // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"

import (
	"errors"
	"time"
)

// SharedConfig holds the configuration fields common to all EMA processor variants.
type SharedConfig struct {
	AdjustmentInterval     time.Duration `mapstructure:"adjustment_interval"`
	Weight                 float64       `mapstructure:"weight"`
	AgeOutValue            float64       `mapstructure:"age_out_value"`
	BurstMultiple          float64       `mapstructure:"burst_multiple"`
	BurstDetectionDelay    uint          `mapstructure:"burst_detection_delay"`
	SamplingAttributes     []string      `mapstructure:"sampling_attributes"`
	MaxKeys                int           `mapstructure:"max_keys"`
	UseTraceLength         bool          `mapstructure:"use_trace_length"`
	DecisionWait           time.Duration `mapstructure:"decision_wait"`
	NumTraces              uint64        `mapstructure:"num_traces"`
	AddSampleRateAttribute bool          `mapstructure:"add_sample_rate_attribute"`
}

// ValidateShared checks SharedConfig fields for invalid values.
func (c *SharedConfig) ValidateShared() error {
	if c.Weight <= 0 || c.Weight >= 1 {
		return errors.New("weight must be in the range (0, 1) exclusive")
	}
	if c.AgeOutValue <= 0 || c.AgeOutValue >= 1 {
		return errors.New("age_out_value must be in the range (0, 1) exclusive")
	}
	if len(c.SamplingAttributes) == 0 {
		return errors.New("sampling_attributes must contain at least one attribute key")
	}
	if c.DecisionWait <= 0 {
		return errors.New("decision_wait must be > 0")
	}
	if c.NumTraces == 0 {
		return errors.New("num_traces must be > 0")
	}
	return nil
}
