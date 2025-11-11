package evmtypes

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// TransactionReceipt represents the parsed transaction receipt from storage
type TransactionReceipt struct {
	Hash          common.Hash
	Success       bool
	UsedGas       uint64
	Payload       []byte
	LogsData      []byte
	LogsBloom     [256]byte
	CumulativeGas uint64
	LogIndexStart uint64
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
// Receipt format (Rust services/evm/src/receipt.rs:203-228):
// [logs_payload_len:4][logs_payload:variable][version:1][tx_hash:32][tx_type:1][success:1][used_gas:8][tx_payload_len:4][tx_payload:variable]
func ParseRawReceipt(data []byte) (*TransactionReceipt, error) {
	if len(data) < 4+1+32+1+1+8+4 { // logs_len + version + hash + tx_type + success + gas + payload_len minimum
		return nil, fmt.Errorf("transaction receipt data too short: %d bytes", len(data))
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

	version := data[offset]
	offset += 1

	var hash common.Hash
	copy(hash[:], data[offset:offset+32])
	offset += 32

	txType := data[offset]
	offset += 1

	success := data[offset] == 1
	offset += 1

	usedGas := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

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

	var logsBloom [256]byte
	if len(data) >= offset+256 {
		copy(logsBloom[:], data[offset:offset+256])
		offset += 256
	}

	var cumulativeGas uint64
	if len(data) >= offset+8 {
		cumulativeGas = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	var logIndexStart uint64
	if len(data) >= offset+8 {
		logIndexStart = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	_ = version // Suppress unused variable warning
	_ = txType  // Suppress unused variable warning

	return &TransactionReceipt{
		Hash:          hash,
		Success:       success,
		UsedGas:       usedGas,
		Payload:       payload,
		LogsData:      logsData,
		LogsBloom:     logsBloom,
		CumulativeGas: cumulativeGas,
		LogIndexStart: logIndexStart,
	}, nil
}
