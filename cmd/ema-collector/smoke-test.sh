#!/usr/bin/env bash
# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

image="${1:-otelcol-ema:local}"
docker run --rm "$image" validate --config=/etc/otel/config.yaml
components=$(docker run --rm "$image" components)
for component in ema_sampling ema_throughput ema_sampling_policy ema_throughput_policy; do
  grep -q "${component}" <<< "$components"
done

# A rate of one makes the smoke test deterministic. Production defaults to ten.
container=$(docker run -d --rm -e EMA_GOAL_SAMPLE_RATE=1 \
  -p 127.0.0.1::4318 -p 127.0.0.1::13133 "$image")
cleanup() {
  docker logs "$container"
  docker stop -t 15 "$container" >/dev/null
}
trap cleanup EXIT
http_port=$(docker port "$container" 4318/tcp | awk -F: '{print $NF}')
health_port=$(docker port "$container" 13133/tcp | awk -F: '{print $NF}')
curl --fail --silent --show-error --retry 20 --retry-connrefused --retry-delay 1 \
  "http://127.0.0.1:${health_port}/"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  "http://127.0.0.1:${http_port}/v1/traces" --data-binary @- <<'JSON'
{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"ema-smoke-test"}}]},"scopeSpans":[{"spans":[{"traceId":"0123456789abcdef0123456789abcdef","spanId":"0123456789abcdef","name":"ema-smoke-test","startTimeUnixNano":"1789038000000000000","endTimeUnixNano":"1789038000001000000"}]}]}]}
JSON

# Allow the ten-second decision window and batch timeout to elapse.
for attempt in $(seq 1 30); do
  if docker logs "$container" 2>&1 | grep -Eq '"spans":\s*1([,}])'; then
    echo 'EMA trace export smoke test passed.'
    exit 0
  fi
  sleep 1
done
echo 'No sampled span reached the debug exporter.' >&2
exit 1
