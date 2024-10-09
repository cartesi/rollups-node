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
	@echo export CARTESI_AUTH_MNEMONIC="test test test test test test test test test test test junk"
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
	@go mod tidy
	@go generate -v ./...

check-generate: generate ## Check whether the generated files are in sync
	@echo "Checking differences on the repository..."
	@if git diff --exit-code; then \
		echo "No differences found."; \
	else \
		echo "ERROR: Differences found in the resulting files."; \
		exit 1; \
	fi

.PHONY: docker-build-deps
docker-build-deps: ## Build the dependencies images using bake
	@cd build && docker buildx bake --load rollups-node-devnet rollups-node-snapshot

.PHONY: docker-build
docker-build: submodules ## Build the docker images using bake
	@cd build && docker buildx bake --load

.PHONY: docker-run
docker-run: docker-clean ## Run the node with the anvil devnet
	@docker compose \
		-f ./build/compose-database.yaml \
		-f ./build/compose-devnet.yaml \
		-f ./build/compose-snapshot.yaml \
		-f ./build/compose-node.yaml \
		up

.PHONY: docker-run-sepolia
docker-run-sepolia: docker-clean ## Run the node with the sepolia testnet
	@if [ ! -n "$$RPC_HTTP_URL" ]; then \
		echo "RPC_HTTP_URL was not set"; \
		exit 1; \
	fi
	@if [ ! -n "$$RPC_WS_URL" ]; then \
		echo "RPC_WS_URL was not set"; \
		exit 1; \
	fi
	@docker compose \
		-f ./build/compose-database.yaml \
		-f ./build/compose-snapshot.yaml \
		-f ./build/compose-node.yaml \
		-f ./build/compose-sepolia.yaml \
		up

.PHONY: docker-clean
docker-clean: ## Remove the containers and volumes from previous compose run
	@docker compose \
		-f ./build/compose-database.yaml \
		-f ./build/compose-devnet.yaml \
		-f ./build/compose-snapshot.yaml \
		-f ./build/compose-node.yaml \
		down -v

.PHONY: help
help: ## Show help for each of the Makefile recipes
	@grep "##" $(MAKEFILE_LIST) | grep -v grep | sed -e 's/:.*##\(.*\)/:\n\t\1\n/'

.PHONY: build build-go build-rust clean clean-go clean-rust test unit-test-go unit-test-rust e2e-test lint fmt vet escape md-lint devnet image run-with-compose shutdown-compose help docs
