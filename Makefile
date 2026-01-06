OUTPUT_DIR := bin
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
SRC := jam.go

NUM_NODES ?= 6
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
JAM_START_TIME ?= $(shell \
	if date --version >/dev/null 2>&1; then \
		date -d "5 seconds" "+%Y-%m-%d %H:%M:%S"; \
	else \
		date -v+5S "+%Y-%m-%d %H:%M:%S"; \
	fi)
RAW_HOSTS_FILE ?= hosts.txt
HOSTS_FILE := ../$(RAW_HOSTS_FILE)

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
GIT_TAG := $(shell git describe --tags --abbrev=0 | awk -F. '{printf "%d.%d.%d.%d\n", $$1, $$2, $$3, $$4 + 1}')
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_FULL_COMMIT   := $(shell git rev-parse HEAD)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

BINARY := jamduna


# Set default PVM backend based on OS.
# On Linux, default to the faster 'compiler'. Otherwise, use 'interpreter'.
ifeq ($(UNAME_S),Linux)
  PVM_BACKEND ?= compiler # Default to 'compiler' on Linux
  #CHAINSPEC ?= chainspecs/$(ARCH)/polkajam-spec.json
  CHAINSPEC ?= chainspecs/jamduna-spec.json
else
  PVM_BACKEND ?= interpreter # Default to 'interpreter' on non-Linux
  #CHAINSPEC ?= chainspecs/$(ARCH)/polkajam-spec.json
  CHAINSPEC ?= chainspecs/jamduna-spec.json
endif


POLKAJAM_BIN ?= bin/polkajam
# Linker flags to strip symbols and embed version info
GO_LDFLAGS := -s -w \
  -X 'main.Version=$(GIT_TAG)' \
  -X 'main.Commit=$(GIT_COMMIT)' \
  -X 'main.BuildTime=$(BUILD_TIME)'

ifeq ($(UNAME_S),Linux)
  ifeq ($(UNAME_M),x86_64)
    ARCH := linux-amd64
  else ifeq ($(UNAME_M),aarch64)
    ARCH := linux-arm64
  endif
else ifeq ($(UNAME_S),Darwin)
  ifeq ($(UNAME_M),x86_64)
    ARCH := mac-amd64
  else ifeq ($(UNAME_M),arm64)
    ARCH := mac-arm64
  endif
endif

.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean jam_without_ffi_build run_parallel_jam kill_parallel_jam run_jam build_remote_nodes run_jam_remote_nodes da jamweb validatetraces testnet init-submodules update-submodules update-pvm-submodule update-services-submodule evm_jamtest algo_jamtest safrole_jamtest telemetry telemetry_viewer kill_telemetry restart_telemetry

# ----------------------------------------
# Port Configuration
# ----------------------------------------
DEFAULT_PORT ?= 40000
SINGLE_NODE_PORT ?= $(shell echo $$(($(DEFAULT_PORT) + 5)))
RPC_BASE_PORT ?= 19800
EVM_RPC_PORT ?= 8600
TELEMETRY_PORT ?= 9999
TELEMETRY_WEB_PORT ?= 8088

# Telemetry settings
TELEMETRY_LOG ?= logs/telemetry.log


update-submodules:
	@echo "Updating git submodules..."
	git submodule update --remote --merge

