.PHONY: tidy build build-all run run-% lint test load-test load-test-% setup init start start-dev stop restart logs clean fmt vet help

ENV_FILE ?= .env.local
SERVICE ?= api
CONFIG_FILE ?= ./configs/config.yaml
SERVICES := api callworker statusworker retryworker scheduler
CAMPAIGNS ?= 3
CALLS ?= 50
CONCURRENT ?= 10

export CONFIG_FILE

all: build-all

fmt:
	@go fmt ./...

vet:
	@go vet ./...

tidy:
	@go mod tidy

build:
	@go build -o bin/$(SERVICE) ./cmd/$(SERVICE)

build-all:
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "Building $$svc"; \
		go build -o bin/$$svc ./cmd/$$svc; \
	done

run:
	@go run ./cmd/$(SERVICE) --config $(CONFIG_FILE)

run-%:
	@go run ./cmd/$* --config $(CONFIG_FILE)

lint:
	@golangci-lint run ./...

test:
	@go test ./...

load-test:
	@bash scripts/load-test.sh $(CAMPAIGNS) $(CALLS) $(CONCURRENT)

load-test-%:
	@bash scripts/load-test.sh $*

setup:
	@bash scripts/install-all.sh

init:
	@bash scripts/init-db.sh

start:
	@ENV_FILE=$(ENV_FILE) bash scripts/run-all.sh

start-dev:
	@ENV_FILE=$(ENV_FILE) PROCFILE=Procfile.dev bash scripts/run-all.sh

stop:
	@bash scripts/stop-all.sh

restart: stop start

logs:
	@bash scripts/tail-logs.sh

clean:
	@rm -rf bin
	@find data -mindepth 1 -maxdepth 1 ! -name '.gitkeep' -exec rm -rf {} +
	@find logs -mindepth 1 -maxdepth 1 ! -name '.gitkeep' -exec rm -rf {} +

help:
	@echo "Available targets:"
	@echo "  setup         - Install system dependencies and tools"
	@echo "  init          - Initialize databases and Kafka topics"
	@echo "  start         - Start all services (API, workers, scheduler)"
	@echo "  start-dev     - Start services with hot reload (requires air)"
	@echo "  stop          - Stop all services"
	@echo "  restart       - Restart all services"
	@echo "  logs          - Tail service logs"
	@echo "  clean         - Remove build artifacts and data"
	@echo ""
	@echo "Development:"
	@echo "  build         - Build a single service (SERVICE=api)"
	@echo "  build-all     - Build all services"
	@echo "  run           - Run a single service (SERVICE=api)"
	@echo "  test          - Run unit tests"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format Go code"
	@echo "  vet           - Run go vet"
	@echo "  tidy          - Clean up Go modules"
	@echo ""
	@echo "Load Testing:"
	@echo "  load-test     - Run load test (CAMPAIGNS=3 CALLS=50 CONCURRENT=10)"
	@echo "  load-test-%   - Run load test with custom parameters"
	@echo ""
	@echo "Examples:"
	@echo "  make load-test CAMPAIGNS=5 CALLS=100 CONCURRENT=20"
	@echo "  make load-test-5-200-50  # 5 campaigns, 200 calls each, 50 concurrent"
