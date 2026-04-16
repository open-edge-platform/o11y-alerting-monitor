# SPDX-FileCopyrightText: (C) 2026 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# Building environment
FROM golang:1.26.2-alpine3.23@sha256:27f829349da645e287cb195a9921c106fc224eeebbdc33aeb0f4fca2382befa6 AS build

WORKDIR /workspace

RUN apk add --upgrade --no-cache make=~4 bash=~5

# Copy everything and download deps
COPY . .

# Build binary
RUN make build-alerting-monitor

# Actual container with alerting monitor
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
# Upgrade zlib to fix CVE-2026-22184
RUN apk add --upgrade --no-cache curl=~8 "zlib>=1.3.2-r0" "musl-utils>=1.2.5-r23"

COPY --from=build /workspace/build/alerting-monitor /alerting-monitor

RUN addgroup -S monitor && adduser -S monitor -G monitor
USER monitor

ENTRYPOINT ["/alerting-monitor"]
