package node

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

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
	// 1. Get all transaction hashes from the block (use canonical metadata)
	evmBlock, err := n.readBlockByNumber(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}
	blockTxHashes := evmBlock.TxHashes
	blockHashHex := evmBlock.ComputeHash().String()

	var blockLogs []EthereumLog

	// For each transaction, get its receipt and extract logs
	for txIndex, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := tx_to_objectID(txHash)
		witness, found, err := n.statedb.ReadStateWitnessRef(statedb.EVMServiceCode, receiptObjectID, false)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		receipt, err := statedb.ParseRawReceipt(witness.Payload)
		if err != nil {
			log.Warn(log.Node, "getLogsFromBlock: Failed to parse receipt", "txHash", txHash.String(), "error", err)
			continue
		}
		ref := witness.Ref

		// Extract and filter logs from this transaction
		if len(receipt.LogsData) > 0 {
			txLogs, err := parseLogsFromReceipt(
				receipt.LogsData,
				txHash,
				fmt.Sprintf("0x%x", blockNumber),
				blockHashHex,
				fmt.Sprintf("0x%x", txIndex),
				uint64(ref.LogIndex),
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

// parseLogsFromReceipt parses logs from receipt LogsData
// Format from Rust helpers.rs:715-746:
// [log_count:2][address:20][topic_count:1][topics:32*N][data_len:4][data:N]
// å
// logIndexCounter is a pointer to a block-scoped counter for Ethereum-compliant log indices.
// Each log increments the counter so logIndex is unique across the entire block, not per-transaction.
func parseLogsFromReceipt(logsData []byte, txHash common.Hash, blockNumber, blockHash, txIndex string, logIndexStart uint64) ([]EthereumLog, error) {
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
		data := common.Bytes2Hex(logsData[offset:offset+int(dataLen)])
		offset += int(dataLen)

		logs = append(logs, EthereumLog{
			Address:          address.String(),
			Topics:           topics,
			Data:             data,
			BlockNumber:      blockNumber,
			TransactionHash:  txHash.String(),
			TransactionIndex: txIndex,
			BlockHash:        blockHash,
			LogIndex:         fmt.Sprintf("0x%x", logIndexStart+uint64(i)),
			Removed:          false,
		})
	}

	return logs, nil
}
