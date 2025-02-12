# Set variables for testing
TIMEOUT ?= 40m
TAGS ?= tiny
MEG_PACKAGES_NUM ?= 911
fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFallback -tags network_test,$(TAGS)

safrole:
	@echo "Running TestNodeSafrole with timeout $(TIMEOUT)..."
	go test -timeout 20m -v -run TestSafrole -tags network_test,$(TAGS)

fib:
	@echo "Running TestFib with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestFib -tags network_test,$(TAGS)

megatron:
	@echo "Running TestMegatron with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags "network_test,$(TAGS)" -packages_num=$(MEG_PACKAGES_NUM)

megatron_pprof:
	@echo "Running TestMegatron with timeout (building pprof mode with test binary)"
	go test -tags network_test -c -o node.test
	./node.test -test.run TestMegatron

debug_eg_error:
	@echo "see the error and status from guarantees"
	@echo "EG_error : "
	cat ./logs/EG_error.log
	@echo "EG_status : "
	cat ./logs/EG_status.log

disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestDisputes -tags network_test,$(TAGS)

# Target for running a specific test function with the timeout
transfer:
	@echo "Running TestTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestTransfer -tags network_test,$(TAGS)

scaled_transfer:
	@echo "Running TestScaledTransfer with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestScaledTransfer -tags network_test,$(TAGS)

balances:
	@echo "Running TestBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestBalances -tags network_test

scaled_balances:
	@echo "Running TestScaledBalances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run TestScaledBalances -tags network_test