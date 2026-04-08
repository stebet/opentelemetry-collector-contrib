// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package emasamplingprocessor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	valid := func() *Config {
		return &Config{
			GoalSampleRate:     10,
			AdjustmentInterval: 15 * time.Second,
			Weight:             0.5,
			AgeOutValue:        0.5,
			BurstMultiple:      2.0,
			SamplingAttributes: []string{"service.name"},
			MaxKeys:            500,
			DecisionWait:       30 * time.Second,
			NumTraces:          50000,
		}
	}

	t.Run("valid config", func(t *testing.T) {
		require.NoError(t, valid().Validate())
	})

	t.Run("goal_sample_rate zero", func(t *testing.T) {
		cfg := valid()
		cfg.GoalSampleRate = 0
		assert.Error(t, cfg.Validate())
	})

	t.Run("goal_sample_rate negative", func(t *testing.T) {
		cfg := valid()
		cfg.GoalSampleRate = -1
		assert.Error(t, cfg.Validate())
	})

	t.Run("weight zero", func(t *testing.T) {
		cfg := valid()
		cfg.Weight = 0
		assert.Error(t, cfg.Validate())
	})

	t.Run("weight one", func(t *testing.T) {
		cfg := valid()
		cfg.Weight = 1.0
		assert.Error(t, cfg.Validate())
	})

	t.Run("age_out_value zero", func(t *testing.T) {
		cfg := valid()
		cfg.AgeOutValue = 0
		assert.Error(t, cfg.Validate())
	})

	t.Run("age_out_value one", func(t *testing.T) {
		cfg := valid()
		cfg.AgeOutValue = 1.0
		assert.Error(t, cfg.Validate())
	})

	t.Run("empty sampling_attributes", func(t *testing.T) {
		cfg := valid()
		cfg.SamplingAttributes = nil
		assert.Error(t, cfg.Validate())
	})

	t.Run("decision_wait zero", func(t *testing.T) {
		cfg := valid()
		cfg.DecisionWait = 0
		assert.Error(t, cfg.Validate())
	})

	t.Run("num_traces zero", func(t *testing.T) {
		cfg := valid()
		cfg.NumTraces = 0
		assert.Error(t, cfg.Validate())
	})
}
