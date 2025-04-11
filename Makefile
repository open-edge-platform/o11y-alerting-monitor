# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

SHELL := bash -eu -o pipefail

PROJECT_NAME                      := alerting-monitor

## Labels to add Docker/Helm/Service CI meta-data.
LABEL_REVISION                    = $(shell git rev-parse HEAD)
LABEL_CREATED                     ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

VERSION                           ?= $(shell cat VERSION | tr -d '[:space:]')
BUILD_DIR                         ?= ./build

## CHART_NAME is specified in Chart.yaml
CHART_NAME                        ?= $(PROJECT_NAME)
## CHART_VERSION is specified in Chart.yaml
CHART_VERSION                     ?= $(shell grep "version:" ./deployments/$(PROJECT_NAME)/Chart.yaml  | cut -d ':' -f 2 | tr -d '[:space:]')
## CHART_APP_VERSION is modified on every commit
CHART_APP_VERSION                 ?= $(VERSION)
## CHART_BUILD_DIR is given based on repo structure
CHART_BUILD_DIR                   ?= $(BUILD_DIR)/chart/
## CHART_PATH is given based on repo structure
CHART_PATH                        ?= "./deployments/$(CHART_NAME)"
## CHART_NAMESPACE can be modified here
CHART_NAMESPACE                   ?= orch-infra
## CHART_RELEASE can be modified here
CHART_RELEASE                     ?= $(PROJECT_NAME)

REGISTRY                          ?= 080137407410.dkr.ecr.us-west-2.amazonaws.com
REGISTRY_NO_AUTH                  ?= edge-orch
REPOSITORY                        ?= o11y
REPOSITORY_NO_AUTH                := $(REGISTRY)/$(REGISTRY_NO_AUTH)/$(REPOSITORY)
DOCKER_IMAGE_NAME                 ?= $(PROJECT_NAME)
DOCKER_MANAGEMENT_IMAGE_NAME      ?= $(PROJECT_NAME)-management
DOCKER_IMAGE_TAG                  ?= $(VERSION)

DOCKER_REGISTRY_READ_PATH          = registry-rs.edgeorchestration.intel.com/edge-orch/o11y


DOCKER_FILES_TO_LINT              := $(shell find . -type f -name 'Dockerfile*' -print )

GOCMD         := CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go
GOCMD_TEST    := CGO_ENABLED=1 GOARCH=amd64 GOOS=linux go
GOEXTRAFLAGS  :=-trimpath -mod=readonly -gcflags="all=-spectre=all -N -l" -asmflags="all=-spectre=all" -ldflags="all=-s -w -X main.version=$(shell cat ./VERSION)"

FUZZ-DURATION-MINUTES ?= 1

.DEFAULT_GOAL := help
.PHONY: build

## CI Mandatory Targets start
dependency-check:
	@# Help: Unsupported target
	@echo '"make $@" is unsupported'

build: build-alerting-monitor build-management
	@# Help: Builds alerting-monitor and management

lint: lint-go lint-schema lint-markdown lint-yaml lint-proto lint-json lint-shell lint-docker lint-license
	@# Help: Runs all linters

test:
	@# Help: Runs tests and creates a coverage report
	@echo "---MAKEFILE TEST---"
	$(GOCMD_TEST) test ./... --race -coverprofile $(BUILD_DIR)/coverage.out -covermode atomic
	gocover-cobertura < $(BUILD_DIR)/coverage.out > $(BUILD_DIR)/coverage.xml
	@echo "---END MAKEFILE TEST---"

docker-build: docker-build-alerting-monitor docker-build-management
	@# Help: Builds all docker images

helm-build: helm-clean
	@# Help: Builds the helm chart
	@echo "---MAKEFILE HELM-BUILD---"
	yq eval -i '.version = "$(VERSION)"' $(CHART_PATH)/Chart.yaml
	yq eval -i '.appVersion = "$(VERSION)"' $(CHART_PATH)/Chart.yaml
	yq eval -i '.annotations.revision = "$(LABEL_REVISION)"' $(CHART_PATH)/Chart.yaml
	yq eval -i '.annotations.created = "$(LABEL_CREATED)"' $(CHART_PATH)/Chart.yaml
	helm package \
		--app-version=$(CHART_APP_VERSION) \
		--debug \
		--dependency-update \
		--destination $(CHART_BUILD_DIR) \
		$(CHART_PATH)

	@$(MAKE) lint-helm # Lint here, because linter expects dependencies to be present
	@echo "---END MAKEFILE HELM-BUILD---"