# Update pvm submodule to a specific commit or latest from local pvm repo
# Usage:
#   make update-pvm-submodule COMMIT=456513b  (specific commit)
#   make update-pvm-submodule                  (use latest from local ../pvm repo)
update-pvm-submodule:
	@if [ -z "$(COMMIT)" ]; then \
		if [ -d "../pvm" ] && [ -d "../pvm/.git" ]; then \
			LATEST_COMMIT=$$(cd ../pvm && git rev-parse HEAD); \
			SHORT_COMMIT=$$(cd ../pvm && git rev-parse --short HEAD); \
			echo "No COMMIT specified. Using latest from local pvm repo: $$SHORT_COMMIT"; \
			echo "Updating pvm submodule to commit $$SHORT_COMMIT..."; \
			(cd pvm && git fetch origin && git checkout $$LATEST_COMMIT); \
			git add pvm; \
			echo "✓ pvm submodule updated to $$SHORT_COMMIT and staged"; \
			echo "Next steps:"; \
			echo "  1. Review: git status"; \
			echo "  2. Commit: git commit -m 'Update pvm submodule to $$SHORT_COMMIT'"; \
			echo "  3. Push: git push origin $$(git rev-parse --abbrev-ref HEAD)"; \
		else \
			echo "Error: No COMMIT specified and ../pvm directory not found"; \
			echo "Usage:"; \
			echo "  make update-pvm-submodule COMMIT=456513b  (specific commit)"; \
			echo "  make update-pvm-submodule                  (use latest from ../pvm)"; \
			exit 1; \
		fi; \
	else \
		echo "Updating pvm submodule to commit $(COMMIT)..."; \
		(cd pvm && git fetch origin && git checkout $(COMMIT)); \
		git add pvm; \
		echo "✓ pvm submodule updated to $(COMMIT) and staged"; \
		echo "Next steps:"; \
		echo "  1. Review: git status"; \
		echo "  2. Commit: git commit -m 'Update pvm submodule to $(COMMIT)'"; \
		echo "  3. Push: git push origin $$(git rev-parse --abbrev-ref HEAD)"; \
	fi

# Update services submodule to a specific commit or latest from local services repo
# Usage:
#   make update-services-submodule COMMIT=abc123  (specific commit)
#   make update-services-submodule                 (use latest from local ../services repo)
update-services-submodule:
	@if [ -z "$(COMMIT)" ]; then \
		if [ -d "../services" ] && [ -d "../services/.git" ]; then \
			LATEST_COMMIT=$$(cd ../services && git rev-parse HEAD); \
			SHORT_COMMIT=$$(cd ../services && git rev-parse --short HEAD); \
			echo "No COMMIT specified. Using latest from local services repo: $$SHORT_COMMIT"; \
			echo "Updating services submodule to commit $$SHORT_COMMIT..."; \
			cd services && git fetch origin && git checkout $$LATEST_COMMIT; \
			git add services; \
			echo "✓ services submodule updated to $$SHORT_COMMIT and staged"; \
			echo "Next steps:"; \
			echo "  1. Review: git status"; \
			echo "  2. Commit: git commit -m 'Update services submodule to $$SHORT_COMMIT'"; \
			echo "  3. Push: git push origin $$(git rev-parse --abbrev-ref HEAD)"; \
		else \
			echo "Error: No COMMIT specified and ../services directory not found"; \
			echo "Usage:"; \
			echo "  make update-services-submodule COMMIT=abc123  (specific commit)"; \
			echo "  make update-services-submodule                 (use latest from ../services)"; \
			exit 1; \
		fi; \
	else \
		echo "Updating services submodule to commit $(COMMIT)..."; \
		cd services && git fetch origin && git checkout $(COMMIT); \
		git add services; \
		echo "✓ services submodule updated to $(COMMIT) and staged"; \
		echo "Next steps:"; \
		echo "  1. Review: git status"; \
		echo "  2. Commit: git commit -m 'Update services submodule to $(COMMIT)'"; \
		echo "  3. Push: git push origin $$(git rev-parse --abbrev-ref HEAD)"; \
	fi

spin_localclient: jam  jam_clean spin_6

spin_6:
	@rm -rf ${HOME}/.jamduna/jam-*
	@for i in 0 1 2 3 4 5; do \
		$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator $$i  --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND)  >logs/jamduna-$$i.log 2>&1 & \
	done

spin_5:
	@rm -rf ${HOME}/.jamduna/jam-*
	@for i in 1 2 3 4 5; do \
		$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator $$i  --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND)  >logs/jamduna-$$i.log 2>&1 & \
	done

spin_0:
	@for i in 0; do \
		RUST_LOG=polkavm=trace,jam_node=trace $(POLKAJAM_BIN) --chain ${CHAINSPEC} run --temp --dev-validator $$i --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) >logs/polkajam-$$i.log 2>&1 & \
	done

run_builder:
	@rm -rf ${HOME}/.jamduna/jam-*
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator 6 --role builder --rpc-port=$$(($(RPC_BASE_PORT) + 6)) --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND) --debug rotation,guarantees --telemetry localhost:$(TELEMETRY_PORT)

