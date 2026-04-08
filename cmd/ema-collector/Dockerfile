# syntax=docker/dockerfile:1
#
# Multi-stage build for the custom OTel Collector distribution that includes
# the EMA Tail Sampling Processor and the components used by HaveIBeenPwned.
#
# Requires BuildKit (enabled by default in Docker 23+ and docker/build-push-action).

# ── build stage ──────────────────────────────────────────────────────────────
# Run the builder on the host platform so we can cross-compile without QEMU.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

RUN apk add --no-cache git bash

# Install the OTel Collector Builder (OCB) at the pinned version.
# The module cache mount avoids re-downloading on every build.
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go install go.opentelemetry.io/collector/cmd/builder@v0.149.1-0.20260402195938-76ede073ee8e

WORKDIR /src

# Copy the repository. .dockerignore at the root excludes .git and large
# generated/test artefacts so the build context stays small.
COPY . .

# Inject replace directives for every local go.mod so OCB resolves all
# in-repo modules from source instead of the module proxy.
RUN bash ./internal/buildscripts/ocb-add-replaces.sh ema-collector

# Generate the collector Go module AND compile the binary.
# The cache mounts keep the module download cache and the Go build cache
# across rebuilds so only changed packages are recompiled.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
# Use a per-arch build cache id so amd64 and arm64 builds don't share
# compiled object files (they are not cross-architecture compatible).
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,id=go-build-${TARGETARCH} \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    builder --config=cmd/ema-collector/builder-config-replaced.yaml

# ── certs stage ──────────────────────────────────────────────────────────────
FROM alpine:3.21 AS certs
RUN apk --update add ca-certificates

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM scratch

ARG USER_UID=10001
ARG USER_GID=10001
USER ${USER_UID}:${USER_GID}

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /src/cmd/ema-collector/ema-collector /ema-collector

EXPOSE 4317 4318 55679 55680
ENTRYPOINT ["/ema-collector"]
CMD ["--config", "/etc/otel/config.yaml"]
