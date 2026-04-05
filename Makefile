.PHONY: build test lint run clean setup docker-build docker-run

BINARY=gateway
BUILD_DIR=bin
IMAGE=git-gateway
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/gateway

test:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

# Run with default config (loads .env automatically if present)
run: build
	$(BUILD_DIR)/$(BINARY) -config gateway.yaml

# Run with example config (for testing without secrets)
run-example: build
	$(BUILD_DIR)/$(BINARY) -config configs/gateway.example.yaml

clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Setup: copy example files for local development
setup:
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env from .env.example"; else echo ".env already exists"; fi
	@if [ ! -f gateway.yaml ]; then cp configs/gateway.example.yaml gateway.yaml && echo "Created gateway.yaml from example"; else echo "gateway.yaml already exists"; fi
	@echo "Edit .env and gateway.yaml with your settings"

# Development helpers
dev:
	go run ./cmd/gateway -config gateway.yaml

dev-example:
	go run ./cmd/gateway -config configs/gateway.example.yaml

fmt:
	go fmt ./...

vet:
	go vet ./...

docker-build:
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-run:
	docker run --rm -p 8081:8081 \
		--env-file .env \
		-v $(PWD)/gateway.yaml:/etc/gateway/gateway.yaml:ro \
		$(IMAGE):latest
