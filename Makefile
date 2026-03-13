.PHONY: all build test lint fmt proto migrate clean docker

BINARY_DIR := bin
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w -X github.com/romashqua-labs/outpost/pkg/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

all: proto build

## Build

build: build-core build-gateway build-proxy build-ctl

build-core:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/outpost-core ./cmd/outpost-core

build-gateway:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/outpost-gateway ./cmd/outpost-gateway

build-proxy:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/outpost-proxy ./cmd/outpost-proxy

build-ctl:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/outpostctl ./cmd/outpostctl

## Test

test:
	$(GO) test ./... -race -count=1

test-cover:
	$(GO) test ./... -race -coverprofile=coverage.txt -covermode=atomic
	$(GO) tool cover -html=coverage.txt -o coverage.html

## Lint & Format

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...
	goimports -w .

vet:
	$(GO) vet ./...

## Proto

proto:
	cd proto && buf generate

proto-lint:
	cd proto && buf lint

proto-breaking:
	cd proto && buf breaking --against '.git#subdir=proto'

## Database

migrate-up:
	migrate -path migrations -database "$${DATABASE_URL}" up

migrate-down:
	migrate -path migrations -database "$${DATABASE_URL}" down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

sqlc:
	cd internal/db && sqlc generate

## Docker

docker:
	docker compose -f deploy/docker/docker-compose.yml build

docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

docker-logs:
	docker compose -f deploy/docker/docker-compose.yml logs -f

## Clean

clean:
	rm -rf $(BINARY_DIR) coverage.txt coverage.html
