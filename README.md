<!--
SPDX-FileCopyrightText: (C) 2025 Intel Corporation
SPDX-License-Identifier: Apache-2.0
-->

# Edge Orchestrator Alerting Monitor

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/open-edge-platform/o11y-alerting-monitor/badge)](https://scorecard.dev/viewer/?uri=github.com/open-edge-platform/o11y-alerting-monitor)

[Web UI]: https://github.com/open-edge-platform/orch-ui
[Edge Node Observability Stack]: https://github.com/open-edge-platform/o11y-charts/tree/main/charts/edgenode-observability
[Keycloak IAM]: https://github.com/open-edge-platform/edge-manageability-framework/blob/main/argocd/applications/templates/platform-keycloak.yaml

[Documentation]: https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/observability/arch/index.html
[Edge Orchestrator Community]: https://github.com/open-edge-platform
[Troubleshooting]: https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/troubleshooting/index.html
[Contact us]: https://github.com/open-edge-platform

[Apache 2.0 License]: LICENSES/Apache-2.0.txt
[Contributor's Guide]: https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/contributor_guide/index.html

[Email Notifications]: https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/observability/tutorials/development/email-notifications.html

## Overview

Alerting Monitor service exposes a [REST API](api/v1/openapi.yaml) that serves as a backend for the [Web UI], which in turn allows users with administrative privileges to configure `Alerts` based on telemetry collected by the [Edge Node Observability Stack]. The service allows configuration of:

- `Alert Definitions` - a pre-defined set of `Alerts` that can be turned on/off and tweaked via setting `Threshold` and `Duration`
- `Alert Receivers` - allows to select `email` recipients from a set of allowed addresses provided by [Keycloak IAM]

The `Alerts` generated as a result are exposed:

- via `REST API` for consumption by the Web UI
- via `email` if an external `Email Server` was provided

Read more about Alerting Monitor in the [Documentation].

## Get Started

To set up the development environment and work on this project, follow the steps below.
All necessary tools will be installed using the `install-tools` target.
Note that `docker` and `asdf` must be installed beforehand.

## Develop

The code of this project is maintained and released in CI using the `VERSION` file.
In addition, the chart is versioned with the same tag as the `VERSION` file.

This is mandatory to keep all chart versions and app versions coherent.

To bump the version, increment the version in the `VERSION` file and run the following command
(to set `version` and `appVersion` in the `Chart.yaml` automatically):

```sh
make helm-build
```

### Install Tools

To install all the necessary tools needed for development the project, run:

```sh
make install-tools
```

### Build

To build the project, use the following command:

```sh
make build
```

### Lint

To lint the code and ensure it adheres to the coding standards, run:

```sh
make lint
```

### Test

To run the tests and verify the functionality of the project, use:

```sh
make test
```

### Docker Build

To build the Docker images for the project, run:

```sh
make docker-build
```

### Helm Build

To package the Helm chart for the project, use:

```sh
make helm-build
```

### Docker Push

To push the Docker images to the registry, run:

```sh
make docker-push
```

### Helm Push

To push the Helm chart to the repository, use:

```sh
make helm-push
```

### Kind All

To load the Docker images into a local Kind cluster, run:

```sh
make kind-all
```

### Codegen All

To generate API code from openapi definition, run:

```sh
make codegen-all
```

### Proto

To generate code from protobuf definitions, use:

```sh
make proto
```

### Verify Migration

To verify if database migration files reflect the current schema, run:

```sh
make verify-migration
```

### Codegen Database

To generate migrate files after database schema update, run:

```sh
make codegen-database
```

### Developing Alert Email Notifications

Details regarding email notifications via MailPit can be found here: [Email Notifications].

## Contribute

To learn how to contribute to the project, see the [Contributor's Guide].

## Community and Support

To learn more about the project, its community, and governance, visit the [Edge Orchestrator Community].

For support, start with [Troubleshooting] or [Contact us].

## License

Edge Orchestrator Alerting Monitor is licensed under [Apache 2.0 License].

Last Updated Date: {March 27, 2025}
