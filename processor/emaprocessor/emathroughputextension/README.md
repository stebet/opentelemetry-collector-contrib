# EMA Throughput Policy Extension

This extension implements `samplingpolicy.Extension` for use with the
[tail_sampling processor](../../tailsamplingprocessor/README.md).
It exposes an `Evaluate` method that uses the `dynsampler-go` `EMAThroughput`
algorithm to make per-trace sampling decisions keyed on configurable attributes,
targeting a configurable traces-per-second throughput.

## Per-Evaluator Isolated Samplers

Each call to `NewEvaluator` creates an independent EMA sampler with its own
rate table, so multiple tail_sampling policies can reference the same extension
instance without sharing state. Each policy's traffic is tracked and adapted
independently toward the same throughput goal.

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

## Example usage with tail_sampling

```yaml
extensions:
  ema_throughput_policy:
    goal_throughput_per_sec: 500  # forward up to 500 traces/sec
    initial_sample_rate: 10       # sample rate before EMA converges
    sampling_attributes:
      - service.name
    adjustment_interval: 15s
    weight: 0.5
    age_out_value: 0.5

processors:
  tail_sampling:
    decision_wait: 10s
    num_traces: 100000
    policies:
      - name: ema-throughput-policy
        type: ema_throughput_policy   # must match the extension component ID above

service:
  extensions: [ema_throughput_policy]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [tail_sampling]
      exporters: [otlp]
```

> **Tip:** To sample errors at full rate alongside EMA throughput control, add a
> separate `status_code: ERROR` policy and combine them with the `and` composite
> policy. The `tailsamplingprocessor` handles this natively.
