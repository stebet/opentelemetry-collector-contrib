// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputextension // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/emathroughputextension"

import (
	"context"
	"math/rand"

	dynsampler "github.com/honeycombio/dynsampler-go"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/emaprocessor/internal/emacore"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor/pkg/samplingpolicy"
)

// emaThroughputEvaluator implements samplingpolicy.Evaluator using EMAThroughput.
type emaThroughputEvaluator struct {
	sampler           *dynsampler.EMAThroughput
	attrs             []string
	traceLen          bool
	addSampleRateAttr bool
}

var _ samplingpolicy.Evaluator = (*emaThroughputEvaluator)(nil)

func (e *emaThroughputEvaluator) Evaluate(_ context.Context, _ pcommon.TraceID, trace *samplingpolicy.TraceData) (samplingpolicy.Decision, error) {
	key := emacore.BuildKey(trace.ReceivedBatches, trace.SpanCount, e.attrs, e.traceLen)
	rate := e.sampler.GetSampleRateMulti(key, int(trace.SpanCount))
	if rate < 1 {
		rate = 1
	}
	if rand.Intn(rate) == 0 { //nolint:gosec
		if e.addSampleRateAttr {
			emacore.AddSampleRateAttr(trace.ReceivedBatches, rate)
		}
		return samplingpolicy.Sampled, nil
	}
	return samplingpolicy.NotSampled, nil
}

func (e *emaThroughputEvaluator) IsStateful() bool { return true }
