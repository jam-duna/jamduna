TIMEOUT          := 10m
BINARY_NAME      := duna_fuzzer
BINARY_NAME_FUZZ := fuzzer
JAMDIR           ?= /tmp/ntust/jam
MODE             ?= generic
DATADIR          ?= rawdata
FUZZDIR          ?= fuzzed
NETWORK          ?= tiny
LATEST_TS        := $(shell ls -1 "$(JAMDIR)" 2>/dev/null | sort -n | tail -1)
TS               ?= $(LATEST_TS)
JOBID            ?=
DEFAULT_ASN_VERSION := 1

# Use the active SDK for header/lib paths on macOS builds.
SDK_PATH         := $(shell xcrun --show-sdk-path)

.PHONY: all build run test-fallback test-safrole test-assurances clean cpnode cpstf fuzz fuzz_only make asn0 asn1 asn0r

all: build

# ASN.0 builds (legacy)
asn0: static_fuzzer0_linux static_fuzzer0_mac

# Default ASN version build (follows DEFAULT_ASN_VERSION)
make: asn$(DEFAULT_ASN_VERSION)

# ASN.1 builds (spec-compliant) - builds both backend variants
asn1: static_fuzzer1_linux static_fuzzer1_interpreter_linux static_fuzzer1_mac

# ASN.0r builds (extended with refinement)
asn0r: static_fuzzer0r_linux static_fuzzer0r_mac

dynamic_build:
	@echo "Building dynamic duna_fuzzer for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK),asn$(DEFAULT_ASN_VERSION)" -o $(BINARY_NAME)_dynamic .

build: static_fuzzer$(DEFAULT_ASN_VERSION)_linux static_fuzzer$(DEFAULT_ASN_VERSION)_interpreter_linux static_fuzzer$(DEFAULT_ASN_VERSION)_mac

fuzzer: build
	./$(BINARY_NAME)

static_fuzzer_linux:
	@echo "Building static duna_fuzzer for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for duna_fuzzer_linux on Linux (amd64)."

static_fuzzer_mac:
	@echo "Building static duna_fuzzer for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for duna_fuzzer_mac on macOS (amd64)."

static_fuzzer1_linux:
	@echo "Building static duna_fuzzer (ASN.1, Compiler - default) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=compiler'" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) on Linux (amd64) with Compiler backend."

static_fuzzer1_interpreter_linux:
	@echo "Building static duna_fuzzer (ASN.1, Interpreter) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=interpreter'" \
	-o $(BINARY_NAME)_linux_interpreter .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(BINARY_NAME)_linux_interpreter on Linux (amd64)."

static_fuzzer1_mac:
	@echo "Building static duna_fuzzer (ASN.1) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac) on macOS (amd64)."

static_fuzzer0r_linux:
	@echo "Building static duna_fuzzer (ASN.0r) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_asn0r_linux .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(BINARY_NAME)_asn0r_linux on Linux (amd64)."

static_fuzzer0r_mac:
	@echo "Building static duna_fuzzer (ASN.0r) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_asn0r_mac .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(BINARY_NAME)_asn0r_mac on macOS (amd64)."

static_fuzzer0_linux:
	@echo "Building static duna_fuzzer (ASN.0) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn0_linux) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn0_linux) on Linux (amd64)."

static_fuzzer0_mac:
	@echo "Building static duna_fuzzer (ASN.0) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0" \
	-ldflags="-s -w" \
	-o $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn0_mac) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete for $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn0_mac) on macOS (amd64)."

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
	rm -f $(BINARY_NAME) $(BINARY_NAME_FUZZ) $(BINARY_NAME)_linux $(BINARY_NAME)_mac $(BINARY_NAME)_dynamic \
	      $(BINARY_NAME)_asn1_linux $(BINARY_NAME)_asn1_mac \
	      $(BINARY_NAME)_asn0r_linux $(BINARY_NAME)_asn0r_mac \
	      $(BINARY_NAME)_linux_interpreter

