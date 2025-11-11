package evmtypes

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/ethereum/go-ethereum/crypto"
)

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

// LogFilter represents the filter criteria for eth_getLogs
type LogFilter struct {
	FromBlock interface{}   `json:"fromBlock,omitempty"` // "latest", "earliest", "pending", or hex number
	ToBlock   interface{}   `json:"toBlock,omitempty"`   // "latest", "earliest", "pending", or hex number
	Address   interface{}   `json:"address,omitempty"`   // string or array of strings
	Topics    []interface{} `json:"topics,omitempty"`    // array of topics (32-byte hex strings or arrays)
}

// ParseLogsFromReceipt parses logs from receipt LogsData
// Format from Rust helpers.rs:715-746:
// [log_count:2][address:20][topic_count:1][topics:32*N][data_len:4][data:N]
//
// logIndexStart is the starting index for Ethereum-compliant log indices.
// Each log increments the counter so logIndex is unique across the entire block, not per-transaction.
func ParseLogsFromReceipt(logsData []byte, txHash common.Hash, blockNumber, blockHash, txIndex string, logIndexStart uint64) ([]EthereumLog, error) {
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

// ShowEthereumLogs displays Ethereum logs with decoded event signatures
func ShowEthereumLogs(txHash common.Hash, logs []EthereumLog, allTopics map[common.Hash]string) {
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

// MatchesLogFilter checks if a log matches the filter criteria
func MatchesLogFilter(ethLog EthereumLog, addresses []common.Address, topics [][]common.Hash) bool {
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
		if !MatchesTopicsFilter(ethLog.Topics, topics) {
			return false
		}
	}

	return true
}

// MatchesTopicsFilter checks if log topics match the topic filter
func MatchesTopicsFilter(logTopics []string, topicFilter [][]common.Hash) bool {
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

// DefaultTopics returns a map of topic hashes to event signatures for common ERC20/ERC1155 events
func DefaultTopics() map[common.Hash]string {
	erc20Events := []string{
		"Transfer(address,address,uint256)", // Most common - token transfers
		"Approval(address,address,uint256)", // Second most common - approve allowances
		"Mint(address,uint256)",             // Minting new tokens
		"Burn(address,uint256)",             // Burning tokens
		"Burn(uint256)",
		"Mint(uint256)",
		"Deposit(address,uint256)",                                   // Wrapped tokens deposit
		"Withdrawal(address,uint256)",                                // Wrapped tokens withdrawal
		"Swap(address,address,uint256,uint256)",                      // Token swaps
		"TransferSingle(address,address,address,uint256,uint256)",    // ERC1155 single transfer
		"TransferBatch(address,address,address,uint256[],uint256[])", // ERC1155 batch transfer
		"ApprovalForAll(address,address,bool)",                       // ERC1155 approval for all
	}

	topics := make(map[common.Hash]string, len(erc20Events))
	for _, eventSig := range erc20Events {
		eventHash := crypto.Keccak256([]byte(eventSig))
		topics[common.BytesToHash(eventHash[:32])] = eventSig
	}

	return topics
}
