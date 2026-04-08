# EMA Sampling Policy Extension

This extension implements `samplingpolicy.Extension` for use with the
[tail_sampling processor](../../tailsamplingprocessor/README.md).
It exposes an `Evaluate` method that uses the `dynsampler-go` `EMASampleRate`
algorithm to make per-trace sampling decisions keyed on configurable attributes.

## Per-Evaluator Isolated Samplers

Each call to `NewEvaluator` creates an independent EMA sampler with its own
rate table, so multiple tail_sampling policies can reference the same extension
instance without sharing state. Each policy's traffic is tracked and adapted
independently.

If you want multiple policies to share the same rate table (so the EMA sees
aggregate traffic), configure them to use the same extension instance and call
`NewEvaluator` once — or implement that aggregation outside of this extension.

## Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `goal_sample_rate` | int | 10 | Target global sample rate (1 in N traces kept). |
| `adjustment_interval` | duration | 15s | How often the EMA is recalculated. |
| `weight` | float64 | 0.5 | EMA smoothing factor (0, 1) exclusive. |
| `age_out_value` | float64 | 0.5 | Decay factor for idle keys (0, 1) exclusive. |
| `burst_multiple` | float64 | 2.0 | Rate multiplier for burst detection. |
| `burst_detection_delay` | uint | 3 | Intervals before burst detection is active. |
| `sampling_attributes` | []string | required | Attributes used to build per-key sampling decisions. |
| `max_keys` | int | 500 | Maximum distinct keys tracked. |
| `use_trace_length` | bool | false | Include span count in the sampling key. |
