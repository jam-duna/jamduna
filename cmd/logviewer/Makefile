# Makefile for logviewer

BINARY_NAME=logviewer
VERSION=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d %H:%M:%S')
GIT_HASH=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go build flags with version info
LDFLAGS=-s -w -X 'main.Version=$(VERSION)-$(GIT_HASH)' -X 'main.BuildTime=$(BUILD_TIME)'

.PHONY: all clean darwin linux build local install run

all: darwin linux

build: darwin linux

# Build for macOS (arm64 only) - default binary name
darwin:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Built $(BINARY_NAME)"

# Build for Linux (amd64)
linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)_linux .
	@echo "Built $(BINARY_NAME)_linux"

# Build for current platform
local:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Built $(BINARY_NAME) $(VERSION)-$(GIT_HASH)"

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)_linux

# Install to GOPATH/bin
install:
	go install -ldflags="$(LDFLAGS)" .
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

# Run logviewer (pointing to jam/logs directory)
run: local
	./$(BINARY_NAME) -dir ../../logs
