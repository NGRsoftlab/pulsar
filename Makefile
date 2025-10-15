.PHONY: test lint build clean install-tools

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=ueba-generator
BINARY_PATH=./cmd/main

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)

# Run tests
test:
	$(GOTEST) ./...

# Run linter
lint:
	golangci-lint run

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Install development tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest