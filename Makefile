.PHONY: test lint build clean install-tools

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=ueba-event-generator.exe
BINARY_PATH=./cmd

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)

# Run tests all tests from root dir
test-all:
	$(GOTEST) ./... -v -race

# Run becnhmarks from some project package 
test-bench:
	$(GOTEST) ./... -bench='.' -benchtime=1s -cpu='4,8'

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
