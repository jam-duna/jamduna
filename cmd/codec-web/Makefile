BINARY_NAME := codec-web
NETWORK  ?= tiny

.PHONY: build

all: buildrun

build:
	@echo "Building $(BINARY_NAME)..."
	go build -tags=$(NETWORK) -o $(BINARY_NAME)

run: 
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

buildrun: build run