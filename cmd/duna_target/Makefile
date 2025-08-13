TIMEOUT          := 10m
BINARY_NAME      := duna_target
NETWORK          ?= tiny
SDK_PATH         := $(shell xcrun --show-sdk-path)

build: static_target_linux static_target_mac

dynamic_build:
	@echo "Building dynamic duna_fuzzer for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK)" -o $(BINARY_NAME)_dynamic .

static_target_linux:
	@echo "Building static $(BINARY_NAME) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_linux"

static_target_mac:
	@echo "Building static $(BINARY_NAME) for macOS (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@MACOSX_DEPLOYMENT_TARGET=14.0 \
	  GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	  CGO_CFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	  CGO_LDFLAGS="-mmacosx-version-min=14.0 -isysroot $(SDK_PATH)" \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_mac"


