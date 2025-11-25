package evmtypes

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// TransactionReceipt represents the parsed transaction receipt from storage
type TransactionReceipt struct {
	TransactionHash common.Hash
	Type            uint8
	Version         uint8
	Success         bool
	UsedGas         uint64
	Payload         []byte
	LogsData        []byte
	CumulativeGas   uint64
	LogIndexStart   uint64
	// not stored in the receipt, but useful for RPC responses
	TransactionIndex uint32
	BlockHash        common.Hash
	BlockNumber      uint32
	Timestamp        uint32
}

// EthereumTransactionReceipt represents an Ethereum transaction receipt for JSON-RPC responses
type EthereumTransactionReceipt struct {
	TransactionHash   string        `json:"transactionHash"`
	TransactionIndex  string        `json:"transactionIndex"`
	BlockHash         string        `json:"blockHash"`
	BlockNumber       string        `json:"blockNumber"`
	From              string        `json:"from"`
	To                *string       `json:"to"` // null for contract creation
	CumulativeGasUsed string        `json:"cumulativeGasUsed"`
	GasUsed           string        `json:"gasUsed"`
	ContractAddress   *string       `json:"contractAddress"` // null if not contract creation
	Logs              []EthereumLog `json:"logs"`
	LogsBloom         string        `json:"logsBloom"`
	Status            string        `json:"status"` // "0x1" for success, "0x0" for failure
	EffectiveGasPrice string        `json:"effectiveGasPrice"`
	Type              string        `json:"type"` // "0x0" for legacy transactions
}

// ParseRawReceipt parses the raw receipt data into a TransactionReceipt struct
// Receipt format (Rust services/evm/src/receipt.rs:103-105):
// [logs_payload_len:4][logs_payload:variable][tx_hash:32][tx_type:1][success:1][used_gas:8]
// [cumulative_gas:4][log_index_start:4][tx_index:4][tx_payload_len:4][tx_payload:variable]
func ParseRawReceipt(w *types.StateWitness) (*TransactionReceipt, error) {
	data := w.Payload
	if len(w.Payload) < 4+32+1+1+8+4+4+4+4 { // logs_len + hash + tx_type + success + gas + cumulative + log_index + tx_index + payload_len minimum
		return nil, fmt.Errorf("transaction receipt data too short: %d bytes", len(w.Payload))
	}

	offset := 0

	logsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(logsLen) {
		return nil, fmt.Errorf("insufficient data for logs")
	}
	logsData := make([]byte, logsLen)
	copy(logsData, data[offset:offset+int(logsLen)])
	offset += int(logsLen)

	var hash common.Hash
	copy(hash[:], data[offset:offset+32])
	offset += 32

	txType := data[offset]
	offset += 1

	success := data[offset] == 1
	offset += 1

	usedGas := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	cumulativeGas := uint64(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	logIndexStart := uint64(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	txIndex := uint32(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for payload length")
	}
	payloadLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(payloadLen) {
		return nil, fmt.Errorf("insufficient data for payload")
	}
	payload := make([]byte, payloadLen)
	copy(payload, data[offset:offset+int(payloadLen)])
	offset += int(payloadLen)

	return &TransactionReceipt{
		TransactionHash:  hash,
		Type:             txType,
		Version:          0, // Version byte removed from format
		Success:          success,
		UsedGas:          usedGas,
		Payload:          payload,
		LogsData:         logsData,
		CumulativeGas:    cumulativeGas,
		LogIndexStart:    logIndexStart,
		TransactionIndex: txIndex,
		BlockHash:        w.Ref.WorkPackageHash,
		BlockNumber:      w.BlockNumber,
		Timestamp:        w.Timeslot,
	}, nil
}
