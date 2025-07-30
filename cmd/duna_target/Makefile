TIMEOUT          := 10m
BINARY_NAME      := duna_target
NETWORK          ?= tiny

build: static_target_linux static_target_mac

dynamic_build:
	@echo "Building dynamic duna_fuzzer for current platform..."
	@GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) \
	go build -tags="cgo,$(NETWORK)" -o $(BINARY_NAME)_dynamic .

static_target_linux:
	@echo "Building static $(BINARY_NAME) for Linux (amd64)..."
	@ln -s $(CURDIR)/../../ffi/libunicorn.linux_amd64.a ../../ffi/libunicorn.a
	-@GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w -extldflags '-static'" \
	-o $(BINARY_NAME)_linux .
	@rm ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_linux"


static_target_mac:
	@echo "Building static $(BINARY_NAME) for macOS (amd64)..."
	@ln -s $(CURDIR)/../../ffi/libunicorn.mac.a ../../ffi/libunicorn.a
	-@GOOS=darwin GOARCH=amd64 CC=clang CGO_ENABLED=1 \
	go build -tags="cgo,$(NETWORK)" \
	-ldflags="-s -w" \
	-o $(BINARY_NAME)_mac .
	@rm ../../ffi/libunicorn.a
	@echo "Build complete: $(BINARY_NAME)_mac"