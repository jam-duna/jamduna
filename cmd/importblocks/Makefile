###############################################################################
# Makefile for Building, Testing, and Managing JAM Data
#
# Usage:
#   make                # Builds the 'importblocks' binary
#   make run            # Builds and runs the fuzzer binary
#   make test-*         # Runs specific tests (fallback, safrole, assurances)
#   make cpnode         # Copies node-generated data based on JAMDIR/TS/MODE
#   make cpstf          # Copies STF data directly without TS structure
#   make fuzz           # Runs fuzz tests
#
# Variables:
#   JAMDIR - Root path for jam data (default: /tmp/root/jam)
#   TS     - Numeric subfolder under JAMDIR (default: highest available)
#   MODE   - Folder to copy data to (default: 'generic')
#   TIMEOUT - Timeout for test and fuzzing (default: 1m)
#
###############################################################################

###############################################################################
# Variables
###############################################################################
TIMEOUT           := 1m
BINARY_NAME       := importblocks
BINARY_NAME_FUZZ  := fuzzer

# User-configurable variables with defaults
JAMDIR            ?= /tmp/root/jam
MODE              ?= generic

# Automatically detect the latest timestamp folder if not provided
LATEST_TS         := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS                ?= $(LATEST_TS)

###############################################################################
# Phony Targets
###############################################################################
.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz

###############################################################################
# Default Target
###############################################################################
all: build

###############################################################################
# Build Targets
###############################################################################
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .

run:
	@echo "Building and running $(BINARY_NAME_FUZZ)..."
	go build -o $(BINARY_NAME_FUZZ) ../fuzzer
	./$(BINARY_NAME_FUZZ)

###############################################################################
# Test Targets
###############################################################################
test-fallback:
	@echo "Running fallback tests with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFallback$$

test-safrole:
	@echo "Running safrole tests with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$$

test-assurances:
	@echo "Running assurances tests with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestAssurances$$

###############################################################################
# Data Management Targets
###############################################################################
cpnode:
ifndef TS
	@echo "ERROR: 'TS' is not set and no valid folder found in '$(JAMDIR)'."
	@echo "Please ensure the directory exists or set TS=<timestamp> manually."
	@exit 1
endif
	@echo "Removing old files in '$(MODE)'..."
	rm -rf $(MODE)/state_transitions/* $(MODE)/blocks/* $(MODE)/state_snapshots/*
	@echo "Copying data from $(JAMDIR)/$(TS) to mode folder '$(MODE)'..."
	mkdir -p $(MODE)/state_transitions $(MODE)/blocks $(MODE)/state_snapshots
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" -exec cp {} "$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" -exec cp {} "$(MODE)/blocks/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" -exec cp {} "$(MODE)/state_snapshots/" \;

cpstf:
	@echo "Removing old files in '$(MODE)'..."
	rm -rf $(MODE)/state_transitions/* $(MODE)/blocks/* $(MODE)/state_snapshots/*
	@echo "Copying STF data from '$(JAMDIR)' to mode folder '$(MODE)'..."
	mkdir -p $(MODE)/state_transitions $(MODE)/blocks $(MODE)/state_snapshots
	find "$(JAMDIR)/state_transitions" -type f -exec cp {} "$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks" -type f -exec cp {} "$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots" -type f -exec cp {} "$(MODE)/state_snapshots/" \;
	
###############################################################################
# Fuzz Testing
###############################################################################
fuzz:
	@echo "Running fuzz tests with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFuzz$$

###############################################################################
# Clean Target
###############################################################################
clean:
	@echo "Cleaning up build artifacts..."
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ)