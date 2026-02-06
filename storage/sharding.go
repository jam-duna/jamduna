package storage

import (
	"github.com/jam-duna/jamduna/common"
	evmtypes "github.com/jam-duna/jamduna/types"
)

type ShardedTxPool struct {
	txs     map[common.Hash]evmtypes.EthereumTransaction // txHash => tx
	sharded map[uint32]map[common.Hash]bool              // shardID => txHash => included
	digests map[uint32]common.Hash                       // shardID => digest
}
