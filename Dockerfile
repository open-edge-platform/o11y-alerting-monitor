# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

# Building environment
FROM golang:1.24.6-alpine@sha256:c8c5f95d64aa79b6547f3b626eb84b16a7ce18a139e3e9ca19a8c078b85ba80d AS build

WORKDIR /workspace

RUN apk add --upgrade --no-cache make=~4 bash=~5

# Copy everything and download deps
COPY . .

# Build binary
RUN go mod download && make build-alerting-monitor

# Actual container with alerting monitor
FROM alpine:3.22@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1

RUN apk add --upgrade --no-cache curl=~8

COPY --from=build /workspace/build/alerting-monitor /alerting-monitor

RUN addgroup -S monitor && adduser -S monitor -G monitor
USER monitor

ENTRYPOINT ["/alerting-monitor"]
