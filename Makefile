# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

TARGET_OS?=$(shell uname)
export TARGET_OS

ROLLUPS_NODE_VERSION := 2.0.0-alpha.8
ROLLUPS_CONTRACTS_VERSION := 2.0.0
ROLLUPS_CONTRACTS_URL:=https://github.com/cartesi/rollups-contracts/releases/download/
ROLLUPS_CONTRACTS_ARTIFACT:=rollups-contracts-$(ROLLUPS_CONTRACTS_VERSION)-artifacts.tar.gz
ROLLUPS_CONTRACTS_SHA256:=7a153b29926857a82e479788cf1e04866a0b8a2cf77ab11a67fe0b3df176cd26
ROLLUPS_PRT_CONTRACTS_VERSION := 1.0.1-test
ROLLUPS_PRT_CONTRACTS_URL:=https://github.com/cartesi/dave/releases/download/
ROLLUPS_PRT_CONTRACTS_ARTIFACT:=cartesi-rollups-prt-contract-artifacts.tar.gz
ROLLUPS_PRT_CONTRACTS_SHA256:=ee5d9f05aef9b573932758be281fdabdaa8a2dab49b7b36f6ea52efeb94ece40

IMAGE_TAG ?= devel

BUILD_TYPE ?= release

ifeq ($(TARGET_OS),Darwin)
PREFIX ?= /opt/cartesi

ifeq ($(MACOSX_DEPLOYMENT_TARGET),)
	export MACOSX_DEPLOYMENT_TARGET := $(shell sw_vers -productVersion | sed -E "s/([[:digit:]]+)\.([[:digit:]]+)\..+/\1.\2.0/")
endif
else
PREFIX ?= /usr
endif

BIN_RUNTIME_PATH= $(PREFIX)/bin
DOC_RUNTIME_PATH= $(PREFIX)/doc/cartesi-rollups-node

BIN_INSTALL_PATH= $(abspath $(DESTDIR)$(BIN_RUNTIME_PATH))
DOC_INSTALL_PATH= $(abspath $(DESTDIR)$(DOC_RUNTIME_PATH))

DEB_ARCH?= $(shell dpkg --print-architecture 2>/dev/null || echo amd64)
DEB_FILENAME= cartesi-rollups-node-v$(ROLLUPS_NODE_VERSION)_$(DEB_ARCH).deb
DEB_PACKAGER_IMG ?= cartesi/rollups-node:debian-packager

# Docker image platform
BUILD_PLATFORM ?=

ifneq ($(BUILD_PLATFORM),)
DOCKER_PLATFORM=--platform $(BUILD_PLATFORM)
endif

# Go artifacts
GO_ARTIFACTS := cartesi-rollups-node cartesi-rollups-cli cartesi-rollups-evm-reader cartesi-rollups-advancer cartesi-rollups-validator cartesi-rollups-claimer cartesi-rollups-jsonrpc-api

# fixme(vfusco): path on all oses
CGO_CFLAGS:= -I$(PREFIX)/include
CGO_LDFLAGS:= -L$(PREFIX)/lib
export CGO_CFLAGS
export CGO_LDFLAGS

CARTESI_TEST_MACHINE_IMAGES_PATH:= $(PREFIX)/share/cartesi-machine/images/
export CARTESI_TEST_MACHINE_IMAGES_PATH

GO_BUILD_PARAMS := -ldflags "-s -w -X 'github.com/cartesi/rollups-node/internal/version.BuildVersion=$(ROLLUPS_NODE_VERSION)' -r $(PREFIX)/lib"
ifeq ($(BUILD_TYPE),debug)
	GO_BUILD_PARAMS += -gcflags "all=-N -l"
endif

GO_TEST_PACKAGES ?= ./...

ROLLUPS_CONTRACTS_ABI_BASEDIR:= rollups-contracts/
ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR:= rollups-prt-contracts/

all: build

# =============================================================================
# Build
# =============================================================================
build: build-go ## Build all artifacts

build-go: $(GO_ARTIFACTS) ## Build Go artifacts (node, cli, evm-reader)

