package node

import (
	"container/list"
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) InsertOrphan(block *types.Block) {
	fmt.Printf("InsertOrphan: %s\n", block.Header.Hash().Hex())
	if n.block_waiting.Len() == 0 {
		new_list := list.New()
		new_list.PushBack(block)
		n.block_waiting.PushBack(new_list)
	} else {
		// for loop the block_waiting list
		for e := n.block_waiting.Front(); e != nil; e = e.Next() {
			tail_block := e.Value.(*list.List).Back().Value.(*types.Block)
			if block.Header.ParentHeaderHash == tail_block.Header.Hash() {
				e.Value.(*list.List).PushBack(block)
				return
			}
		}
		// if not found, create a new list
		new_list := list.New()
		new_list.PushBack(block)
		n.block_waiting.PushBack(new_list)
	}
}
func (n *Node) SynchronizedBlocks(ctx context.Context) {
	for e := n.block_waiting.Front(); e != nil; e = e.Next() {
		// Respect cancellation early
		select {
		case <-ctx.Done():
			log.Info(blk, "SynchronizedBlocks: context canceled, exiting sync loop")
			return
		default:
		}

		blockList, ok := e.Value.(*list.List)
		if !ok || blockList.Len() == 0 {
			continue
		}

		tailBlock := blockList.Back().Value.(*types.Block)
		slotTail := tailBlock.Header.Slot
		root := n.block_tree.GetRoot()
		slotRoot := root.Block.Header.Slot

		diffs := slotRoot - slotTail
		if diffs >= 1 {
			// Randomly select a peer (defensively)
			if len(n.peersInfo) == 0 {
				log.Warn(blk, "SynchronizedBlocks: no peers available to request blocks")
				continue
			}
			var selectedPeer *Peer
			for _, p := range n.peersInfo {
				selectedPeer = p
				break
			}

			if selectedPeer == nil {
				log.Warn(blk, "SynchronizedBlocks: peer selection failed")
				continue
			}

			blocksRaw, err := selectedPeer.SendBlockRequest(ctx, root.Block.Header.ParentHeaderHash, 1, diffs)
			if err != nil {
				log.Error(blk, "SendBlockRequest failed", "peer", selectedPeer.String(), "err", err)
				continue
			}

			for _, newBlock := range blocksRaw {
				frontBlock := blockList.Front().Value.(*types.Block)
				if frontBlock.Header.ParentHeaderHash == newBlock.Header.Hash() {
					blockList.PushFront(newBlock)
				} else {
					log.Warn(blk, "SynchronizedBlocks: mismatched parent", "expected", frontBlock.Header.ParentHeaderHash, "got", newBlock.Header.Hash())
				}
			}

			// Add all blocks to the block tree
			for blockElem := blockList.Front(); blockElem != nil; blockElem = blockElem.Next() {
				block := blockElem.Value.(*types.Block)
				if err := n.block_tree.AddBlock(block); err != nil {
					log.Error(blk, "AddBlock failed", "block", block.Header.Hash(), "err", err)
				}
			}
		} else {
			// Just add the tail block
			if err := n.block_tree.AddBlock(tailBlock); err != nil {
				log.Error(blk, "AddBlock failed", "block", tailBlock.Header.Hash(), "err", err)
			}
		}
	}
}
