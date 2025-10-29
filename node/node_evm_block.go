package node

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

// Ethereum internal methods implementation on NodeContent
const (
	// Chain ID for JAM testnet
	jamChainID = 0x1107

	// Hardhat Account #0 (Issuer/Alice) - First account from standard Hardhat/Anvil test mnemonic
	// "test test test test test test test test test test test junk"
	issuerPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
)

// EthereumBlock represents an Ethereum block for JSON-RPC responses
type EthereumBlock struct {
	Number           string      `json:"number"`
	Hash             string      `json:"hash"`
	ParentHash       string      `json:"parentHash"`
	Nonce            string      `json:"nonce"`
	Sha3Uncles       string      `json:"sha3Uncles"`
	LogsBloom        string      `json:"logsBloom"`
	TransactionsRoot string      `json:"transactionsRoot"`
	StateRoot        string      `json:"stateRoot"`
	ReceiptsRoot     string      `json:"receiptsRoot"`
	Miner            string      `json:"miner"`
	Difficulty       string      `json:"difficulty"`
	TotalDifficulty  string      `json:"totalDifficulty"`
	ExtraData        string      `json:"extraData"`
	Size             string      `json:"size"`
	GasLimit         string      `json:"gasLimit"`
	GasUsed          string      `json:"gasUsed"`
	Timestamp        string      `json:"timestamp"`
	Transactions     interface{} `json:"transactions"`
	Uncles           []string    `json:"uncles"`
}

// GetChainId returns the chain ID for the current network
func (n *NodeContent) GetChainId() uint64 {
	// For now return JAM testnet chain ID: 0x1107 = 4359
	return jamChainID
}

// GetAccounts returns the list of addresses owned by the client
func (n *NodeContent) GetAccounts() []common.Address {
	// Node does not manage accounts - users should use wallets like MetaMask
	return []common.Address{}
}

// GetGasPrice returns the current gas price in wei
func (n *NodeContent) GetGasPrice() uint64 {
	// TODO: Implement dynamic gas pricing based on network conditions
	// For now return fixed 1 gwei
	return 1000000000 // 1 gwei in wei
}

// GetBlockByHash fetches a block by hash
// getLatestBlockNumber reads the current block number from EVM service storage
func (n *NodeContent) getLatestBlockNumber() (uint32, error) {
	// Read the "blocknumber" key from EVM service storage
	key := []byte("blocknumber")

	valueBytes, found, err := n.statedb.ReadServiceStorage(statedb.EVMServiceCode, key)
	if err != nil {
		return 0, fmt.Errorf("failed to read block number from storage: %v", err)
	}
	if !found {
		return 1, nil // Return genesis block number if not found
	}

	if len(valueBytes) < 4 {
		return 1, nil // Return genesis block number if malformed
	}

	// Parse little-endian uint32
	blockNumber := binary.LittleEndian.Uint32(valueBytes[:4])
	return blockNumber, nil
}

// readBlockFromStorage reads block data (transaction hashes) for a given block number
func (n *NodeContent) readBlockFromStorage(blockNumber uint32) ([]common.Hash, error) {
	// Convert block number to little-endian bytes as key
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, blockNumber)

	valueBytes, found, err := n.statedb.ReadServiceStorage(statedb.EVMServiceCode, key)
	if err != nil {
		return nil, fmt.Errorf("failed to read block data: %v", err)
	}
	if !found {
		return nil, fmt.Errorf("block %d not found", blockNumber)
	}

	// Parse the value as concatenated 32-byte hashes
	if len(valueBytes)%32 != 0 {
		return nil, fmt.Errorf("invalid block data length: %d", len(valueBytes))
	}

	numTxs := len(valueBytes) / 32
	txHashes := make([]common.Hash, numTxs)

	for i := 0; i < numTxs; i++ {
		copy(txHashes[i][:], valueBytes[i*32:(i+1)*32])
	}

	return txHashes, nil
}

