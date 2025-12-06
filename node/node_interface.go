package node

import (
	"context"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
)

type JNode interface {
	GetFinalizedBlock() (blk *types.Block, err error)
	SetJCEManager(jceManager *ManualJCEManager) (err error)
	GetJCEManager() (jceManager *ManualJCEManager, err error)
	SubmitAndWaitForWorkPackageBundle(ctx context.Context, b *types.WorkPackageBundle) (common.Hash, error)
	SubmitAndWaitForWorkPackageBundles(ctx context.Context, b []*types.WorkPackageBundle) ([]common.Hash, error)
	SubmitAndWaitForPreimage(ctx context.Context, serviceID uint32, preimage []byte) error
	GetWorkReport(requestedHash common.Hash) (*types.WorkReport, error)
	GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error)
	GetServiceStorage(serviceID uint32, storageKey []byte) ([]byte, bool, error)

	GetRefineContext() (types.RefineContext, error)
	BuildBundle(types.WorkPackage, []types.ExtrinsicsBlobs, uint16, []common.Hash) (*types.WorkPackageBundle, *types.WorkReport, error)

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
	ReadGlobalDepth(serviceID uint32) (uint8, error)

	// Transaction Operations
	EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte) (uint64, error)
	Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string) ([]byte, error)
	SendRawTransaction(signedTxData []byte) (common.Hash, error)

	// Transaction Queries
	GetTransactionReceipt(txHash common.Hash) (*evmtypes.EthereumTransactionReceipt, error)
	GetTransactionByHash(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error)
	GetTransactionByBlockHashAndIndex(serviceID uint32, blockHash common.Hash, index uint32) (*evmtypes.EthereumTransactionResponse, error)
	GetTransactionByBlockNumberAndIndex(serviceID uint32, blockNumber string, index uint32) (*evmtypes.EthereumTransactionResponse, error)
	GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error)

	// Block Queries
	GetLatestBlockNumber() (uint32, error)
	GetBlockByHash(blockHash common.Hash, fullTx bool) (*evmtypes.EthereumBlock, error)
	GetBlockByNumber(blockNumber string, fullTx bool) (*evmtypes.EthereumBlock, error)
}
