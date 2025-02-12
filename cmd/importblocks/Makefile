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
JAMDIR           ?= /tmp/ntust/jam
MODE             ?= generic
DATADIR		  	 ?= data
NETWORK          ?= tiny
LATEST_TS        := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS               ?= $(LATEST_TS)

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz

all: build

build:
	go build -tags=$(NETWORK) -o $(BINARY_NAME) .

importblocks: build
	./$(BINARY_NAME)

run:
	go build -tags=$(NETWORK) -o $(BINARY_NAME_FUZZ) ../fuzzer
	./$(BINARY_NAME_FUZZ)

cpnode:
ifndef TS
	@echo "ERROR: 'TS' is not set and no valid folder found in '$(JAMDIR)'." && \
	echo "Please ensure the directory exists or set TS=<timestamp> manually." && exit 1
endif
	mkdir -p ${DATADIR}/$(MODE)/state_transitions ${DATADIR}/$(MODE)/blocks ${DATADIR}/$(MODE)/state_snapshots
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" -exec cp {} "${DATADIR}/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" -exec cp {} "${DATADIR}/$(MODE)/blocks/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" -exec cp {} "${DATADIR}/$(MODE)/state_snapshots/" \;

cpstf:
	mkdir -p ${DATADIR}/$(MODE)/state_transitions ${DATADIR}/$(MODE)/blocks ${DATADIR}/$(MODE)/state_snapshots
	find "$(JAMDIR)/state_transitions" -type f -exec cp {} "${DATADIR}/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks" -type f -exec cp {} "${DATADIR}/$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots" -type f -exec cp {} "${DATADIR}/$(MODE)/state_snapshots/" \;

fuzz:
	(cd ../../fuzz/ && go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestFuzz$$)
	./fuzzed_zipper.sh

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ)