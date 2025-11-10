package statedb

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
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

// GetLogs fetches event logs matching a filter
func (n *StateDB) GetLogs(serviceID, fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]EthereumLog, error) {
	var allLogs []EthereumLog

	// Collect logs from the specified block range
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockLogs, err := n.getLogsFromBlock(serviceID, blockNum, addresses, topics)
		if err != nil {
			log.Warn(log.Node, "GetLogs: Failed to get logs from block", "blockNumber", blockNum, "error", err)
			continue // Skip failed blocks but continue processing
		}
		allLogs = append(allLogs, blockLogs...)
	}

	return allLogs, nil
}

// getLogsFromBlock retrieves logs from a specific block that match the filter criteria
func (n *StateDB) getLogsFromBlock(serviceID uint32, blockNumber uint32, addresses []common.Address, topics [][]common.Hash) ([]EthereumLog, error) {
	// 1. Get all transaction hashes from the block (use canonical metadata)
	evmBlock, err := n.ReadBlockByNumber(serviceID, blockNumber)
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
		witness, found, err := n.ReadStateWitnessRef(EVMServiceCode, receiptObjectID, false)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		receipt, err := ParseRawReceipt(witness.Payload)
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
func (n *StateDB) matchesLogFilter(ethLog EthereumLog, addresses []common.Address, topics [][]common.Hash) bool {
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
		data := common.Bytes2Hex(logsData[offset : offset+int(dataLen)])
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

func showEthereumLogs(txHash common.Hash, logs []EthereumLog, allTopics map[common.Hash]string) {
	for _, lg := range logs {
		if len(lg.Topics) == 0 {
			continue
		}
		// First topic is the event signature
		topicHash := common.HexToHash(lg.Topics[0])
		if eventSig, found := allTopics[topicHash]; found {
			// Build a string with event signature and decoded data
			topicStr := eventSig

			// Add indexed topics (Topics[1:])
			if len(lg.Topics) > 1 {
				for i := 1; i < len(lg.Topics); i++ {
					topicStr = topicStr + fmt.Sprintf("indexed[%d]=%s", i-1, lg.Topics[i])
				}
			}

			// Add data (non-indexed parameters)
			dataStr := lg.Data
			// Fix double 0x prefix issue
			if strings.HasPrefix(dataStr, "0x0x") {
				dataStr = dataStr[2:]
			}
			dataBytes := common.FromHex(dataStr)

			// Convert logIndex from hex to decimal
			logIndexInt := new(big.Int)
			logIndexInt.SetString(strings.TrimPrefix(lg.LogIndex, "0x"), 16)

			if len(dataBytes) > 0 && len(dataBytes)%32 == 0 {
				var decodedValues []string
				for i := 0; i < len(dataBytes); i += 32 {
					value := new(big.Int).SetBytes(dataBytes[i : i+32])
					decodedValues = append(decodedValues, value.String())
				}
				// Extract event name without parameter types for decoded output
				eventName := eventSig
				if idx := strings.Index(eventSig, "("); idx != -1 {
					eventName = eventSig[:idx]
				}
				decodedStr := fmt.Sprintf("%s(%s)", eventName, strings.Join(decodedValues, ","))
				log.Info(log.Node, decodedStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			} else if len(dataBytes) > 0 {
				log.Info(log.Node, dataStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			} else {
				log.Info(log.Node, topicStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			}
		} else {
			log.Warn(log.Node, fmt.Sprintf("%s-%s", txHash, lg.Topics[0]), "logIndex", lg.LogIndex, "data", lg.Data)
		}
	}
}
