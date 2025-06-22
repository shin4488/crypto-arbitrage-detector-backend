.PHONY: build run clean test deps

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=crypto-arb-backend

# Default target
all: test build

# Install dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) -v

# Run the application
run:
	$(GOCMD) run main.go

# Run with race detection
run-race:
	$(GOCMD) run -race main.go

# Build and run
build-run: build
	./$(BINARY_NAME)

# Test the application
test:
	$(GOTEST) -v ./...

# Test with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f coverage.out

# Format code
fmt:
	$(GOCMD) fmt ./...

# Vet code
vet:
	$(GOCMD) vet ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Development workflow
dev: fmt vet test run

# Production build
prod:
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -a -installsuffix cgo -o $(BINARY_NAME) .

# Docker build (optional)
docker-build:
	docker build -t crypto-arb-backend .

# Show help
help:
	@echo "Available targets:"
	@echo "  deps         Install dependencies"
	@echo "  build        Build the application"
	@echo "  run          Run the application"
	@echo "  run-race     Run with race detection"
	@echo "  build-run    Build and run"
	@echo "  test         Run tests"
	@echo "  test-coverage Run tests with coverage"
	@echo "  clean        Clean build artifacts"
	@echo "  fmt          Format code"
	@echo "  vet          Vet code"
	@echo "  lint         Lint code"
	@echo "  dev          Development workflow (fmt, vet, test, run)"
	@echo "  prod         Production build"
	@echo "  docker-build Build Docker image"
	@echo "  help         Show this help" 