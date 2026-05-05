# Convenience wrapper around the Go toolchain. Every target is short — the
# canonical commands are `go ...` and `goreleaser ...`. This file exists so
# `make verify` runs everything CI runs in one command.

GO ?= go
GORELEASER ?= goreleaser
GOLANGCI_LINT ?= golangci-lint

.PHONY: help build run test test-race cover lint fmt fmt-check vet vuln \
        snapshot release-check verify clean

help:
	@awk 'BEGIN{FS=":.*##"} /^[a-zA-Z_-]+:.*?##/{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

build: ## Build the clyde binary into ./clyde
	$(GO) build -o clyde ./cmd/clyde

run: ## Run clyde --demo (offline mock data)
	$(GO) run ./cmd/clyde --demo

test: ## Unit + integration tests
	$(GO) test ./...

test-race: ## Tests with the race detector (matches CI)
	$(GO) test -race -cover ./...

cover: ## Coverage profile
	$(GO) test -cover -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -5

lint: ## golangci-lint (matches CI version pin in .github/workflows/ci.yml)
	$(GOLANGCI_LINT) run ./...

fmt: ## Apply gofmt + goimports
	gofmt -w .

fmt-check: ## Fail if gofmt would change anything
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then echo "$$diff"; exit 1; fi

vet: ## go vet
	$(GO) vet ./...

vuln: ## govulncheck (matches CI)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

snapshot: ## Local goreleaser snapshot (skips publish/sbom/sign)
	$(GORELEASER) release --snapshot --clean --skip=publish,sbom,sign

release-check: ## Validate .goreleaser.yml
	$(GORELEASER) check

verify: fmt-check vet lint test-race ## Run everything CI runs (~ 1 min)
	@echo "verify: all checks passed"

clean: ## Remove build artifacts
	rm -rf ./clyde ./dist ./coverage.out
