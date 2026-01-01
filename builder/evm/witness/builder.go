// Package witness provides EVM rollup builder functionality for JAM services
package witness

import (
	"fmt"
	"math/big"

	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	evmverkle "github.com/colorfulnotion/jam/builder/evm/verkle"
	"github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-verkle"
)

// EVMBuilder provides builder-specific EVM operations for rollup processing
type EVMBuilder struct {
	serviceID uint32
	storage   evmverkle.VerkleStateStorage
	jamClient types.NodeClient // CE146/147/148 interface to JAM network
}

// NewEVMBuilder creates a new EVM builder instance
func NewEVMBuilder(serviceID uint32, storage evmverkle.VerkleStateStorage, jamClient types.NodeClient) *EVMBuilder {
	return &EVMBuilder{
		serviceID: serviceID,
		storage:   storage,
		jamClient: jamClient,
	}
}

// GetServiceID returns the service ID for this builder
func (b *EVMBuilder) GetServiceID() uint32 {
	return b.serviceID
}

// BuildBundle executes a batch of transactions and generates a work package
func (b *EVMBuilder) BuildBundle(txs []evmtypes.EthereumTransaction) (*types.WorkPackage, error) {
	// Get current EVM service state
	evmService, ok, err := b.jamClient.GetService(b.serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM service: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("EVM service not found")
	}

	// Create work package template
	workPackage := types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationCodeHash: common.Hash{}, // Will be set by caller
		AuthorizationToken:    nil,
		ConfigurationBlob:     nil,
		RefineContext:         types.RefineContext{},
		WorkItems: []types.WorkItem{
			{
				Service:            b.serviceID,
				CodeHash:           evmService.CodeHash,
				RefineGasLimit:     types.RefineGasAllocation,
				AccumulateGasLimit: types.AccumulationGasAllocation,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0,
			},
		},
	}

	// Convert transactions to extrinsics
	var extrinsics []types.WorkItemExtrinsic
	var extrinsicsBlobs []types.ExtrinsicsBlobs
	var extrinsicDataArray [][]byte

	for _, tx := range txs {
		// Convert Ethereum transaction to JAM extrinsic format
		extrinsicData, err := b.convertEthTxToExtrinsic(tx)
		if err != nil {
			return nil, fmt.Errorf("failed to convert transaction to extrinsic: %v", err)
		}

		txHash := common.Blake2Hash(extrinsicData)

		extrinsics = append(extrinsics, types.WorkItemExtrinsic{
			Hash: txHash,
			Len:  uint32(len(extrinsicData)),
		})

		extrinsicDataArray = append(extrinsicDataArray, extrinsicData)
	}

	// Set extrinsics and payload
	workPackage.WorkItems[0].Extrinsics = extrinsics

	// Create payload for transaction processing
	globalDepth, err := b.jamClient.ReadGlobalDepth(evmService.ServiceIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to read global depth: %v", err)
	}

	workPackage.WorkItems[0].Payload = b.buildPayload(len(txs), globalDepth, 0, common.Hash{})

	// Package extrinsics data
	extrinsicsBlobs = append(extrinsicsBlobs, extrinsicDataArray)

	// Execute bundle to generate witness
	_, workReport, err := b.jamClient.BuildBundle(workPackage, extrinsicsBlobs, 0, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to execute bundle: %v", err)
	}

	if workReport == nil {
		return nil, fmt.Errorf("build bundle returned nil work report")
	}

	return &workPackage, nil
}

// GetBalance retrieves account balance from Verkle storage
func (b *EVMBuilder) GetBalance(address common.Address, tree verkle.VerkleNode) (common.Hash, error) {
	return b.storage.GetBalance(tree, address)
}

// GetNonce retrieves account nonce from Verkle storage
func (b *EVMBuilder) GetNonce(address common.Address, tree verkle.VerkleNode) (uint64, error) {
	return b.storage.GetNonce(tree, address)
}

// GetCode retrieves contract code from Verkle storage
func (b *EVMBuilder) GetCode(address common.Address, tree verkle.VerkleNode) ([]byte, error) {
	return b.storage.GetCode(tree, address)
}

// GetStorageAt retrieves contract storage value from Verkle storage
func (b *EVMBuilder) GetStorageAt(address common.Address, key common.Hash, tree verkle.VerkleNode) (common.Hash, error) {
	return b.storage.GetStorageAt(tree, address, key)
}

// GenerateWitness creates a witness for the specified keys
func (b *EVMBuilder) GenerateWitness(tree verkle.VerkleNode, keys [][]byte) ([]byte, error) {
	return b.storage.GenerateWitness(tree, keys)
}

// convertEthTxToExtrinsic converts an Ethereum transaction to JAM extrinsic format
func (b *EVMBuilder) convertEthTxToExtrinsic(tx evmtypes.EthereumTransaction) ([]byte, error) {
	// JAM extrinsic format: caller(20) + target(20) + gas_limit(32) + gas_price(32) + value(32) + call_kind(4) + data_len(8) + data
	dataLen := len(tx.Data)
	extrinsicSize := 148 + dataLen // 20+20+32+32+32+4+8 + data
	extrinsic := make([]byte, extrinsicSize)

	offset := 0

	// caller (20 bytes)
	copy(extrinsic[offset:offset+20], tx.From.Bytes())
	offset += 20

	// target (20 bytes) - use zero address for contract creation
	if tx.To != nil {
		copy(extrinsic[offset:offset+20], tx.To.Bytes())
	} else {
		// Contract creation - use zero address
		copy(extrinsic[offset:offset+20], make([]byte, 20))
	}
	offset += 20

	// gas_limit (32 bytes, big-endian)
	gasLimitBig := new(big.Int).SetUint64(tx.Gas)
	copy(extrinsic[offset:offset+32], gasLimitBig.FillBytes(make([]byte, 32)))
	offset += 32

	// gas_price (32 bytes, big-endian)
	copy(extrinsic[offset:offset+32], tx.GasPrice.FillBytes(make([]byte, 32)))
	offset += 32

	// value (32 bytes, big-endian)
	copy(extrinsic[offset:offset+32], tx.Value.FillBytes(make([]byte, 32)))
	offset += 32

	// call_kind (4 bytes, little-endian) - 0 = CALL, 1 = CREATE
	callKind := uint32(0) // CALL
	if tx.To == nil {
		callKind = 1 // CREATE
	}
	copy(extrinsic[offset:offset+4], []byte{byte(callKind), 0, 0, 0})
	offset += 4

	// data_len (8 bytes, little-endian)
	dataLenBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		dataLenBytes[i] = byte(dataLen >> (i * 8))
	}
	copy(extrinsic[offset:offset+8], dataLenBytes)
	offset += 8

	// data
	copy(extrinsic[offset:], tx.Data)

	return extrinsic, nil
}

// buildPayload constructs a payload byte array for transaction processing
func (b *EVMBuilder) buildPayload(count int, globalDepth uint8, numWitnesses int, blockAccessListHash common.Hash) []byte {
	payload := make([]byte, 40) // 1 + 4 + 1 + 2 + 32 = 40 bytes
	payload[0] = 0x01 // PayloadTypeTransactions

	// count (4 bytes, little-endian)
	for i := 0; i < 4; i++ {
		payload[1+i] = byte(count >> (i * 8))
	}

	payload[5] = globalDepth

	// numWitnesses (2 bytes, little-endian)
	payload[6] = byte(numWitnesses)
	payload[7] = byte(numWitnesses >> 8)

	copy(payload[8:40], blockAccessListHash[:])
	return payload
}