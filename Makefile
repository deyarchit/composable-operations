## Build all binaries
build-all:
	go build -o bin/server ./cmd/server
	go build -o bin/flowctl ./cmd/flowctl

## Start Temporal dev server and API server together (Ctrl+C stops both)
dev:
	@{ temporal server start-dev & TPID=$$!; trap "kill $$TPID 2>/dev/null" EXIT INT TERM; sleep 2; go run ./cmd/server; }

## Run unit tests
test:
	go test ./... -timeout 300s -coverprofile=coverage.txt
	go tool cover -html=coverage.txt -o coverage.html

## Tidy modules
tidy:
	go mod tidy

## Lint with golangci-lint
lint:
	golangci-lint config verify
	golangci-lint run ./...

## Format with golangci-lint (auto-fix formatting)
fmt:
	golangci-lint fmt ./...

## Prepare for pull request
pr: tidy lint fmt test

.PHONY: *
