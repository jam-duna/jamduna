OUTPUT_DIR := bin
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)
SRC := jam.go
net-spec_FILE ?= tiny
#net-spec_FILE ?= chainspecs/tiny-00000000.json
CHAINSPEC ?= jamduna
NUM_NODES ?= 6
DEFAULT_PORT ?= 40000
SINGLE_NODE_PORT ?= 40005
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
JAM_start-time ?= $(shell \
	if date --version >/dev/null 2>&1; then \
		date -d "5 seconds" "+%Y-%m-%d %H:%M:%S"; \
	else \
		date -v+5S "+%Y-%m-%d %H:%M:%S"; \
	fi)
RAW_HOSTS_FILE ?= hosts.txt
HOSTS_FILE := ../$(RAW_HOSTS_FILE)

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

GIT_TAG := $(shell git describe --tags --abbrev=0)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Linker flags to strip symbols and embed version info
GO_LDFLAGS := -s -w \
  -X 'main.Version=$(GIT_TAG)' \
  -X 'main.Commit=$(GIT_COMMIT)' \
  -X 'main.BuildTime=$(BUILD_TIME)'

ifeq ($(UNAME_S),Linux)
  ifeq ($(UNAME_M),x86_64)
    BINARY := jamduna-linux-amd64
  else ifeq ($(UNAME_M),aarch64)
    BINARY := jamduna-linux-arm64
  endif
else ifeq ($(UNAME_S),Darwin)
  ifeq ($(UNAME_M),x86_64)
    BINARY := jamduna-mac-amd64
  else ifeq ($(UNAME_M),arm64)
    BINARY := jamduna-mac-arm64
  endif
endif


.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean jam_without_ffi_build run_parallel_jam kill_parallel_jam run_jam build_remote_nodes run_jam_remote_nodes da jamweb validatetraces testnet

jam_with_ffi_build: ffi_force
	@echo "Building JAM...  "
	mkdir -p $(OUTPUT_DIR)
	go build  -tags="cgo" -o $(OUTPUT_DIR)/$(BINARY) .
jam:
	@echo "Building JAM...  "
	mkdir -p $(OUTPUT_DIR)
	go build -tags=  -o $(OUTPUT_DIR)/$(BINARY) .

