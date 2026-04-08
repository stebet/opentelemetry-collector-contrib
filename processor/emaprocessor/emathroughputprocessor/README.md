# EMA Throughput Tail Sampling Processor

The EMA Throughput processor uses an Exponential Moving Average (EMA) algorithm to
dynamically adjust per-key sampling rates so that the aggregate throughput of kept
traces converges toward a configurable goal (traces per second).

## Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `goal_throughput_per_sec` | int | 100 | Target traces per second to forward. |
| `initial_sample_rate` | int | 10 | Sample rate before EMA converges. |
| `adjustment_interval` | duration | 15s | How often the EMA is recalculated. |
| `weight` | float64 | 0.5 | EMA smoothing factor (0, 1). |
| `age_out_value` | float64 | 0.5 | Decay factor for idle keys (0, 1). |
| `burst_multiple` | float64 | 2.0 | Rate multiplier when a burst is detected. |
| `burst_detection_delay` | uint | 3 | Intervals to wait before burst detection activates. |
| `sampling_attributes` | []string | required | Attributes used to build per-key sampling decisions. |
| `max_keys` | int | 500 | Maximum distinct keys tracked. |
| `use_trace_length` | bool | false | Include span count in the sampling key. |
| `decision_wait` | duration | 30s | Time to wait before making a sampling decision. |
| `num_traces` | uint64 | 50000 | Maximum traces held in memory. |
| `add_sample_rate_attribute` | bool | true | Stamp `sampling.sample_rate` on kept spans. |
