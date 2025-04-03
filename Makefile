OUTPUT_DIR := bin
BINARY := jam
SRC := jam.go
NETWORK  ?= tiny
NUM_NODES ?= 6
DEFAULT_PORT ?= 9900
BRANCH ?= jam_update
JAM_START_TIME ?= $(shell date -d "5 seconds" +"%Y-%m-%d %H:%M:%S")
.PHONY: bls bandersnatch ffi jam clean beauty fmt-check allcoverage coveragetest coverage cleancoverage clean jam_without_ffi_build run_parallel_jam kill_parallel_jam run_jam build_remote_nodes run_jam_remote_nodes da jamweb validatetraces testnet

jam_with_ffi_build: ffi_force
	@echo "Building JAM... $(NETWORK)"
	mkdir -p $(OUTPUT_DIR)
	go build -tags=$(NETWORK) -o $(OUTPUT_DIR)/$(BINARY) $(SRC) 
jam:
	@echo "Building JAM... $(NETWORK)"
	mkdir -p $(OUTPUT_DIR)
	go build -tags=$(NETWORK) -o $(OUTPUT_DIR)/$(BINARY) $(SRC)
tiny: jam
	ansible-playbook -u root -i /root/go/src/github.com/colorfulnotion/jam/hosts.txt -e "MODE=immediate" /root/go/src/github.com/colorfulnotion/jam/yaml/jam_restart.yaml 
run_parallel_jam:
	@mkdir -p logs 
	@echo "Starting $(NUM_NODES) instances of bin/jam..."
	@seq 0 $(shell echo $$(($(NUM_NODES) - 1))) | xargs -I{} -P $(NUM_NODES) sh -c 'PORT=$$(($(DEFAULT_PORT) + {})); bin/jam -net_spec $(NETWORK) -port $$PORT -start_time "$(JAM_START_TIME)"; echo "Instance {} finished with port $$PORT"' sh
	@echo "All instances started."
run_parallel_jam_with_deadnode:
	@mkdir -p logs 
	@echo "Starting $(NUM_NODES) instances of bin/jam..."
	@seq 0 $(shell echo $$(($(NUM_NODES) - 2))) | xargs -I{} -P $(NUM_NODES) sh -c 'PORT=$$(($(DEFAULT_PORT) + {})); bin/jam -net_spec $(NETWORK) -port $$PORT -start_time "$(JAM_START_TIME)"; echo "Instance {} finished with port $$PORT"' sh
	@echo "All instances started."
kill_parallel_jam:
	@echo "Killing all instances of bin/jam..."
	@pgrep -f "bin/jam"
	@pkill -f "bin/jam"
	@echo "All instances killed."
run_jam:
	@echo "Starting bin/jam... with network $(NETWORK) port $(DEFAULT_PORT) start_time $(JAM_START_TIME)"
	@$(OUTPUT_DIR)/$(BINARY) -net_spec $(NETWORK) -port $(DEFAULT_PORT) -start_time "$(JAM_START_TIME)"
	@echo "Instance started."

# env setup for remote nodes
jam_set:
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && echo \"export CARGO_MANIFEST_DIR=\$(pwd)\" >> ~/.bashrc'"
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "source ~/.bashrc"
# build ffi and apply latest code
build_remote_nodes:
	@echo "Building JAM on all remote nodes..."
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && git fetch origin && git reset --hard origin/$(BRANCH) && git clean -fd'"
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && make bandersnatchlib NETWORK=$(NETWORK)'"
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && make blslib NETWORK=$(NETWORK)'"
	@echo "All remote nodes built."
# clean the process and delete the storage
clean_remote_nodes:
	@echo "Cleaning JAM on all remote nodes..."
	@sudo /usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'rm -rf .jam'"
	#grep the pid from port 9900 and kill it
	@sudo /usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -c 'command -v lsof >/dev/null && lsof -t -i:9900 | xargs --no-run-if-empty kill -9'"
	@echo "All remote nodes cleaned."
# update the latest commit on remote nodes
reset_remote_nodes:
	@echo "Resetting JAM on all remote nodes..."
	@/usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && git fetch origin && git reset --hard origin/$(BRANCH) && git clean -fd'"
	@echo "All remote nodes reset."
# run jam.go on remote nodes on port 9900
run_jam_remote_nodes:
	@echo "Starting run_jam on all remote nodes..."
	@sudo /usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'cdj && make jam_without_ffi_build NETWORK=$(NETWORK)'"
	@sudo /usr/bin/parallel-ssh -h hosts.txt -l root -i "bash -i -c 'export NETWORK=$(NETWORK); export DEFAULT_PORT=$(DEFAULT_PORT); export JAM_START_TIME=\"$(shell date +'%Y-%m-%d %H:%M:%S')\"; cdj && make run_jam'"
	@echo "All remote nodes started."

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


polkavmscp:
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/bootstrap/bootstrap.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/megatron/megatron.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/fib/fib.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/tribonacci/tribonacci.pvm services
	scp polkavm:/root/go/src/github.com/colorfulnotion/polkavm/services/corevm/corevm.pvm services

jamx_start:
	ansible-playbook -u root -i hosts.txt  yaml/jam_start.yaml
	@echo "update jam binary and start on jam instances"

jamx_stop:
	ansible-playbook -u root -i hosts.txt  yaml/jam_stop.yaml
	@echo "stop on jam instances"

