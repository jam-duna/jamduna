package types

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/xlab/treeprint"
)

type BT_Node struct {
	Parent                 *BT_Node
	Children               []*BT_Node
	Block                  *Block
	Height                 int
	VotesWeight            uint64
	Cumulative_VotseWeight uint64
	Finalized              bool
}

func (root *BT_Node) Copy() (*BT_Node, map[common.Hash]*BT_Node, map[common.Hash]*BT_Node) {

	newRoot := &BT_Node{
		Parent:                 nil,
		Children:               make([]*BT_Node, 0),
		Block:                  root.Block.Copy(),
		Height:                 root.Height,
		VotesWeight:            root.VotesWeight,
		Cumulative_VotseWeight: root.Cumulative_VotseWeight,
		Finalized:              root.Finalized,
	}
	nodeMap := make(map[common.Hash]*BT_Node)
	nodeMap[root.Block.Header.Hash()] = newRoot
	leafMap := make(map[common.Hash]*BT_Node)
	var traverse func(*BT_Node, *BT_Node)
	traverse = func(oldNode, newParent *BT_Node) {
		for _, child := range oldNode.Children {
			newChild := &BT_Node{
				Parent:                 newParent,
				Children:               make([]*BT_Node, 0),
				Block:                  child.Block.Copy(),
				Height:                 child.Height,
				VotesWeight:            0,
				Cumulative_VotseWeight: 0,
				Finalized:              child.Finalized,
			}
			newParent.Children = append(newParent.Children, newChild)
			nodeMap[child.Block.Header.Hash()] = newChild
			if len(newParent.Children) == 0 {
				leafMap[child.Block.Header.Hash()] = newChild
			} else {
				delete(leafMap, child.Block.Header.Hash())
			}
			traverse(child, newChild)
		}
	}

	traverse(root, newRoot)
	return newRoot, nodeMap, leafMap
}

func (node *BT_Node) Print() {
	fmt.Printf("Height: %d, Hash: %s, Parent: %s, vote weight: %v, cumulative :%d, finalized :%v\n", node.Height, node.Block.Header.Hash().String_short(), node.Block.Header.ParentHeaderHash.String_short(), node.VotesWeight, node.Cumulative_VotseWeight, node.Finalized)
	fmt.Printf("Children: ")
	for _, child := range node.Children {
		fmt.Printf("%s / ", child.Block.Header.Hash().String_short())
	}
	fmt.Println()
	for _, child := range node.Children {
		child.Print()
	}
}

func (node *BT_Node) AddChild(newBlock *Block) *BT_Node {
	child := &BT_Node{
		Parent:    node,
		Children:  make([]*BT_Node, 0),
		Block:     newBlock,
		Height:    node.Height + 1,
		Finalized: false,
	}
	node.Children = append(node.Children, child)
	return child
}

func (node *BT_Node) IsLeaf() bool {
	return len(node.Children) == 0
}

func (node *BT_Node) SetVotesWeight(weight uint64) {
	node.VotesWeight = weight
}

func (node *BT_Node) GetVotesWeight() uint64 {
	return node.VotesWeight
}

func (node *BT_Node) GetCumulativeVotesWeight() uint64 {
	return node.Cumulative_VotseWeight
}

// it will check if the input is the ancestor of the node
func (btn *BT_Node) IsAncestor(ancestor *BT_Node) bool {
	if btn == ancestor {
		return false
	}
	btn = btn.Parent
	for btn != nil {
		if btn == ancestor {
			return true
		}
		btn = btn.Parent
	}
	return false
}

func (node *BT_Node) IsFinalized() bool {
	return node.Finalized
}

type BlockTree struct {
	Root    *BT_Node                 // currently this in genesis node
	Leafs   map[common.Hash]*BT_Node // this is the block that has no children
	TreeMap map[common.Hash]*BT_Node // all the blocks in the tree
	Mutex   sync.RWMutex
}

func (node *BT_Node) ToTree() treeprint.Tree {
	tree := treeprint.New()
	tree.SetValue(fmt.Sprintf(
		"\033[1;34mHash: %s\033[0m, \033[1;32mHeight: %d\033[0m, \033[1;33mParent: %s\033[0m, \033[1;35mvote weight: %v\033[0m, \033[1;36mcumulative: %d\033[0m, \033[1;37mFinalized: %v\033[0m",
		node.Block.Header.Hash().String_short(),
		node.Height,
		node.Block.Header.ParentHeaderHash.String_short(),
		node.VotesWeight,
		node.Cumulative_VotseWeight,
		node.Finalized,
	))
	for _, child := range node.Children {
		childTree := child.ToTree()
		tree.AddNode(childTree.String())
	}

	return tree
}

func (bt *BlockTree) Print() {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	if bt.Root == nil {
		fmt.Println("Tree is empty.")
		return
	}

	tree := bt.Root.ToTree()

	fmt.Println(tree.String())
}

// this function is used to create a new block tree
func NewBlockTree(root *BT_Node) *BlockTree {
	return &BlockTree{
		Root:    root,
		TreeMap: map[common.Hash]*BT_Node{root.Block.Header.Hash(): root},
		Leafs:   map[common.Hash]*BT_Node{root.Block.Header.Hash(): root},
	}
}

