TIMEOUT          := 10m
BINARY_NAME      := duna_fuzzer
BINARY_NAME_FUZZ := fuzzer
JAMDIR           ?= /tmp/ntust/jam
MODE             ?= generic
DATADIR		  	 ?= rawdata
FUZZDIR			 ?= fuzzed
NETWORK          ?= tiny
LATEST_TS        := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS               ?= $(LATEST_TS)
JOBID            ?= 

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz fuzz_only

all: build

dynamic_build:
	@echo "Building dynamic duna_fuzzer for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK)" -o $(BINARY_NAME)_dynamic .

build: static_fuzzer_linux static_fuzzer_mac

fuzzer: build
	./$(BINARY_NAME)

static_fuzzer_linux:
	@echo "Building static duna_fuzzer for Linux (amd64)..."
	@ln -s $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm ../../ffi/libunicorn.a
	@echo "Build complete for duna_fuzzer_linux on Linux (amd64)."


static_fuzzer_mac:
	@echo "Building static duna_fuzzer for macOS (amd64)..."
	@ln -s $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@rm ../../ffi/libunicorn.a
	@echo "Build complete for duna_fuzzer_mac on macOS (amd64)."

run:
	go build -tags=$(NETWORK) -o $(BINARY_NAME_FUZZ) ../fuzzer
	./$(BINARY_NAME_FUZZ)
cpnode:
ifeq ($(JOBID),)
	@echo "Detecting latest timestamp directory in $(JAMDIR)..."
	$(eval TS=$(shell ls -1 "$(JAMDIR)" 2>/dev/null | grep '_' | sort -n | tail -1))
	$(eval JOBID=$(shell echo $(TS) | cut -d'_' -f2))
else
	@echo "JOBID specified: $(JOBID), filtering directory in $(JAMDIR)..."
	$(eval TS=$(shell ls -1 "$(JAMDIR)" 2>/dev/null | grep '_$(JOBID)' | sort -n | tail -1))
endif

	@echo "Using directory: $(TS) with jobID: $(JOBID)"
	@echo "Removing old data from $(DATADIR)/$(MODE)/..."
	rm -rf "$(DATADIR)/$(MODE)/state_transitions" "$(DATADIR)/$(MODE)/blocks" "$(DATADIR)/$(MODE)/state_snapshots"
	rm -f "$(DATADIR)/$(MODE)"/*.log

	mkdir -p "$(DATADIR)/$(MODE)/state_transitions" \
	         "$(DATADIR)/$(MODE)/blocks" \
	         "$(DATADIR)/$(MODE)/state_snapshots"

	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_transitions/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/blocks/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/blocks/" \;
	find "$(JAMDIR)/$(TS)" -type f -path "*/data/state_snapshots/*" \
	     -exec cp {} "$(DATADIR)/$(MODE)/state_snapshots/" \;

	@echo "Copying log file from $(JAMDIR)/$(TS)/$(JOBID).log to $(DATADIR)/$(MODE)/"
	cp "$(JAMDIR)/$(TS)/$(JOBID).log" "$(DATADIR)/$(MODE)/"

cpstf:
	mkdir -p ${DATADIR}/$(MODE)/state_transitions ${DATADIR}/$(MODE)/blocks ${DATADIR}/$(MODE)/state_snapshots
	find "$(JAMDIR)/state_transitions" -type f -exec cp {} "${DATADIR}/$(MODE)/state_transitions/" \;
	find "$(JAMDIR)/blocks" -type f -exec cp {} "${DATADIR}/$(MODE)/blocks/" \;
	find "$(JAMDIR)/state_snapshots" -type f -exec cp {} "${DATADIR}/$(MODE)/state_snapshots/" \;

fuzz_only:
	rm -rf "$(FUZZDIR)"
	mkdir -p "$(FUZZDIR)"
	(cd ../../fuzz/ && go test -timeout $(TIMEOUT) -tags=$(NETWORK) -v -run ^TestFuzz$$)

fuzz: fuzz_only
	./fuzzed_zipper.sh

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ)
