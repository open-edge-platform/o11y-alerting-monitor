# SPDX-FileCopyrightText: (C) 2026 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# Building environment
FROM golang:1.26.3-alpine3.23@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS build

WORKDIR /workspace

RUN apk add --upgrade --no-cache make=~4 bash=~5

# Copy everything and download deps
COPY . .

# Build binary
RUN make build-alerting-monitor

# Actual container with alerting monitor
FROM alpine:3.24@sha256:a2d49ea686c2adfe3c992e47dc3b5e7fa6e6b5055609400dc2acaeb241c829f4
# Upgrade zlib to fix CVE-2026-22184
RUN apk add --upgrade --no-cache curl=~8 "zlib>=1.3.2-r0" "musl-utils>=1.2.5-r23"

COPY --from=build /workspace/build/alerting-monitor /alerting-monitor

RUN addgroup -S monitor && adduser -S monitor -G monitor
USER monitor

ENTRYPOINT ["/alerting-monitor"]
