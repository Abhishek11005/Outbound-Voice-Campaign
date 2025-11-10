.PHONY: tidy build build-all run run-% lint test setup init start start-dev stop restart logs clean fmt vet

ENV_FILE ?= .env.local
SERVICE ?= api
CONFIG_FILE ?= ./configs/config.yaml
SERVICES := api callworker statusworker retryworker scheduler

export CONFIG_FILE

all: build-all

fmt:
	@go fmt ./...

vet:
	@go vet ./...

tidy:
	@go mod tidy

build:
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(SERVICE) ./cmd/$(SERVICE)

build-all:
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "Building $$svc"; \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$$svc ./cmd/$$svc; \
	done

run:
	@go run ./cmd/$(SERVICE) --config $(CONFIG_FILE)

run-%:
	@go run ./cmd/$* --config $(CONFIG_FILE)

lint:
	@golangci-lint run ./...

test:
	@go test ./...

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
