###############################################################################
# Makefile for Building and Managing JAM Data
#
# Common usage:
#   make                # Builds 'importblocks'
#   make test-fallback  # Tests fallback mode
#   make test-safrole   # Tests safrole mode
#   make test-assurances# Tests assurances mode
#   make cpnode     	# Copy node-generated data from the default or highest TS in $(JAMDIR)
#	make cpstf     		# Copy stfdump data from the default or highest TS in $(JAMDIR)
#
# Variables:
#   JAMDIR - Root path for jam data (default /tmp/root/jam).
#            This directory typically contains numeric subfolders, e.g. 5505186.
#            Each numeric folder may contain node0/node1 etc.
#   TS     - Specific numeric subfolder to use. If not set, the Makefile
#            automatically detects the highest numbered folder in $(JAMDIR).
#   MODE   - The mode folder to which data is copied (default "generic").
#
###############################################################################

###############################################################################
# Variables
###############################################################################
TIMEOUT       := 1m
BINARY_NAME   := importblocks

# Allow user to override the base JAM directory, default to /tmp/root/jam
JAMDIR        ?= /tmp/root/jam

# Automatically detect the highest numeric directory under $(JAMDIR) if none provided
LATEST_TS     := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS            ?= $(LATEST_TS)

# If MODE is not defined by user, default to 'generic'
MODE          ?= generic

###############################################################################
# Phony Targets (not tied to actual file names)
###############################################################################
.PHONY: all build test-fallback test-safrole test-assurances clean cpmodedata cpstf fuzz

###############################################################################
# Default Target
###############################################################################
all: build

###############################################################################
# Build
###############################################################################
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .

###############################################################################
# Test Targets
###############################################################################
test-fallback:
	@echo "Running TestFallback with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFallback$$

test-safrole:
	@echo "Running TestSafrole with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestSafrole$$

test-assurances:
	@echo "Running TestAssurances with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestAssurances$$

###############################################################################
# Copy Node Data with Mode (cpnode)
#
# Copies node data from an existing jam directory structure.
# The path is built as $(JAMDIR)/$(TS)/node*/data/.
# 
# Examples:
#   make cpnode
#   make cpnode JAMDIR=/tmp/root/jam TS=5505186 MODE=safrole
# 
# By default:
#   JAMDIR=/tmp/root/jam
#   TS= <largest numeric folder under JAMDIR>
#   MODE=assurances
###############################################################################
cpnode:
ifndef TS
	@echo "WARNING: 'TS' is empty, and no numeric directory found under '$(JAMDIR)'."
	@echo "Please ensure that folder is valid or manually provide TS=<folder>."
	@exit 1
endif
	@echo "Copying data from $(JAMDIR)/$(TS)/ for MODE='$(MODE)'..."
	mkdir -p $(MODE)/state_transitions/
	mkdir -p $(MODE)/blocks/
	mkdir -p $(MODE)/state_snapshots/

	# Copy all files under any node*/data/state_transitions/
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" -exec cp {} "$(MODE)/state_transitions/" \;

	# Copy all files under any node*/data/blocks/
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" -exec cp {} "$(MODE)/blocks/" \;

	# Copy all files under any node*/data/state_snapshots/
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" -exec cp {} "$(MODE)/state_snapshots/" \;

###############################################################################
# Copy STF Data with mode (cpstf)
#
# Copies the state_transitions/, blocks/, and state_snapshots/ subfolders from
# a custom jam directory that does NOT have the numeric TS structure. 
#
# Example:
#   make cpstf JAMDIR=/path/to/my_dump MODE=safrole
###############################################################################
cpstf:
	@echo "Copying STF data from '$(JAMDIR)' to mode='$(MODE)'..."
	mkdir -p $(MODE)/state_transitions/
	mkdir -p $(MODE)/blocks/
	mkdir -p $(MODE)/state_snapshots/
	find "$(JAMDIR)/state_transitions/" -type f -exec cp {} "$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks/" -type f -exec cp {} "$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots/" -type f -exec cp {} "$(MODE)/state_snapshots/" \;

###############################################################################
# Fuzz Testing
#
# Runs 'TestMKFuzz' with the specified TIMEOUT (default 1m).
###############################################################################
fuzz:
	@echo "Running Fuzzing (TestFuzz) with timeout $(TIMEOUT)..."
	go test -timeout $(TIMEOUT) -v -run ^TestFuzz$$