// convertToEthereumBlock converts JAM block data to Ethereum JSON-RPC block format
func (n *NodeContent) convertToEthereumBlock(blockNumber uint32, txHashes []common.Hash, fullTx bool) (*EthereumBlock, error) {
	// Create Ethereum block structure

	// Compute block hash from block number for now (simplified)
	blockHash := common.Blake2Hash(binary.LittleEndian.AppendUint32(nil, blockNumber))
	parentHash := common.Hash{}
	if blockNumber > 1 {
		parentHash = common.Blake2Hash(binary.LittleEndian.AppendUint32(nil, blockNumber-1))
	}

	// Calculate total gas used by summing all receipts
	var totalGasUsed uint64 = 0
	for _, txHash := range txHashes {
		receiptObjectID := tx_to_objectID(txHash)
		witness, found, err := n.statedb.ReadStateWitness(statedb.EVMServiceCode, receiptObjectID, true)
		if err == nil && found {
			receipt, err := parseRawReceipt(witness.Payload)
			if err == nil {
				totalGasUsed += receipt.UsedGas
			}
		}
	}

	block := &EthereumBlock{
		Number:           fmt.Sprintf("0x%x", blockNumber),
		Hash:             blockHash.String(),
		ParentHash:       parentHash.String(),
		Nonce:            "0x0000000000000000",
		Sha3Uncles:       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		LogsBloom:        "0x" + strings.Repeat("0", 512),                                      // Empty logs bloom
		TransactionsRoot: "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421", // Empty merkle root
		StateRoot:        n.statedb.StateRoot.String(),
		ReceiptsRoot:     "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421", // Empty merkle root
		Miner:            "0x0000000000000000000000000000000000000000",
		Difficulty:       "0x0",
		TotalDifficulty:  "0x0",
		ExtraData:        "0x",
		Size:             fmt.Sprintf("0x%x", len(txHashes)*32+200), // Rough estimate
		GasLimit:         "0x1c9c380",                               // 30M gas
		GasUsed:          fmt.Sprintf("0x%x", totalGasUsed),         // Sum of all transaction gas
		Timestamp:        fmt.Sprintf("0x%x", time.Now().Unix()),
		Uncles:           []string{},
	}

	if fullTx {
		// Fetch full transaction objects from storage using txHashes
		transactions := make([]interface{}, 0, len(txHashes))
		blockHashStr := blockHash.String()
		blockNumberStr := fmt.Sprintf("0x%x", blockNumber)

		for i, hash := range txHashes {
			ethTx, err := n.GetTransactionByHash(hash)
			if err != nil {
				log.Warn(log.Node, "convertToEthereumBlock: Failed to get transaction",
					"txHash", hash.String(), "error", err)
				continue // Skip transactions that can't be retrieved
			}
			if ethTx != nil {
				// Set block context for the transaction
				ethTx.BlockHash = &blockHashStr
				ethTx.BlockNumber = &blockNumberStr
				txIndex := fmt.Sprintf("0x%x", i)
				ethTx.TransactionIndex = &txIndex

				transactions = append(transactions, ethTx)
			}
		}
		block.Transactions = transactions
	} else {
		// Return just the transaction hashes
		hashStrings := make([]string, len(txHashes))
		for i, hash := range txHashes {
			hashStrings[i] = hash.String()
		}
		block.Transactions = hashStrings
	}

	return block, nil
}

// resolveBlockNumberToState resolves a block number string to a stateDB
func (n *NodeContent) resolveBlockNumberToState(blockNumberStr string) (*statedb.StateDB, error) {
	// For now, always use the current state
	// TODO: Implement historical state lookup
	if blockNumberStr == "latest" || blockNumberStr == "pending" {
		return n.statedb, nil
	}
	return n.statedb, nil
}

func (n *NodeContent) GetBlockByHash(blockHash common.Hash, fullTx bool) (*EthereumBlock, error) {
	// Get latest block number to determine search range
	latestBlock, err := n.getLatestBlockNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block number: %v", err)
	}

	// Search through blocks to find one with matching hash
	// Start from latest and go backwards (most queries are for recent blocks)
	const maxSearchDepth = 100 // Search last 100 blocks max
	startBlock := uint32(1)
	if latestBlock > maxSearchDepth {
		startBlock = latestBlock - maxSearchDepth
	}

	for blockNum := latestBlock; blockNum >= startBlock; blockNum-- {
		// Calculate the hash for this block number
		computedHash := common.Blake2Hash(binary.LittleEndian.AppendUint32(nil, blockNum))

		if computedHash == blockHash {
			// Found matching block! Read its data and return
			txHashes, err := n.readBlockFromStorage(blockNum)
			if err != nil {
				return nil, fmt.Errorf("failed to read block data: %v", err)
			}

			return n.convertToEthereumBlock(blockNum, txHashes, fullTx)
		}

		if blockNum == 1 {
			break // Avoid underflow
		}
	}

	// Block not found in search range
	return nil, nil
}

// GetBlockByNumber fetches a block by number
func (n *NodeContent) GetBlockByNumber(blockNumberStr string, fullTx bool) (*EthereumBlock, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = n.getLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	case "earliest":
		targetBlockNumber = 1 // Genesis block
	case "pending":
		// For pending, return latest for now
		targetBlockNumber, err = n.getLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	default:
		// Parse hex block number
		if len(blockNumberStr) >= 2 && blockNumberStr[:2] == "0x" {
			blockNum, parseErr := strconv.ParseUint(blockNumberStr[2:], 16, 32)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid block number format: %v", parseErr)
			}
			targetBlockNumber = uint32(blockNum)
		} else {
			return nil, fmt.Errorf("invalid block number format: %s", blockNumberStr)
		}
	}

	// 2. Read the block data from EVM service storage
	blockData, err := n.readBlockFromStorage(targetBlockNumber)
	if err != nil {
		return nil, err
	}

	// 3. Convert to Ethereum block format
	ethBlock, err := n.convertToEthereumBlock(targetBlockNumber, blockData, fullTx)
	if err != nil {
		return nil, fmt.Errorf("failed to convert block to Ethereum format: %v", err)
	}

	return ethBlock, nil
}
