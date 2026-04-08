// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingextension

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension/extensiontest"
)

func TestFactory_CreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg)

	c := cfg.(*Config)
	assert.Equal(t, 15*time.Second, c.AdjustmentInterval)
	assert.Equal(t, 0.5, c.Weight)
	assert.Equal(t, 0.5, c.AgeOutValue)
	assert.Equal(t, 2.0, c.BurstMultiple)
	assert.Equal(t, uint(3), c.BurstDetectionDelay)
	assert.Equal(t, 500, c.MaxKeys)
	assert.Equal(t, 10, c.GoalSampleRate)
}

func TestFactory_CreateExtension_Valid(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.SamplingAttributes = []string{"service.name"}

	ext, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)
}

func TestFactory_CreateExtension_Invalid(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	// Missing SamplingAttributes

	_, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.Error(t, err)
}

func TestExtension_StartShutdown(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.SamplingAttributes = []string{"service.name"}

	ext, err := factory.Create(context.Background(), extensiontest.NewNopSettings(typeStr), cfg)
	require.NoError(t, err)

	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, ext.Shutdown(context.Background()))
}

func TestExtension_NewEvaluator(t *testing.T) {
	cfg := &Config{
		AdjustmentInterval:  15 * time.Second,
		Weight:              0.5,
		AgeOutValue:         0.5,
		BurstMultiple:       2.0,
		BurstDetectionDelay: 3,
		MaxKeys:             500,
		GoalSampleRate:      10,
		SamplingAttributes:  []string{"service.name"},
	}

	ext := newEmaSamplingExtension(cfg, componenttest.NewNopTelemetrySettings().Logger)
	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	defer ext.Shutdown(context.Background()) //nolint:errcheck

	evaluator, err := ext.NewEvaluator("test-policy", nil)
	require.NoError(t, err)
	require.NotNil(t, evaluator)
	assert.True(t, evaluator.IsStateful())
}
