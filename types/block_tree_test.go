package types

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/stretchr/testify/assert"
)

func TestBT(t *testing.T) {

	tmp_hash := common.Hash{}
	tmp_hash2 := common.Hash{}
	main_chain := []*Block{}
	sub_chain := []*Block{}
	bt_tree := &BlockTree{}
	for i := 0; i < 10; i++ {
		//fmt.Printf("Adding block %d\n", i)
		if i == 0 {
			blockNode := &BT_Node{
				Parent:   nil,
				Children: nil,
				Block: &Block{
					Header: BlockHeader{
						ParentHeaderHash: tmp_hash,
						Slot:             uint32(i),
					},
				},
				Height:    0,
				Finalized: true,
			}
			bt_tree = NewBlockTree(blockNode)
			main_chain = append(main_chain, blockNode.Block)
			tmp_hash = blockNode.Block.Header.Hash()
		} else {
			var block = &Block{
				Header: BlockHeader{
					ParentHeaderHash: tmp_hash,
					Slot:             uint32(i),
				},
			}
			err := bt_tree.AddBlock(block)
			main_chain = append(main_chain, block)
			if err != nil {
				t.Fatalf("Error adding block: %s", err)
			}
			if i > 5 {
				if i == 6 {
					tmp_hash2 = block.Header.Hash()
				}
				// create a fork from block 5
				var block = &Block{
					Header: BlockHeader{
						ParentHeaderHash: tmp_hash2,
						Slot:             uint32(i),
					},
				}
				sub_chain = append(sub_chain, block)
				err := bt_tree.AddBlock(block)
				if err != nil {
					t.Errorf("Error adding block: %s", err)
				}
				tmp_hash2 = block.Header.Hash()
			}
			tmp_hash = block.Header.Hash()

		}

	}
	//bt_tree.Print()
	// check if the sub chain is a fork
	main9, find := bt_tree.GetBlockNode(main_chain[9].Header.Hash())
	if !find {
		t.Errorf("Block not found")
	}
	//fmt.Printf("Main9: %s\n", main9.Block.Header.Hash().String_short())
	sub_chain3, find := bt_tree.GetBlockNode(sub_chain[3].Header.Hash())
	if !find {
		t.Errorf("Block not found")
	}
	fmt.Print(sub_chain3.Block.Header.Hash().String_short())
	common_ancestor := bt_tree.GetCommonAncestor(main9, sub_chain3)
	if common_ancestor == nil {
		fmt.Printf("Common ancestor not found\n")
	}
	if common_ancestor.Block.Header.Hash() != main_chain[6].Header.Hash() {
		fmt.Printf("Common ancestor is not correct %v\n", common_ancestor.Block.Header.Hash())
		for i, main := range main_chain {
			if main.Header.Hash() == common_ancestor.Block.Header.Hash() {
				fmt.Printf("Found common ancestor %d\n", i)
			}
		}
	}
	// check the length between the common ancestor and the sub chain 3
	height := bt_tree.GetLengthToAncestor(common_ancestor.Block.Header.Hash(), sub_chain3.Block.Header.Hash())
	if height != 4 {
		t.Fatalf("Height is not correct")
	}
	err := bt_tree.FinalizeBlock(main_chain[1].Header.Hash())
	if err != nil {
		t.Fatalf("Error finalizing block: %s", err)
	}
	//fmt.Printf("Finalizing block %s\n", main_chain[1].Header.Hash().String_short())
	err = bt_tree.FinalizeBlock(main_chain[2].Header.Hash())
	if err != nil {
		t.Fatalf("Error finalizing block: %s", err)
	}
	//fmt.Printf("Finalizing block %s\n", main_chain[2].Header.Hash().String_short())

	err = bt_tree.FinalizeBlock(main_chain[3].Header.Hash())
	if err != nil {
		t.Fatalf("Error finalizing block: %s", err)
	}
	//fmt.Printf("Finalizing block %s\n", main_chain[3].Hash().String_short())
	_, children, err := bt_tree.GetDescendingBlocks(main_chain[5].Header.Hash())
	if err != nil {
		t.Fatalf("Error getting descending blocks: %s", err)
	}
	for _, child := range children {
		if false {
			fmt.Printf("descending %s\n", child.String_short())
		}
	}
}

