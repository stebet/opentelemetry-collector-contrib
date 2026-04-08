# EMA Throughput Policy Extension

This extension implements `samplingpolicy.Extension` for use with the
[tail_sampling processor](../../tailsamplingprocessor/README.md).
It exposes an `Evaluate` method that uses the `dynsampler-go` `EMAThroughput`
algorithm to make per-trace sampling decisions keyed on configurable attributes,
targeting a configurable traces-per-second throughput.

## Shared Sampler Semantics

**Each extension instance maintains its own EMA rate table.** All evaluators
created from the same extension instance share that single rate table — this is
intentional, so that the EMA algorithm sees the aggregate traffic across all
policies using the same extension and converges correctly.

If you need separate rate tracking for different policies (e.g., different
throughput budgets per service tier), configure separate extension instances:

```yaml
extensions:
  ema_throughput_policy/tier1:
    goal_throughput_per_sec: 50
    sampling_attributes: [service.name]
  ema_throughput_policy/tier2:
    goal_throughput_per_sec: 500
    sampling_attributes: [service.name]
```

## Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `goal_throughput_per_sec` | int | 100 | Target traces per second to forward. |
| `initial_sample_rate` | int | 10 | Sample rate used before EMA converges. |
| `adjustment_interval` | duration | 15s | How often the EMA is recalculated. |
| `weight` | float64 | 0.5 | EMA smoothing factor (0, 1) exclusive. |
| `age_out_value` | float64 | 0.5 | Decay factor for idle keys (0, 1) exclusive. |
| `burst_multiple` | float64 | 2.0 | Rate multiplier for burst detection. |
| `burst_detection_delay` | uint | 3 | Intervals before burst detection is active. |
| `sampling_attributes` | []string | required | Attributes used to build per-key sampling decisions. |
| `max_keys` | int | 500 | Maximum distinct keys tracked. |
| `use_trace_length` | bool | false | Include span count in the sampling key. |
