<!--
SPDX-FileCopyrightText: (C) 2025 Intel Corporation
SPDX-License-Identifier: Apache-2.0
-->

# Alerting Monitor Changelog

## [v1.6.30](https://github.com/open-edge-platform/o11y-alerting-monitor/tree/1.6.30) (2025-03-31)

- Initial release
- Application `alerting-monitor` added:
  - Deployable via Helm Chart, scalable with [HPA](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
  - Exposes [REST API](./api/v1/openapi.yaml) endpoints for configuring and viewing Alerts
  - Multitenancy support with management endpoint exposed via `gRPC`
  - Alert configuration persisted in [PostgreSQL](https://www.postgresql.org/)-compliant Database
  - Database schema management and migrations done using [Atlas](https://atlasgo.io/)
  - Asynchronous reconfiguration of `Grafana Mimir` and `Alertmanager`
  - Email notifications delivery via dependent [Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/)
  - Integrated `devMode` with support for [Mailpit](https://github.com/axllent/mailpit) email testing
