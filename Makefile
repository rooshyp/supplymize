.DEFAULT_GOAL := help

.PHONY: help bootstrap dev test test-web test-go test-python test-rust build compose-up compose-down fmt clean

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*##"; printf "\nSupplyMize commands:\n\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

bootstrap: ## Install workspace dependencies
	pnpm install --frozen-lockfile=false
	python3 -m venv services/forecasting/.venv
	services/forecasting/.venv/bin/python -m pip install -e 'services/forecasting[dev]'

dev: ## Start the local stack with Compose
	docker compose up --build

test: test-web test-go test-python test-rust ## Run all language test suites

test-web: ## Test the TypeScript console
	pnpm --filter @supplymize/console test

test-go: ## Test the Go control plane
	cd services/control-plane && go test ./...

test-python: ## Test the Python forecasting service
	cd services/forecasting && .venv/bin/python -m pytest

test-rust: ## Test the Rust optimizer
	cd services/optimizer && cargo test --all-targets

build: ## Build all components
	pnpm --filter @supplymize/console build
	mkdir -p bin
	cd services/control-plane && go build -trimpath -o ../../bin/control-plane ./cmd/control-plane
	cd services/optimizer && cargo build --release

compose-up: ## Start all services in the background
	docker compose up --build -d

compose-down: ## Stop the local stack
	docker compose down --remove-orphans

fmt: ## Apply language-native formatters
	cd services/control-plane && find . -name '*.go' -type f -print0 | xargs -0 gofmt -w
	cd services/forecasting && .venv/bin/ruff format .
	cd services/optimizer && cargo fmt --all

clean: ## Remove generated build output
	rm -rf apps/console/dist bin services/forecasting/.pytest_cache services/forecasting/.ruff_cache services/optimizer/target
