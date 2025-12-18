# Makefile for logviewer

BINARY_NAME=logviewer

# Go build flags
LDFLAGS=-s -w

.PHONY: all clean darwin linux build local install

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
	@echo "Built $(BINARY_NAME)"

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)_linux

# Install to GOPATH/bin
install:
	go install -ldflags="$(LDFLAGS)" .
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"
