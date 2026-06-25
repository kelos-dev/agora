SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

BINARY ?= agora
BINDIR ?= bin
REGISTRY ?= ghcr.io/kelos-dev
IMAGE_NAME ?= agora
VERSION ?= latest
IMAGE ?= $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
CONTAINER_TOOL ?= docker
PUSH ?= false
GO_FILES := $(shell find . -name '*.go' -not -path './$(BINDIR)/*')

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: ## Run unit tests.
	go test ./...

.PHONY: update
update: ## Run formatters and update module metadata.
	gofmt -w $(GO_FILES)
	go mod tidy

.PHONY: verify
verify: ## Verify formatting, module metadata, tests, and vet checks.
	go mod tidy -diff
	test -z "$$(gofmt -l $(GO_FILES))"
	go test ./...
	go vet ./...

##@ Build

.PHONY: build
build: ## Build the Agora binary.
	mkdir -p $(BINDIR)
	CGO_ENABLED=0 go build -o $(BINDIR)/$(BINARY) ./cmd/agora

.PHONY: image
image: ## Build the Agora server container image.
	$(CONTAINER_TOOL) buildx build $(if $(filter true,$(PUSH)),--push,--load) --tag $(IMAGE) .

.PHONY: run
run: ## Run the Agora server.
	go run ./cmd/agora

.PHONY: clean
clean: ## Clean build artifacts.
	rm -rf $(BINDIR)
	rm -f cover.out
