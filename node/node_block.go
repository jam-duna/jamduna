package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
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

func (p *Peer) GetMiddleBlocks(headerhash common.Hash, count uint32, ctx context.Context) ([]types.Block, error) {
	blocksRaw, lastErr := p.SendBlockRequest(ctx, headerhash, 1, count)
	if lastErr != nil {
		return nil, lastErr
	}
	if len(blocksRaw) == 0 {
		return nil, fmt.Errorf("no blocks received")
	}
	return blocksRaw, lastErr
}

func (p *Peer) GetAllBlocks(headerhash common.Hash, ctx context.Context) ([]types.Block, error) {
	blocksRaw, lastErr := p.SendBlockRequest(ctx, headerhash, 1, 1<<32-1)
	if lastErr != nil {
		return nil, lastErr
	}
	if len(blocksRaw) == 0 {
		return nil, fmt.Errorf("no blocks received")
	}
	return blocksRaw, lastErr
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
