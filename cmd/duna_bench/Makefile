TIMEOUT          := 10m
BINARY_NAME      := duna_bench
NETWORK          ?= tiny
SDK_PATH         := $(shell xcrun --show-sdk-path)
DEFAULT_ASN_VERSION := 1

build: static_bench$(DEFAULT_ASN_VERSION)_linux static_bench$(DEFAULT_ASN_VERSION)_interpreter_linux static_bench$(DEFAULT_ASN_VERSION)_mac

# Default ASN version build (follows DEFAULT_ASN_VERSION)
make: asn$(DEFAULT_ASN_VERSION)

# ASN.1 builds (spec-compliant) - builds both backend variants
asn1: static_bench1_linux static_bench1_interpreter_linux static_bench1_mac

# ASN.0r builds (extended with refinement)
asn0r: static_bench0r_linux static_bench0r_mac

dynamic_build:
	@echo "Building dynamic $(BINARY_NAME) for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK),asn$(DEFAULT_ASN_VERSION)" -o $(BINARY_NAME)_dynamic .

dynamic_compiler:
	@echo "Building dynamic $(BINARY_NAME) with compiler backend for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK),asn$(DEFAULT_ASN_VERSION)" \
	-ldflags="-X 'main.defaultBackend=compiler'" \
	-o $(BINARY_NAME)_dynamic_compiler .

static_bench_linux:
	@echo "Building static $(BINARY_NAME) for Linux (amd64)..."
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@echo "Build complete: $(BINARY_NAME)_linux"

static_bench_mac:
	@echo "Building static $(BINARY_NAME) for macOS (amd64)..."
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@echo "Build complete: $(BINARY_NAME)_mac"

static_bench1_linux:
	@echo "Building static $(BINARY_NAME) (ASN.1, Compiler - default) for Linux (amd64)..."
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=compiler'" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) .
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_linux,$(BINARY_NAME)_asn1_linux) with Compiler backend."

static_bench1_interpreter_linux:
	@echo "Building static $(BINARY_NAME) (ASN.1, Interpreter) for Linux (amd64)..."
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w -extldflags '-static' -X 'main.defaultBackend=interpreter'" \
	-o $(BINARY_NAME)_linux_interpreter .
	@echo "Build complete: $(BINARY_NAME)_linux_interpreter with Interpreter backend."

static_bench1_mac:
	@echo "Building static $(BINARY_NAME) (ASN.1) for macOS (amd64)..."
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn1" \
	-ldflags="-s -w" \
	-o $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac) .
	@echo "Build complete: $(if $(filter 1,$(DEFAULT_ASN_VERSION)),$(BINARY_NAME)_mac,$(BINARY_NAME)_asn1_mac)"

static_bench0r_linux:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for Linux (amd64)..."
	-@CGO_LDFLAGS="" GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_asn0r_linux .
	@echo "Build complete: $(BINARY_NAME)_asn0r_linux"

static_bench0r_mac:
	@echo "Building static $(BINARY_NAME) (ASN.0r) for macOS (amd64)..."
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK),asn0r" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_asn0r_mac .
	@echo "Build complete: $(BINARY_NAME)_asn0r_mac"

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)_linux $(BINARY_NAME)_mac $(BINARY_NAME)_dynamic \
	      $(BINARY_NAME)_asn1_linux $(BINARY_NAME)_asn1_mac \
	      $(BINARY_NAME)_asn0r_linux $(BINARY_NAME)_asn0r_mac \
	      $(BINARY_NAME)_linux_interpreter
