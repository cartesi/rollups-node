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

ROLLUPS_NODE_VERSION := 2.0.0-alpha.11
ROLLUPS_CONTRACTS_VERSION := 3.0.0-alpha.6
ROLLUPS_CONTRACTS_URL:=https://github.com/cartesi/rollups-contracts/releases/download/
ROLLUPS_CONTRACTS_ARTIFACT:=rollups-contracts-$(ROLLUPS_CONTRACTS_VERSION)-artifacts.tar.gz
ROLLUPS_CONTRACTS_SHA256:=ad1e0880766d25419fc6da1858ea4e7b9074b400e9d9ef68da88b12f4a8bba45
ROLLUPS_PRT_CONTRACTS_VERSION := 3.0.0-alpha.3
ROLLUPS_PRT_CONTRACTS_URL:=https://github.com/cartesi/dave/releases/download/
ROLLUPS_PRT_CONTRACTS_ARTIFACT:=cartesi-rollups-prt-$(ROLLUPS_PRT_CONTRACTS_VERSION)-contract-artifacts.tar.gz
ROLLUPS_PRT_CONTRACTS_SHA256:=240f4934df7a313dc05a4ae6cc3eee97b5c146952c4218502fec0db83f36a5a5

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
GO_ARTIFACTS := $(addprefix cartesi-rollups-,node cli evm-reader advancer validator claimer jsonrpc-api prt machine-tool)

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
GO_TEST_FLAGS ?=

VERBOSE ?=
ifeq ($(VERBOSE),true)
	GO_BUILD_PARAMS += -v
	GO_TEST_FLAGS += -v
endif

TEST_PATTERN ?=
ifneq ($(TEST_PATTERN),)
	GO_TEST_FLAGS += -run $(TEST_PATTERN)
endif

TEST_PACKAGES ?=
ifneq ($(TEST_PACKAGES),)
	GO_TEST_PACKAGES := $(addprefix ./, $(addsuffix /..., $(subst :, ,$(TEST_PACKAGES))))
endif

COVERAGE_DIR := coverage
COVER ?=
ifeq ($(COVER),true)
	GO_TEST_FLAGS += -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic -coverpkg=./...
	COVER_DEPS := $(COVERAGE_DIR)
	COVER_REPORT := coverage-report