func (bt *BlockTree) Copy() *BlockTree {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	newRoot, nodeMap, newLeaf := bt.Root.Copy()
	newTree := NewBlockTree(newRoot)
	newTree.TreeMap = nodeMap
	newTree.Leafs = newLeaf
	return newTree
}

// this function is used to add a block to the block tree
func (bt *BlockTree) AddBlock(newBlock *Block) error {
	bt.Mutex.Lock()
	defer bt.Mutex.Unlock()

	parentNode, ok := bt.TreeMap[newBlock.GetParentHeaderHash()]
	if !ok {
		return fmt.Errorf("parent of block %v not found", newBlock.Header.Hash())
	}
	_, ok = bt.TreeMap[newBlock.Header.Hash()]
	if ok {
		return fmt.Errorf("block %v already exists", newBlock.Header.Hash())
	}
	childNode := parentNode.AddChild(newBlock)
	bt.TreeMap[newBlock.Header.Hash()] = childNode
	if _, ok := bt.Leafs[newBlock.Header.Hash()]; !ok {
		bt.Leafs[newBlock.Header.Hash()] = childNode
		if _, ok := bt.Leafs[parentNode.Block.Header.Hash()]; ok {
			delete(bt.Leafs, parentNode.Block.Header.Hash())
		}
	}
	return nil
}
func (bt *BlockTree) Prune(newRoot *BT_Node) {
	bt.Mutex.Lock()
	defer bt.Mutex.Unlock()

	// Check if the newRoot is valid and if TreeMap is initialized
	if newRoot == nil || bt.TreeMap == nil {
		return
	}

	// Store old TreeMap and Leafs for cleanup
	oldTreeMap := bt.TreeMap

	// Initialize new TreeMap and Leafs
	bt.TreeMap = make(map[common.Hash]*BT_Node)
	bt.Leafs = make(map[common.Hash]*BT_Node)

	// Recursive function to rebuild the TreeMap and Leafs from the new root
	var dfs func(node *BT_Node)
	dfs = func(node *BT_Node) {
		if node == nil {
			return
		}

		// Calculate the hash for the current node (needs implementation)
		nodeHash := node.Block.Header.Hash()

		// Add the current node to the new TreeMap
		bt.TreeMap[nodeHash] = node

		// If the node has no children, add it to Leafs
		if len(node.Children) == 0 {
			bt.Leafs[nodeHash] = node
		}

		// Recursively process children
		for _, child := range node.Children {
			dfs(child)
		}
	}

	// Start rebuilding the tree from the new root
	dfs(newRoot)

	// Update the root of the BlockTree
	bt.Root = newRoot
	bt.Root.Parent = nil
	// Cleanup unused nodes to free memory
	for hash, node := range oldTreeMap {
		if _, exists := bt.TreeMap[hash]; !exists {
			// Remove references to help with memory cleanup
			node.Parent = nil
			node.Children = nil
			node.Block = nil
		}
	}
}

// this function is used to get the block node by hash
func (bt *BlockTree) GetBlockNode(hash common.Hash) (*BT_Node, bool) {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	node, ok := bt.TreeMap[hash]
	return node, ok
}

func (bt *BlockTree) GetBlockHashes() []common.Hash {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	block_hashes := make([]common.Hash, 0)
	for hash := range bt.TreeMap {
		block_hashes = append(block_hashes, hash)
	}
	return block_hashes
}

// this function is used to get the height of the block
func (bt *BlockTree) GetBlockHeight(hash common.Hash) int {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	node, ok := bt.TreeMap[hash]
	if !ok {
		return -1
	}
	return node.Height
}

func (bt *BlockTree) GetBlockVotesWeight(hash common.Hash) uint64 {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	node, ok := bt.TreeMap[hash]
	if !ok {
		return 0
	}
	return node.VotesWeight
}

// UpdateCumulateVotesWeight updates the cumulative votes weight for the block tree
func (bt *BlockTree) UpdateCumulateVotesWeight(unvoted_weight uint64, eq_staked uint64) {
	bt.Mutex.Lock()
	defer bt.Mutex.Unlock()
	bt.Root.UpdateNodeCumulativeVotesWeight()
	if unvoted_weight > 0 {
		for _, block := range bt.TreeMap {
			block.Cumulative_VotseWeight += unvoted_weight
		}
	}

	if eq_staked > 0 {
		for _, block := range bt.TreeMap {
			block.Cumulative_VotseWeight += eq_staked
		}
	}

}

func (node *BT_Node) UpdateNodeCumulativeVotesWeight() {
	node.Cumulative_VotseWeight = node.VotesWeight
	for _, child := range node.Children {
		child.UpdateNodeCumulativeVotesWeight()
		node.Cumulative_VotseWeight += child.Cumulative_VotseWeight
	}
}

// this function is used to get the common ancestor of two nodes
func (bt *BlockTree) GetCommonAncestor(node1, node2 *BT_Node) *BT_Node {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	visited := make(map[*BT_Node]bool)
	for node1 != nil {
		visited[node1] = true
		node1 = node1.Parent
	}
	for node2 != nil {
		if visited[node2] {
			return node2
		}
		node2 = node2.Parent
	}
	return nil
}

