BINARY  := focci
BIN_DIR := bin
PKG     := ./...

.DEFAULT_GOAL := build

.PHONY: build
build: ## Compile the binary into bin/
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) .

.PHONY: install
install: ## Install focci into $$(go env GOPATH)/bin
	go install .

.PHONY: test
test: ## Run the test suite
	go test $(PKG)

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: fmt
fmt: ## Format all Go files in place
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file needs formatting
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

.PHONY: check
check: fmt-check vet test ## Run all checks (mirrors CI)

.PHONY: run
run: ## Build and run `focci doctor`
	go run . doctor

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	go clean

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
