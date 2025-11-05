# Solidity Contracts Makefile
# Compiles Solidity contracts to binary format

SOLC = solc
SOLC_FLAGS = --bin --abi --optimize --overwrite

# Default target
.PHONY: all
all: math.bin math-runtime.bin usdm-runtime.bin

# Math contract compilation
.PHONY: math
math: Math.sol
	@echo "Compiling Math.sol..."
	$(SOLC) $(SOLC_FLAGS) -o . Math.sol
	@mv Math.bin math.evmhex
	@xxd -r -p math.evmhex math.bin
	$(SOLC) $(SOLC_FLAGS) --bin-runtime --optimize-runs 200 -o . Math.sol
	@mv Math.bin-runtime math-runtime.evmhex
	@xxd -r -p math-runtime.evmhex math-runtime.bin
	@echo "✅ math compiled successfully"

# USDM contract compilation (if needed)
.PHONY: usdm
usdm: usdm.bin

usdm-runtime.bin: usdm.sol
	@echo "Compiling usdm.sol to usdm.bin..."
	$(SOLC) $(SOLC_FLAGS) --bin-runtime -o . usdm.sol
	@mv USDM.bin usdm-runtime.evmhex
	@xxd -r -p usdm-runtime.evmhex usdm-runtime.bin
	@echo "✅ usdm-runtime.bin created successfully"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -f math.bin math.evmhex math-runtime.bin math-runtime.evmhex
	@echo "✅ Cleanup complete"

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  math        - Compile Math.sol to math.bin"
	@echo "  usdm        - Compile usdm.sol to usdm.bin"
	@echo "  all         - Compile all contracts"
	@echo "  clean       - Remove build artifacts"
	@echo "  debug       - Compile with debug output"
	@echo "  help        - Show this help message"