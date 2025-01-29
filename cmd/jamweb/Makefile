BINARY_NAME := jamweb
NETWORK  ?= tiny

run:
	@echo "Building and running $(BINARY_NAME)..."
	go build -tags=$(NETWORK) -o $(BINARY_NAME)
	./$(BINARY_NAME)
