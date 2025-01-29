# Build, test and manage JAM data
# Configure with:
#   JAMDIR  - Root path for jam data (default: /tmp/root/jam)
#   TS      - Numeric subfolder under JAMDIR (auto-detected if not set)
#   MODE    - Folder to copy data to (default: 'generic')
#   NETWORK - Build tag for the fuzzer (default: 'tiny')
#   TIMEOUT - Timeout for test and fuzzing (default: 1m)

TIMEOUT          := 1m
BINARY_NAME      := importblocks
BINARY_NAME_FUZZ := fuzzer
JAMDIR           ?= /tmp/root/jam
MODE             ?= generic
NETWORK          ?= tiny
LATEST_TS        := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS               ?= $(LATEST_TS)

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz

all: build

build:
	go build -tags=$(NETWORK) -o $(BINARY_NAME) .

run:
	go build -tags=$(NETWORK) -o $(BINARY_NAME_FUZZ) ../fuzzer
	./$(BINARY_NAME_FUZZ)

test-fallback:
	go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestFallback$$

test-safrole:
	go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestSafrole$$

test-assurances:
	go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestAssurances$$

cpnode:
ifndef TS
	@echo "ERROR: 'TS' is not set and no valid folder found in '$(JAMDIR)'." && \
	echo "Please ensure the directory exists or set TS=<timestamp> manually." && exit 1
endif
	mkdir -p $(MODE)/state_transitions $(MODE)/blocks $(MODE)/state_snapshots
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" -exec cp {} "$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" -exec cp {} "$(MODE)/blocks/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" -exec cp {} "$(MODE)/state_snapshots/" \;

cpstf:
	mkdir -p $(MODE)/state_transitions $(MODE)/blocks $(MODE)/state_snapshots
	find "$(JAMDIR)/state_transitions" -type f -exec cp {} "$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks" -type f -exec cp {} "$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots" -type f -exec cp {} "$(MODE)/state_snapshots/" \;

fuzz:
	go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestFuzz$$

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ)