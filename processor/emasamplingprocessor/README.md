# EMA Tail Sampling Processor

| Status        |                   |
| ------------- | ----------------- |
| Stability     | [development]: traces |
| Distributions | [contrib]         |
| Issues        | [![Open issues](https://img.shields.io/github/issues-search/open-telemetry/opentelemetry-collector-contrib?query=is%3Aissue%20is%3Aopen%20label%3Aprocessor%2Femasampleing%20&label=open&color=orange&logo=opentelemetry)](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues?q=is%3Aopen+is%3Aissue+label%3Aprocessor%2Femasampleing) |
| Code Owners   | Seeking code owners! |

[development]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/component-stability.md#development
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib

The **EMA Tail Sampling Processor** applies an [Exponential Moving Average (EMA)](https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average)
dynamic sampling algorithm to traces. It automatically adjusts per-key sample
rates to hit a target goal sample rate, reacting to changes in traffic patterns
over time.

All spans for a given trace **must** be received by the same collector instance.

This processor is based on the `EMASampleRate` algorithm from
[honeycombio/dynsampler-go](https://github.com/honeycombio/dynsampler-go).

## How it works

1. Spans arriving at the processor are buffered in memory, grouped by trace ID.
2. After `decision_wait`, the processor builds a **sampling key** for the trace
   by reading the first value found for each configured `sampling_attributes`
   across the trace's resource and span attributes.
3. The EMA sampler returns a sample rate for that key based on recent traffic
   observed for the same key.
4. The processor keeps the trace with probability `1/rate` and forwards all its
   spans to the next consumer.

The EMA algorithm ensures hot keys (high-traffic attribute combinations) are
sampled more aggressively while cold keys (low-traffic) approach a rate of 1
(keep all). The sample rates are updated every `adjustment_interval`.

## Configuration

```yaml
processors:
  ema_sampling:
    # Required: attributes used to build the per-key sampling decision.
    sampling_attributes:
      - service.name
      - http.route

    # Target 1-in-N sampling rate. Default: 10.
    goal_sample_rate: 10

    # How often the EMA algorithm recalculates rates. Default: 15s.
    adjustment_interval: 15s

    # EMA smoothing factor in (0, 1). Higher = faster reaction. Default: 0.5.
    weight: 0.5

    # EMA decay for cold keys, in (0, 1). Default: 0.5.
    age_out_value: 0.5

    # Sample rate multiplier during detected burst traffic. Default: 2.0.
    burst_multiple: 2.0

    # Intervals before burst detection activates after startup. Default: 3.
    burst_detection_delay: 3

    # Max distinct sampling keys to track. Default: 500.
    max_keys: 500

    # When true, span count is appended to the sampling key. Default: false.
    use_trace_length: false

    # When true (default), adds a sampling.sample_rate integer attribute to
    # every span of a kept trace. The value is the EMA 1-in-N rate used for
    # the sampling decision. Useful for downstream rate-correction calculations.
    add_sample_rate_attribute: true

    # When true, any trace containing a span with StatusCode == Error or an
    # event named "exception" is always forwarded, bypassing the EMA sampling
    # decision. Default: false.
    always_sample_errors: false

    # Time to buffer a trace before deciding. Default: 30s.
    decision_wait: 30s

    # Maximum number of traces held in memory. Default: 50000.
    num_traces: 50000
```

## Configuration reference

| Field | Type | Default | Required | Description |
|---|---|---|---|---|
| `sampling_attributes` | `[]string` | — | ✅ | Span or resource attribute keys used to build the sampling key. |
| `goal_sample_rate` | `int` | `10` | | Target 1-in-N sample rate. Must be ≥ 1. |
| `adjustment_interval` | `duration` | `15s` | | How often the EMA recalculates per-key sample rates. |
| `weight` | `float64` | `0.5` | | EMA smoothing factor. Must be in (0, 1). |
| `age_out_value` | `float64` | `0.5` | | EMA decay for unused keys. Must be in (0, 1). |
| `burst_multiple` | `float64` | `2.0` | | Sample rate multiplier during burst traffic. |
| `burst_detection_delay` | `uint` | `3` | | Adjustment intervals before burst detection activates. |
| `max_keys` | `int` | `500` | | Maximum distinct sampling keys tracked. |
| `use_trace_length` | `bool` | `false` | | Append span count to the sampling key. |
| `add_sample_rate_attribute` | `bool` | `true` | | Stamp `sampling.sample_rate` (int) on every span of a kept trace. |
| `always_sample_errors` | `bool` | `false` | | Always forward traces containing a span with `StatusCode == Error` or an `"exception"` event. |
| `decision_wait` | `duration` | `30s` | | Time to buffer a trace before a sampling decision. |
| `num_traces` | `uint64` | `50000` | | Maximum traces held in memory. Oldest traces are evicted when exceeded. |

## Differences from the Tail Sampling Processor

| Feature | `tailsamplingprocessor` | `emasamplingprocessor` |
|---|---|---|
| Policy model | Multiple configurable policies | Single EMA policy |
| Rate adaptation | Static per-policy rules | Dynamic — adjusts automatically to hit goal rate |
| Configuration | Policy list | Simple key/value EMA parameters |
| Use case | Complex routing logic | Dynamic rate control for high-cardinality keys |

## Known limitations

- **Late-arriving spans**: If spans for a trace arrive _after_ the sampling
  decision has been made (i.e. after `decision_wait` has elapsed), those spans
  start a fresh buffer entry with a new independent sampling decision. This can
  result in a trace being partially kept and partially dropped. This is a
  fundamental trade-off of all tail sampling approaches that use a fixed
  decision window.

- **Single instance**: All spans for a given trace must arrive at the **same**
  collector instance. Use a load balancer that routes by trace ID (e.g. the
  [Load Balancing Exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/loadbalancingexporter))
  upstream of this processor.

## Internal telemetry

See [documentation.md](./documentation.md) for a full description of the
metrics emitted by this processor.
