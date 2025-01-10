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

disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestDisputes -tags network

# Target for running a specific test function with the timeout
transfer:
	@echo "Running TestTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestTransfer -tags network


# Host function testing
refine:
	@echo "Running Refine host function testing..."
	go test -run TestGenerateRefineTestVectors -tags network
	go test -run TestRefine -tags network

accumulate:
	@echo "Running Accumulate host function testing..."
	go test -run TestGenerateAccumulateTestVectors -tags network
	go test -run TestAccumulate -tags network

general:
	@echo "Running General host function testing..."
	go test -run TestGenerateGeneralTestVectors -tags network
	go test -run TestGeneral -tags network