run_1:
	@mkdir -p logs
	@rm -rf ${HOME}/.jamduna/jam-*
	@echo ">> Starting jamduna validator 5..."
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator 5 --rpc-port=$$(($(RPC_BASE_PORT) + 5)) --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND) --debug rotation,guarantees --telemetry localhost:$(TELEMETRY_PORT) >logs/jamduna-5.log 2>&1 &
	@sleep 1
	@if pgrep -f "$(BINARY).*dev-validator 5" >/dev/null; then \
		echo "✅ jamduna validator 5 started (log: logs/jamduna-5.log)"; \
	else \
		echo "❌ jamduna validator 5 failed to start. Check logs/jamduna-5.log"; \
		tail -20 logs/jamduna-5.log 2>/dev/null || true; \
	fi

run_5:
	@mkdir -p logs
	@if [ ! -x "$(POLKAJAM_BIN)" ]; then \
		echo "❌ Error: $(POLKAJAM_BIN) not found or not executable"; \
		exit 1; \
	fi
	@echo ">> Starting polkajam validators 0-4..."
	@for i in 0 1 2 3 4; do \
		echo "   Starting validator $$i on port $$(($(RPC_BASE_PORT) + $$i))..."; \
		RUST_LOG=chain-core=debug,jam_node=trace $(POLKAJAM_BIN) --chain ${CHAINSPEC} run --pvm-backend $(PVM_BACKEND) --temp --dev-validator $$i --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) --telemetry localhost:$(TELEMETRY_PORT) >logs/polkajam-$$i.log 2>&1 & \
	done
	@sleep 2
	@started=$$(pgrep -f "$(POLKAJAM_BIN).*dev-validator" 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$started" -ge 5 ]; then \
		echo "✅ All 5 polkajam validators started"; \
	else \
		echo "⚠️  Only $$started/5 polkajam validators started. Check logs/polkajam-*.log"; \
		for i in 0 1 2 3 4; do \
			if ! pgrep -f "$(POLKAJAM_BIN).*dev-validator $$i" >/dev/null 2>&1; then \
				echo "   ❌ Validator $$i failed:"; \
				tail -5 logs/polkajam-$$i.log 2>/dev/null || echo "      (no log)"; \
			fi; \
		done; \
	fi

run_6:
	@mkdir -p logs
	@if [ ! -x "$(POLKAJAM_BIN)" ]; then \
		echo "❌ Error: $(POLKAJAM_BIN) not found or not executable"; \
		exit 1; \
	fi
	@echo ">> Starting polkajam validators 0-5..."
	@for i in 0 1 2 3 4 5; do \
		RUST_LOG=chain-core=debug,jam_node=trace $(POLKAJAM_BIN) --chain ${CHAINSPEC} run --pvm-backend $(PVM_BACKEND) --temp --dev-validator $$i --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) --telemetry localhost:$(TELEMETRY_PORT) >logs/polkajam-$$i.log 2>&1 & \
	done
	@sleep 2
	@started=$$(pgrep -f "$(POLKAJAM_BIN).*dev-validator" 2>/dev/null | wc -l | tr -d ' '); \
	echo "✅ $$started/6 polkajam validators started"

jam:
	@echo "Building JAM...  "
	mkdir -p $(OUTPUT_DIR)
	go build -tags=  -o $(OUTPUT_DIR)/$(ARCH)/$(BINARY) .

evm-builder:
	@echo "Building EVM Builder..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(ARCH)/evm-builder ./cmd/evm-builder

orchard-builder:
	@echo "Building Orchard Builder..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(ARCH)/orchard-builder ./cmd/orchard-builder

builders: evm-builder orchard-builder
	@echo "All builders built successfully"

run_evm_builder:
	@echo "Building EVM Builder..."
	@mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(ARCH)/evm-builder ./cmd/evm-builder
	@echo "Stopping any existing EVM Builder..."
	@ps aux | grep 'bin/.*[e]vm-builder' | awk '{print $$2}' | while read pid; do kill $$pid 2>/dev/null || true; done
	@mkdir -p logs
	@sleep 1
	@echo "Starting EVM Builder..."
	@echo "EVM RPC available at: http://localhost:$(EVM_RPC_PORT)"
	@$(OUTPUT_DIR)/$(ARCH)/evm-builder run --dev-validator 6 --chain $(CHAINSPEC) --pvm-backend $(PVM_BACKEND) --debug rotation,guarantees --evm-rpc-port $(EVM_RPC_PORT) --telemetry localhost:$(TELEMETRY_PORT) 2>&1 | tee logs/evm-builder.log

