# Set variables for testing
TIMEOUT := 30m

# Target for running tests with the timeout
test:
	@echo "Running tests with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) ./...

# Target for running a specific test function with the timeout
testsf:
	@echo "Running TestNodeSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestNodeSafrole
