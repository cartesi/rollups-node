.PHONY: all generate cargo diff

all: diff

generate:
	@echo "Generating files in tools.go"
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go generate %

cargo: generate
	@cd offchain; cargo run --bin generate-schema
	@mv offchain/schema.graphql api/graphql/reader.graphql

diff: cargo
	@echo "Checking differences on the repository..."
	@if git diff --exit-code; then \
		echo "No differences found."; \
	else \
		echo "ERROR: Differences found in the resulting files."; \
		exit 1; \
	fi
