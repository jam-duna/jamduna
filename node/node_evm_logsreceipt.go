package node

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

// TransactionReceipt represents the parsed transaction receipt from storage
type TransactionReceipt struct {
	Hash     common.Hash
	Success  bool
	UsedGas  uint64
	Payload  []byte
	LogsData []byte
}

// parseRawReceipt parses the raw receipt data into a TransactionReceipt struct
//
// Receipt format: [version:1][tx_hash:32][tx_type:1][status:1][used_gas:8][tx_payload_len:4][tx_payload]
// [logs_payload_len:4][logs_payload][tx_index:2][cumulative_gas:8][logs_bloom:256][receipt_hash:32]
func parseRawReceipt(data []byte) (*TransactionReceipt, error) {
	if len(data) < 1+32+1+1+8+4 { // version + hash + tx_type + success + gas + payload_len minimum
		return nil, fmt.Errorf("transaction receipt data too short: %d bytes", len(data))
	}

	offset := 0

	// Parse version (1 byte)
	version := data[offset]
	offset += 1

	// Parse hash (32 bytes)
	var hash common.Hash
	copy(hash[:], data[offset:offset+32])
	offset += 32

	// Parse tx_type (1 byte) - EIP-2718
	txType := data[offset]
	offset += 1

	// Parse success flag (1 byte)
	success := data[offset] == 1
	offset += 1

	// Parse used gas (8 bytes, little-endian)
	usedGas := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Parse payload length and payload
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

	// Parse logs length and logs
	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for logs length")
	}
	logsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(logsLen) {
		return nil, fmt.Errorf("insufficient data for logs")
	}
	logsData := make([]byte, logsLen)
	copy(logsData, data[offset:offset+int(logsLen)])
	offset += int(logsLen)

	// Optional canonical fields (version 1): [tx_index:2][cumulative_gas:8][logs_bloom:256][receipt_hash:32]
	// These fields are present if version == 1, but we don't need to parse them for current RPC implementation
	// They will be used by future RPC enhancements for proper Ethereum receipt compatibility
	_ = version // Suppress unused variable warning
	_ = txType  // Suppress unused variable warning

	return &TransactionReceipt{
		Hash:     hash,
		Success:  success,
		UsedGas:  usedGas,
		Payload:  payload,
		LogsData: logsData,
	}, nil
}

// LogFilter represents the filter criteria for eth_getLogs
type LogFilter struct {
	FromBlock interface{}   `json:"fromBlock,omitempty"` // "latest", "earliest", "pending", or hex number
	ToBlock   interface{}   `json:"toBlock,omitempty"`   // "latest", "earliest", "pending", or hex number
	Address   interface{}   `json:"address,omitempty"`   // string or array of strings
	Topics    []interface{} `json:"topics,omitempty"`    // array of topics (32-byte hex strings or arrays)
}

