package node

import (
	"container/list"
	"fmt"
	rand0 "math/rand"

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

func (n *Node) SyncornizedBlocks() {
	for e := n.block_waiting.Front(); e != nil; e = e.Next() {
		tail_block := e.Value.(*list.List).Back().Value.(*types.Block)
		slot1 := tail_block.Header.Slot
		root := n.block_tree.GetRoot()
		slot2 := root.Block.Header.Slot

		diffs := slot2 - slot1
		if diffs >= 1 {
			// send the request to the selected peer
			// get the blocks from the selected peer
			selected_peer := n.peersInfo[uint16(rand0.Intn(types.TotalValidators))]
			blocksRaw, err := selected_peer.SendBlockRequest(root.Block.Header.ParentHeaderHash, 1, diffs)
			if err != nil {
				log.Error(blk, "SendBlockRequest", "err", err)
			}
			for i := 0; i < len(blocksRaw); i++ {
				front_block := e.Value.(*list.List).Front().Value.(*types.Block)
				if front_block.Header.ParentHeaderHash == blocksRaw[i].Header.Hash() {
					e.Value.(*list.List).PushFront(blocksRaw[i])
				} else {
					log.Error(blk, "SyncornizedBlocks", "err", "block not found")
				}
			}
			block_list := e.Value.(*list.List)
			for e := block_list.Front(); e != nil; e = e.Next() {
				block := e.Value.(*types.Block)
				err := n.block_tree.AddBlock(block)
				if err != nil {
					log.Error(blk, "AddBlock", "err", err)
				}
			}
		} else {
			err := n.block_tree.AddBlock(tail_block)
			if err != nil {
				log.Error(blk, "AddBlock", "err", err)
			}
		}
	}
}