// this function is used to get the length of the chain from the hash to one of the ancestor
func (bt *BlockTree) GetLengthToAncestor(ancestor common.Hash, hash common.Hash) int {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	node, ok := bt.TreeMap[hash]
	if !ok {
		return -1
	}
	ancestorNode, ok := bt.TreeMap[ancestor]
	if !ok {
		return -1
	}
	if node.IsAncestor(ancestorNode) {
		return node.Height - ancestorNode.Height
	}
	return -1
}

func (bt *BlockTree) FinalizeBlock(hash common.Hash) error {
	bt.Mutex.Lock()
	defer bt.Mutex.Unlock()

	node, ok := bt.TreeMap[hash]
	if !ok {
		return fmt.Errorf("block %s not found", hash.String())
	}
	if node.Finalized {
		return fmt.Errorf("block %s already finalized", hash.String())
	}
	for !node.Finalized {
		node.Finalized = true
		node = node.Parent
	}

	// fmt.Printf("block %s finalized\n", hash.String_short())
	return nil
}

func (bt *BlockTree) GetLastFinalizedBlock() *BT_Node {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	//if the block get finalized, it will be a chain
	//so we can start from the root node
	node := bt.Root
	finalize := true
	for finalize {
		finalize = false
		for _, child := range node.Children {
			if child.Finalized {
				node = child
				finalize = true
				break
			}
		}
	}
	return node
}

func (bt *BlockTree) GetDescendingBlocks(blockHash common.Hash) ([]*BT_Node, []common.Hash, error) {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	startNode, ok := bt.TreeMap[blockHash]
	if !ok {
		return nil, nil, fmt.Errorf("block %s not found", blockHash.String())
	}

	var result []*BT_Node
	var blockHashes []common.Hash
	var traverse func(node *BT_Node)
	traverse = func(node *BT_Node) {
		for _, child := range node.Children {
			result = append(result, child)
			blockHashes = append(blockHashes, child.Block.Header.Hash())
			traverse(child)
		}
	}
	traverse(startNode)
	return result, blockHashes, nil
}

func (bt *BlockTree) GetAncestors(blockHash common.Hash) ([]*BT_Node, []common.Hash, error) {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	startNode, ok := bt.TreeMap[blockHash]
	if !ok {
		return nil, nil, fmt.Errorf("block %s not found", blockHash.String())
	}

	var result []*BT_Node
	var blockHashes []common.Hash
	node := startNode
	for node.Parent != nil {
		result = append(result, node.Parent)
		blockHashes = append(blockHashes, node.Parent.Block.Header.Hash())
		node = node.Parent
	}
	return result, blockHashes, nil
}

// B'>=B? return true
func (bt *BlockTree) ChildOrBrother(node1 *BT_Node, node2 *BT_Node) bool {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()
	if node1.Parent == node2.Parent || node1 == node2 {
		return true
	}
	// see if node2 is the ancestor of node1
	if node1.IsAncestor(node2) {
		return true
	}
	return false
}

// FindGhost finds the GHOST (Greediest Heaviest Observed Subtree) node.
// It starts from the root or a given node and traverses the tree to find
// the heaviest subtree that satisfies the provided condition.
func (bt *BlockTree) FindGhost(startHash common.Hash, threshold uint64) (*BT_Node, error) {
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	startNode, ok := bt.TreeMap[startHash]
	if !ok {
		return nil, fmt.Errorf("start block %s not found", startHash.String())
	}

	// Check if the starting node satisfies the condition.
	if startNode.Cumulative_VotseWeight < threshold {
		return nil, fmt.Errorf("start block %s does not satisfy the threshold. Cumulative_VotseWeight %d threshold %d", startHash.String(), startNode.Cumulative_VotseWeight, threshold)
	}

	// BFS setup
	type queueEntry struct {
		node  *BT_Node
		depth int
	}

	queue := []queueEntry{{node: startNode, depth: 0}}
	var resultNode *BT_Node
	maxDepth := -1
	maxSlot := uint32(0) // Track the maximum Slot value when depths are equal

	for len(queue) > 0 {
		// Dequeue
		entry := queue[0]
		queue = queue[1:]

		currentNode := entry.node
		currentDepth := entry.depth

		// Check if the current node satisfies the condition
		if currentNode.Cumulative_VotseWeight >= threshold {
			if currentDepth > maxDepth || (currentDepth == maxDepth && currentNode.Block.TimeSlot() > maxSlot) {
				resultNode = currentNode
				maxDepth = currentDepth
				maxSlot = currentNode.Block.TimeSlot()
			}
		}

		// Enqueue children
		for _, child := range currentNode.Children {
			if child.Cumulative_VotseWeight >= threshold {
				queue = append(queue, queueEntry{node: child, depth: currentDepth + 1})
			}
		}
	}

	if resultNode == nil {
		return nil, fmt.Errorf("no suitable node found satisfying the threshold")
	}

	return resultNode, nil
}
