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

ROLLUPS_NODE_VERSION := 2.0.0

IMAGE_TAG ?= devel

BUILD_TYPE ?= release

ifeq ($(TARGET_OS),Darwin)
PREFIX ?= /opt/cartesi
else
PREFIX ?= /usr
endif

# Rust artifacts
CLAIMER := cmd/authority-claimer/target/$(BUILD_TYPE)/cartesi-rollups-authority-claimer
RUST_ARTIFACTS := $(CLAIMER)

# Go artifacts
GO_ARTIFACTS := cartesi-rollups-node cartesi-rollups-cli cartesi-rollups-evm-reader cartesi-rollups-advancer cartesi-rollups-validator

# fixme(vfusco): path on all oses
CGO_CFLAGS:= -I$(PREFIX)/include
CGO_LDFLAGS:= -L$(PREFIX)/lib
export CGO_CFLAGS
export CGO_LDFLAGS

CARTESI_TEST_MACHINE_IMAGES_PATH:= $(PREFIX)/share/cartesi-machine/images/
export CARTESI_TEST_MACHINE_IMAGES_PATH

GO_BUILD_PARAMS := -ldflags "-s -w -X 'main.buildVersion=$(ROLLUPS_NODE_VERSION)' -r $(PREFIX)/lib"
CARGO_BUILD_PARAMS := --release
ifeq ($(BUILD_TYPE),debug)
	GO_BUILD_PARAMS += -gcflags "all=-N -l"
	CARGO_BUILD_PARAMS =
endif

GO_TEST_PACKAGES ?= ./...

ROLLUPS_CONTRACTS_ABI_BASEDIR:= rollups-contracts/export/artifacts/contracts

all: build

# =============================================================================
# Build
# =============================================================================
build: build-go build-rust ## Build all artifacts

build-rust: $(RUST_ARTIFACTS) ## Build rust artifacts (claimer)

build-go: $(GO_ARTIFACTS) ## Build Go artifacts (node, cli, evm-reader)

env:
	@echo export CGO_CFLAGS=\"$(CGO_CFLAGS)\"
	@echo export CGO_LDFLAGS=\"$(CGO_LDFLAGS)\"
	@echo export CARTESI_LOG_LEVEL="info"
	@echo export CARTESI_BLOCKCHAIN_HTTP_ENDPOINT="http://localhost:8545"
	@echo export CARTESI_BLOCKCHAIN_WS_ENDPOINT="ws://localhost:8545"
	@echo export CARTESI_BLOCKCHAIN_ID="31337"
	@echo export CARTESI_CONTRACTS_INPUT_BOX_ADDRESS="0x593E5BCf894D6829Dd26D0810DA7F064406aebB6"
	@echo export CARTESI_CONTRACTS_INPUT_BOX_DEPLOYMENT_BLOCK_NUMBER="10"
	@echo export CARTESI_AUTH_MNEMONIC=\"test test test test test test test test test test test junk\"
	@echo export CARTESI_POSTGRES_ENDPOINT="postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable"
	@echo export CARTESI_TEST_POSTGRES_ENDPOINT="postgres://test_user:password@localhost:5432/test_rollupsdb?sslmode=disable"
	@echo export CARTESI_TEST_MACHINE_IMAGES_PATH=\"$(CARTESI_TEST_MACHINE_IMAGES_PATH)\"

# =============================================================================
# Artifacts
# =============================================================================
$(GO_ARTIFACTS):
	@echo "Building Go artifact $@"
	go build $(GO_BUILD_PARAMS) ./cmd/$@

$(CLAIMER): | $(ROLLUPS_CONTRACTS_ABI_BASEDIR)
	@echo "Building Rust artifact $@"
	@cd cmd/authority-claimer && cargo build $(CARGO_BUILD_PARAMS)

tidy-go:
	@go mod tidy

generate: $(ROLLUPS_CONTRACTS_ABI_BASEDIR) ## Generate the file that are committed to the repo
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

contracts: $(ROLLUPS_CONTRACTS_ABI_BASEDIR) ## Export the rollups-contracts artifacts

$(ROLLUPS_CONTRACTS_ABI_BASEDIR):
	@echo "Exporting rollups-contracts artifacts"
	@cd rollups-contracts && pnpm install && pnpm export

# =============================================================================
# Clean
# =============================================================================

clean: clean-go clean-rust clean-contracts clean-docs clean-devnet-files clean-dapps ## Clean all artifacts

clean-go: ## Clean Go artifacts
	@echo "Cleaning Go artifacts"
	@go clean -i -r -cache
	@rm -f $(GO_ARTIFACTS)

