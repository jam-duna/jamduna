TIMEOUT          := 10m
BINARY_NAME      := duna_bench
NETWORK          ?= tiny
SDK_PATH         := $(shell xcrun --show-sdk-path)

build: static_bench_linux static_bench_mac

dynamic_build:
	@echo "Building dynamic $(BINARY_NAME) for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK)" -o $(BINARY_NAME)_dynamic .

static_bench_linux:
	@echo "Building static $(BINARY_NAME) for Linux (amd64)..."
	@ln -sf $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm -f ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_linux"

static_bench_mac:
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

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)_linux $(BINARY_NAME)_mac $(BINARY_NAME)_dynamic