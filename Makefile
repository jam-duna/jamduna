OUTPUT_DIR := bin
BINARY := jam
SRC := jam.go

.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean

jam:
	@echo "Building JAM..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/$(BINARY) $(SRC)

da:
	@echo "Building JAM..."
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/da da.go

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

#clean:
#	rm -f $(OUTPUT_DIR)/$(BINARY)

# Target to build BLS FFI library
blslib:
	@echo "Building BLS..."
	@cd bls && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release
	@echo "Built BLS library!"

# Target to build Bandersnatch FFI library
bandersnatchlib:
	@echo "Building Bandersnatch..."
	@cd bandersnatch && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release
	@echo "Built Bandersnatch library!"

sp1lib:
	@echo "Building SP1..."
	@cd sp1 && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release
	@echo "Built SP1 library!"

cargo_clean:
	@echo "Clean Up FFI libraries (BLS + Bandersnatch)!"
	@cd bandersnatch && cargo clean
	@cd ..
	@cd bls && cargo clean
	@cd ..

ffi_force: cargo_clean ffi

# Target to build both BLS and Bandersnatch FFI libraries
ffi: blslib bandersnatchlib sp1lib
	@echo "Built all FFI libraries (BLS + Bandersnatch)!"

beauty:
	@echo "Running go fmt on all Go files..."
	@go fmt ./...

fmt-check:
	@echo "Checking formatting..."
	@diff -u <(echo -n) <(gofmt -d .)


# List of packages (missing node)
PACKAGES := $(shell go list ./... | grep -E "bandersnatch|bls|common|erasurecoding|pvm|statedb|trie|types")


clean:
	go clean -testcache


COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Default target
allcoverage: coveragetest coverage

# Run tests with coverage
coveragetest:
	@echo "Running tests with coverage..."
	@go test -coverprofile=$(COVERAGE_FILE) $(PACKAGES)

# Generate coverage HTML
coverage: coveragetest
	@echo "Generating HTML coverage report..."
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

# Clean up
cleancoverage:
	@echo "Cleaning up..."
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "Done."

