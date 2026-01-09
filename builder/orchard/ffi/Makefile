.PHONY: all clean ffi test

# Default target
all: ffi

# Build the FFI library
ffi:
	@echo "Building Orchard FFI library..."
	cd $(CURDIR) && RUSTFLAGS="-C prefer-dynamic=no" cargo build --release --lib
	@echo "Copying FFI artifacts..."
	cp target/release/liborchard_ffi.dylib .
	rm -f liborchard_ffi.a
	@echo "FFI build complete"

# Clean build artifacts
clean:
	cargo clean
	rm -f liborchard_ffi.a liborchard_ffi.dylib

# Test the FFI (requires Go)
test: ffi
	@echo "Testing FFI integration..."
	GOCACHE=/tmp/go-cache CGO_ENABLED=1 go test -v .

# Install dependencies (none needed - header is maintained manually)
deps:
	@echo "No dependencies to install"
