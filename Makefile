# Makefile for VNIM (Virtual Network Interface Manager)

BINARY_NAME=vnim
BUILD_DIR=build
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
INSTALL_PATH=/usr/local/bin

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -s -w -X main.Version=$(VERSION)

.PHONY: all build build-all install uninstall fetch update clean test

all: build

build:
	@echo "Building $(BINARY_NAME) $(VERSION) inside $(BUILD_DIR)/..."
	mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./cmd/vnim/main.go

build-all:
	@echo "Building $(BINARY_NAME) $(VERSION) for all platforms..."
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/vnim/main.go
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/vnim/main.go

install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	sudo install -m 0755 $(BINARY_PATH) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) successfully installed!"

uninstall:
	@echo "Uninstalling $(BINARY_NAME) from $(INSTALL_PATH)..."
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) successfully uninstalled!"

fetch:
	@echo "Fetching dependencies..."
	go mod download

update: fetch build
	@echo "Updating installed $(BINARY_NAME) in $(INSTALL_PATH)..."
	sudo install -m 0755 $(BINARY_PATH) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) successfully updated!"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)

test:
	@echo "Running unit tests..."
	go test -v ./...
