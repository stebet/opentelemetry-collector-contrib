#!/usr/bin/env bash
# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

image="${1:-otelcol-ema:local}"
docker run --rm "$image" validate --config=/etc/otel/config.yaml
components=$(docker run --rm "$image" components)
for component in ema_sampling ema_throughput ema_sampling_policy ema_throughput_policy adaptive_tail_sampling redis_fleet_tracker; do
  if ! grep -qw "$component" <<< "$components"; then
    echo "Missing component in image: $component" >&2
    exit 1
  fi
done

adaptive_config=$(cat <<'YAML'
processors:
  adaptive_tail_sampling:
    decision_delay: 100ms
    rules:
      - name: smoke-test
        sampler:
          type: adaptive_percentage
          goal_percentage: 100
          fingerprint_attributes: ['resource.attributes["service.name"]']
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:
      processors: [memory_limiter, adaptive_tail_sampling, batch]
YAML
)
fleet_config=$(cat <<'YAML'
extensions:
  redis_fleet_tracker:
    endpoint: localhost:6379
processors:
  adaptive_tail_sampling:
    fleet_tracker: redis_fleet_tracker
    rules:
      - name: fleet-throughput
        sampler:
          type: adaptive_throughput
          goal_throughput: 1000
          fingerprint_attributes: ['resource.attributes["service.name"]']
service:
  extensions: [health_check, redis_fleet_tracker]
YAML
)
# Validate the optional fleet configuration without requiring a Redis server.
docker run --rm "$image" validate --config=/etc/otel/config.yaml \
  --config="yaml:$adaptive_config" --config="yaml:$fleet_config"

container=
cleanup() {
  if [[ -n "$container" ]]; then
    docker logs "$container"
    docker stop -t 15 "$container" >/dev/null
  fi
}
trap cleanup EXIT

smoke_test() {
  local name="$1" expected="$2"
  shift 2
  container=$(docker run -d --rm -e EMA_GOAL_SAMPLE_RATE=1 \
    -p 127.0.0.1::4318 -p 127.0.0.1::13133 "$image" "$@")
  local http_port health_port
  http_port=$(docker port "$container" 4318/tcp | awk -F: '{print $NF}')
  health_port=$(docker port "$container" 13133/tcp | awk -F: '{print $NF}')
  curl --fail --silent --show-error --retry 20 --retry-all-errors --retry-delay 1 \
    "http://127.0.0.1:${health_port}/"
  curl --fail --silent --show-error -H 'Content-Type: application/json' \
    "http://127.0.0.1:${http_port}/v1/traces" --data-binary @- <<'JSON'
{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"ema-smoke-test"}}]},"scopeSpans":[{"spans":[{"traceId":"0123456789abcdef0123456789abcdef","spanId":"0123456789abcdef","name":"ema-smoke-test","startTimeUnixNano":"1789038000000000000","endTimeUnixNano":"1789038000001000000"}]}]}]}
JSON

  # Allow the EMA decision window and batch timeout to elapse.
  for _ in $(seq 1 30); do
    if docker logs "$container" 2>&1 | grep -Eq "$expected"; then
      echo "$name trace export smoke test passed."
      docker stop -t 15 "$container" >/dev/null
      container=
      return
    fi
    sleep 1
  done
  echo "$name: no sampled span reached the debug exporter." >&2
  return 1
}

# Keep both tests deterministic without changing the bundled EMA configuration.
smoke_test EMA '"spans":\s*1([,}])'
smoke_test adaptive_tail_sampling 'otelcol.processor.adaptive_tail_sampling.rule: Str\(smoke-test\)' \
  --config=/etc/otel/config.yaml --config="yaml:$adaptive_config"
