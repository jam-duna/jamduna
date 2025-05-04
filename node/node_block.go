package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func (p *Peer) GetOneBlock(headerHash common.Hash, ctx context.Context) ([]types.Block, error) {
	blocksRaw, lastErr := p.SendBlockRequest(ctx, headerHash, 1, 1)
	if lastErr != nil {
		return nil, lastErr
	}
	if len(blocksRaw) == 0 {
		return nil, fmt.Errorf("no blocks received")
	}
	return blocksRaw, lastErr
}

const maxBlockCount = 100

func (p *Peer) GetMultiBlocks(latest_genesis_headerhash common.Hash, ctx context.Context) ([]types.Block, error) {
	var currentHash common.Hash
	currentHash = latest_genesis_headerhash
	blocks := make([]types.Block, 0)
	for {
		block_ctx, blockCancel := context.WithTimeout(ctx, MediumTimeout)
		blocksRaw, lastErr := p.SendBlockRequest(block_ctx, currentHash, 0, maxBlockCount)
		if lastErr != nil {
			return nil, lastErr
		}
		blocks = append(blocks, blocksRaw...)
		if len(blocksRaw) < maxBlockCount {
			break
		}
		blockCancel()
		log.Info(debugBlock, "GetMultiBlocks", "blocksRaw", len(blocksRaw), "currentHash", currentHash)
		currentHash = blocksRaw[len(blocksRaw)-1].Header.Hash()
	}
	return blocks, nil
}

func (n *Node) CanProposeFirstBlock() bool {
	is_ok := true
	for _, peer := range n.peersInfo {
		block, err := peer.SendBlockRequest(context.Background(), genesisBlockHash, 0, 1)
		if err != nil {
			continue
		}
		if len(block) != 0 {
			is_ok = false
			break
		}
	}
	return is_ok
}
