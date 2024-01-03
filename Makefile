.PHONY: all generate cargo diff

all: diff

generate:
	@echo "Generating files in tools.go"
	@go generate -v ./...

cargo:
	@cd offchain; cargo run --bin generate-schema
	@mv offchain/schema.graphql api/graphql/reader.graphql

diff: cargo generate
	@echo "Checking differences on the repository..."
	@if git diff --exit-code; then \
		echo "No differences found."; \
	else \
		echo "ERROR: Differences found in the resulting files."; \
		exit 1; \
	fi
