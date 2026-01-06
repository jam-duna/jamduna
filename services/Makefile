# Self-Contained JAM Services Makefile
# This Makefile can build all JAM services independently of the main repository

# Available services
SERVICES = evm auth_copy null_authorizer algo fib orchard

# Test services (use -m flag for main() entry point)
TEST_SERVICES = $(wildcard test_services/*/Cargo.toml)
TEST_SERVICE_NAMES = $(foreach svc,$(TEST_SERVICES),$(notdir $(patsubst %/,%,$(dir $(svc)))))

# Build configuration
TARGET_DIR = riscv64emac-unknown-none-polkavm/release
TARGET_JSON = $(CURDIR)/crates/polkavm-linker/riscv64emac-unknown-none-polkavm.json
CARGO_FLAGS = +nightly build -Z build-std=core,alloc --release

# Platform detection for cross-platform compatibility
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

.PHONY: all $(SERVICES) test-services clean list help check-toolchain

# Default target
all: $(SERVICES)

# Build all test services
test-services: check-toolchain
	@echo "Building test services with main() entry point..."
	@for test_svc in $(TEST_SERVICE_NAMES); do \
		echo "Building test service: $$test_svc"; \
		cd test_services/$$test_svc && cargo $(CARGO_FLAGS) --target $(TARGET_JSON); \
		cd ../..; \
		cargo run -p polkatool jam-service -m target/$(TARGET_DIR)/$$test_svc \
			-o test_services/$$test_svc/$$test_svc.pvm \
			-d test_services/$$test_svc/$${test_svc}_blob.pvm && \
		cargo run -p polkatool disassemble test_services/$$test_svc/$${test_svc}_blob.pvm \
			--show-raw-bytes > test_services/$$test_svc/$$test_svc.txt && \
		echo "✓ Test service $$test_svc built successfully!"; \
	done

# Build individual services
$(SERVICES): check-toolchain
	@echo "Building $@ service..."
	@cd $@ && cargo $(CARGO_FLAGS) --target $(TARGET_JSON) --target-dir ./target
	cargo run -p polkatool jam-service $$JAM_FLAGS $@/target/$(TARGET_DIR)/$@ -o $@/$@.pvm -d $@/$@_blob.pvm && \
	cargo run -p polkatool disassemble $@/$@_blob.pvm --show-raw-bytes > ./$@/$@.txt && \
	echo "Service $@ built successfully!"

# Pattern rule for building any service with _service suffix (e.g., make evm_service)
%_service: check-toolchain
	@$(MAKE) $(patsubst %_service,%,$@)

# Clean all build artifacts
clean:
	@echo "Cleaning all build artifacts..."
	@for service in $(SERVICES); do \
		echo "Cleaning $$service..."; \
		rm -rf $$service/target; \
		rm -f $$service/*.pvm; \
		rm -f $$service/*.txt; \
	done
	@for test_svc in $(TEST_SERVICE_NAMES); do \
		echo "Cleaning test service $$test_svc..."; \
		rm -rf test_services/$$test_svc/target; \
		rm -f test_services/$$test_svc/*.pvm; \
		rm -f test_services/$$test_svc/*.txt; \
	done
	@rm -rf target
	@echo "Cleanup complete!"

# List available services
list:
	@echo "Available JAM services:"
	@for service in $(SERVICES); do \
		echo "  - $$service"; \
	done
	@echo ""
	@echo "Test services (main() entry point):"
	@for test_svc in $(TEST_SERVICE_NAMES); do \
		echo "  - $$test_svc"; \
	done
	@echo ""
	@echo "Usage:"
	@echo "  make <service>      - Build specific service"
	@echo "  make all           - Build all production services"
	@echo "  make test-services - Build all test services"
	@echo "  make clean         - Clean all build artifacts"
	@echo "  make list          - Show this list"

# Help target
help: list

# Check that required toolchain is available
check-toolchain:
	@command -v cargo >/dev/null 2>&1 || { echo "Error: cargo not found. Please install Rust toolchain."; exit 1; }
	@cargo +nightly --version >/dev/null 2>&1 || { echo "Error: nightly Rust not found. Please install with: rustup install nightly"; exit 1; }
	@test -f $(TARGET_JSON) || { echo "Error: Target JSON not found at $(TARGET_JSON)"; exit 1; }

# Development targets
dev-evm: evm
dev-algo: algo
dev-bootstrap: bootstrap

# Quick build verification (builds EVM as a smoke test)
verify: evm
	@echo "Build verification successful! EVM service built correctly."

# Show build environment information
info:
	@echo "JAM Services Build Environment"
	@echo "=============================="
	@echo "Platform: $(UNAME_S) $(UNAME_M)"
	@echo "Target JSON: $(TARGET_JSON)"
	@echo "Available services: $(SERVICES)"
	@echo "Cargo version:"
	@cargo --version 2>/dev/null || echo "  Not found"
	@echo "Nightly Rust:"
	@cargo +nightly --version 2>/dev/null || echo "  Not found"
	@echo "Workspace status:"
	@cargo check --workspace >/dev/null 2>&1 && echo "  OK" || echo "  Issues detected"

# Advanced: Build all services in parallel (use with caution)
parallel:
	@echo "Building all services in parallel..."
	@$(MAKE) -j$(shell nproc 2>/dev/null || echo 4) $(SERVICES)

# Build specific groups of services
core-services: bootstrap evm
auth-services: auth_copy null_authorizer
compute-services: algo doom

# Installation helpers (for CI/CD)
ci-build: check-toolchain all
	@echo "CI build completed successfully"

ci-test: ci-build
	@echo "Verifying all service artifacts exist..."
	@for service in $(SERVICES); do \
		test -f $$service/$$service.pvm || { echo "Missing $$service.pvm"; exit 1; }; \
		test -f $$service/$${service}_blob.pvm || { echo "Missing $${service}_blob.pvm"; exit 1; }; \
		test -f $$service/$$service.txt || { echo "Missing $$service.txt"; exit 1; }; \
		echo "✓ $$service artifacts verified"; \
	done
	@echo "All service artifacts verified successfully!"

# Debug: Show build commands without executing
dry-run:
	@echo "Dry run - showing build commands for EVM service:"
	@echo "1. cd evm && cargo $(CARGO_FLAGS) --target $(TARGET_JSON) --target-dir ./target"
	@echo "2. cargo run -p polkatool jam-service evm/target/$(TARGET_DIR)/evm -o evm/evm.pvm -d evm/evm_blob.pvm"
	@echo "3. cargo run -p polkatool disassemble evm/evm_blob.pvm --show-raw-bytes > ./evm/evm.txt"