// EthereumLog represents an Ethereum event log
type EthereumLog struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	BlockHash        string   `json:"blockHash"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
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

// GetLogs fetches event logs matching a filter
func (n *NodeContent) GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]EthereumLog, error) {
	var allLogs []EthereumLog

	// Collect logs from the specified block range
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockLogs, err := n.getLogsFromBlock(blockNum, addresses, topics)
		if err != nil {
			log.Warn(log.Node, "GetLogs: Failed to get logs from block", "blockNumber", blockNum, "error", err)
			continue // Skip failed blocks but continue processing
		}
		allLogs = append(allLogs, blockLogs...)
	}

	return allLogs, nil
}

// getLogsFromBlock retrieves logs from a specific block that match the filter criteria
func (n *NodeContent) getLogsFromBlock(blockNumber uint32, addresses []common.Address, topics [][]common.Hash) ([]EthereumLog, error) {
	// 1. Get all transaction hashes from the block
	blockTxHashes, err := n.readBlockFromStorage(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}

	var blockLogs []EthereumLog

	// Block-scoped log index counter for Ethereum-compliant unique log indices
	var logIndexCounter uint64 = 0

	// 2. For each transaction, get its receipt and extract logs
	for txIndex, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := tx_to_objectID(txHash)
		witness, found, err := n.statedb.ReadStateWitness(statedb.EVMServiceCode, receiptObjectID, true)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		// Parse receipt
		receipt, err := parseRawReceipt(witness.Payload)
		if err != nil {
			log.Warn(log.Node, "getLogsFromBlock: Failed to parse receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		// Extract and filter logs from this transaction
		if len(receipt.LogsData) > 0 {
			txLogs, err := parseLogsFromReceipt(
				receipt.LogsData,
				txHash,
				fmt.Sprintf("0x%x", blockNumber),
				"0x0000000000000000000000000000000000000000000000000000000000000001", // TODO: Real block hash
				fmt.Sprintf("0x%x", txIndex),
				&logIndexCounter, // Block-scoped counter for unique log indices
			)
			if err != nil {
				log.Warn(log.Node, "getLogsFromBlock: Failed to parse logs", "txHash", txHash.String(), "error", err)
				continue
			}

			// Apply address and topic filters
			for _, ethLog := range txLogs {
				if n.matchesLogFilter(ethLog, addresses, topics) {
					blockLogs = append(blockLogs, ethLog)
				}
			}
		}
	}

	return blockLogs, nil
}

// matchesLogFilter checks if a log matches the filter criteria
func (n *NodeContent) matchesLogFilter(ethLog EthereumLog, addresses []common.Address, topics [][]common.Hash) bool {
	// Check address filter
	if len(addresses) > 0 {
		matched := false
		logAddr := common.HexToAddress(ethLog.Address)
		for _, addr := range addresses {
			if logAddr == addr {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check topics filter
	if len(topics) > 0 {
		if !matchesTopicsFilter(ethLog.Topics, topics) {
			return false
		}
	}

	return true
}

// matchesTopicsFilter checks if log topics match the topic filter
func matchesTopicsFilter(logTopics []string, topicFilter [][]common.Hash) bool {
	// Each position in topicFilter is OR'd together, and positions are AND'd
	for i, filterTopics := range topicFilter {
		if len(filterTopics) == 0 {
			continue // null/empty means match any
		}

		if i >= len(logTopics) {
			return false // Log doesn't have enough topics
		}

		// Check if any of the OR'd topics match
		matched := false
		logTopic := common.HexToHash(logTopics[i])
		for _, filterTopic := range filterTopics {
			if logTopic == filterTopic {
				matched = true
				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// calculateLogsBloom generates a 256-byte Ethereum logs bloom filter from logs
// For now, returns an empty bloom (all zeros). Full implementation will be added in Phase II.
func calculateLogsBloom(logs []EthereumLog) string {
	// TODO: Implement proper Ethereum bloom filter per LOGS-BLOOM.md
	// For each log: hash address and topics with Keccak-256, set bits using 3x 11-bit slices
	bloom := make([]byte, 256)
	return "0x" + common.Bytes2Hex(bloom)
}

// parseLogsFromReceipt parses logs from receipt LogsData
// Format from Rust helpers.rs:715-746:
// [log_count:2][address:20][topic_count:1][topics:32*N][data_len:4][data:N]
//
// logIndexCounter is a pointer to a block-scoped counter for Ethereum-compliant log indices.
// Each log increments the counter so logIndex is unique across the entire block, not per-transaction.
func parseLogsFromReceipt(logsData []byte, txHash common.Hash, blockNumber, blockHash, txIndex string, logIndexCounter *uint64) ([]EthereumLog, error) {
	if len(logsData) < 2 {
		// No logs or insufficient data
		return []EthereumLog{}, nil
	}

	offset := 0

	// Parse log count (2 bytes, little-endian)
	logCount := binary.LittleEndian.Uint16(logsData[offset : offset+2])
	offset += 2

	logs := make([]EthereumLog, 0, logCount)

	for i := uint16(0); i < logCount; i++ {
		if offset+20 > len(logsData) {
			return nil, fmt.Errorf("insufficient data for log %d address", i)
		}

		// Parse address (20 bytes)
		address := common.BytesToAddress(logsData[offset : offset+20])
		offset += 20

		// Parse topic count (1 byte)
		if offset+1 > len(logsData) {
			return nil, fmt.Errorf("insufficient data for log %d topic count", i)
		}
		topicCount := logsData[offset]
		offset += 1

		// Parse topics (32 bytes each)
		topics := make([]string, topicCount)
		for j := uint8(0); j < topicCount; j++ {
			if offset+32 > len(logsData) {
				return nil, fmt.Errorf("insufficient data for log %d topic %d", i, j)
			}
			var topic common.Hash
			copy(topic[:], logsData[offset:offset+32])
			topics[j] = topic.String()
			offset += 32
		}

		// Parse data length (4 bytes, little-endian)
		if offset+4 > len(logsData) {
			return nil, fmt.Errorf("insufficient data for log %d data length", i)
		}
		dataLen := binary.LittleEndian.Uint32(logsData[offset : offset+4])
		offset += 4

		// Parse data
		if offset+int(dataLen) > len(logsData) {
			return nil, fmt.Errorf("insufficient data for log %d data", i)
		}
		data := "0x" + common.Bytes2Hex(logsData[offset:offset+int(dataLen)])
		offset += int(dataLen)

		// Build Ethereum log
		// Use block-scoped logIndexCounter for Ethereum-compliant unique log indices
		currentLogIndex := *logIndexCounter
		*logIndexCounter += 1 // Increment for next log in block

		logs = append(logs, EthereumLog{
			Address:          address.String(),
			Topics:           topics,
			Data:             data,
			BlockNumber:      blockNumber,
			TransactionHash:  txHash.String(),
			TransactionIndex: txIndex,
			BlockHash:        blockHash,
			LogIndex:         fmt.Sprintf("0x%x", currentLogIndex),
			Removed:          false,
		})
	}

	return logs, nil
}
