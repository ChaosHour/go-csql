BIN_DIR := bin
BINARY := $(BIN_DIR)/go-csql

.PHONY: all build clean docker-build test test-verbose test-cover test-bench

all: build

build: $(BINARY)

$(BINARY): $(shell find . -name '*.go' -type f)
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/csql

clean:
	rm -rf $(BIN_DIR)/*

# Build Docker image
docker-build:
	docker build -t ghcr.io/chaoshour/go-csql:latest .

# Run tests
test:
	go test ./...

# Run tests with verbose output
test-verbose:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -cover ./...

# Run benchmark tests
test-bench:
	go test -bench=. ./...
