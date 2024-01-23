.PHONY: all
all: help

.PHONY: submodules
submodules: ## Download the git submodules
	@git submodule update --init --recursive

.PHONY: test
test: ## Execute the node tests
	@echo "Running the tests"
	@go test ./...

.PHONY: lint
lint: ## Run the linter
	@echo "Running the linter"
	@golangci-lint run

.PHONY: md-lint
md-lint: ## Lint Markdown docs. Each dir has its own .markdownlint.yaml.
	@echo "Running markdownlint-cli"
	@docker run -v $$PWD:/workdir ghcr.io/igorshubovych/markdownlint-cli:latest "*.md"
	@docker run -v $$PWD/docs:/workdir ghcr.io/igorshubovych/markdownlint-cli:latest "*.md"

.PHONY: generate
generate: ## Generate the file that are commited to the repo
	@echo "Generating Go files"
	@go mod tidy
	@go generate -v ./...

.PHONY: graphql-schema
graphql-schema: ## Generate the graphql schema file
	@cd offchain; cargo run --bin generate-schema
	@mv offchain/schema.graphql api/graphql/reader.graphql

.PHONY: check-generate
check-generate: generate graphql-schema ## Check whether the generated files are in sync
	@echo "Checking differences on the repository..."
	@if git diff --exit-code; then \
		echo "No differences found."; \
	else \
		echo "ERROR: Differences found in the resulting files."; \
		exit 1; \
	fi

.PHONY: docker-build
docker-build: ## Build the docker images using bake
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