env:
	@echo export CGO_CFLAGS=\"$(CGO_CFLAGS)\"
	@echo export CGO_LDFLAGS=\"$(CGO_LDFLAGS)\"
	@echo export CARTESI_LOG_LEVEL="info"
	@echo export CARTESI_BLOCKCHAIN_DEFAULT_BLOCK="latest"
	@echo export CARTESI_BLOCKCHAIN_HTTP_ENDPOINT="http://localhost:8545"
	@echo export CARTESI_BLOCKCHAIN_WS_ENDPOINT="ws://localhost:8545"
	@echo export CARTESI_BLOCKCHAIN_ID="13370"
	@echo export CARTESI_CONTRACTS_INPUT_BOX_ADDRESS="0xc70074BDD26d8cF983Ca6A5b89b8db52D5850051"
	@echo export CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS="0xC7003566dD09Aa0fC0Ce201aC2769aFAe3BF0051"
	@echo export CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS="0xc7006f70875BaDe89032001262A846D3Ee160051"
	@echo export CARTESI_CONTRACTS_SELF_HOSTED_APPLICATION_FACTORY_ADDRESS="0xc700285Ab555eeB5201BC00CFD4b2CC8DED90051"
	@echo export CARTESI_AUTH_MNEMONIC=\"test test test test test test test test test test test junk\"
	@echo export CARTESI_DATABASE_CONNECTION="postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable"
	@echo export CARTESI_SNAPSHOTS_DIR="snapshots"
	@echo export CARTESI_TEST_DATABASE_CONNECTION="postgres://test_user:password@localhost:5432/test_rollupsdb?sslmode=disable"
	@echo export CARTESI_TEST_MACHINE_IMAGES_PATH=\"$(CARTESI_TEST_MACHINE_IMAGES_PATH)\"
	@echo export PATH=\"$(CURDIR):$$PATH\"

# =============================================================================
# Artifacts
# =============================================================================
$(GO_ARTIFACTS):
	@echo "Building Go artifact $@"
	go build $(GO_BUILD_PARAMS) ./cmd/$@

tidy-go:
	@go mod tidy

generate: contracts ## Generate the file that are committed to the repo
	@echo "Generating Go files"
	@go generate ./internal/... ./pkg/...

check-generate: generate ## Check whether the generated files are in sync
	@echo "Checking differences on the repository..."
	@if git diff --exit-code; then \
		echo "No differences found."; \
	else \
		echo "ERROR: Differences found in the resulting files."; \
		exit 1; \
	fi

contracts: $(ROLLUPS_CONTRACTS_ABI_BASEDIR)/.stamp $(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)/.stamp ## Export the contract artifacts

$(ROLLUPS_CONTRACTS_ABI_BASEDIR)/.stamp:
	@echo "Downloading rollups-contracts artifacts"
	@mkdir -p $(ROLLUPS_CONTRACTS_ABI_BASEDIR)
	@curl -sSL $(ROLLUPS_CONTRACTS_URL)/v$(ROLLUPS_CONTRACTS_VERSION)/$(ROLLUPS_CONTRACTS_ARTIFACT) -o $(ROLLUPS_CONTRACTS_ARTIFACT)
	@echo "$(ROLLUPS_CONTRACTS_SHA256)  $(ROLLUPS_CONTRACTS_ARTIFACT)" | shasum -a 256 --check > /dev/null
	@tar -zxf $(ROLLUPS_CONTRACTS_ARTIFACT) -C $(ROLLUPS_CONTRACTS_ABI_BASEDIR)
	@touch $@
	@rm $(ROLLUPS_CONTRACTS_ARTIFACT)

$(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)/.stamp:
	@echo "Downloading rollups-prt-contracts artifacts"
	@mkdir -p $(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)
	@curl -sSL $(ROLLUPS_PRT_CONTRACTS_URL)/v$(ROLLUPS_PRT_CONTRACTS_VERSION)/$(ROLLUPS_PRT_CONTRACTS_ARTIFACT) -o $(ROLLUPS_PRT_CONTRACTS_ARTIFACT)
	@echo "$(ROLLUPS_PRT_CONTRACTS_SHA256)  $(ROLLUPS_PRT_CONTRACTS_ARTIFACT)" | shasum -a 256 --check > /dev/null
	@tar -zxf $(ROLLUPS_PRT_CONTRACTS_ARTIFACT) -C $(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)
	@touch $@
	@rm $(ROLLUPS_PRT_CONTRACTS_ARTIFACT)

migrate: ## Run migration on development database
	@echo "Running PostgreSQL migration"
	@go run $(GO_BUILD_PARAMS) dev/migrate/main.go

generate-db: ## Generate repository/db with Jet
	@echo "Generating internal/repository/postgres/db with jet"
	@rm -rf internal/repository/postgres/db
	@go run github.com/go-jet/jet/v2/cmd/jet -dsn=$$CARTESI_DATABASE_CONNECTION -schema=public -path=./internal/repository/postgres/db
	@rm -rf internal/repository/postgres/db/rollupsdb/public/model

# =============================================================================
# Clean
# =============================================================================

clean: clean-go clean-contracts clean-docs clean-devnet-files clean-dapps clean-test-dependencies clean-debian-packages ## Clean all artifacts

clean-go: ## Clean Go artifacts
	@echo "Cleaning Go artifacts"
	@go clean -i -r -cache
	@rm -f $(GO_ARTIFACTS)