clean-rust: ## Clean Rust artifacts
	@echo "Cleaning Rust artifacts"
	@cd cmd/authority-claimer && cargo clean

clean-contracts: ## Clean contract artifacts
	@echo "Cleaning contract artifacts"
	@cd rollups-contracts && rm -rf artifacts cache export/artifacts node_modules src && git checkout .

clean-docs: ## Clean the documentation
	@echo "Cleaning the documentation"
	@rm -rf docs/cli docs/node docs/evm-reader docs/advancer docs/validator

clean-devnet-files: ## Clean the devnet files
	@echo "Cleaning devnet files"
	@rm -f deployment.json anvil_state.json

clean-dapps: ## Clean the dapps
	@echo "Cleaning dapps"
	@rm -rf applications

# =============================================================================
# Tests
# =============================================================================
test: unit-test-go unit-test-rust ## Execute all tests

unit-test-go: deployment.json ## Execute go unit tests
	@echo "Running go unit tests"
	@go clean -testcache
	@go test -p 1 $(GO_BUILD_PARAMS) $(GO_TEST_PACKAGES)

unit-test-rust: ## Execute unit tests
	@echo "Running rust unit tests"
	@cd cmd/authority-claimer && cargo test

e2e-test: ## Execute e2e tests
	@echo "Running end-to-end tests"
	@go test -count=1 ./test --tags=endtoendtests

echo-dapp: applications/echo-dapp ## Echo the dapp

applications/echo-dapp: ## Create echo-dapp test application
	@echo "Creating echo-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/echo-dapp --final-hash -- ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1

deploy-echo-dapp: ## Deploy echo-dapp test application
	@echo "Deploying echo-dapp test application"
	@./cartesi-rollups-cli app deploy -t applications/echo-dapp/ -v

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
	@docker build -t cartesi/rollups-node-devnet:$(IMAGE_TAG) -f test/devnet/Dockerfile .

image: ## Build the docker images using bake
	@docker build -t cartesi/rollups-node:$(IMAGE_TAG) .

run-with-compose: ## Run the node with the anvil devnet
	@docker compose up

run-devnet: ## Run the anvil devnet docker container
	@docker run --rm --name devnet -p 8545:8545 -d cartesi/rollups-node-devnet:$(IMAGE_TAG)
	@$(MAKE) copy-devnet-files

copy-devnet-files deployment.json: ## Copy the devnet files to the host
	@docker cp devnet:/usr/share/devnet/deployment.json deployment.json
	@docker cp devnet:/usr/share/devnet/anvil_state.json anvil_state.json

run-postgres: ## Run the PostgreSQL 16 docker container
	@docker run --rm --name postgres -p 5432:5432 -d -e POSTGRES_PASSWORD=password -e POSTGRES_DB=rollupsdb -v $(CURDIR)/test/postgres/init-test-db.sh:/docker-entrypoint-initdb.d/init-test-db.sh postgres:16-alpine

run-postgraphile: ## Run the GraphQL server docker container
	@docker run --rm --name postgraphile -p 10002:10002 -d --init \
		graphile/postgraphile:4.14.0 \
		--retry-on-init-fail \
		--dynamic-json \
		--no-setof-functions-contain-nulls \
		--no-ignore-rbac \
		--enable-query-batching \
		--enhance-graphiql \
		--extended-errors errcode \
		--legacy-relations omit \
		--connection "postgres://postgres:password@host.docker.internal:5432/rollupsdb?sslmode=disable" \
		--schema graphql \
		--host "0.0.0.0" \
		--port 10002
#		--append-plugins @graphile-contrib/pg-simplify-inflector \

stop-devnet: ## Stop the anvil devnet docker container
	@docker stop devnet

stop-postgres: ## Stop the PostgreSQL 16 docker container
	@docker stop postgres

stop: stop-devnet stop-postgres ## Stop all running docker containers

restart-devnet: ## Restart the anvil devnet docker container
	@$(MAKE) stop-devnet
	@$(MAKE) run-devnet

shutdown-compose: ## Remove the containers and volumes from previous compose run
	@docker compose down -v

help: ## Show help for each of the Makefile recipes
	@grep "##" $(MAKEFILE_LIST) | grep -v grep | sed -e 's/:.*##\(.*\)/:\n\t\1\n/'

.PHONY: build build-go build-rust clean clean-go clean-rust test unit-test-go unit-test-rust e2e-test lint fmt vet escape md-lint devnet image run-with-compose shutdown-compose help docs
