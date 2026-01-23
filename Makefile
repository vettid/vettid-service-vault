# VettID Service Vault Makefile

# Build configuration
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go configuration
GO := go
GOFLAGS := -v
VAULT_DIR := vault
CDK_DIR := cdk

.PHONY: all build test lint clean help

# Default target
all: build

##@ Building

.PHONY: build build-vault build-cdk

build: build-vault build-cdk ## Build all components

build-vault: ## Build Go service vault
	@echo "Building service vault..."
	cd $(VAULT_DIR) && $(GO) build $(GOFLAGS) $(LDFLAGS) -o ../bin/service-vault ./cmd/service-vault

build-cdk: ## Build CDK infrastructure
	@echo "Building CDK..."
	cd $(CDK_DIR) && npm run build

##@ Testing

.PHONY: test test-vault test-cdk test-integration

test: test-vault test-cdk ## Run all tests

test-vault: ## Run Go unit tests
	@echo "Running vault tests..."
	cd $(VAULT_DIR) && $(GO) test -v -race -coverprofile=coverage.out ./...

test-integration: ## Run integration tests (requires local services)
	@echo "Running integration tests..."
	cd $(VAULT_DIR) && $(GO) test -v -race -tags=integration ./...

test-cdk: ## Run CDK tests
	@echo "Running CDK tests..."
	cd $(CDK_DIR) && npm test

##@ Code Quality

.PHONY: lint lint-vault lint-cdk fmt vet

lint: lint-vault lint-cdk ## Run all linters

lint-vault: ## Run Go linter
	@echo "Linting vault..."
	cd $(VAULT_DIR) && golangci-lint run ./...

lint-cdk: ## Lint CDK TypeScript
	@echo "Linting CDK..."
	cd $(CDK_DIR) && npm run lint

fmt: ## Format Go code
	@echo "Formatting Go code..."
	cd $(VAULT_DIR) && $(GO) fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	cd $(VAULT_DIR) && $(GO) vet ./...

##@ Dependencies

.PHONY: deps deps-vault deps-cdk tidy

deps: deps-vault deps-cdk ## Install all dependencies

deps-vault: ## Download Go dependencies
	@echo "Downloading Go dependencies..."
	cd $(VAULT_DIR) && $(GO) mod download

deps-cdk: ## Install CDK npm dependencies
	@echo "Installing CDK dependencies..."
	cd $(CDK_DIR) && npm install

tidy: ## Tidy Go modules
	@echo "Tidying Go modules..."
	cd $(VAULT_DIR) && $(GO) mod tidy

##@ Development

.PHONY: run dev generate

run: build-vault ## Run the service vault
	@echo "Starting service vault..."
	./bin/service-vault

dev: ## Run in development mode with hot reload
	@echo "Starting in development mode..."
	cd $(VAULT_DIR) && $(GO) run ./cmd/service-vault

generate: ## Run go generate
	@echo "Running go generate..."
	cd $(VAULT_DIR) && $(GO) generate ./...

##@ Infrastructure

.PHONY: cdk-synth cdk-diff cdk-deploy cdk-destroy

cdk-synth: ## Synthesize CDK stacks
	@echo "Synthesizing CDK stacks..."
	cd $(CDK_DIR) && npx cdk synth

cdk-diff: ## Show CDK diff
	@echo "Showing CDK diff..."
	cd $(CDK_DIR) && npx cdk diff

cdk-deploy: ## Deploy CDK stacks
	@echo "Deploying CDK stacks..."
	cd $(CDK_DIR) && npx cdk deploy --all

cdk-destroy: ## Destroy CDK stacks
	@echo "Destroying CDK stacks..."
	cd $(CDK_DIR) && npx cdk destroy --all

##@ Local Development Services

.PHONY: local-up local-down local-nats local-dynamodb

local-up: local-nats local-dynamodb ## Start local development services

local-down: ## Stop local development services
	@echo "Stopping local services..."
	docker stop nats-dev dynamodb-local 2>/dev/null || true
	docker rm nats-dev dynamodb-local 2>/dev/null || true

local-nats: ## Start local NATS server
	@echo "Starting local NATS server..."
	docker run -d --name nats-dev -p 4222:4222 -p 8222:8222 nats:latest -js || true

local-dynamodb: ## Start local DynamoDB
	@echo "Starting local DynamoDB..."
	docker run -d --name dynamodb-local -p 8000:8000 amazon/dynamodb-local || true

##@ Utilities

.PHONY: clean help

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf $(VAULT_DIR)/coverage.out
	rm -rf $(CDK_DIR)/cdk.out
	rm -rf $(CDK_DIR)/node_modules

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
