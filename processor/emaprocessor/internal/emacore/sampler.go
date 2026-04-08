// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emacore // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"

// Sampler is the interface that the EMA dynsampler implementations must satisfy.
type Sampler interface {
	// Start initialises the sampler and begins background adjustment.
	Start() error
	// Stop halts background adjustment and cleans up resources.
	Stop() error
	// GetSampleRateMulti returns the recommended 1-in-N sample rate for the
	// given key, accounting for count spans arriving simultaneously.
	GetSampleRateMulti(key string, count int) int
}
