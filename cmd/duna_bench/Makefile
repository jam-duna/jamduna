TIMEOUT          := 10m
BINARY_NAME      := duna_bench
NETWORK          ?= tiny
SDK_PATH         := $(shell xcrun --show-sdk-path)
DEFAULT_ASN_VERSION := 1

build: static_bench$(DEFAULT_ASN_VERSION)_linux static_bench$(DEFAULT_ASN_VERSION)_mac

# Default ASN version build (follows DEFAULT_ASN_VERSION)
make: asn$(DEFAULT_ASN_VERSION)

# ASN.1 builds (spec-compliant)
asn1: static_bench1_linux static_bench1_mac

# ASN.0r builds (extended with refinement)
asn0r: static_bench0r_linux static_bench0r_mac

# ASN.0 builds (legacy)
asn0: static_bench0_linux static_bench0_mac

dynamic_build:
	@echo "Building dynamic $(BINARY_NAME) for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK),asn$(DEFAULT_ASN_VERSION)" -o $(BINARY_NAME)_dynamic .

static_bench_linux:
	@echo "Building static $(BINARY_NAME) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.linux_amd64.a ../../ffi/libpvm.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(BINARY_NAME)_linux"

static_bench_mac:
	@echo "Building static $(BINARY_NAME) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.mac_amd64.a ../../ffi/libpvm.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(BINARY_NAME)_mac"

static_bench1_linux:
	@echo "Building static $(BINARY_NAME) (ASN.1) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.linux_amd64.a ../../ffi/libpvm.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux)"

static_bench1_mac:
	@echo "Building static $(BINARY_NAME) (ASN.1) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.mac_amd64.a ../../ffi/libpvm.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac) .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac)"

static_bench0r_linux:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.linux_amd64.a ../../ffi/libpvm.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_asn0r_linux .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(BINARY_NAME)_asn0r_linux"

static_bench0r_mac:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.mac_amd64.a ../../ffi/libpvm.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_asn0r_mac .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(BINARY_NAME)_asn0r_mac"

static_bench0_linux:
	@echo "Building static $(BINARY_NAME) (ASN.0) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.linux_amd64.a ../../ffi/libpvm.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn0_linux) .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn0_linux)"

static_bench0_mac:
	@echo "Building static $(BINARY_NAME) (ASN.0) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libpvm.mac_amd64.a ../../ffi/libpvm.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0" \
	-ldflags="-s -w" \
	-o $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn0_mac) .
	@rm -f ../../ffi/libpvm.a
	@echo "Build complete: $(if $(filter 0,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn0_mac)"


clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)_linux $(BINARY_NAME)_mac $(BINARY_NAME)_dynamic \
	      $(BINARY_NAME)_asn1_linux $(BINARY_NAME)_asn1_mac \
	      $(BINARY_NAME)_asn0r_linux $(BINARY_NAME)_asn0r_mac