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
	// Parsed logs (populated from LogsData when needed for RPC)
	Logs []Log
	// not stored in the receipt, but useful for RPC responses
	TransactionIndex uint32
	BlockHash        common.Hash
	BlockNumber      uint32
	Timestamp        uint32
}

// Log represents a parsed event log within a receipt
type Log struct {
	Address  common.Address // Contract address that emitted the log
	Topics   []common.Hash  // Indexed event parameters (up to 4)
	Data     []byte         // Non-indexed event parameters
	LogIndex uint64         // Global log index within the block
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

func (tr *EthereumTransactionReceipt) String() string {
	return types.ToJSONHexIndent(tr)
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

// ParseBlockReceiptsBlob parses a block receipts blob containing all receipts for a block
// Format: [receipt_count:4][receipt_0][receipt_1]...[receipt_n]
// Each receipt has format: [receipt_len:4][receipt_data:receipt_len]
// where receipt_data follows the same format as ParseRawReceipt
func ParseBlockReceiptsBlob(data []byte) ([]*TransactionReceipt, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("block receipts blob too short: %d bytes", len(data))
	}

	receiptCount := binary.LittleEndian.Uint32(data[0:4])
	offset := 4

	receipts := make([]*TransactionReceipt, 0, receiptCount)

	for i := uint32(0); i < receiptCount; i++ {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("truncated at receipt %d length field", i)
		}

		receiptLen := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		if offset+int(receiptLen) > len(data) {
			return nil, fmt.Errorf("truncated at receipt %d data (need %d, have %d)", i, receiptLen, len(data)-offset)
		}

		receiptData := data[offset : offset+int(receiptLen)]
		offset += int(receiptLen)

		// Parse individual receipt using same format as ParseRawReceipt but without StateWitness wrapper
		receipt, err := parseReceiptData(receiptData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse receipt %d: %v", i, err)
		}
		// Note: receipt.TransactionIndex is already set from the parsed payload (tx_index field)
		// We don't override it here - the payload's tx_index is authoritative
		receipts = append(receipts, receipt)
	}

	return receipts, nil
}

// parseReceiptData parses raw receipt bytes (same format as ParseRawReceipt but from raw bytes)
// Format: [logs_payload_len:4][logs_payload:variable][tx_hash:32][tx_type:1][success:1][used_gas:8]
// [cumulative_gas:4][log_index_start:4][tx_index:4][tx_payload_len:4][tx_payload:variable]
func parseReceiptData(data []byte) (*TransactionReceipt, error) {
	minLen := 4 + 32 + 1 + 1 + 8 + 4 + 4 + 4 + 4 // logs_len + hash + tx_type + success + gas + cumulative + log_index + tx_index + payload_len
	if len(data) < minLen {
		return nil, fmt.Errorf("receipt data too short: %d bytes, need at least %d", len(data), minLen)
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

	return &TransactionReceipt{
		TransactionHash:  hash,
		Type:             txType,
		Version:          0,
		Success:          success,
		UsedGas:          usedGas,
		Payload:          payload,
		LogsData:         logsData,
		CumulativeGas:    cumulativeGas,
		LogIndexStart:    logIndexStart,
		TransactionIndex: txIndex,
	}, nil
}