func addChain(blk_tree *BlockTree, merge_point *BT_Node, num int, start_slot uint32) ([]*BT_Node, error) {
	tmp_hash := merge_point.Block.Header.Hash()
	main_chain := []*BT_Node{}
	for i := 0; i < num; i++ {
		var block = &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             start_slot + uint32(i+1),
			},
		}
		err := blk_tree.AddBlock(block)
		if err != nil {
			return nil, fmt.Errorf("Error adding block: %s", err)
		}
		btnode, ok := blk_tree.GetBlockNode(block.Header.Hash())
		if !ok {
			return nil, fmt.Errorf("Block not found")
		}
		main_chain = append(main_chain, btnode)

		tmp_hash = block.Header.Hash()

	}
	return main_chain, nil
}

func TestFindGhost(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	// setup a chain with two forks
	tmp_hash := common.Hash{}
	blockNode := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             0,
			},
		},
		Height:    0,
		Finalized: false,
	}

	bt_tree := NewBlockTree(blockNode)
	chain_a, err := addChain(bt_tree, blockNode, 9, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	node_b, _ := bt_tree.GetBlockNode(chain_a[5].Block.Header.Hash())
	chain_b, err := addChain(bt_tree, node_b, 4, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	chain_a[8].SetVotesWeight(100)
	chain_b[3].SetVotesWeight(100)
	bt_tree.UpdateCumulateVotesWeight(100, 0)
	ghost, err := bt_tree.FindGhost(bt_tree.Root.Block.Header.HeaderHash(), 201)
	if err != nil {
		t.Fatalf("Error finding ghost: %s", err)
	}
	//bt_tree.Print()
	fmt.Printf("Ghost block %s\n", ghost.Block.Header.Hash().String_short())
	assert.Equal(t, ghost.Block.Header.Hash(), chain_a[5].Block.Header.Hash())

}

func TestFindHeaviestChain(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	// setup a chain with two forks
	tmp_hash := common.Hash{}
	blockNode := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             0,
			},
		},
		Height:    0,
		Finalized: false,
	}

	bt_tree := NewBlockTree(blockNode)
	chain_a, err := addChain(bt_tree, blockNode, 9, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	node_b, _ := bt_tree.GetBlockNode(chain_a[5].Block.Header.Hash())
	chain_b, err := addChain(bt_tree, node_b, 4, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	bt_tree.UpdateCumulateVotesWeight(300, 0)
	ghost, err := bt_tree.FindGhost(bt_tree.Root.Block.Header.HeaderHash(), 201)
	if err != nil {
		t.Fatalf("Error finding ghost: %s", err)
	}
	//bt_tree.Print()
	fmt.Printf("Ghost block %s\n", ghost.Block.Header.Hash().String_short())
	assert.Equal(t, ghost.Block.Header.Hash(), chain_b[3].Block.Header.Hash())
}

func TestFindHeaviestChainTwoChain(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	// setup a chain with two forks
	tmp_hash := common.Hash{}
	blockNode := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             0,
			},
		},
		Height:    0,
		Finalized: false,
	}

	bt_tree := NewBlockTree(blockNode)
	_, err := addChain(bt_tree, blockNode, 9, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	chain_b, err := addChain(bt_tree, blockNode, 9, 1)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	bt_tree.UpdateCumulateVotesWeight(300, 0)
	ghost, err := bt_tree.FindGhost(bt_tree.Root.Block.Header.HeaderHash(), 201)
	if err != nil {
		t.Fatalf("Error finding ghost: %s", err)
	}
	//bt_tree.Print()
	fmt.Printf("Ghost block %s\n", ghost.Block.Header.Hash().String_short())
	assert.Equal(t, ghost.Block.Header.Hash(), chain_b[8].Block.Header.Hash())
}

