# Set variables for testing
TIMEOUT := 10m

fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFallback -tags network

safrole:
	@echo "Running TestNodeSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestSafrole -tags network

fib:
	@echo "Running TestFib with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFib -tags network

megatron:
	@echo "Running TestMegatron with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags network

megatron_pprof:
	@echo "Running TestMegatron with timeout (building pprof mode with test binary)"
	go test -tags network -c -o node.test
	./node.test -test.run TestMegatron
disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestDisputes -tags network

# Target for running a specific test function with the timeout
transfer:
	@echo "Running TestTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestTransfer -tags network

balances:
	@echo "Running TestBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestBalances -tags network

scaled_balances:
	@echo "Running TestScaledBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestScaledBalances -tags network