# ANSI color codes
GREEN=\033[0;32m
YELLOW=\033[1;33m
RESET=\033[0m

define build_with_status
	@echo "Building $(1)..."
	@$(2) && echo "$(GREEN)✓ Done: $(1)$(RESET)" || echo "$(YELLOW)⚠ Failed to build: $(1)$(RESET)"
endef

static_jam_linux_amd64:
	@echo "Building JamDuna binary for Linux (x86_64)..."
	$(call build_with_status,static_jam_linux_amd64,\
	GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS) -extldflags '-static'" \
	-o $(OUTPUT_DIR)/jamduna-linux-amd64 . && strip $(OUTPUT_DIR)/jamduna-linux-amd64 2>/dev/null)

static_jam_linux_arm64:
	@echo "Building JamDuna binary for Linux (aarch64)..."
	$(call build_with_status,static_jam_linux_arm64,\
	GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS) -extldflags '-static'" \
	-o $(OUTPUT_DIR)/jamduna-linux-arm64 . && strip $(OUTPUT_DIR)/jamduna-linux-arm64 2>/dev/null)

static_jam_darwin_amd64:
	@echo "Building JamDuna binary for macOS (x86_64)..."
	$(call build_with_status,static_jam_darwin_amd64,\
	GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS)" \
	-o $(OUTPUT_DIR)/jamduna-mac-amd64 . && strip -x $(OUTPUT_DIR)/jamduna-mac-amd64)

static_jam_darwin_arm64:
	@echo "Building JamDuna binary for macOS (aarch64)..."
	$(call build_with_status,static_jam_darwin_arm64,\
	CGO_ENABLED=1 CC=clang \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS)" \
	-o $(OUTPUT_DIR)/jamduna-mac-arm64 . && strip -x $(OUTPUT_DIR)/jamduna-mac-arm64)

static_jam_windows_amd64:
	@echo "Building JamDuna binary for Windows AMD64..."
	$(call build_with_status,static_jam_windows_amd64,\
	GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 \
	go build -tags "cgo" \
	-ldflags "$(GO_LDFLAGS)" \
	-o $(OUTPUT_DIR)/jamduna-windows-amd64.exe . && x86_64-w64-mingw32-strip $(OUTPUT_DIR)/jamduna-windows-amd64.exe)

static_jam_all:
	@echo "Building static JAM for available platforms..."

	# Always build Linux x86_64
	@$(MAKE) static_jam_linux_amd64

	# Build Linux ARM64 only if compiler exists or on ARM host
	@if command -v aarch64-linux-musl-gcc >/dev/null 2>&1; then \
	  $(MAKE) static_jam_linux_arm64; \
	else \
	  echo "⚠ Skipping Linux ARM64 (no aarch64-linux-musl-gcc)"; \
	fi

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
	@echo "Cleaning all jam data directories under ~/.jam..."
	@rm -rf ${HOME}/.jamduna/jam-*
	@echo "Done."

run_parallel_jam:
	@mkdir -p logs
	@echo "Starting $(NUM_NODES) instances of $(OUTPUT_DIR)/$(BINARY) with start_time=$(JAM_start-time)..."
	@for i in $$(seq 0 $$(($(NUM_NODES) - 1))); do \
		PORT=$$(($(DEFAULT_PORT) + $$i)); \
		V_IDX=$$i; \
		echo ">> Starting instance $$V_IDX on port $$PORT..."; \
		$(OUTPUT_DIR)/$(BINARY) run \
			--chain $(CHAINSPEC) \
			--dev-validator $$V_IDX \
			--start-time "$(JAM_start-time)" & \
	done; \
	wait
	@echo "✅ All instances started and running in parallel."



run_localclient_jam: jam_clean run_parallel_jam
run_localclient_jam_dead: jam_clean run_parallel_jam_with_deadnode

run_single_node:jam_clean
	@echo "Starting single node JAM instance..."
	@echo "Starting $(OUTPUT_DIR)/$(BINARY)... with network $(NETWORK) port $(SINGLE_NODE_PORT) start-time $(JAM_start-time)"
	@$(OUTPUT_DIR)/$(BINARY) run --chain $(CHAINSPEC) --port $(SINGLE_NODE_PORT) --start-time "$(JAM_start-time)" --dev-validator 5
	@echo "Instance started."
run_parallel_jam_with_deadnode:
	@mkdir -p logs
	@echo "Starting $(NUM_NODES) instances of $(OUTPUT_DIR)/$(BINARY)..."
	@seq 0 $(shell echo $$(($(NUM_NODES) - 2))) | xargs -I{} -P $(NUM_NODES) sh -c 'PORT=$$(($(DEFAULT_PORT) + {})); $(OUTPUT_DIR)/$(BINARY) run  --chain $(CHAINSPEC) --dev-validator {}; echo "Instance {} finished with port $$PORT"' sh
	@echo "All instances started."
kill_parallel_jam:
	@echo "Killing all instances of $(OUTPUT_DIR)/$(BINARY)..."
	@pgrep -f "$(OUTPUT_DIR)/$(BINARY)"
	@pkill -f "$(OUTPUT_DIR)/$(BINARY)"
	@echo "All instances killed."
run_jam:
	@echo "Starting $(OUTPUT_DIR)/$(BINARY)... port $(DEFAULT_PORT) start-time $(JAM_start-time)"
	@$(OUTPUT_DIR)/$(BINARY) -net-spec $(net-spec_FILE) -port $(DEFAULT_PORT) -start-time "$(JAM_start-time)"
	@echo "Instance started."

# env setup for remote nodes
jam_set:
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && echo \"export CARGO_MANIFEST_DIR=\$(pwd)\" >> ~/.bashrc'"
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "source ~/.bashrc"
# build ffi and apply latest code
build_remote_nodes:
	@echo "Building JAM on all remote nodes..."
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && git fetch origin && git reset --hard origin/$(BRANCH) && git clean -fd'"
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && make bandersnatchlib'"
	@/usr/bin/parallel-ssh -h $(HOSTS_FILE) -l root -i "bash -i -c 'cdj && make blslib'"
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
blslib:
	@echo "Building BLS (static) for all platforms..."
	@cd bls && \
	for TARGET in x86_64-unknown-linux-musl aarch64-unknown-linux-musl x86_64-apple-darwin aarch64-apple-darwin x86_64-pc-windows-gnu; do \
		echo "  Building for $$TARGET..."; \
		RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --target=$$TARGET; \
	done
	@echo "Copying libbls.a artifacts to bls/target/release for Go linker..."
	@cp bls/target/x86_64-unknown-linux-musl/release/libbls.a bls/target/release/libbls.linux_amd64.a || true
	@cp bls/target/aarch64-unknown-linux-musl/release/libbls.a bls/target/release/libbls.linux_arm64.a || true
	@cp bls/target/x86_64-apple-darwin/release/libbls.a bls/target/release/libbls.mac_amd64.a || true
	@cp bls/target/aarch64-apple-darwin/release/libbls.a bls/target/release/libbls.mac_arm64.a || true
	@cp bls/target/x86_64-pc-windows-gnu/release/libbls.a bls/target/release/libbls.windows_amd64.a || true
	@mkdir -p ffi
	@cp bls/target/release/libbls.*.a ffi/
	@echo "All libbls.a versions prepared."

bandersnatchlib:
	@echo "Building Bandersnatch for   statically for all platforms..."
	@cd bandersnatch && \
	for TARGET in x86_64-unknown-linux-musl aarch64-unknown-linux-musl x86_64-apple-darwin aarch64-apple-darwin x86_64-pc-windows-gnu; do \
		echo "  Building for $$TARGET..."; \
		RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --target=$$TARGET --features " "; \
	done
	@mkdir -p bandersnatch/target/release
	@cp bandersnatch/target/x86_64-unknown-linux-musl/release/libbandersnatch.a bandersnatch/target/release/libbandersnatch.linux_amd64.a || true
	@cp bandersnatch/target/aarch64-unknown-linux-musl/release/libbandersnatch.a bandersnatch/target/release/libbandersnatch.linux_arm64.a || true
	@cp bandersnatch/target/x86_64-apple-darwin/release/libbandersnatch.a bandersnatch/target/release/libbandersnatch.mac_amd64.a || true
	@cp bandersnatch/target/aarch64-apple-darwin/release/libbandersnatch.a bandersnatch/target/release/libbandersnatch.mac_arm64.a || true
	@cp bandersnatch/target/x86_64-pc-windows-gnu/release/libbandersnatch.a bandersnatch/target/release/libbandersnatch.windows_amd64.a || true
	@mkdir -p ffi
	@cp bandersnatch/target/release/libbandersnatch.*.a ffi/
	@echo "All libbandersnatch.a versions prepared."

cargo_clean:
	@echo "Clean Up FFI libraries (BLS + Bandersnatch)!"
	@cd bandersnatch && cargo clean
	@cd ..
	@cd bls && cargo clean
	@cd ..

ffi_force: cargo_clean ffi

# Target to build both BLS and Bandersnatch FFI libraries
ffi: bandersnatchlib blslib
	@echo "Built all FFI libraries (BLS + Bandersnatch)!"

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
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/bootstrap/bootstrap.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/megatron/megatron.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/fib/fib.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/tribonacci/tribonacci.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/corevm/corevm.pvm services

jamx_start:
	ansible-playbook -u root -i $(HOSTS_FILE)  yaml/jam_start.yaml
	@echo "update jam binary and start on jam instances"

jamx_stop:
	ansible-playbook -u root -i $(HOSTS_FILE)  yaml/jam_stop.yaml
	@echo "stop on jam instances"
