BINARY_NAME      := duna_replay
NETWORK          ?= tiny
DEFAULT_ASN_VERSION := 1

# Use the active SDK for header/lib paths on macOS builds.
SDK_PATH         := $(shell xcrun --show-sdk-path)

.PHONY: all build asn1 asn0r clean

all: build

# Default ASN version build (follows DEFAULT_ASN_VERSION)
build: static_replay$(DEFAULT_ASN_VERSION)_linux static_replay$(DEFAULT_ASN_VERSION)_compiler_linux static_replay$(DEFAULT_ASN_VERSION)_mac

# ASN.1 builds (spec-compliant) - builds both backend variants
asn1: static_replay1_linux static_replay1_compiler_linux static_replay1_mac

# ASN.0r builds (extended with refinement)
asn0r: static_replay0r_linux static_replay0r_mac

static_replay1_linux:
	@echo "Building static $(BINARY_NAME) (ASN.1, Interpreter - default) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=interpreter'" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) with Interpreter backend."

static_replay1_compiler_linux:
	@echo "Building static $(BINARY_NAME) (ASN.1, Compiler) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=compiler'" \
	-o $(BINARY_NAME)_linux_compiler .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_linux_compiler with Compiler backend."

static_replay1_mac:
	@echo "Building static $(BINARY_NAME) (ASN.1) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac) .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac)"

static_replay0r_linux:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_asn0r_linux .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_asn0r_linux"

static_replay0r_mac:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_asn0r_mac .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_asn0r_mac"

clean:
	@rm -f $(BINARY_NAME)_linux $(BINARY_NAME)_mac \
	       $(BINARY_NAME)_asn1_linux $(BINARY_NAME)_asn1_mac \
	       $(BINARY_NAME)_asn0r_linux $(BINARY_NAME)_asn0r_mac \
	       $(BINARY_NAME)_linux_compiler
	@echo "Clean complete"