package evmtypes

import (
	"github.com/colorfulnotion/jam/common"
)

// Phase1Result captures the output of Phase 1 EVM execution (Builder Network).
// This is the data that builders agree on before JAM submission.
// No witnesses are generated in Phase 1 - only state roots and execution results.
type Phase1Result struct {
	// State roots (critical for determinism verification)
	EVMPreStateRoot  common.Hash // UBT root BEFORE batch execution
	EVMPostStateRoot common.Hash // UBT root AFTER batch execution

	// Per-block outputs (for batched execution)
	Blocks []Phase1BlockResult

	// Aggregate metrics
	TotalGasUsed uint64

	// State changes blob (for Phase 2 witness generation)
	// This is the contractWitnessBlob that will be used in Phase 2
	StateChanges []byte

	// Transaction receipts extracted from refine output.
	// These are stored locally by the builder for RPC serving (eth_getTransactionReceipt)
	// without requiring DA lookup. Populated by ExtractReceiptsFromRefineOutput.
	Receipts []*TransactionReceipt
}

// Phase1BlockResult captures the result of executing a single EVM block in Phase 1.
type Phase1BlockResult struct {
	BlockNumber  uint64
	BlockHash    common.Hash
	StateRoot    common.Hash // Post-state root for this block
	ReceiptsRoot common.Hash
	GasUsed      uint64
}

// BlockContext contains the deterministic inputs for EVM block execution.
// These must be identical in Phase 1 and Phase 2 to ensure deterministic results.
type BlockContext struct {
	Number    uint64
	Timestamp uint64
	GasLimit  uint64
	Coinbase  common.Address
	ChainID   uint64
}
