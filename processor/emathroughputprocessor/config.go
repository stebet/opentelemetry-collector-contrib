// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emathroughputprocessor"

import (
	"errors"
	"time"
)

// Config holds the configuration for the EMA Throughput tail sampling processor.
type Config struct {
	// GoalThroughputPerSec is the target number of traces to forward per second.
	// The EMA algorithm adjusts per-key sample rates so that the aggregate
	// throughput of kept traces converges toward this value.
	// Must be >= 1. Default: 100.
	GoalThroughputPerSec int `mapstructure:"goal_throughput_per_sec"`

	// InitialSampleRate is the sample rate used during startup, before enough
	// traffic has been observed to calculate a meaningful EMA-based rate.
	// Must be >= 1. Default: 10.
	InitialSampleRate int `mapstructure:"initial_sample_rate"`

	// AdjustmentInterval controls how often the EMA algorithm recalculates
	// sample rates for each sampling key. Default: 15s.
	AdjustmentInterval time.Duration `mapstructure:"adjustment_interval"`

	// Weight is the EMA smoothing factor, controlling how quickly the sampler
	// reacts to changes in traffic. Must be in the range (0, 1).
	// Higher values make the sampler react faster but with more variance.
	// Default: 0.5.
	Weight float64 `mapstructure:"weight"`

	// AgeOutValue is the factor applied to the EMA for keys that receive no
	// traffic during an adjustment interval, causing their rate to decay
	// toward zero. Must be in the range (0, 1). Default: 0.5.
	AgeOutValue float64 `mapstructure:"age_out_value"`

	// BurstMultiple is a multiplier applied to the base sample rate when a
	// burst is detected. A value of 2.0 means burst traffic is sampled at
	// twice the base rate. Default: 2.0.
	BurstMultiple float64 `mapstructure:"burst_multiple"`

	// BurstDetectionDelay is the number of adjustment intervals to wait before
	// burst detection becomes active after startup. Default: 3.
	BurstDetectionDelay uint `mapstructure:"burst_detection_delay"`

	// SamplingAttributes is the list of span or resource attribute keys used
	// to build the per-key sampling decision. Traces sharing the same values
	// for these attributes are grouped together for sampling rate calculation.
	// Required.
	SamplingAttributes []string `mapstructure:"sampling_attributes"`

	// MaxKeys is the maximum number of distinct sampling keys the EMA sampler
	// will track. Defaults to 500.
	MaxKeys int `mapstructure:"max_keys"`

	// UseTraceLength, when true, includes the span count of the trace in the
	// sampling key. This causes traces with different span counts to be treated
	// as distinct for sampling purposes. Default: false.
	UseTraceLength bool `mapstructure:"use_trace_length"`

	// DecisionWait is how long the processor waits after the first span of a
	// trace arrives before making a sampling decision. Default: 30s.
	DecisionWait time.Duration `mapstructure:"decision_wait"`

	// NumTraces is the maximum number of traces held in memory at once.
	// When this limit is reached, the oldest traces are evicted and their
	// sampling decision is made immediately. Default: 50000.
	NumTraces uint64 `mapstructure:"num_traces"`

	// AddSampleRateAttribute, when true, adds a sampling.sample_rate integer
	// attribute to every span of a kept trace. The value is the EMA-calculated
	// 1-in-N sample rate that was used for the sampling decision. Default: true.
	AddSampleRateAttribute bool `mapstructure:"add_sample_rate_attribute"`

	// AlwaysSampleErrors, when true, forces any trace that contains at least
	// one span with StatusCode == Error or an event named "exception" to be
	// kept regardless of the EMA sampling rate. Default: false.
	AlwaysSampleErrors bool `mapstructure:"always_sample_errors"`
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
	if c.DecisionWait <= 0 {
		return errors.New("decision_wait must be > 0")
	}
	if c.NumTraces == 0 {
		return errors.New("num_traces must be > 0")
	}
	return nil
}
