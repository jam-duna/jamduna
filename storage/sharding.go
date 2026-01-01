package storage

import (
	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
)

type ShardedTxPool struct {
	txs     map[common.Hash]evmtypes.EthereumTransaction // txHash => tx
	sharded map[uint32]map[common.Hash]bool              // shardID => txHash => included
	digests map[uint32]common.Hash                       // shardID => digest
}
