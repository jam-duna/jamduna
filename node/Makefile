TIMEOUT            ?= 400h
TAGS               ?= tiny
MEG_L              ?= 911
MEG_S              ?= 911
TargetN            ?= 911
JCE_MANUAL         ?= true
JAM_NODE    	   ?= true
JAM_LOCAL_CLIENT   ?= false

# Conditionally set -targetN, or empty if TargetN=-1
TARGET_PARAM = $(if $(filter -1,$(TargetN)),,$(shell echo -targetN=$(TargetN)))

# fib, fib2, game_of_life, megatron, disputes are default with -jam_node=false
fib :
	@echo "Running TestFib$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)

# (Default) Fib2-10
fib2 :
	@echo "Running TestFib2$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)

# (Long) Fib2-100
fib2L :
	$(MAKE) fib2 TargetN=100

game_of_life :
	@echo "Running TestGameOfLife$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestGameOfLife -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)"
	@go test -timeout $(TIMEOUT) -v -run TestGameOfLife \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)

auth_copy:
	@echo "Running TestAuthCopy$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@go test -timeout $(TIMEOUT) -v -run '^TestAuthCopy$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)
megatron:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Running TestMegatron with targetN=$$TARGET_N and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron \
	  -tags "network_test,$(TAGS)" \
	  -targetN=$$TARGET_N \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT)

# local only test [fallback, safrole, disputes]
fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFallback -tags \"network_test,$(TAGS)\" -jam_node=true"
	@go test -timeout $(TIMEOUT) -v -run TestFallback -tags "network_test,$(TAGS)" -jam_node=true

safrole:
	@echo "Running TestNodeSafrole with timeout 20m..."
	@echo "go test -timeout 20m -v -run TestSafrole -tags \"network_test,$(TAGS)\" -jam_node=true"
	@go test -timeout 20m -v -run TestSafrole -tags "network_test,$(TAGS)" -jam_node=true

disputes:
	@echo "Running TestFibDisputes with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFibDisputes -tags \"network_test,$(TAGS)\" -jce_manual=$(JCE_MANUAL) -jam_node=true"
	@go test -timeout $(TIMEOUT) -v -run TestFibDisputes \
	  -tags "network_test,$(TAGS)" \
	  -jce_manual=$(JCE_MANUAL) -jam_node=true

# syntactic sugar for LOCAL NODE test
%_node:
	$(MAKE) $* JAM_NODE=true JCE_MANUAL=$(JCE_MANUAL)

# syntactic sugar for REMOTE CLIENT test [fib_remote, fib2_remote, game_of_life_remote, megatron_remote...]
%_client:
	$(MAKE) $* JAM_NODE=false JCE_MANUAL=false JAM_LOCAL_CLIENT=false

# syntactic sugar for LOCAL CLIENT test (requires run_parallel_jam in the background)
%_localclient:
	$(MAKE) $* JAM_NODE=false JCE_MANUAL=false JAM_LOCAL_CLIENT=true