docker-push: docker-push-alerting-monitor docker-push-management
	@# Help: Pushes all docker images

helm-push:
	@# Help: Pushes the helm chart
	@echo "---MAKEFILE HELM-PUSH---"
	aws ecr create-repository --region us-west-2 --repository-name $(REGISTRY_NO_AUTH)/$(REPOSITORY)/charts/$(CHART_NAME) || true
	helm push $(CHART_BUILD_DIR)$(CHART_NAME)*.tgz oci://$(REPOSITORY_NO_AUTH)/charts
	@echo "---END MAKEFILE HELM-PUSH---"

docker-list: docker-list-header docker-list-alerting-monitor docker-list-management  ## list all docker containers built by this repo

docker-list-header:
	@echo "images:"

helm-list: ## List helm charts, tag format, and versions in YAML format
	@echo "charts:" ;\
  echo "  $(CHART_NAME):" ;\
  echo -n "    "; grep "^version" "${CHART_PATH}/Chart.yaml"  ;\
  echo "    gitTagPrefix: 'v'" ;\
  echo "    outDir: '${CHART_BUILD_DIR}'" ;\

## CI Mandatory Targets end

## Helper Targets start
all: clean build lint test
	@# Help: Runs clean, build, lint, test targets

clean:
	@# Help: Deletes directories created by build targets
	@echo "---MAKEFILE CLEAN---"
	rm -rf $(BUILD_DIR)
	rm -rf $(CHART_PATH)/charts
	rm -rf $(CHART_PATH)/Chart.lock
	@echo "---END MAKEFILE CLEAN---"

helm-clean:
	@# Help: Cleans the build directory of the helm chart
	@echo "---MAKEFILE HELM-CLEAN---"
	rm -rf $(CHART_BUILD_DIR)
	@echo "---END MAKEFILE HELM-CLEAN---"

build-alerting-monitor:
	@# Help: Builds alerting-monitor
	@echo "---MAKEFILE BUILD-ALERTING-MONITOR---"
	$(GOCMD) build $(GOEXTRAFLAGS) -o $(BUILD_DIR)/$(PROJECT_NAME) ./cmd/$(PROJECT_NAME)/$(PROJECT_NAME).go
	@echo "---END MAKEFILE BUILD-ALERTING-MONITOR---"

build-management:
	@# Help: Builds management
	@echo "---MAKEFILE BUILD-MANAGEMENT---"
	$(GOCMD) build $(GOEXTRAFLAGS) -o $(BUILD_DIR)/management ./cmd/management/management.go
	@echo "---END MAKEFILE BUILD-MANAGEMENT---"

lint-go:
	@# Help: Runs linters for golang source code files
	@echo "---MAKEFILE LINT-GO---"
	golangci-lint -v run --build-tags mage
	@echo "---END MAKEFILE LINT-GO---"

lint-schema:
	@# Help: Runs linter for openapi schema
	@echo "---MAKEFILE LINT-SCHEMA---"
	spectral lint -q --display-only-failures api/v1/openapi.yaml
	@echo "---END MAKEFILE LINT-SCHEMA---"

lint-markdown:
	@# Help: Runs linter for markdown files
	@echo "---MAKEFILE LINT-MARKDOWN---"
	markdownlint-cli2 '**/*.md' "!.github" "!**/ci/*"
	@echo "---END MAKEFILE LINT-MARKDOWN---"

lint-yaml:
	@# Help: Runs linter for for yaml files
	@echo "---MAKEFILE LINT-YAML---"
	yamllint -v
	yamllint -f parsable -c yamllint-conf.yaml .
	@echo "---END MAKEFILE LINT-YAML---"

lint-proto:
	@# Help: Runs linter for for proto files
	@echo "---MAKEFILE LINT-PROTO---"
	protolint version
	protolint lint -reporter unix api/
	@echo "---END MAKEFILE LINT-PROTO---"

lint-json:
	@# Help: Runs linter for json files
	@echo "---MAKEFILE LINT-JSON---"
	./scripts/lintJsons.sh
	@echo "---END MAKEFILE LINT-JSON---"

