TIMEOUT            ?= 400h
TAGS               ?= tiny
MEG_L              ?= 911
MEG_S              ?= 911
TargetN            ?= 10
TargetAlgoShort    ?= 39
JCE_MANUAL         ?= true
JAM_NODE           ?= true
JAM_LOCAL_CLIENT   ?= false
MANIFEST           ?= false
UNAME_S            := $(shell uname -s)
# Set default PVM backend based on OS.
# On Linux, default to the faster 'compiler'. Otherwise, use 'interpreter'.
ifeq ($(UNAME_S),Linux)
  PVM_BACKEND ?= compiler # Default to 'compiler' on Linux
else
  PVM_BACKEND ?= interpreter # Default to 'interpreter' on non-Linux
endif

# Run as make <test_name>_<node|client|localclient>_<interpreter|compiler|sandbox>

# Conditionally set -targetN, or empty if TargetN=-1
TARGET_PARAM = $(if $(filter -1,$(TargetN)),,$(shell echo -targetN=$(TargetN)))
# PVM mode parameter to be passed to go test
PVM_PARAM = -pvm_backend=$(PVM_BACKEND)

fib :
	@echo "Running TestFib$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

algo :
	@echo "Running TestAlgo$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestAlgo$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestAlgo$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

algo_short :
	@echo "Running TestAlgo$(if $(filter -1,$(TargetAlgoShort)),, with targetAlgoShort=$(TargetAlgoShort)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestAlgo$$' -tags \"network_test,$(TAGS)\" -targetN=$(TargetAlgoShort) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestAlgo$$' \
	  -tags "network_test,$(TAGS)" -targetN=$(TargetAlgoShort) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

doom :
	@echo "Running TestDoom$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestDoom$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestDoom$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

rubic :
	@echo "Running TestRubic$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestRubic$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestRubic$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

fib2 :
	@echo "Running TestFib2$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' -tags \"network_test,$(TAGS)\" $(TARGET_PARAM)  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run '^TestFib2$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

fib2L :
	$(MAKE) fib2 TargetN=100

game_of_life :
	@echo "Running TestGameOfLife$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestGameOfLife -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) -manifest=$(MANIFEST) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestGameOfLife \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) -manifest=$(MANIFEST) $(PVM_PARAM)

auth_copy:
	@echo "Running TestAuthCopy$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@go test -timeout $(TIMEOUT) -v -run '^TestAuthCopy$$' \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)
megatron:
	@TARGET_N=$(if $(filter -1,$(TargetN)),$(MEG_S),$(TargetN)); \
	echo "Running TestMegatron with targetN=$$TARGET_N and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."; \
	echo "go test -timeout $(TIMEOUT) -v -run TestMegatron -tags \"network_test,$(TAGS)\" -targetN=$$TARGET_N  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"; \
	go test -timeout $(TIMEOUT) -v -run TestMegatron \
	  -tags "network_test,$(TAGS)" \
	  -targetN=$$TARGET_N \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

revm :
	@echo "Running TestRevm$(if $(filter -1,$(TargetN)),, with targetN=$(TargetN)) and timeout $(TIMEOUT) in $(PVM_BACKEND) mode..."
	@echo "go test -timeout $(TIMEOUT) -v -run TestRevm -tags \"network_test,$(TAGS)\" $(TARGET_PARAM) -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)"
	@go test -timeout $(TIMEOUT) -v -run TestRevm \
	  -tags "network_test,$(TAGS)" $(TARGET_PARAM) \
	  -jce_manual=$(JCE_MANUAL) -jam_node=$(JAM_NODE) -jam_local_client=$(JAM_LOCAL_CLIENT) $(PVM_PARAM)

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

# For execution mode
%_node:
	$(MAKE) $* JAM_NODE=true JCE_MANUAL=$(JCE_MANUAL)

%_client:
	$(MAKE) $* JAM_NODE=false JCE_MANUAL=false JAM_LOCAL_CLIENT=false

%_localclient:
	$(MAKE) $* JAM_NODE=false JCE_MANUAL=false JAM_LOCAL_CLIENT=true


# For PVM mode (These rules are now conditional) - only apply to _node tests.
%_interpreter:
	@if echo "$*" | grep -q "_client$$" || echo "$*" | grep -q "_localclient$$"; then \
		echo "Warning: PVM mode '_interpreter' is ignored for non-node test '$*'."; \
		$(MAKE) $*; \
	else \
		$(MAKE) $* PVM_BACKEND=interpreter; \
	fi

%_compiler:
	@if [ "$(UNAME_S)" != "Linux" ]; then \
		echo "Error: The 'compiler' PVM mode is only supported on Linux."; \
		exit 1; \
	fi
	@if echo "$*" | grep -q "_client$$" || echo "$*" | grep -q "_localclient$$"; then \
		echo "Warning: PVM mode '_compiler' is ignored for non-node test '$*'."; \
		$(MAKE) $*; \
	else \
		$(MAKE) $* PVM_BACKEND=compiler; \
	fi

%_sandbox:
	@if echo "$*" | grep -q "_client$$" || echo "$*" | grep -q "_localclient$$"; then \
		echo "Warning: PVM mode '_sandbox' is ignored for non-node test '$*'."; \
		$(MAKE) $*; \
	else \
		$(MAKE) $* PVM_BACKEND=sandbox; \
	fi