clean-contracts: ## Clean contract artifacts
	@echo "Cleaning contract artifacts"
	@rm -rf $(ROLLUPS_CONTRACTS_ABI_BASEDIR) $(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)

clean-docs: ## Clean the documentation
	@echo "Cleaning the documentation"
	@rm -rf docs/cli docs/node docs/evm-reader docs/advancer docs/validator docs/config.md

clean-devnet-files: ## Clean the devnet files
	@echo "Cleaning devnet files"
	@rm -f deployment.json anvil_state.json

clean-debian-packages:
	@echo "Cleaning debian package"
	@rm -f cartesi-rollups-node-v*.deb

clean-dapps: ## Clean the dapps
	@echo "Cleaning dapps"
	@rm -rf applications snapshots

clean-test-dependencies: ## Clean the test dependencies
	@echo "Cleaning test dependencies"
	@rm -rf $(DOWNLOADS_DIR)

# =============================================================================
# Tests
# =============================================================================
test: unit-test ## Execute all tests

unit-test: ## Execute go unit tests
	@echo "Running go unit tests"
	@go clean -testcache
	@go test -p 1 $(GO_BUILD_PARAMS) $(GO_TEST_PACKAGES)

integration-test: ## Execute e2e tests
	@echo "Running end-to-end tests"
	@go test -count=1 ./test --tags=endtoendtests

echo-dapp: applications/echo-dapp ## Echo the dapp

applications/echo-dapp: ## Create echo-dapp test application
	@echo "Creating echo-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/echo-dapp --final-hash -- ioctl-echo-loop --vouchers=1 --delegate-call-vouchers=1 --notices=1 --reports=1 --verbose=1

deploy-echo-dapp: applications/echo-dapp ## Deploy echo-dapp test application
	@echo "Deploying echo-dapp test application"
	@./cartesi-rollups-cli deploy application echo-dapp applications/echo-dapp/

# Temporary test dependencies target while we are not using distribution packages
DOWNLOADS_DIR = test/downloads
CARTESI_TEST_MACHINE_IMAGES = $(DOWNLOADS_DIR)/linux.bin
$(CARTESI_TEST_MACHINE_IMAGES):
	@mkdir -p $(DOWNLOADS_DIR)
	@wget -nc -i test/dependencies -P $(DOWNLOADS_DIR)
	@shasum -ca 256 test/dependencies.sha256
	@cd $(DOWNLOADS_DIR) && ln -s rootfs-tools.ext2 rootfs.ext2
	@cd $(DOWNLOADS_DIR) && ln -s linux-6.5.13-ctsi-1-v0.20.0.bin linux.bin

download-test-dependencies: | $(CARTESI_TEST_MACHINE_IMAGES)

dependencies.sha256:
	@shasum -a 256 $(DOWNLOADS_DIR)/rootfs-tools* $(DOWNLOADS_DIR)/linux-*.bin > $@

# =============================================================================
# Static Analysis
# =============================================================================
lint: ## Run the linter
	@echo "Running the linter"
	@golangci-lint run ./...

fmt: ## Run go fmt
	@echo "Running go fmt"
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet"
	@go vet ./...

escape: ## Run go escape analysis
	@echo "Running go escape analysis"
	go build -gcflags="-m -m" ./...

# =============================================================================
# Docs
# =============================================================================

docs: ## Generate the documentation
	@echo "Generating documentation"
	@go run $(GO_BUILD_PARAMS) dev/gen-docs/main.go

# =============================================================================
# Docker
# =============================================================================
devnet: clean-contracts ## Build docker devnet image
	@docker build $(DOCKER_PLATFORM) -t cartesi/rollups-node-devnet:$(IMAGE_TAG) -f test/devnet/Dockerfile .

image: ## Build the docker images using bake
	@docker build $(DOCKER_PLATFORM) -t cartesi/rollups-node:$(IMAGE_TAG) .

tester-image: ## Build the docker images using bake
	@docker build $(DOCKER_PLATFORM) --target=go-builder -t cartesi/rollups-node:tester .

debian-packager: ## Build debian packager image
	@echo "Building debian packager image $(DEB_PACKAGER_IMG) $(BUILD_PLATFORM)"
	@docker build $(DOCKER_PLATFORM) --target debian-packager -t $(DEB_PACKAGER_IMG) .

run-with-compose: ## Run the node with the anvil devnet
	@docker compose up

start-devnet: ## Run the anvil devnet docker container
	@echo "Starting devnet"
	@docker run --rm --name devnet -p 8545:8545 -d cartesi/rollups-node-devnet:$(IMAGE_TAG)
	@$(MAKE) copy-devnet-files

