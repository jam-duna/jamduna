OUTPUT_DIR := bin
BINARY := jam
SRC := jam.go

# Phony targets to ensure they always run
.PHONY: bls bandersnatch ffi jam clean beauty fmt-check

jam:
	@echo "Building JAM..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(BINARY) $(SRC)

jamweb:
		@echo "Building JAM WEB..."
		@cd jamweb && go build

validatetraces:
		@echo "Building JAM validatetraces..."
		@cd cmd/validatetraces && go build

testnet:
	make jam
	docker build -t colorfulnotion/jam .
	docker-compose up

clean:
	rm -f $(OUTPUT_DIR)/$(BINARY)

# Target to build BLS FFI library
bls:
	@echo "Building BLS..."
	@cd bls && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release
	@echo "Built BLS library!"

# Target to build Bandersnatch FFI library
bandersnatch:
	@echo "Building Bandersnatch..."
	@cd bandersnatch && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release
	@echo "Built Bandersnatch library!"

cargo_clean:
	@echo "Clean Up FFI libraries (BLS + Bandersnatch)!"
	@cd bandersnatch && cargo clean
	@cd ..
	@cd bls && cargo clean
	@cd ..

ffi_force: cargo_clean ffi

# Target to build both BLS and Bandersnatch FFI libraries
ffi: bls bandersnatch
	@echo "Built all FFI libraries (BLS + Bandersnatch)!"

beauty:
	@echo "Running go fmt on all Go files..."
	@go fmt ./...

fmt-check:
	@echo "Checking formatting..."
	@diff -u <(echo -n) <(gofmt -d .)
