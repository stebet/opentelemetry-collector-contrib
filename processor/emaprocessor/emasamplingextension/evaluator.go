// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emasamplingextension"

import (
	"context"
	"math/rand"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaSamplingEvaluator implements samplingpolicy.Evaluator using EMASampleRate.
type emaSamplingEvaluator struct {
	sampler  *dynsampler.EMASampleRate
	attrs    []string
	traceLen bool
}

var _ samplingpolicy.Evaluator = (*emaSamplingEvaluator)(nil)

func (e *emaSamplingEvaluator) Evaluate(_ context.Context, _ pcommon.TraceID, trace *samplingpolicy.TraceData) (samplingpolicy.Decision, error) {
	key := emacore.BuildKey(trace.ReceivedBatches, trace.SpanCount, e.attrs, e.traceLen)
	rate := e.sampler.GetSampleRateMulti(key, int(trace.SpanCount))
	if rate < 1 {
		rate = 1
	}
	if rand.Intn(rate) == 0 { //nolint:gosec
		return samplingpolicy.Sampled, nil
	}
	return samplingpolicy.NotSampled, nil
}

func (e *emaSamplingEvaluator) IsStateful() bool { return true }