func TestCumulativeWeight(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	/*
		chain a 0-1-2-3-4-5-6-7-8-9
		chain b 6-10-11-12-13
		chain c 5-14-15-16-17
		chain a has weight 1
		chain b has weight 2
		chain c has weight 3
		chain a : 0-1-2-3-4-5-6-7-8-9
		weight  : 1-1-1-1-1-1-1-1-1-1
		chain b : 0-1-2-3-4-5-6-10-11-12-13
		weight  :                2- 2- 2- 2
		chain c : 0-1-2-3-4-5-14-15-16-17
		weight  :              3- 3- 3- 3
		so : should be
		7  cumulative weight:3
		10 cumulative weight:8
		14 cumulative weight:12
		6  cumulative weight: = 3+8+1 = 12
		5  cumulative weight: = 11+12+1 = 25
	*/
	// setup the block tree
	tmp_hash := common.Hash{}
	blockNode := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             0,
			},
		},
		Height:    0,
		Finalized: false,
	}

	bt_tree := NewBlockTree(blockNode)
	basePort := uint16(10000)
	graph_server := NewGraphServer(basePort)
	go graph_server.StartServer()
	graph_server.Update(bt_tree)
	chain_a, err := addChain(bt_tree, blockNode, 9, 100)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	graph_server.Update(bt_tree)
	node_b, _ := bt_tree.GetBlockNode(chain_a[5].Block.Header.Hash())

	chain_b, err := addChain(bt_tree, node_b, 4, 200)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	graph_server.Update(bt_tree)
	node_c, _ := bt_tree.GetBlockNode(chain_a[4].Block.Header.Hash())
	chain_c, err := addChain(bt_tree, node_c, 4, 300)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	// random add some weight to the chain
	chain_a[2].SetVotesWeight(100)
	chain_b[3].SetVotesWeight(100)
	chain_c[2].SetVotesWeight(100)
	bt_tree.UpdateCumulateVotesWeight(300, 0)
	//fmt.Printf("block tree\n")
	//bt_tree.Print()
	assert.Equal(t, uint64(500), chain_a[4].GetCumulativeVotesWeight())
	assert.Equal(t, uint64(600), chain_a[2].GetCumulativeVotesWeight())
	ghost, err := bt_tree.FindGhost(bt_tree.Root.Block.Header.HeaderHash(), 500)
	if err != nil {
		t.Fatalf("Error finding ghost: %s", err)
	}
	fmt.Printf("Ghost block %s\n", ghost.Block.Header.Hash().String_short())
	assert.Equal(t, ghost.Block.Header.Hash(), chain_a[4].Block.Header.Hash())
}

func TestConcatTwoTree(t *testing.T) {
	// setup two tree
	// the second tree root is the child of the first tree leaf
	tmp_hash := common.Hash{}
	blockNode := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: tmp_hash,
				Slot:             0,
			},
		},
		Height:    0,
		Finalized: false,
	}

	bt_tree := NewBlockTree(blockNode)
	_, err := addChain(bt_tree, blockNode, 9, 0)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	//bt_tree.Print()

	//get the leaf of the first tree
	leaves_1 := bt_tree.GetLeafs()
	leaf_1 := &BT_Node{}
	if len(leaves_1) != 1 {
		t.Fatalf("Error getting leaves")
	} else {
		for _, leaf := range leaves_1 {
			leaf_1 = leaf
		}
	}
	// create a second tree
	concat_point_block := leaf_1.Block
	blockNode2 := &BT_Node{
		Parent:   nil,
		Children: nil,
		Block: &Block{
			Header: BlockHeader{
				ParentHeaderHash: concat_point_block.Header.Hash(),
				Slot:             10000,
			},
		},
		Height:    0,
		Finalized: false,
	}
	bt_tree2 := NewBlockTree(blockNode2)

	_, err = addChain(bt_tree2, blockNode2, 9, 10000)
	if err != nil {
		t.Fatalf("Error adding chain: %s", err)
	}
	//bt_tree2.Print()
	// concat the two tree
	new_block_tree, err := MergeTwoBlockTree(bt_tree, bt_tree2)
	if err != nil {
		t.Fatalf("Error merging two tree: %s", err)
	}
	new_block_tree.UpdateHeight()
	//new_block_tree.Print()

}