copy-devnet-files deployment.json: ## Copy the devnet files to the host (it must be running)
	@echo "Copying devnet files"
	@docker cp devnet:/usr/share/devnet/deployment.json deployment.json
	@docker cp devnet:/usr/share/devnet/anvil_state.json anvil_state.json

start-postgres: ## Run the PostgreSQL 16 docker container
	@echo "Starting portgres"
	@docker run --rm --name postgres -p 5432:5432 -d -e POSTGRES_PASSWORD=password -e POSTGRES_DB=rollupsdb -v $(CURDIR)/test/postgres/init-test-db.sh:/docker-entrypoint-initdb.d/init-test-db.sh postgres:17-alpine
	@$(MAKE) migrate

start: start-postgres start-devnet ## Start the anvil devnet and PostgreSQL 16 docker containers

stop-devnet: ## Stop the anvil devnet docker container
	@docker stop devnet || true

stop-postgres: ## Stop the PostgreSQL 16 docker container
	@docker stop postgres || true

stop: stop-devnet stop-postgres ## Stop all running docker containers

restart-devnet: ## Restart the anvil devnet docker container
	@$(MAKE) stop-devnet
	@$(MAKE) start-devnet

restart-postgres: ## Restart the PostgreSQL 16 docker container and migrate it
	@$(MAKE) stop-postgres
	@$(MAKE) start-postgres

restart: ## Restart all running docker containers
	@$(MAKE) stop-devnet
	@$(MAKE) stop-postgres
	@$(MAKE) start-devnet
	@$(MAKE) start-postgres

shutdown-compose: ## Remove the containers and volumes from previous compose run
	@docker compose down -v

unit-test-with-compose: $(CARTESI_TEST_MACHINE_IMAGES) ## Run unit tests using docker compose with auto-shutdown
	@trap 'docker compose -f test/compose/compose.test.yaml down -v || true' EXIT && \
		docker compose -f test/compose/compose.test.yaml run --remove-orphans unit-test

#integration-test-with-compose: $(CARTESI_TEST_MACHINE_IMAGES) ## Run integration tests using docker compose with auto-shutdown
#	@trap 'docker compose -f test/compose/compose.test.yaml down -v || true' EXIT && \
#		docker compose -f test/compose/compose.test.yaml run integration-test

test-with-compose: ## Run all tests using docker compose with auto-shutdown
	@$(MAKE) unit-test-with-compose
#	@$(MAKE) integration-test-with-compose

clean-test-compose-resources: ## Clean up compose resources after some unexpected test failure
	@echo "Cleaning up Docker Compose resources..."
	@docker compose -f test/compose/compose.test.yaml down -v || true

help: ## Show help for each of the Makefile recipes
	@grep "##" $(MAKEFILE_LIST) | grep -v grep | sed -e 's/:.*##\(.*\)/:\n\t\1\n/'

version: ## Show the current version
	@echo $(ROLLUPS_NODE_VERSION)

THIRD_PARTY_LICENSES.md: dev/licenses.tpl go.mod ## Update the THIRD_PARTY_LICENSES.md file
	go-licenses report --template dev/licenses.tpl ./... > $@

# =============================================================================
# Install
# =============================================================================
install: $(GO_ARTIFACTS) ## Install all Go artifacts
	@echo "Installing Go artifacts to $(BIN_INSTALL_PATH)"
	@mkdir -m 0755 -p $(BIN_INSTALL_PATH)
	@for artifact in $(GO_ARTIFACTS); do \
		install -m0755 $$artifact $(BIN_INSTALL_PATH)/; \
	done

copy-debian-package: ## Copy debian package from debian packager image
	@echo "Copying debian package from image $(DEB_PACKAGER_IMG) $(BUILD_PLATFORM)"
	@docker create --name debian-packager $(DOCKER_PLATFORM) $(DEB_PACKAGER_IMG)
	@docker cp debian-packager:/build/cartesi/go/rollups-node/$(DEB_FILENAME) .
	@docker rm debian-packager > /dev/null

build-debian-package: install
	mkdir -p $(DESTDIR)/DEBIAN $(DOC_INSTALL_PATH)
	install -m0644 LICENSE $(DOC_INSTALL_PATH)/copyright
	sed 's|ARG_VERSION|$(ROLLUPS_NODE_VERSION)|g;s|ARG_ARCH|$(DEB_ARCH)|g' control.template > $(DESTDIR)/DEBIAN/control
	dpkg-deb -Zxz --root-owner-group --build $(DESTDIR) $(DEB_FILENAME)

.PHONY: build build-go clean clean-go test unit-test-go e2e-test lint fmt vet escape md-lint devnet image run-with-compose shutdown-compose help docs $(GO_ARTIFACTS)
