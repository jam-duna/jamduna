package node

import (
	"context"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type JNode interface {
	GetFinalizedBlock() (blk *types.Block, err error)
	SetJCEManager(jceManager *ManualJCEManager) (err error)
	GetJCEManager() (jceManager *ManualJCEManager, err error)
	SubmitAndWaitForWorkPackage(ctx context.Context, wpr *WorkPackageRequest) (common.Hash, error)
	SubmitAndWaitForWorkPackages(ctx context.Context, wpr []*WorkPackageRequest) ([]common.Hash, error)
	SubmitAndWaitForPreimage(ctx context.Context, serviceID uint32, preimage []byte) error
	GetWorkReport(requestedHash common.Hash) (*types.WorkReport, error)
	GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error)
	GetServiceStorage(serviceID uint32, storageKey []byte) ([]byte, bool, error)
	ReadStateWitness(serviceID uint32, objectID common.Hash, fetchPayloadFromDA bool) (types.StateWitness, bool, error)
	GetStateWitnesses(workReports []*types.WorkReport) ([]types.StateWitness, common.Hash, error)
	BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16) (b *types.WorkPackageBundle, wr *types.WorkReport, err error)

	// Ethereum internal methods (called by Jam RPC wrappers)
	// Network Metadata
	GetChainId() uint64
	GetAccounts() []common.Address
	GetGasPrice() uint64

	// Contract State
	GetBalance(address common.Address, blockNumber string) (common.Hash, error)
	GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error)
	GetTransactionCount(address common.Address, blockNumber string) (uint64, error)
	GetCode(address common.Address, blockNumber string) ([]byte, error)

	// Transaction Operations
	EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte) (uint64, error)
	Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string) ([]byte, error)
	SendRawTransaction(signedTxData []byte) (common.Hash, error)

	// Transaction Queries
	GetTransactionReceipt(txHash common.Hash) (*EthereumTransactionReceipt, error)
	GetTransactionByHash(txHash common.Hash) (*EthereumTransactionResponse, error)
	GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]EthereumLog, error)

	// Block Queries
	GetBlockByHash(blockHash common.Hash, fullTx bool) (*EthereumBlock, error)
	GetBlockByNumber(blockNumber string, fullTx bool) (*EthereumBlock, error)
}
