# Set variables for testing
TIMEOUT ?= 10m
TAGS ?= tiny

fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFallback -tags network,$(TAGS)

safrole:
	@echo "Running TestNodeSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestSafrole -tags network,$(TAGS)

fib:
	@echo "Running TestFib with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFib -tags network,$(TAGS)

megatron:
	@echo "Running TestMegatron with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags network,$(TAGS)

megatron_pprof:
	@echo "Running TestMegatron with timeout (building pprof mode with test binary)"
	go test -tags network -c -o node.test
	./node.test -test.run TestMegatron
disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestDisputes -tags network,$(TAGS)

# Target for running a specific test function with the timeout
transfer:
	@echo "Running TestTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestTransfer -tags network,$(TAGS)

balances:
	@echo "Running TestBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestBalances -tags network

scaled_balances:
	@echo "Running TestScaledBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestScaledBalances -tags network