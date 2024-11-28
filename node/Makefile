# Set variables for testing
TIMEOUT := 30m

fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFallback$

safrole:
	@echo "Running TestNodeSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$

fib:
	@echo "Running TestFib with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFib$

megatron:
	@echo "Running TestMegatron with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestMegatron$

disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestDisputes$
# Target for running a specific test function with the timeout
testwgTransfer:
	@echo "Running TestTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestTransfer$

# Target for running a specific test function with the timeout
testRefine:
	@echo "Running TestRefine with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestRefine$

# Target for running a specific test function with the timeout
testAccumulate:
	@echo "Running TestAccumulate with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestAccumulate$

# Target for running a specific test function with the timeout
testGeneral:
	@echo "Running TestGeneral with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestGeneral$

# Target for running a specific test function with the timeout
testGenerateRefineTestVectors:
	@echo "Running TestGenerateRefineTestVectors with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestGenerateRefineTestVectors$

# Target for running a specific test function with the timeout
testGenerateAccumulateTestVectors:
	@echo "Running TestGenerateAccumulateTestVectors with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestGenerateAccumulateTestVectors$

# Target for running a specific test function with the timeout
testGenerateGeneralTestVectors:
	@echo "Running TestGenerateGeneralTestVectors with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestGenerateGeneralTestVectors$
