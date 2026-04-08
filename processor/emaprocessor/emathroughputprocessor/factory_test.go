// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emathroughputprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/processor/processortest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)
	assert.Equal(t, typeStr, factory.Type())
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	tCfg, ok := cfg.(*Config)
	require.True(t, ok)
	assert.Equal(t, 100, tCfg.GoalThroughputPerSec)
	assert.Equal(t, 10, tCfg.InitialSampleRate)
	assert.Equal(t, 0.5, tCfg.Weight)
	assert.Equal(t, 0.5, tCfg.AgeOutValue)
	assert.Equal(t, 500, tCfg.MaxKeys)
	assert.EqualValues(t, 50000, tCfg.NumTraces)
}

func TestCreateTracesProcessor(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.SamplingAttributes = []string{"service.name"}

	set := processortest.NewNopSettings(typeStr)
	next := &consumertest.TracesSink{}

	proc, err := factory.CreateTraces(t.Context(), set, cfg, next)
	require.NoError(t, err)
	require.NotNil(t, proc)
}

func TestCreateTracesProcessorInvalidConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	// sampling_attributes is required but left empty.

	set := processortest.NewNopSettings(typeStr)
	next := &consumertest.TracesSink{}

	_, err := factory.CreateTraces(t.Context(), set, cfg, next)
	require.Error(t, err)
}
