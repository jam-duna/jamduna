OUTPUT_DIR := bin
BINARY := jam
SRC := jam.go
NETWORK  ?= tiny
.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean publish

jam: ffi_force
	@echo "Building JAM... $(NETWORK)"
	mkdir -p $(OUTPUT_DIR)
	go build -tags=$(NETWORK) -o $(OUTPUT_DIR)/$(BINARY) $(SRC) 

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
	@echo "Copying libbls.a from $(JAM_PATH)/bls/target/release/libbls.a to $(JAM_PATH)/ffi/ For Network $(NETWORK)"
	@cp $(JAM_PATH)/bls/target/release/libbls.a $(JAM_PATH)/ffi/

# Target to build Bandersnatch FFI library
bandersnatchlib:
	@echo "Building Bandersnatch For Network $(NETWORK)..."
	@cd bandersnatch && echo "Target: $$(rustc --version --verbose | grep 'host')" && cargo build --release --features "$(NETWORK)"
	@echo "Built Bandersnatch library For Network $(NETWORK)!"
	@echo "Copying libbandersnatch.a from $(JAM_PATH)/bandersnatch/target/release/libbandersnatch.a to $(JAM_PATH)/ffi/ For Network $(NETWORK)"
	@cp $(JAM_PATH)/bandersnatch/target/release/libbandersnatch.a $(JAM_PATH)/ffi/

cargo_clean:
	@echo "Clean Up FFI libraries (BLS + Bandersnatch)!"
	@cd bandersnatch && cargo clean
	@cd ..
	@cd bls && cargo clean
	@cd ..

ffi_force: cargo_clean ffi

# Target to build both BLS and Bandersnatch FFI libraries
ffi: bandersnatchlib blslib  
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


publish:
	@echo "Publishing JAM..."
	@make -C node fallback
	@make -C cmd/importblocks cpnode MODE=fallback
	@echo "fallback data finished..."
	@make -C node safrole
	@make -C cmd/importblocks cpnode MODE=safrole
	@echo "safrole data finished..."
	@make -C node fib
	@make -C cmd/importblocks cpnode MODE=assurances
	@echo "assurances data finished..."
	@make -C node megatron MEG_PACKAGES_NUM=30
	@make -C cmd/importblocks cpnode MODE=orderedaccumulation
	@echo "orderedaccumulation data finished..."
	@make -C cmd/importblocks fuzz TIMEOUT=30m
	@echo "fuzzing finished..."
	@cd cmd/importblocks&&./data_zipper.sh