endif


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
	@echo export CARTESI_BLOCKCHAIN_ID="31337"
	@echo export CARTESI_CONTRACTS_INPUT_BOX_ADDRESS="0x346B3df038FE9f8380071eC6514D5a83aD143939"
	@echo export CARTESI_CONTRACTS_AUTHORITY_FACTORY_ADDRESS="0x3C1FE01c542a88A523FF6847eD1E26176c8C4ED0"
	@echo export CARTESI_CONTRACTS_QUORUM_FACTORY_ADDRESS="0x1f94009389F408B8D0ADfFcF8BBDCe5552BaCa5F"
	@echo export CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS="0xC549F89cF1ca43eDDECC64Ac2208F4b283B1c483"
	@echo export CARTESI_CONTRACTS_SELF_HOSTED_APPLICATION_FACTORY_ADDRESS="0x6145C5996a71a379E030aEb0440df79D60833418"
	@echo export CARTESI_CONTRACTS_DAVE_APP_FACTORY_ADDRESS="0x33FFf0b681c90664dD048a60400AE2D827a4c5bb"
	@echo export CARTESI_DEVNET_ERC20_PORTAL_ADDRESS="0x22E57511C30CcE6CDaa742E13CE3b774fDC663b1"
	@echo export CARTESI_DEVNET_TEST_ERC20_ADDRESS="0x88A2120B7068E78692C8fd12E751d610B6377E4d"
	@echo export CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS="0x0745787835A019cd4dae8EDB541Fbc0647793d63"
	@echo export CARTESI_AUTH_MNEMONIC=\"test test test test test test test test test test test junk\"
	@echo export CARTESI_DATABASE_CONNECTION="postgres://postgres:password@localhost:5432/rollupsdb?sslmode=disable"
	@echo export CARTESI_SNAPSHOTS_DIR="snapshots"
	@echo export CARTESI_TEST_DATABASE_CONNECTION="postgres://test_user:password@localhost:5432/test_rollupsdb?sslmode=disable"
	@echo export CARTESI_TEST_MACHINE_IMAGES_PATH=\"$(CARTESI_TEST_MACHINE_IMAGES_PATH)\"
	@echo export PATH=\"$(CURDIR):$$PATH\"
	@$(if $(MACOSX_DEPLOYMENT_TARGET),echo export MACOSX_DEPLOYMENT_TARGET=\"$(MACOSX_DEPLOYMENT_TARGET)\")

# =============================================================================
# Artifacts
# =============================================================================
$(GO_ARTIFACTS):
	@echo "Building Go artifact $@"
	go build $(GO_BUILD_PARAMS) ./cmd/$@

tidy-go:
	@go mod tidy

generate: generate-contracts generate-config generate-inspect ## Generate all code files committed to the repo

generate-contracts: contracts ## Generate contract ABI bindings
	@echo "Generating contract bindings"
	@go generate ./pkg/contracts/...

generate-config: ## Generate config code from Config.toml
	@echo "Generating config code"
	@go generate ./internal/config/...

generate-inspect: ## Generate inspect API client
	@echo "Generating inspect client"
	@go generate ./pkg/inspectclient/...

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
	@rm -f $(ROLLUPS_CONTRACTS_ARTIFACT)

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
	@rm -rf $(COVERAGE_DIR)

clean-contracts: ## Clean contract artifacts
	@echo "Cleaning contract artifacts"
	@rm -rf $(ROLLUPS_CONTRACTS_ABI_BASEDIR) $(ROLLUPS_PRT_CONTRACTS_ABI_BASEDIR)
	@rm -f $(ROLLUPS_CONTRACTS_ARTIFACT) $(ROLLUPS_PRT_CONTRACTS_ARTIFACT)

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

$(COVERAGE_DIR):
	@mkdir -p $@

coverage-report:
	@go tool cover -func=$(COVERAGE_DIR)/coverage.out
	@go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

unit-test: $(COVER_DEPS) ## Execute go unit tests
	@echo "Running go unit tests"
	@go clean -testcache
	@go test -p 1 $(GO_BUILD_PARAMS) $(GO_TEST_FLAGS) $(GO_TEST_PACKAGES)
	@$(if $(COVER_REPORT),$(MAKE) $(COVER_REPORT))

GOTESTSUM_FORMAT ?= testdox
ifeq ($(VERBOSE),true)
	GOTESTSUM_FORMAT = standard-verbose
endif

integration-test: ## Execute e2e tests
	@echo "Running end-to-end tests"
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format $(GOTESTSUM_FORMAT) -- -count=1 -timeout 55m $(GO_BUILD_PARAMS) $(GO_TEST_FLAGS) -tags=endtoendtests ./test/integration/...; \
	else \
		go test -count=1 -timeout 55m $(GO_BUILD_PARAMS) $(GO_TEST_FLAGS) -tags=endtoendtests ./test/integration/...; \
	fi

echo-dapp: applications/echo-dapp ## Echo dapp

reject-dapp: applications/reject-dapp ## Reject dapp

exception-dapp: applications/exception-dapp ## Exception dapp

applications/echo-dapp: ## Create echo-dapp test application
	@echo "Creating echo-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/echo-dapp --final-hash -- ioctl-echo-loop --vouchers=1 --delegate-call-vouchers=1 --notices=1 --reports=1 --verbose=1

applications/reject-dapp: ## Create reject-dapp test application
	@echo "Creating reject-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/reject-dapp --final-hash -- "rollup accept && echo '{\"payload\": \"0x726561736f6e20666f722072656a656374696e67\" }' | rollup report && rollup reject"

applications/exception-dapp: ## Create exception-dapp test application
	@echo "Creating exception-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/exception-dapp --final-hash -- "rollup accept && echo '{\"payload\": \"0x7468697320697320612064756d6d7920657863657074696f6e2074657874\"}' | rollup exception"

reject-loop-dapp: applications/reject-loop-dapp ## Reject loop dapp

exception-loop-dapp: applications/exception-loop-dapp ## Exception loop dapp

erc20-withdrawal-dapp: applications/erc20-withdrawal-dapp ## ERC-20 withdrawal test dapp

applications/reject-loop-dapp: ## Create reject-loop-dapp test application
	@echo "Creating reject-loop-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/reject-loop-dapp --final-hash -- ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --reject=1 --verbose=1

applications/exception-loop-dapp: ## Create exception-loop-dapp test application
	@echo "Creating exception-loop-dapp test application"
	@mkdir -p applications
	@cartesi-machine --ram-length=128Mi --store=applications/exception-loop-dapp --final-hash -- ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --exception=1 --verbose=1

applications/erc20-withdrawal-dapp: test/dapps/erc20-withdrawal/install.sh ## Create ERC-20 withdrawal test application
	@echo "Creating ERC-20 withdrawal test application"
	@mkdir -p applications
	@PORTAL=$${CARTESI_DEVNET_ERC20_PORTAL_ADDRESS:-0x22E57511C30CcE6CDaa742E13CE3b774fDC663b1}; \
	TOKEN=$${CARTESI_DEVNET_TEST_ERC20_ADDRESS:-0x88A2120B7068E78692C8fd12E751d610B6377E4d}; \
	cartesi-machine --ram-length=128Mi \
		--flash-drive=label:accounts,length:4Mi,mke2fs:false,mount:false,user:dapp \
		--env=TRUSTED_ERC20_PORTAL=$$PORTAL \
		--env=TRUSTED_ERC20_TOKEN=$$TOKEN \
		--append-init-file=test/dapps/erc20-withdrawal/install.sh \
		--store=applications/erc20-withdrawal-dapp --final-hash -- /usr/local/bin/erc20-withdrawal-dapp

deploy-echo-dapp: applications/echo-dapp ## Deploy echo-dapp test application
	@echo "Deploying echo-dapp test application"
	@./cartesi-rollups-cli deploy application echo-dapp applications/echo-dapp/

deploy-reject-dapp: applications/reject-dapp ## Deploy reject-dapp test application
	@echo "Deploying reject-dapp test application"
	@./cartesi-rollups-cli deploy application reject-dapp applications/reject-dapp/

deploy-exception-dapp: applications/exception-dapp ## Deploy exception-dapp test application
	@echo "Deploying exception-dapp test application"
	@./cartesi-rollups-cli deploy application exception-dapp applications/exception-dapp/

deploy-prt-echo-dapp: applications/echo-dapp ## Deploy echo-dapp test application
	@echo "Deploying echo-dapp test application"
	@./cartesi-rollups-cli deploy application prt-echo-dapp applications/echo-dapp/ --prt

deploy-erc20-withdrawal-dapp: applications/erc20-withdrawal-dapp ## Deploy ERC-20 withdrawal test application
	@set -e; \
	APP=$${APP:-erc20-withdrawal-dapp}; \
	GUARDIAN=$${GUARDIAN:-0x70997970C51812dc3A010C7d01b50e0d17dc79C8}; \
	BUILDER=$${CARTESI_DEVNET_WITHDRAWAL_OUTPUT_BUILDER_ADDRESS:-0x0745787835A019cd4dae8EDB541Fbc0647793d63}; \
	DRIVE_START_INDEX=$$(jq -r '.config.flash_drive[] | select(.length == 4194304) | (.start / 4194304 | floor)' \
		applications/erc20-withdrawal-dapp/config.json); \
	WITHDRAWAL_CONFIG=$$(jq -cn \
		--arg guardian "$$GUARDIAN" \
		--arg builder "$$BUILDER" \
		--argjson drive "$$DRIVE_START_INDEX" \
		'{guardian:$$guardian,log2_leaves_per_account:0,log2_max_num_of_accounts:17,accounts_drive_start_index:$$drive,withdrawal_output_builder:$$builder}'); \
	echo "Deploying $$APP with accounts-drive start index $$DRIVE_START_INDEX"; \
	./cartesi-rollups-cli deploy application "$$APP" applications/erc20-withdrawal-dapp \
		--salt "$$(openssl rand -hex 32)" \
		--withdrawal-config "$$WITHDRAWAL_CONFIG" \
		--enable=false; \
	./cartesi-rollups-cli app execution-parameters set "$$APP" snapshot_policy EVERY_EPOCH; \
	./cartesi-rollups-cli app status "$$APP" enabled --yes

fund-wallet: ## Fund the default Anvil wallet with ETH and test ERC-20
	@set -e; \
	RPC_URL=$${CARTESI_BLOCKCHAIN_HTTP_ENDPOINT:-http://localhost:8545}; \
	TOKEN=$${CARTESI_DEVNET_TEST_ERC20_ADDRESS:-0x88A2120B7068E78692C8fd12E751d610B6377E4d}; \
	WALLET=$${WALLET:-$$(cast rpc --rpc-url "$$RPC_URL" eth_accounts | jq -r '.[0]')}; \
	ETH_WEI=$${ETH_WEI:-0x8ac7230489e80000}; \
	TOKEN_AMOUNT=$${TOKEN_AMOUNT:-1000000}; \
	echo "Funding $$WALLET with $$ETH_WEI wei"; \
	cast rpc --rpc-url "$$RPC_URL" anvil_setBalance "$$WALLET" "$$ETH_WEI" >/dev/null; \
	echo "Minting $$TOKEN_AMOUNT test ERC-20 units to $$WALLET"; \
	cast send --rpc-url "$$RPC_URL" --from "$$WALLET" --unlocked "$$TOKEN" "mint(uint256)" "$$TOKEN_AMOUNT" >/dev/null

withdraw-wallet: cartesi-rollups-cli ## Send a test withdrawal request from the current signer
	@set -e; \
	APP=$${APP:-erc20-withdrawal-dapp}; \
	AMOUNT=$${AMOUNT:-25}; \
	PAYLOAD=$$(printf '0x01%016x' "$$AMOUNT"); \
	echo "Sending withdrawal request to $$APP: amount=$$AMOUNT payload=$$PAYLOAD"; \
	./cartesi-rollups-cli send "$$APP" "$$PAYLOAD" --hex --yes --json

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

fmt-check: ## Check go formatting (non-destructive)
	@echo "Checking go formatting"
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files:" && gofmt -l . && exit 1)

vet: ## Run go vet
	@echo "Running go vet"
	@go vet ./...

escape: ## Run go escape analysis
	@echo "Running go escape analysis"
	go build -gcflags="-m -m" ./...

# =============================================================================
# Docs
# =============================================================================

docs: generate-cli-docs generate-config-docs ## Generate all documentation

generate-cli-docs: ## Generate CLI documentation
	@echo "Generating CLI documentation"
	@go run $(GO_BUILD_PARAMS) dev/gen-docs/main.go

generate-config-docs: ## Generate config documentation from Config.toml
	@echo "Generating config documentation"
	@cd internal/config/generate && go run . -mode=docs

# =============================================================================
# Docker
# =============================================================================
devnet: clean-contracts ## Build docker devnet image
	@docker build $(DOCKER_PLATFORM) -t cartesi/rollups-node-devnet:$(IMAGE_TAG) -f test/devnet/Dockerfile .

image: ## Build the docker images using bake
	@docker build $(DOCKER_PLATFORM) -t cartesi/rollups-node:$(IMAGE_TAG) .

tester-image: ## Build the docker images using bake
	@docker build $(DOCKER_PLATFORM) --target=tester -t cartesi/rollups-node:tester .

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
		docker compose -f test/compose/compose.test.yaml run --rm --remove-orphans unit-test

lint-with-docker: ## Run linting inside Docker (no host Go needed)
	@docker run --rm cartesi/rollups-node:tester sh -c 'make lint && make vet && make fmt-check'

integration-test-with-compose: $(CARTESI_TEST_MACHINE_IMAGES) ## Run integration tests using docker compose with auto-shutdown
	@trap 'docker compose -f test/compose/compose.integration.yaml logs --no-color > integration-logs.txt 2>&1 || true; docker compose -f test/compose/compose.integration.yaml down -v || true' EXIT && \
		docker compose -f test/compose/compose.integration.yaml run --rm --remove-orphans integration-test

test-with-compose: ## Run all tests using docker compose with auto-shutdown
	@$(MAKE) unit-test-with-compose
	@$(MAKE) integration-test-with-compose

integration-test-local: build cartesi-rollups-machine-tool echo-dapp reject-loop-dapp exception-loop-dapp erc20-withdrawal-dapp ## Run integration tests locally (requires: make start && eval $$(make env))
	@cartesi-rollups-cli db init
	@if lsof -ti:10000 >/dev/null 2>&1; then \
		echo "Killing stale node on port 10000..."; \
		kill $$(lsof -ti:10000) 2>/dev/null || true; \
		sleep 2; \
	fi
	@export CARTESI_TEST_DAPP_PATH=$(CURDIR)/applications/echo-dapp; \
	export CARTESI_TEST_REJECT_DAPP_PATH=$(CURDIR)/applications/reject-loop-dapp; \
	export CARTESI_TEST_EXCEPTION_DAPP_PATH=$(CURDIR)/applications/exception-loop-dapp; \
	export CARTESI_TEST_ERC20_WITHDRAWAL_DAPP_PATH=$(CURDIR)/applications/erc20-withdrawal-dapp; \
	$(MAKE) integration-test

deploy-load-test-apps: applications/echo-dapp ## Deploy 3 echo-dapp instances for load testing
	@echo "Deploying load-test apps (3 echo-dapps with different salts)..."
	@./cartesi-rollups-cli deploy application load-test-flood applications/echo-dapp/ \
		--salt=0000000000000000000000000000000000000000000000000000000000000A01
	@./cartesi-rollups-cli deploy application load-test-trickle-1 applications/echo-dapp/ \
		--salt=0000000000000000000000000000000000000000000000000000000000000A02
	@./cartesi-rollups-cli deploy application load-test-trickle-2 applications/echo-dapp/ \
		--salt=0000000000000000000000000000000000000000000000000000000000000A03
	@echo "Done. 3 apps deployed: load-test-flood, load-test-trickle-1, load-test-trickle-2"

load-test: deploy-load-test-apps ## Deploy 3 apps and run advancer starvation load test
	@echo "NOTE: Start the node (separate terminal) with: CARTESI_ADVANCER_INPUT_BATCH_SIZE=10 cartesi-rollups-node"
	@scripts/load-test.sh

ci-test: ## Run the full CI test pipeline locally (lint + unit + integration)
#	@$(MAKE) lint-with-docker
	@$(MAKE) unit-test-with-compose
	@$(MAKE) integration-test-with-compose

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

.PHONY: \
	build build-go $(GO_ARTIFACTS) cartesi-rollups-machine-tool \
	clean clean-go clean-contracts clean-docs clean-devnet-files clean-dapps clean-test-dependencies clean-debian-packages \
	test unit-test unit-test-with-compose integration-test integration-test-with-compose integration-test-local test-with-compose ci-test coverage-report \
	generate generate-contracts generate-config generate-inspect check-generate generate-db \
	docs generate-cli-docs generate-config-docs \
	lint fmt fmt-check vet escape \
	devnet image tester-image debian-packager run-with-compose shutdown-compose \
	start start-devnet start-postgres stop stop-devnet stop-postgres restart restart-devnet restart-postgres \
	install copy-debian-package build-debian-package \
	deploy-erc20-withdrawal-dapp fund-wallet withdraw-wallet \
	env help version
