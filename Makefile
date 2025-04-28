BIN_DIR := bin
BINARY := $(BIN_DIR)/go-csql

.PHONY: all build clean docker-build

all: build

build: $(BINARY)

$(BINARY):
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/csql

clean:
	rm -rf $(BIN_DIR)/*

# Build Docker image
docker-build:
	docker build -t ghcr.io/chaoshour/go-csql:latest .
