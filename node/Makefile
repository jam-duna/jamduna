# Default variables
TIMEOUT ?= 40m
TAGS ?= tiny
MEG_L ?= 911
MEG_S ?= 8
TargetN ?= -1
PREREQ ?= false
PVMAUTHLOG ?= false

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
	@echo "go test -timeout $(TIMEOUT) -v -run TestFib -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -pvm_authoring_log=$(PVMAUTHLOG)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib$$' -tags "network_test,$(TAGS)" $(TARGET_PARAM) -pvm_authoring_log=$(PVMAUTHLOG)

fib2:
	@echo "Running TestFib2$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFib2 -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -pvm_authoring_log=$(PVMAUTHLOG)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' -tags "network_test,$(TAGS)" $(TARGET_PARAM) -pvm_authoring_log=$(PVMAUTHLOG)

megatron:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_L),$(TargetN)); \
	echo "Running TestMegatron with targetN=$$TARGET and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N -prereq_test=$(PREREQ) -pvm_authoring_log=$(PVMAUTHLOG)"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags "network_test,$(TAGS)" -targetN=$$TARGET_N -prereq_test=$(PREREQ) -pvm_authoring_log=$(PVMAUTHLOG)

megatron_short:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Running TestMegatron (short) with targetN=$$TARGET_N and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N -prereq_test=$(PREREQ) -pvm_authoring_log=$(PVMAUTHLOG)"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron -tags "network_test,$(TAGS)" -targetN=$$TARGET_N -prereq_test=$(PREREQ) -pvm_authoring_log=$(PVMAUTHLOG)

megatron_pprof:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Building test binary for TestMegatron in pprof mode..."; \
	echo "go test -tags \"network_test $(TAGS)\" -c -o node.test"; \
	go test -tags "network_test $(TAGS)" -c -o node.test; \
	echo "Running TestMegatron using test binary with timeout $(TIMEOUT)..."; \
	echo "./node.test -test.run TestMegatron -test.timeout $(TIMEOUT) -targetN=10 -prereq_test=true"; \
	./node.test -test.run TestMegatron -test.timeout $(TIMEOUT) -targetN=10 -prereq_test=true
	go tool pprof -http=":8080" cpu.pprof

archive_node:
	@echo "Running TestArchiveNode with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestArchiveNode -tags \"network_test,$(TAGS)\""
	@go test -timeout $(TIMEOUT) -v -run TestArchiveNode -tags "network_test,$(TAGS)"

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

empty:
	@echo "Running TestEmpty$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestEmpty -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestEmpty -tags "network_test,$(TAGS)" $(TARGET_PARAM)

blake2b:
	@echo "Running TestBlake2b$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestBlake2b -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestBlake2b -tags "network_test,$(TAGS)" $(TARGET_PARAM)

megatron_short_with_prereq:
	@$(MAKE) megatron_short PREREQ=true

megatron_short_with_prereq_log:
	@$(MAKE) megatron_short PREREQ=true PVMAUTHLOG=true

fib2_log:
	@$(MAKE) fib2 PVMAUTHLOG=true

game_of_life:
	@echo "Running TestGameOfLife$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestGameOfLife -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestGameOfLife -tags "network_test,$(TAGS)" $(TARGET_PARAM)
