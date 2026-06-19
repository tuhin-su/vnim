# Makefile for VNIM (Virtual Network Interface Manager)

BINARY_NAME=vnim
BUILD_DIR=build
BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)
INSTALL_PATH=/usr/local/bin

.PHONY: all build install uninstall fetch update clean test

all: build

build:
	@echo "Building $(BINARY_NAME) inside $(BUILD_DIR)/..."
	mkdir -p $(BUILD_DIR)
	go build -o $(BINARY_PATH) ./cmd/vnim/main.go

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
