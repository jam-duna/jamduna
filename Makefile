OUTPUT_DIR := bin
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
SRC := jam.go

NUM_NODES ?= 6
DEFAULT_PORT ?= 40000
SINGLE_NODE_PORT ?= 40005
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
  CHAINSPEC ?= chainspecs/$(ARCH)/polkajam-spec.json
#  CHAINSPEC ?= chainspecs/jamduna-spec.json
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

.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean jam_without_ffi_build run_parallel_jam kill_parallel_jam run_jam build_remote_nodes run_jam_remote_nodes da jamweb validatetraces testnet init-submodules update-submodules update-pvm-submodule update-services-submodule evm_jamtest algo_jamtest safrole_jamtest railgun


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

spin_localclient: jam kill_jam jam_clean spin_5 spin_0

spin_5:
	@rm -rf ${HOME}/.jamduna/jam-*
	@for i in 1 2 3 4 5; do \
		$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator $$i  --rpc-port=$$((19800 + $$i)) --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND)  >logs/jamduna-$$i.log 2>&1 & \
	done

spin_0:
	@for i in 0; do \
		RUST_LOG=polkavm=trace,jam_node=trace $(POLKAJAM_BIN) --chain ${CHAINSPEC} run --temp --dev-validator $$i --rpc-port=$$((19800 + $$i)) >logs/polkajam-$$i.log 2>&1 & \
	done

run_1:
	@rm -rf ${HOME}/.jamduna/jam-*
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --dev-validator 5 --rpc-port=19805 --chain ${CHAINSPEC} --pvm-backend $(PVM_BACKEND) --debug rotation,guarantees

run_5:
	@for i in 0 1 2 3 4; do \
		RUST_LOG=chain-core=debug,jam_node=trace $(POLKAJAM_BIN)  --chain ${CHAINSPEC} run --pvm-backend $(PVM_BACKEND) --temp --dev-validator $$i --rpc-port=$$((19800 + $$i)) >logs/polkajam-$$i.log 2>&1 & \
	done

run_6:
	@for i in 0 1 2 3 4 5; do \
		RUST_LOG=chain-core=debug,jam_node=trace $(POLKAJAM_BIN)  --chain ${CHAINSPEC} run --pvm-backend $(PVM_BACKEND) --temp --dev-validator $$i --rpc-port=$$((19800 + $$i)) >logs/polkajam-$$i.log 2>&1 & \
	done

jam:
	@echo "Building JAM...  "
	mkdir -p $(OUTPUT_DIR)
	go build -tags=  -o $(OUTPUT_DIR)/$(ARCH)/$(BINARY) .

duna_spec: jam
	@echo "Generating Duna chainspec..."
	./$(OUTPUT_DIR)/$(ARCH)/$(BINARY) gen-spec chainspecs/dev-config.json chainspecs/jamduna-spec.json

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
		$(POLKAJAM_BIN) --chain $(CHAINSPEC) --pvm-backend $(PVM_BACKEND) run  --temp  --dev-validator $$V_IDX --rpc-port=$$((19800 + $$i)) & \
	done; \

run_localclient: kill jam jam_clean
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_5 run_1
run_localclient_jam: kill duna_spec jam jam_clean
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_parallel_jam
run_localclient_jam_dead: kill jam jam_clean
	@echo "Using chainspec: $(CHAINSPEC)"
	@$(MAKE) run_parallel_jam_with_deadnode

run_single_node:jam_clean
	@echo "Starting single node JAM instance..."
	@echo "Starting $(OUTPUT_DIR)/$(ARCH)/$(BINARY)... with network $(NETWORK) port $(SINGLE_NODE_PORT) start-time $(JAM_START_TIME)"
	@$(OUTPUT_DIR)/$(ARCH)/$(BINARY) run --chain $(CHAINSPEC) --port $(SINGLE_NODE_PORT) --start-time "$(JAM_START_TIME)" --dev-validator 5
	@echo "Instance started."
run_parallel_jam_with_deadnode:
	@mkdir -p logs
	@echo "Starting $(NUM_NODES) instances of $(OUTPUT_DIR)/$(ARCH)/$(BINARY)..."
	@seq 0 $(shell echo $$(($(NUM_NODES) - 2))) | xargs -I{} -P $(NUM_NODES) sh -c 'PORT=$$(($(DEFAULT_PORT) + {})); $(OUTPUT_DIR)/$(ARCH)/$(BINARY) run  --chain $(CHAINSPEC) --dev-validator {}; echo "Instance {} finished with port $$PORT"' sh
	@echo "All instances started."
kill_parallel_jam:
	@echo "Killing all instances of $(OUTPUT_DIR)/$(BINARY)..."
	@pgrep -f "$(OUTPUT_DIR)/$(BINARY)"
	@pkill -f "$(OUTPUT_DIR)/$(BINARY)"
	@echo "All instances killed."

kill:
	@echo "Kill Jam Binaries(if any)..."
	@pkill jam || true
	sleep 1
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
	@rustup target add x86_64-unknown-linux-musl
	@cd bandersnatch && \
	for TARGET in x86_64-unknown-linux-musl aarch64-unknown-linux-musl x86_64-apple-darwin aarch64-apple-darwin x86_64-pc-windows-gnu; do \
		echo "  Building for $$TARGET..."; \
		RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --target=$$TARGET --features " "; \
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

# JAM Railgun Service
railgun:
	@echo "Building Railgun privacy service..."
	@echo "→ Checking Rust code..."
	@cd services/railgun && cargo check --lib
	@echo "✓ Railgun service code checked successfully"
	@echo ""
	@echo "🔫 Railgun service structure ready"
	@echo "   Based on RAILGUN.md specification with all security fixes:"
	@echo "   - Fee tally DELTA writes (prevents parallel conflicts)"
	@echo "   - Gas limit binding in withdraw proofs"
	@echo "   - 64-bit range constraints (prevents modulus wrap)"
	@echo "   - has_change boolean flag (no sentinel collision)"
	@echo "   - Encrypted payload enforcement"
	@echo "   - Tree capacity overflow protection"
	@echo ""
	@echo "   Entry points:"
	@echo "   - refine: ZK proof verification + state validation"
	@echo "   - accumulate: Apply writes + process deposits"
	@echo ""
	@echo "   Note: Full RISC-V compilation requires polkavm target configuration"

.PHONY: compare_stf railgun