run_evm_builder_remote: evm-builder
	@echo "Stopping any existing EVM Builder..."
	@ps aux | grep 'bin/.*[e]vm-builder' | awk '{print $$2}' | while read pid; do kill $$pid 2>/dev/null || true; done
	@rm -rf /root/.jamduna/jam-6
	@mkdir -p logs
	@sleep 1
	@echo "Starting EVM Builder (distributed mode)..."
	@echo "EVM RPC available at: http://localhost:$(EVM_RPC_PORT)"
	@$(OUTPUT_DIR)/$(ARCH)/evm-builder run --dev-validator 6 --chain chainspecs/jamduna-distributed-spec.json --pvm-backend $(PVM_BACKEND) --debug rotation,guarantees --evm-rpc-port $(EVM_RPC_PORT) --telemetry localhost:$(TELEMETRY_PORT) 2>&1 | tee logs/remote-evm-builder.log

duna_spec: jam
	@echo "Generating Duna chainspec (local)..."
	./$(OUTPUT_DIR)/$(ARCH)/$(BINARY) gen-spec chainspecs/dev-config.json chainspecs/jamduna-spec.json

duna_spec_remote: jam
	@echo "Generating Duna chainspec (distributed)..."
	./$(OUTPUT_DIR)/$(ARCH)/$(BINARY) gen-spec chainspecs/distributed-config.json chainspecs/jamduna-distributed-spec.json

stop_remoteclient_jam:
	@echo "Stopping all remote validators..."
	ansible-playbook -i yaml/hosts.txt yaml/jamduna_stop.yaml

run_remoteclient_jam:
	@echo "Starting all remote validators..."
	ansible-playbook -i yaml/hosts.txt yaml/jamduna_start.yaml

polka_spec: jam
	@echo "Generating Polka chainspec..."
	@mkdir -p chainspecs/$(ARCH)
	./$(POLKAJAM_BIN) gen-spec chainspecs/dev-config.json chainspecs/$(ARCH)/polkajam-spec.json

