# Default variables
TIMEOUT ?= 40m
TAGS ?= tiny
MEG_L ?= 911
MEG_S ?= 25
TargetN ?= -1

# Conditionally set -targetN, or empty if TargetN=-1
TARGET_PARAM = $(if $(filter -1,$(TargetN)),,$(shell echo -targetN=$(TargetN)))

fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFallback -tags \"network_test,$(TAGS)\""
	@go test -timeout $(TIMEOUT) -v -run TestFallback -tags "network_test,$(TAGS)"

safrole:
	@echo "Running TestNodeSafrole with timeout 20m..."
	@echo "go test -timeout 20m -v -run TestSafrole -tags \"network_test,$(TAGS)\""
	@go test -timeout 20m -v -run TestSafrole -tags "network_test,$(TAGS)"

fib:
	@echo "Running TestFib$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFib -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestFib -tags "network_test,$(TAGS)" $(TARGET_PARAM)

megatron:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_L),$(TargetN)); \
	echo "Running TestMegatron with targetN=$$TARGET and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags "network_test,$(TAGS)" -targetN=$$TARGET_N

megatron_short:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Running TestMegatron (short) with targetN=$$TARGET_N and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags "network_test,$(TAGS)" -targetN=$$TARGET_N

megatron_pprof:
	@echo "Building test binary for TestMegatron in pprof mode..."
	@echo "go test -tags network_test -c -o node.test"
	@go test -tags network_test -c -o node.test
	@echo "Running TestMegatron using test binary..."
	@echo "./node.test -test.run TestMegatron"
	@./node.test -test.run TestMegatron

debug_eg_error:
	@echo "Displaying error and status logs from guarantees..."
	@echo "EG_error:"
	@cat ./logs/EG_error.log
	@echo "EG_status:"
	@cat ./logs/EG_status.log

disputes:
	@echo "Running TestDisputes with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestDisputes -tags \"network_test,$(TAGS)\""
	@go test -timeout $(TIMEOUT) -v -run TestDisputes -tags "network_test,$(TAGS)"

transfer:
	@echo "Running TestTransfer$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestTransfer -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestTransfer -tags "network_test,$(TAGS)" $(TARGET_PARAM)

scaled_transfer:
	@echo "Running TestScaledTransfer$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestScaledTransfer -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestScaledTransfer -tags "network_test,$(TAGS)" $(TARGET_PARAM)

balances:
	@echo "Running TestBalances$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestBalances -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestBalances -tags "network_test,$(TAGS)" $(TARGET_PARAM)

scaled_balances:
	@echo "Running TestScaledBalances$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestScaledBalances -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestScaledBalances -tags "network_test,$(TAGS)" $(TARGET_PARAM)