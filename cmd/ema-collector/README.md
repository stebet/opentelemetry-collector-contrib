# EMA Collector

Custom Collector distribution containing the `ema_sampling` and `ema_throughput`
trace processors and the `ema_sampling_policy` and `ema_throughput_policy`
extensions for `tail_sampling`. The complete component list is in
[builder-config.yaml](builder-config.yaml); this is a selected distribution,
not the full Contrib binary.

## Build and run

Run from the repository root with Docker Desktop in Linux container mode or a
Linux Docker engine. The build uses Go 1.26 and the Collector Builder pinned to
the core revision used by this checkout. All Contrib modules, including EMA and
tail sampling, are resolved from local source.

```sh
docker build -f cmd/ema-collector/Dockerfile -t otelcol-ema:local .
docker run --rm -p 4317:4317 -p 4318:4318 -p 13133:13133 otelcol-ema:local
```

The bundled [configuration](config.yaml) receives OTLP traces over gRPC (4317)
and HTTP (4318), applies EMA sampling keyed by `service.name`, then exports
surviving traces to the console. Health is available at `http://localhost:13133/`.
EMA targets a sample rate of 1 in 10; override it with
`-e EMA_GOAL_SAMPLE_RATE=20`, for example. Decisions wait ten seconds for spans
to arrive. Metrics and logs pipelines are not enabled in this example.

The image runs as UID/GID 10001, includes CA certificates, and supports
`linux/amd64` and `linux/arm64`. Build for another architecture with
`docker buildx build --platform linux/arm64 --load` and the same Dockerfile/tag
arguments. For a multi-platform registry image, use
`--platform linux/amd64,linux/arm64 --push` with your registry tag.

## Export to a backend

Copy `config.yaml`, replace the `debug` exporter with your backend, and update
the pipeline's exporters list. For an OTLP/gRPC backend with TLS:

```yaml
exporters:
  otlp_grpc/backend:
    endpoint: ${env:BACKEND_OTLP_ENDPOINT}

# Inside service.pipelines.traces:
# exporters: [otlp_grpc/backend]
```

Mount the complete edited configuration, readable by UID 10001:

```sh
docker run --rm -p 4317:4317 -p 4318:4318 -p 13133:13133 \
  -e BACKEND_OTLP_ENDPOINT=collector.example.com:4317 \
  --mount type=bind,source=/absolute/path/config.yaml,target=/etc/otel/config.yaml,readonly \
  otelcol-ema:local
```

Set authentication and TLS options as required by your backend. Keep all spans
of a trace routed to the same Collector instance for tail sampling. Adjust the
memory limiter and trace capacity for your workload and container memory limit.
To compose sampling policies, use the [EMA policy extension](../../processor/emaprocessor/emasamplingextension/README.md)
with `tail_sampling` instead of the standalone `ema_sampling` processor.

## Verify

```sh
docker run --rm otelcol-ema:local components
docker run --rm otelcol-ema:local validate --config=/etc/otel/config.yaml
bash cmd/ema-collector/smoke-test.sh otelcol-ema:local
```

The smoke test requires Bash, Docker and curl. It checks the four EMA factories,
configuration, health endpoint, and export of an OTLP trace with the sample rate
set to one. It removes its temporary container on exit. The Docker workflow runs
EMA tests with the race detector and this smoke test before publishing.

When merging upstream, update this manifest, the Dockerfile's builder version,
and `processor/emaprocessor/go.mod` together to match the core/Contrib versions
in `cmd/otelcontribcol/builder-config.yaml`, then run `go mod tidy` and the tests
in `processor/emaprocessor`.