lint-shell:
	@# Help: Runs linter for shell scripts
	@echo "---MAKEFILE LINT-SHELL---"
	shellcheck --version
	shellcheck ./scripts/*.sh
	@echo "---END MAKEFILE LINT-SHELL---"

lint-helm:
	@# Help: Runs linter for helm chart
	@echo "---MAKEFILE LINT-HELM---"
	helm version
	helm lint --strict $(CHART_PATH) --values $(CHART_PATH)/values.yaml
	@echo "---END MAKEFILE LINT-HELM---"

lint-docker:
	@# Help: Runs linter for docker files
	@echo "---MAKEFILE LINT-DOCKER---"
	hadolint --version
	hadolint $(DOCKER_FILES_TO_LINT)
	@echo "---END MAKEFILE LINT-DOCKER---"

lint-license:
	@# Help: Runs license check
	@echo "---MAKEFILE LINT-LICENSE---"
	mage -v lint:license
	@echo "---END MAKEFILE LINT-LICENSE---"

docker-build-alerting-monitor:
	@# Help: Builds alerting-monitor docker image
	@echo "---MAKEFILE DOCKER-BUILD-ALERTING-MONITOR---"
	docker rmi $(REPOSITORY_NO_AUTH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) --force
	docker build -f Dockerfile \
		-t $(REPOSITORY_NO_AUTH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) \
		--build-arg http_proxy="$(http_proxy)" --build-arg https_proxy="$(https_proxy)" --build-arg no_proxy="$(no_proxy)" \
		--platform linux/amd64 --no-cache .
	@echo "---END MAKEFILE DOCKER-BUILD-ALERTING-MONITOR---"

docker-build-management:
	@# Help: Builds management docker image
	@echo "---MAKEFILE DOCKER-BUILD-MANAGEMENT---"
	docker rmi $(REPOSITORY_NO_AUTH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG) --force
	docker build -f Dockerfile.mgmt \
		-t $(REPOSITORY_NO_AUTH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG) \
		--build-arg http_proxy="$(http_proxy)" --build-arg https_proxy="$(https_proxy)" --build-arg no_proxy="$(no_proxy)" \
		--platform linux/amd64 --no-cache .
	@echo "---END MAKEFILE DOCKER-BUILD-MANAGEMENT---"

docker-push-alerting-monitor:
	@# Help: Pushes alerting-monitor docker image
	@echo "---MAKEFILE DOCKER-PUSH-ALERTING-MONITOR---"
	aws ecr create-repository --region us-west-2 --repository-name $(REGISTRY_NO_AUTH)/$(REPOSITORY)/$(DOCKER_IMAGE_NAME) || true
	docker push $(REPOSITORY_NO_AUTH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	@echo "---END MAKEFILE DOCKER-PUSH-ALERTING-MONITOR---"

docker-push-management:
	@# Help: Pushes the management docker image
	@echo "---MAKEFILE DOCKER-PUSH-MANAGEMENT---"
	aws ecr create-repository --region us-west-2 --repository-name $(REGISTRY_NO_AUTH)/$(REPOSITORY)/$(DOCKER_MANAGEMENT_IMAGE_NAME) || true
	docker push $(REPOSITORY_NO_AUTH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	@echo "---END MAKEFILE DOCKER-PUSH-MANAGEMENT---"

docker-list-alerting-monitor:
	@echo "  $(DOCKER_IMAGE_NAME):"
	@echo "    name: '$(REPOSITORY_NO_AUTH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)'"
	@echo "    version: '$(DOCKER_IMAGE_TAG)'"
	@echo "    gitTagPrefix: 'v'"
	@echo "    buildTarget: 'docker-build-alerting-monitor'"

docker-list-management:
	@echo "  $(DOCKER_MANAGEMENT_IMAGE_NAME):"
	@echo "    name: '$(REPOSITORY_NO_AUTH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG)'"
	@echo "    version: '$(DOCKER_IMAGE_TAG)'"
	@echo "    gitTagPrefix: 'v'"
	@echo "    buildTarget: 'docker-build-management'"

kind-all: helm-clean docker-build kind-load helm-build
	@# Help: Builds all images, loads them into the kind cluster and builds the helm chart

kind-load: kind-load-alerting-monitor kind-load-management
	@# Help: Loads all docker images into the kind cluster

kind-load-alerting-monitor:
	@# Help: Loads alerting-monitor docker image into the kind cluster
	@echo "---MAKEFILE KIND-LOAD-ALERTING-MONITOR---"
	docker tag $(REPOSITORY_NO_AUTH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) $(DOCKER_REGISTRY_READ_PATH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	kind load docker-image $(DOCKER_REGISTRY_READ_PATH)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	@echo "---END MAKEFILE KIND-LOAD-ALERTING-MONITOR---"

kind-load-management:
	@# Help: Loads management docker image into the kind cluster
	@echo "---MAKEFILE KIND-LOAD-MANAGEMENT---"
	docker tag $(REPOSITORY_NO_AUTH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG) $(DOCKER_REGISTRY_READ_PATH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	kind load docker-image $(DOCKER_REGISTRY_READ_PATH)/$(DOCKER_MANAGEMENT_IMAGE_NAME):$(DOCKER_IMAGE_TAG)
	@echo "---END MAKEFILE KIND-LOAD-MANAGEMENT---"

test-fuzz:
	@# Help: Runs fuzz test stage for a specified duration (in minutes), example: make test-fuzz FUZZ-DURATION-MINUTES=60
	@echo "---MAKEFILE TEST-FUZZ---"
	mage -v test:fuzz $(FUZZ-DURATION-MINUTES)
	@echo "---END MAKEFILE TEST-FUZZ---"

verify-migration:
	@# Help: Verify if migration files reflect the current schema
	@echo "---MAKEFILE VERIFY-MIGRATION---"
	# Requires installed atlas; to install, run:
	# curl -sSf https://atlasgo.sh | sh
	mage migrate:verify
	@echo "---END MAKEFILE VERIFY-MIGRATION---"

codegen-all: codegen-clean codegen codegen-replace
	@# Help: Runs codegen_clean, codegen, codegen_replace targets

codegen-clean:
	@# Help: Removes the generated boilerplate
	@echo "---MAKEFILE CODEGEN-CLEAN---"
	rm -rf ./api/boilerplate
	@echo "---END MAKEFILE CODEGEN-CLEAN---"

codegen:
	@# Help: Generates code from openapi definition
	@echo "---MAKEFILE CODEGEN---"
	mkdir ./api/boilerplate/
	oapi-codegen -package api -generate types ./api/v1/openapi.yaml > ./api/boilerplate/types.gen.go
	oapi-codegen -package api -generate server ./api/v1/openapi.yaml > ./api/boilerplate/server.gen.go
	@echo "---END MAKEFILE CODEGEN---"

codegen-replace:
	@# Help: Replace snake_case from openapi generated code and copies it to ./api/v1 directory
	@echo "---MAKEFILE CODEGEN-REPLACE---"
	# codegen version 2.3.0 spits out a "WARNING: You are using an OpenAPI 3.1.x specification" which needs to be removed, hence the `tail`
	tail -n +2 api/boilerplate/types.gen.go > ./api/v1/types.go
	tail -n +2 api/boilerplate/server.gen.go > ./api/v1/server.go

	sed -i "s/openapi_types/openapiTypes/" ./api/v1/types.go
	sed -i "s/openapi_types/openapiTypes/" ./api/v1/server.go
	@echo "---END MAKEFILE CODEGEN-REPLACE---"

codegen-database:
	@# Help: Generate migrate files after database schema update
	@echo "---MAKEFILE CODEGEN-DATABASE---"
	# Requires installed atlas; to install, run:
	# curl -sSf https://atlasgo.sh | sh
	mage migrate:schema
	@echo "---END MAKEFILE CODEGEN-DATABASE---"

gen-license:
	@# Help: Runs reuse annotate to set copyright and license headers
	@echo "---MAKEFILE GEN-LICENSE---"
	mage -v gen:license
	@echo "---END MAKEFILE GEN-LICENSE---"

proto:
	@# Help: Regenerates proto-based code
	@echo "---MAKEFILE PROTO---"
    # Requires installed: protoc, protoc-gen-go and protoc-gen-go-grpc
    # See: https://grpc.io/docs/languages/go/quickstart/
	protoc api/v1/management/*.proto --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --proto_path=.
	@echo "---END-MAKEFILE PROTO---"

install-tools:
	@# Help: Installs tools required for the project
	# Requires installed: asdf
	@echo "---MAKEFILE INSTALL-TOOLS---"
	./scripts/installTools.sh .tool-versions
	@echo "---END MAKEFILE INSTALL-TOOLS---"
## Helper Targets end

list: help
	@# Help: Displays make targets

help:
	@# Help: Displays make targets
	@printf "%-35s %s\n" "Target" "Description"
	@printf "%-35s %s\n" "------" "-----------"
	@grep -E '^[a-zA-Z0-9_%-]+:|^[[:space:]]+@# Help:' Makefile | \
	awk '\
		/^[a-zA-Z0-9_%-]+:/ { \
			target = $$1; \
			sub(":", "", target); \
		} \
		/^[[:space:]]+@# Help:/ { \
			if (target != "") { \
				help_line = $$0; \
				sub("^[[:space:]]+@# Help: ", "", help_line); \
				printf "%-35s %s\n", target, help_line; \
				target = ""; \
			} \
		}' | sort -k1,1
