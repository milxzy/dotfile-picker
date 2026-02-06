# makefile for dotfile picker
.PHONY: build test clean install run lint fmt help

# variables
BINARY_NAME=dotpicker
BUILD_DIR=bin
INSTALL_DIR=/usr/local/bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO=go
GOFLAGS=-v
LDFLAGS=-X main.Version=$(VERSION)

# default target
all: build

# build the application
build:
	@echo "building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dotpicker

# run tests
test:
	@echo "running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...

# run tests with coverage report
test-coverage: test
	$(GO) tool cover -html=coverage.out

# clean build artifacts
clean:
	@echo "cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out

# install to system
install: build
	@echo "installing to $(INSTALL_DIR)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	@echo "installed! run 'dotpicker' to start"

# uninstall from system
uninstall:
	@echo "uninstalling..."
	rm -f $(INSTALL_DIR)/$(BINARY_NAME)

# run the application
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# lint the code
lint:
	@echo "linting..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed, run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# format the code
fmt:
	@echo "formatting..."
	$(GO) fmt ./...
	@which goimports > /dev/null && goimports -w . || echo "goimports not found, skipping"

# run mod tidy
tidy:
	@echo "tidying modules..."
	$(GO) mod tidy

# show help
help:
	@echo "available targets:"
	@echo "  build          - build the application"
	@echo "  test           - run tests"
	@echo "  test-coverage  - run tests with coverage report"
	@echo "  clean          - clean build artifacts"
	@echo "  install        - install to $(INSTALL_DIR)"
	@echo "  uninstall      - remove from $(INSTALL_DIR)"
	@echo "  run            - build and run the application"
	@echo "  lint           - run linters"
	@echo "  fmt            - format code"
	@echo "  tidy           - tidy go modules"
	@echo "  help           - show this help"
