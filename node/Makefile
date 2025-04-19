TIMEOUT            ?= 40m
TAGS               ?= tiny
MEG_L              ?= 911
MEG_S              ?= 8
TargetN            ?= -1
JCE_MANUAL         ?= true
JAM_LOCAL    	   ?= true

# Conditionally set -targetN, or empty if TargetN=-1
TARGET_PARAM = $(if $(filter -1,$(TargetN)),,$(shell echo -targetN=$(TargetN)))

# fib, fib2, game_of_life, megatron, disputes are default with -jam_local=false
fib :
	@echo "Running TestFib$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)

# (Default) Fib2-10
fib2 :
	@echo "Running TestFib2$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)

# (Long) Fib2-100
fib2L :
	$(MAKE) fib2 TargetN=100

game_of_life :
	@echo "Running TestGameOfLife$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestGameOfLife -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)"
	@go test -timeout $(TIMEOUT) -v -run TestGameOfLife \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)

megatron:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Running TestMegatron with targetN=$$TARGET_N and timeout $(TIMEOUT)..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron \
	  -tags "network_test,$(TAGS)" \
	  -targetN=$$TARGET_N \
	  -jce_manual=$(JCE_MANUAL) -jam_local=$(JAM_LOCAL)

# local only test [fallback, safrole, disputes]
fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFallback -tags \"network_test,$(TAGS)\" -jam_local=true"
	@go test -timeout $(TIMEOUT) -v -run TestFallback -tags "network_test,$(TAGS)" -jam_local=true

safrole:
	@echo "Running TestNodeSafrole with timeout 20m..."
	@echo "go test -timeout 20m -v -run TestSafrole -tags \"network_test,$(TAGS)\" -jam_local=true"
	@go test -timeout 20m -v -run TestSafrole -tags "network_test,$(TAGS)" -jam_local=true

disputes:
	@echo "Running TestFibDisputes with timeout $(TIMEOUT)..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestFibDisputes -tags \"network_test,$(TAGS)\" -jce_manual=$(JCE_MANUAL) -jam_local=true"
	@go test -timeout $(TIMEOUT) -v -run TestFibDisputes \
	  -tags "network_test,$(TAGS)" \
	  -jce_manual=$(JCE_MANUAL) -jam_local=true

# syntactic sugar for local test [fib_local, fib2_local, game_of_life_local, megatron_local...]
%_local:
	$(MAKE) $* JAM_LOCAL=true JCE_MANUAL=$(JCE_MANUAL)

# syntactic sugar for remote test [fib_remote, fib2_remote, game_of_life_remote, megatron_remote...]
%_remote:
	$(MAKE) $* JAM_LOCAL=false JCE_MANUAL=false
