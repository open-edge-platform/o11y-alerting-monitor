# SPDX-FileCopyrightText: (C) 2026 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# Building environment
FROM golang:1.26.2-alpine3.23@sha256:c2a1f7b2095d046ae14b286b18413a05bb82c9bca9b25fe7ff5efef0f0826166 AS build

WORKDIR /workspace

RUN apk add --upgrade --no-cache make=~4 bash=~5

# Copy everything and download deps
COPY . .

# Build binary
RUN make build-alerting-monitor

# Actual container with alerting monitor
FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659
# Upgrade zlib to fix CVE-2026-22184
RUN apk add --upgrade --no-cache curl=~8 "zlib>=1.3.2-r0"

COPY --from=build /workspace/build/alerting-monitor /alerting-monitor

RUN addgroup -S monitor && adduser -S monitor -G monitor
USER monitor

ENTRYPOINT ["/alerting-monitor"]