# ANSI color codes
GREEN=\033[0;32m
YELLOW=\033[1;33m
RESET=\033[0m

define build_with_status
	@echo "Building $(1)..."
	@$(2) && echo "$(GREEN)✓ Done: $(1)$(RESET)" || echo "$(YELLOW)⚠ Failed to build: $(1)$(RESET)"
endef

native_static_jam_linux_amd64:
	@echo "Building JamDuna static binary natively for Linux (x86_64)..."
	@echo "--> Creating temporary symlink for libunicorn..."
	@ln -s ${PWD}/ffi/libunicorn.linux_amd64.a ffi/libunicorn.a
	-@$(call build_with_status,native_static_jam_linux_amd64,\
		GOOS=linux GOARCH=amd64 CC=musl-gcc CGO_ENABLED=1 \
		go build -tags "cgo" \
		-ldflags "$(GO_LDFLAGS) -extldflags '-static'" \
		-o $(OUTPUT_DIR)/linux-amd64/$(BINARY) . && strip $(OUTPUT_DIR)/linux-amd64/jamduna 2>/dev/null)
	@rm ffi/libunicorn.a

static_jam_linux_amd64:
	@echo "Building JamDuna binary for Linux (x86_64)..."
	@echo "--> Creating temporary symlink for libunicorn..."
	@ln -s $(CURDIR)/ffi/libunicorn.linux_amd64.a ffi/libunicorn.a
	@# The '-' at the beginning of the next line tells 'make' to continue
	@# to the cleanup step even if the build command fails.
	-@$(call build_with_status,static_jam_linux_amd64,\
	GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS) -extldflags '-static'" \
	-o $(OUTPUT_DIR)/linux-amd64/$(BINARY) . && strip $(OUTPUT_DIR)/linux-amd64/jamduna 2>/dev/null)
	@echo "--> Cleaning up temporary symlink..."
	@rm ffi/libunicorn.a

static_jam_linux_arm64:
	@echo "Building JamDuna binary for Linux (aarch64)..."
	$(call build_with_status,static_jam_linux_arm64,\
	GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS) -extldflags '-static'" \
	-o $(OUTPUT_DIR)/linux-arm64/$(BINARY) . && strip $(OUTPUT_DIR)/linux-arm64/jamduna 2>/dev/null)

static_jam_darwin_amd64:
	@echo "Building JamDuna binary for macOS (x86_64)..."
	@echo "--> Creating temporary symlink for libunicorn..."
	@ln -s $(CURDIR)/ffi/libunicorn.mac.a ffi/libunicorn.a
	-@$(call build_with_status,static_jam_darwin_amd64,\
	GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS)" \
	-o $(OUTPUT_DIR)/mac-amd64/$(BINARY) . && strip -x $(OUTPUT_DIR)/mac-amd64/jamduna)
	@echo "--> Cleaning up temporary symlink..."
	@rm ffi/libunicorn.a

static_jam_darwin_arm64:
	@echo "Building JamDuna binary for macOS (aarch64)..."
	@echo "--> Creating temporary symlink for libunicorn..."
	@ln -s $(CURDIR)/ffi/libunicorn.mac.a ffi/libunicorn.a
	-@$(call build_with_status,static_jam_darwin_arm64,\
	CGO_ENABLED=1 CC=clang \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS)" \
	-o $(OUTPUT_DIR)/mac-arm64/$(BINARY) . && strip -x $(OUTPUT_DIR)/mac-arm64/jamduna)
	@echo "--> Cleaning up temporary symlink..."
	@rm ffi/libunicorn.a

static_jam_all:
	@echo "Building static JAM for available platforms..."

	# Always build Linux x86_64
	@$(MAKE) static_jam_linux_amd64

	# Build Linux ARM64 only if compiler exists or on ARM host
	#@if command -v aarch64-linux-musl-gcc >/dev/null 2>&1; then \
	#  $(MAKE) static_jam_linux_arm64; \
	#else \
	#  echo "⚠ Skipping Linux ARM64 (no aarch64-linux-musl-gcc)"; \
	#fi

	# Build macOS binaries only on macOS
	@if [ "$(UNAME_S)" = "Darwin" ]; then \
	  $(MAKE) static_jam_darwin_amd64; \
	  $(MAKE) static_jam_darwin_arm64; \
	else \
	  echo "⚠ Skipping macOS targets (not on Darwin)"; \
	fi

	# Build Windows AMD64 if mingw compiler exists
	@if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then \
	  echo "⚠ Skipping Windows AMD64 (no x86_64-w64-mingw32-gcc) for now"; \
	else \
	  echo "⚠ Skipping Windows AMD64 (no x86_64-w64-mingw32-gcc)"; \
	fi

tiny: jam reset_remote_nodes
	ansible-playbook -u root -i $(HOSTS_FILE) -e "MODE=immediate" /root/go/src/github.com/colorfulnotion/jam/yaml/jam_restart.yaml

jam_clean:
	@echo "Cleaning all jam data directories under ~/.jamduna..."
	@rm -rf ${HOME}/.jamduna/jam-*
	@echo "Done."

run_parallel_jam:
	@mkdir -p logs
	@echo "Starting $(NUM_NODES) instances of $(OUTPUT_DIR)/$(BINARY) with start_time=$(JAM_START_TIME)..."
	@for i in $$(seq 0 $$(($(NUM_NODES) - 1))); do \
		PORT=$$(($(DEFAULT_PORT) + $$i)); \
		V_IDX=$$i; \
		echo ">> Starting instance $$V_IDX on port $$PORT..."; \
		$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run \
			--chain $(CHAINSPEC) \
			--dev-validator $$V_IDX \
			--debug rotation,guarantees \
			--pvm-backend $(PVM_BACKEND) \
			--start-time "$(JAM_START_TIME)" \
			--telemetry localhost:$(TELEMETRY_PORT) \
			>logs/jamduna-$$i.log 2>&1 & \
	done
	@sleep 1
	@echo "✅ All instances started and running in parallel."
	@tail -f logs/jamduna-$(shell echo $$(($(NUM_NODES)-1))).log

run_polkajam_all:
	@mkdir -p logs
	@for i in $$(seq 0 $$(($(NUM_NODES) - 1))); do \
		PORT=$$(($(DEFAULT_PORT) + $$i)); \
		V_IDX=$$i; \
		echo ">> Starting instance $$V_IDX on port $$PORT..."; \
		$(POLKAJAM_BIN) --chain $(CHAINSPEC) --pvm-backend $(PVM_BACKEND) run  --temp  --dev-validator $$V_IDX --rpc-port=$$(($(RPC_BASE_PORT) + $$i)) & \
	done; \

run_localclient: kill jam jam_clean start_telemetry
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_5 run_1
	@echo ""
	@echo "========================================"
	@echo "Local JAM network started (mixed mode)"
	@echo "  Validators 0-4: polkajam (Rust)"
	@echo "  Validator 5:    jamduna (Go)"
	@echo "  Telemetry:      http://localhost:$(TELEMETRY_WEB_PORT)"
	@echo "  Logs:           logs/*.log"
	@echo "========================================"
	@echo ""
	@echo "To monitor: tail -f logs/jamduna-5.log"
	@echo "To stop:    make kill"

run_localclient_jam: kill duna_spec jam jam_clean start_telemetry
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_parallel_jam

run_localclient_jam_dead: kill jam jam_clean start_telemetry
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_parallel_jam_with_deadnode

run_single_node:jam_clean
	@echo "Starting single node JAM instance..."
	@echo "Starting $(OUTPUT_DIR)/$(ARCH)/$(BINARY)... with network $(NETWORK) port $(SINGLE_NODE_PORT) start-time $(JAM_START_TIME)"
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --chain $(CHAINSPEC) --port $(SINGLE_NODE_PORT) --start-time "$(JAM_START_TIME)" --dev-validator 5
	@echo "Instance started."
run_parallel_jam_with_deadnode:
	@mkdir -p logs
	@echo "Starting $(NUM_NODES) instances of $(OUTPUT_DIR)/$(BINARY) (one dead node)..."
	@for i in $$(seq 0 $$(($(NUM_NODES) - 2))); do \
		PORT=$$(($(DEFAULT_PORT) + $$i)); \
		V_IDX=$$i; \
		echo ">> Starting instance $$V_IDX on port $$PORT..."; \
		$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run \
			--chain $(CHAINSPEC) \
			--dev-validator $$V_IDX \
			--debug rotation,guarantees \
			--pvm-backend $(PVM_BACKEND) \
			--telemetry localhost:$(TELEMETRY_PORT) \
			>logs/jamduna-$$i.log 2>&1 & \
	done
	@sleep 1
	@echo "✅ All instances started (node $$(($(NUM_NODES) - 1)) is dead)."
kill_parallel_jam:
	@echo "Killing all instances of $(OUTPUT_DIR)/$(BINARY)..."
	@pgrep -f "$(OUTPUT_DIR)/$(BINARY)"
	@pkill -f "$(OUTPUT_DIR)/$(BINARY)"
	@echo "All instances killed."

kill: kill_telemetry
	@echo "Kill Jam Binaries(if any)..."
	@ps aux | grep 'bin/.*[j]amduna' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@ps aux | grep 'bin/.*[e]vm-builder' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@ps aux | grep 'bin/.*[o]rchard-builder' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@sleep 1
	@ps aux | grep 'bin/.*[j]amduna' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@ps aux | grep 'bin/.*[e]vm-builder' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@ps aux | grep 'bin/.*[o]rchard-builder' | awk '{print $$2}' | while read pid; do kill -9 $$pid 2>/dev/null || true; done
	@sleep 1
	@if ps aux | grep -E 'bin/.*[j]amduna|bin/.*[e]vm-builder|bin/.*[o]rchard-builder' > /dev/null; then echo "WARNING: Some processes still running"; ps aux | grep -E 'bin/.*[j]amduna|bin/.*[e]vm-builder|bin/.*[o]rchard-builder'; else echo "All jam processes terminated."; fi
	@echo "Process cleanup complete."


# env setup for remote nodes
jam_set:
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && echo \"export CARGO_MANIFEST_DIR=\$(pwd)\" >> ~/.bashrc'"
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "source ~/.bashrc"
# build ffi and apply latest code
build_remote_nodes:
	@echo "Building JAM on all remote nodes..."
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && git fetch origin && git reset --hard origin/$(BRANCH) && git clean -fd'"
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && make cryptolib'"
	@echo "All remote nodes built."
# clean the process and delete the storage
clean_remote_nodes:
	@echo "Cleaning JAM on all remote nodes..."
	@sudo /usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'rm -rf .jam'"
	#grep the pid from port  and kill it
	@sudo /usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -c 'command -v lsof >/dev/null && lsof -t -i:9900 | xargs --no-run-if-empty kill -9'"
	@echo "All remote nodes cleaned."
# update the latest commit on remote nodes
reset_remote_nodes:
	@echo "Resetting JAM on all remote nodes..."
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && git fetch origin && git reset --hard origin/$(BRANCH) && git clean -fd'"
	@echo "All remote nodes reset."

da:
	@echo "Building JAM..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/da da.go
jamweb:
		@echo "Building JAM WEB..."
		@cd jamweb && go build

validatetraces:
		@echo "Building JAM validatetraces..."
		@cd cmd/validatetraces && go build

testnet:
	make jam
	docker build -t colorfulnotion/jam .
	docker-compose up

#clean:
#	rm -f $(OUTPUT_DIR)/$(BINARY)

# Target to build BLS FFI library with musl
# TODO : everyone should run $rustup target add x86_64-unknown-linux-musl

cryptolib:
	@echo "Building crypto library statically for all platforms..."
	@rustup target add x86_64-unknown-linux-musl || true
	@cd bandersnatch && \
	INSTALLED_TARGETS="$$(rustup target list --installed 2>/dev/null || true)"; \
	for TARGET in x86_64-unknown-linux-musl aarch64-unknown-linux-musl x86_64-apple-darwin aarch64-apple-darwin x86_64-pc-windows-gnu; do \
		if echo "$$INSTALLED_TARGETS" | grep -qx "$$TARGET"; then \
			echo "  Building for $$TARGET..."; \
			RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --target=$$TARGET --features " " || \
				echo "  Build failed for $$TARGET; continuing."; \
		else \
			echo "  Skipping $$TARGET (target not installed)."; \
		fi; \
	done
	@mkdir -p bandersnatch/target/release
	@cp bandersnatch/target/x86_64-unknown-linux-musl/release/libbandersnatch.a bandersnatch/target/release/libcrypto.linux_amd64.a || true
	@cp bandersnatch/target/aarch64-unknown-linux-musl/release/libbandersnatch.a bandersnatch/target/release/libcrypto.linux_arm64.a || true
	@cp bandersnatch/target/x86_64-apple-darwin/release/libbandersnatch.a bandersnatch/target/release/libcrypto.mac_amd64.a || true
	@cp bandersnatch/target/aarch64-apple-darwin/release/libbandersnatch.a bandersnatch/target/release/libcrypto.mac_arm64.a || true
	@cp bandersnatch/target/x86_64-pc-windows-gnu/release/libbandersnatch.a bandersnatch/target/release/libcrypto.windows_amd64.a || true
	@mkdir -p ffi
	@cp bandersnatch/target/release/libcrypto.*.a ffi/
	@echo "libcrypto.a files ready."

cargo_clean:
	@echo "Cleaning FFI libraries!"
	@cd bandersnatch && cargo clean
	@cd ..

ffi_force: cargo_clean ffi

ffi: cryptolib 



beauty:
	@echo "Running go fmt on all Go files..."
	@go fmt ./...

fmt-check:
	@echo "Checking formatting..."
	@diff -u <(echo -n) <(gofmt -d .)


# List of packages (missing node)
PACKAGES := $(shell go list ./... | grep -E "bandersnatch|bls|common|erasurecoding|pvm|statedb|trie|types")


clean:
	go clean -testcache


COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Default target
allcoverage: coveragetest coverage

# Run tests with coverage
coveragetest:
	@echo "Running tests with coverage..."
	@go test -coverprofile=$(COVERAGE_FILE) $(PACKAGES)

# Generate coverage HTML
coverage: coveragetest
	@echo "Generating HTML coverage report..."
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

# Clean up
cleancoverage:
	@echo "Cleaning up..."
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "Done."


polkavmscp:
	scp polkavm:/root/polkavm/services/bootstrap/bootstrap.pvm services
	scp polkavm:/root/polkavm/services/megatron/megatron.pvm services
	scp polkavm:/root/polkavm/services/fib/fib.pvm services
	scp polkavm:/root/polkavm/services/tribonacci/tribonacci.pvm services
	scp polkavm:/root/polkavm/services/corevm/corevm.pvm services

jamx_start:
	ansible-playbook -u root -i $(HOSTS_FILE)  yaml/jam_start.yaml
	@echo "update jam binary and start on jam instances"

jamx_stop:
	ansible-playbook -u root -i $(HOSTS_FILE)  yaml/jam_stop.yaml
	@echo "stop on jam instances"

# ----------------------------------------
# Release: build all binaries and package
PLATFORMS := linux-amd64 linux-arm64 mac-amd64 mac-arm64
BIN_DIR    := bin
RELEASE_DIR:= release

jam_tar:
	@echo "Packaging binaries for commit $(GIT_FULL_COMMIT)..."
	@mkdir -p $(RELEASE_DIR)/$(GIT_COMMIT)
	@for plat in $(PLATFORMS); do \
	  echo "  → $$plat"; \
	  mkdir -p tmp/$$plat && cp $(BIN_DIR)/$$plat/$(BINARY) tmp/$$plat/; \
	  tar czf $(RELEASE_DIR)/$(GIT_COMMIT)/$(BINARY)_$(GIT_FULL_COMMIT)_$$plat.tgz \
	    -C tmp $$plat; \
	  rm -rf tmp/$$plat; \
	done
	@rmdir tmp || true
	@echo "Binaries packaged in $(RELEASE_DIR)/$(GIT_COMMIT)/"
	@echo "To create a GitHub release, run:"
	@echo "gh release create \"$(GIT_TAG)\" $(RELEASE_DIR)/$(GIT_COMMIT)/*.tgz --title \"Release $(GIT_TAG)\" --notes \"Release $(GIT_TAG) - commit $(GIT_FULL_COMMIT)\""

release: static_jam_all jam_tar
# ----------------------------------------

# JAM Service Tests
evm_jamtest: jam
	@echo "Running EVM JAM tests..."
	go test -tags=network_test ./node -run TestEVM -v

algo_jamtest: jam
	@echo "Running Algo JAM tests..."
	go test -tags=network_test ./node -run TestAlgo -v

safrole_jamtest: jam
	@echo "Running Safrole JAM tests..."
	go test -tags=network_test ./node -run TestSafrole -v

fallback_jamtest: jam
	@echo "Running Fallback JAM tests..."
	go test -tags=network_test ./node -run TestFallback -v

compare_stf:
	@echo "Building compare_stf tool..."
	go build -o bin/compare_stf scripts/compare_stf.go

.PHONY: compare_stf

# ----------------------------------------
# Telemetry targets
# ----------------------------------------

# Build telemetry viewer
telemetry_viewer:
	@echo "Building telemetry viewer..."
	@go build -o $(OUTPUT_DIR)/telemetryViewer ./cmd/telemetryViewer

# Start telemetry server (headless, logs only)
telemetry:
	@echo "Starting telemetry server on port $(TELEMETRY_PORT)..."
	@mkdir -p logs
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) telemetry --addr 0.0.0.0:$(TELEMETRY_PORT) --log $(TELEMETRY_LOG)

# Start telemetry viewer with web UI
start_telemetry: telemetry_viewer
	@echo "Starting telemetry viewer..."
	@mkdir -p logs
	@ps aux | grep 'bin/.*[t]elemetryViewer' | awk '{print $$2}' | while read pid; do kill $$pid 2>/dev/null || true; done
	@$(OUTPUT_DIR)/telemetryViewer -telemetry 0.0.0.0:$(TELEMETRY_PORT) -web 0.0.0.0:$(TELEMETRY_WEB_PORT) -log $(TELEMETRY_LOG) >logs/telemetry-viewer.log 2>&1 &
	@sleep 1
	@echo "Telemetry viewer started: http://localhost:$(TELEMETRY_WEB_PORT) | http://polkavm.jamduna.org:$(TELEMETRY_WEB_PORT)"

# Kill telemetry processes
kill_telemetry:
	@echo "Killing telemetry processes..."
	@ps aux | grep 'bin/.*[t]elemetryViewer' | awk '{print $$2}' | while read pid; do kill $$pid 2>/dev/null || true; done
	@ps aux | grep 'bin/.*jamduna.*[t]elemetry' | awk '{print $$2}' | while read pid; do kill $$pid 2>/dev/null || true; done
	@echo "Telemetry processes killed."

# Restart telemetry viewer (kills and restarts)
restart_telemetry: kill_telemetry start_telemetry
	@echo "Telemetry restarted: http://localhost:$(TELEMETRY_WEB_PORT)"
