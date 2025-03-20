package trie

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

//"golang.org/x/crypto/blake2b"

var H0 = make([]byte, 32)
var H0Hash = common.BytesToHash(H0)
var PageFixedDepth = 6
var debugCDT = false

// CDT Node structure represents a node in the CDT
type CDTNode struct {
	Hash  []byte
	Value []byte
	Left  *CDTNode
	Right *CDTNode
}

// CDMerkleTree represents the Merkle Tree structure
type CDMerkleTree struct {
	root   *CDTNode
	leaves []*CDTNode
}

// NewCDMerkleTree creates a new constant-depth Merkle tree
func NewCDMerkleTree(values [][]byte) *CDMerkleTree {
	// If there are no leaves, return a tree with a hash of 0
	if len(values) == 0 {
		return &CDMerkleTree{
			root: &CDTNode{Hash: H0},
		}
	}

	// Padding leaves to the next power of 2
	// Equation(E.7) in GP 0.6.2
	paddedLeaves := values
	if len(values) != 1 {
		paddedLeaves = padLeaves(values)
	}

	// Creating leaf nodes
	var leaves []*CDTNode
	for _, value := range paddedLeaves {
		if !compareBytes(value, H0) {
			leaves = append(leaves, &CDTNode{Hash: computeLeaf(value), Value: value})
		} else {
			leaves = append(leaves, &CDTNode{Hash: H0, Value: H0})
		}
	}

	// Build the tree recursively
	root := buildTree(leaves)

	return &CDMerkleTree{
		root:   root,
		leaves: leaves,
	}
}

// padLeaves pads the leaves to the next power of 2
func padLeaves(values [][]byte) [][]byte {
	n := len(values)
	nextPowerOfTwo := 1
	for nextPowerOfTwo < n {
		nextPowerOfTwo <<= 1
	}
	for len(values) < nextPowerOfTwo {
		values = append(values, H0)
	}
	return values
}

func ComputePageIdx(exportedSegmentCnt int, index int) (pgIdx int, isFixedPadding bool) {
	pageSize := 1 << PageFixedDepth
	pgIdx = index / pageSize
	if exportedSegmentCnt > pageSize {
		isFixedPadding = true
	}
	return pgIdx, isFixedPadding
}

func padLeafHashes(nonEmptyleafHashes []common.Hash, isFixedPadding bool) []common.Hash {
	leafHashes := make([]common.Hash, len(nonEmptyleafHashes))
	copy(leafHashes, nonEmptyleafHashes)
	fixedSize := 1 << PageFixedDepth
	if isFixedPadding {
		for len(leafHashes) < fixedSize {
			leafHashes = append(leafHashes, H0Hash)
		}
		return leafHashes
	}
	n := len(leafHashes)
	nextPowerOfTwo := 1
	for nextPowerOfTwo < n {
		nextPowerOfTwo <<= 1
	}
	for len(leafHashes) < nextPowerOfTwo {
		leafHashes = append(leafHashes, H0Hash)
	}
	return leafHashes
}

func NewCDTSubtree(nonEmptyleafHashes []common.Hash, isFixedPadding bool) *CDMerkleTree {
	if len(nonEmptyleafHashes) == 0 {
		return &CDMerkleTree{
			root: &CDTNode{Hash: H0},
		}
	}
	paddedLeafHashes := padLeafHashes(nonEmptyleafHashes, isFixedPadding)

	// Creating leaf nodes WITHOUT hashing & value
	var leaves []*CDTNode
	for _, leafHash := range paddedLeafHashes {
		leaves = append(leaves, &CDTNode{Hash: leafHash.Bytes()})
	}

	// Build the tree recursively
	root := buildTree(leaves)
	subTree := CDMerkleTree{
		root:   root,
		leaves: leaves,
	}
	if debugCDT {
		fmt.Printf("NewCDTSubtree: root=%v, leaves=%v\n", common.Bytes2Hex(subTree.Root()), len(subTree.leaves))
	}
	return &subTree
}

// Equation(E.4) in GP 0.6.2
// buildTree recursively builds the tree from the leaf nodes
func buildTree(leaves []*CDTNode) *CDTNode {
	// Base case: only one node, return it
	if len(leaves) == 1 {
		return leaves[0]
	}

	var nextLevel []*CDTNode

	// Combine nodes in pairs
	for i := 0; i < len(leaves); i += 2 {
		left := leaves[i]
		right := leaves[i+1]
		parent := &CDTNode{
			Hash:  computeNode(append(left.Hash, right.Hash...)),
			Left:  left,
			Right: right,
		}
		nextLevel = append(nextLevel, parent)
	}

	return buildTree(nextLevel)
}

// PrintTree prints the tree structure for debugging
func (tree *CDMerkleTree) PrintTree2() {
	printCDTNode(tree.root, 0, "Root")
}

func printCDTNode(node *CDTNode, level int, pos string) {
	if node == nil {
		return
	}
	prefix := strings.Repeat("  ", level)
	if node.Left == nil && node.Right == nil {
		fmt.Printf("%s[Leaf %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
	} else {
		fmt.Printf("%s[Branch %s]: %s\n", prefix, pos, common.Bytes2Hex(node.Hash))
	}
	printCDTNode(node.Left, level+1, "Left")
	printCDTNode(node.Right, level+1, "Right")
}

func (tree *CDMerkleTree) PrintTree() {
	if tree.root == nil {
		fmt.Println("Empty tree")
		return
	}

	shortHash := func(hash []byte) string {
		hexStr := common.Bytes2Hex(hash)
		maxStr := 12
		if len(hexStr) <= maxStr {
			return hexStr
		} else if compareBytes(hash, H0) {
			return "H0"
		}
		return fmt.Sprintf("%s...%s", hexStr[:maxStr/2], hexStr[len(hexStr)-maxStr/2:])
	}

	queue := []*CDTNode{tree.root}
	level := 0

	for len(queue) > 0 {
		levelSize := len(queue)
		line := make([]string, 0, levelSize)

		for i := 0; i < levelSize; i++ {
			node := queue[i]
			if node == nil {
				continue
			}

			if node.Left == nil && node.Right == nil {
				line = append(line, fmt.Sprintf("[Leaf: %s]", shortHash(node.Hash)))
			} else {
				line = append(line, fmt.Sprintf("[Branch: %s]", shortHash(node.Hash)))
			}

			if node.Left != nil {
				queue = append(queue, node.Left)
			}
			if node.Right != nil {
				queue = append(queue, node.Right)
			}
		}

		if len(line) > 0 {
			fmt.Printf("Level %d: %s\n", level, strings.Join(line, "  "))
		}

		queue = queue[levelSize:]
		level++
	}
}

// // Root returns the root of the Merkle Tree
func (tree *CDMerkleTree) Root() []byte {
	return tree.root.Hash
}

func (tree *CDMerkleTree) RootHash() common.Hash {
	return common.BytesToHash(tree.Root())
}

func (mt *CDMerkleTree) Length() int {
	return len(mt.leaves)
}

func MaxPageIndex(exportedSegmentCnt int) int {
	pageSize := 1 << PageFixedDepth
	return exportedSegmentCnt / pageSize
}

func (mt *CDMerkleTree) MaxPageIndex() int {
	pageSize := 1 << PageFixedDepth
	return mt.Length() / pageSize
}

func (mt *CDMerkleTree) IsOutOfRange(index int) (bool, error) {
	if index < 0 || index >= mt.Length() {
		return true, fmt.Errorf("index %v out of range", index)
	}
	return false, nil
}

// Get returns the value of the leaf at the given index
func (mt *CDMerkleTree) Get(index int) ([]byte, error) {
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
	}
	return mt.leaves[index].Value, nil
}

// GP 0.6.4 (E.5)
// GenerateCDTJustificationX returns the justification for a given index and xDepth (function J_x)
func (mt *CDMerkleTree) GenerateCDTJustificationX(index int, xDepth int) ([]common.Hash, error) {
	x := 0
	//x := xDepth
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
	}
	if len(mt.leaves) == 1 {
		return []common.Hash{}, nil
	}

	justification := make([]common.Hash, 0)
	currentNode := mt.leaves[index]

	for currentNode != mt.root {
		parent := findParent(mt.root, currentNode)
		sibling := findSibling(parent, currentNode)
		if sibling != nil {
			justification = append(justification, common.BytesToHash(sibling.Hash))
		} else {
			justification = append(justification, H0Hash)
		}
		currentNode = parent
		if x > 0 {
			x--
			if x == 0 {
				break
			}
		}
	}
	if xDepth == 0 {
		// full mode
		return justification, nil
	} else {
		fullJustificationLen := len(justification)
		if fullJustificationLen <= xDepth {
			// global is empty
			return []common.Hash{}, nil
		}
		//J_Depth
		//J0 := fullJustification			     // full justification
		//J6 := fullJustification[J_Depth:]      // global justification above the page root
		justification = justification[xDepth:]
	}

	return justification, nil
}

func VerifyCDTJustificationX(leafHash []byte, index int, justification []common.Hash, x_depth int) []byte {
	if debugCDT {
		fmt.Printf("VerifyCDTJustificationX:\n")
		fmt.Printf("  Starting hash: 0x%x\n", leafHash)
		fmt.Printf("  Starting index: %d\n", index)
		fmt.Printf("  Justification length: %d\n", len(justification))
		fmt.Printf("  X (max steps): %d\n", x_depth)
	}

	currentHash := leafHash
	maxSteps := x_depth
	if x_depth == 0 {
		// If x==0, we interpret that as "use the entire justification"
		maxSteps = len(justification)
	}

	for i := 0; i < maxSteps && i < len(justification); i++ {
		siblingHash := justification[i].Bytes()
		updatedHash := currentHash
		isCurrLeft := index%2 == 0
		if isCurrLeft {
			updatedHash = computeNode(append(currentHash, siblingHash...))
		} else {
			updatedHash = computeNode(append(siblingHash, currentHash...))
		}
		if debugCDT {
			fmt.Printf("  Step %d:\n", i+1)
			fmt.Printf("    Current index: %d\n", index)
			fmt.Printf("    Current node:  0x%x\n", currentHash)
			fmt.Printf("    Sibling node:  0x%x\n", siblingHash)
			if isCurrLeft {
				fmt.Printf("    Merged as: Curr || Sibl => 0x%x\n", updatedHash)
			} else {
				fmt.Printf("    Merged as: Sibl || Curr => 0x%x\n", updatedHash)
			}
		}
		currentHash = updatedHash
		index /= 2
	}
	if debugCDT {
		fmt.Printf("  Final computed hash: 0x%x\n", currentHash)
	}
	return currentHash
}

// E(6) in GP 0.6.4 - return all leaves within the 2^x subtree. Leaves
func (mt *CDMerkleTree) GenerateLocalLeavesX(pageIndex int, x int) (hashedLeaves []common.Hash, err error) {

	if mt.Length() == 0 {
		return nil, fmt.Errorf("empty tree")
	}
	if pageIndex < 0 || pageIndex > mt.MaxPageIndex() {
		return nil, fmt.Errorf("leafIndex out of range")
	}
	pageSize := 1 << PageFixedDepth
	segIdxStart := pageIndex * pageSize
	segIdxEnd := segIdxStart + pageSize
	if segIdxEnd > mt.Length() {
		segIdxEnd = mt.Length()
	}
	//fmt.Printf("GenerateLocalLeavesX: pageIndex=%d, segIdxStart=%d, segIdxEnd=%d. Len=%d\n", pageIndex, segIdxStart, segIdxEnd, segIdxEnd-segIdxStart)
	// Collect the non-empty hashed leaves for that page
	for i := segIdxStart; i < segIdxEnd; i++ {
		hashedLeaf := common.BytesToHash(mt.leaves[i].Hash)
		if hashedLeaf != H0Hash {
			hashedLeaves = append(hashedLeaves, hashedLeaf)
		}
	}
	return hashedLeaves, nil
}

func computeMerkleRoot(hashes [][]byte) []byte {
	if len(hashes) == 0 {
		return nil
	}
	for len(hashes) > 1 {
		nextLevel := [][]byte{}
		for i := 0; i < len(hashes); i += 2 {
			if i+1 < len(hashes) {
				nextLevel = append(nextLevel, computeNode(append(hashes[i], hashes[i+1]...)))
			} else {
				nextLevel = append(nextLevel, hashes[i])
			}
		}
		hashes = nextLevel
	}
	return hashes[0]
}

func findParent(root, node *CDTNode) *CDTNode {
	if root == nil || root == node {
		return nil
	}
	if root.Left == node || root.Right == node {
		return root
	}
	parent := findParent(root.Left, node)
	if parent != nil {
		return parent
	}
	return findParent(root.Right, node)
}

func findSibling(parent, node *CDTNode) *CDTNode {
	if parent == nil {
		return nil
	}
	if parent.Left == node {
		return parent.Right
	}
	return parent.Left
}

func computePageProofHashes(leafHashes [][]byte) [][]byte {
	if len(leafHashes) == 0 {
		return nil
	}

	var treeLevels [][][]byte
	treeLevels = append(treeLevels, leafHashes)

	for len(treeLevels[len(treeLevels)-1]) > 1 {
		currentLevel := treeLevels[len(treeLevels)-1]
		var nextLevel [][]byte
		for i := 0; i < len(currentLevel); i += 2 {
			left := currentLevel[i]
			right := currentLevel[i+1]
			parent := computeNode(append(left, right...))
			nextLevel = append(nextLevel, parent)
		}
		treeLevels = append(treeLevels, nextLevel)
	}
	// printNodePageProof(treeLevels, len(treeLevels)-1, 0, "")

	var allHashes [][]byte
	for _, level := range treeLevels {
		for _, h := range level {
			allHashes = append(allHashes, h)
		}
	}
	return allHashes
}

func printNodePageProof(treeLevels [][][]byte, level, index int, indent string) {
	var label string
	switch {
	case level == len(treeLevels)-1:
		label = "Root"
	case level == 0:
		label = "Leaf"
	default:
		label = "Branch"
	}

	fmt.Printf("%s%s 0x%x\n", indent, label, treeLevels[level][index])

	if level > 0 {
		printNodePageProof(treeLevels, level-1, index*2, indent+"  ")
		printNodePageProof(treeLevels, level-1, index*2+1, indent+"  ")
	}
}

// generatePageProof creates paged proofs from segments
// Equation(14.10) in GP 0.6.2
func GeneratePageProof(segments [][]byte) (pageProofBytes [][]byte, err error) {
	// Build the Merkle tree
	tree := NewCDMerkleTree(segments)
	leaves := make([][]byte, len(tree.leaves))
	for i, leaf := range tree.leaves {
		leaves[i] = leaf.Value
	}
	// Count the number of pages
	pageSize := 1 << PageFixedDepth
	numPages := (len(segments) + pageSize - 1) / pageSize
	pageProofBytes = make([][]byte, numPages)
	for pageIdx := 0; pageIdx < numPages; pageIdx++ {
		// The start and end index of the current page
		start := pageIdx * pageSize
		end := start + pageSize
		if end > len(leaves) {
			end = len(leaves)
		}

		// Calculate the index of the leaf node
		i := pageIdx * pageSize

		// J_PageFixedDepth ---> from page root to the global root
		globalJustification, err := tree.GenerateCDTJustificationX(i, PageFixedDepth) //first segment within each page
		if err != nil {
			return pageProofBytes, err
		}
		// L_PageFixedDepth ----> collection of hashedleaves within the page (excluding empty H0)
		localHashedLeaves, err := tree.GenerateLocalLeavesX(pageIdx, PageFixedDepth)
		// (14.10) E(↕J6(s,i),↕L6(s,i))
		pageProof := types.PageProof{
			JustificationX: globalJustification,
			LeafHashes:     localHashedLeaves,
		}
		pageProofByte, encodeErr := types.Encode(pageProof)
		if encodeErr != nil {
			return pageProofBytes, err
		}
		pageProofBytes[pageIdx] = pageProofByte
	}
	return pageProofBytes, nil
}

func RecoverIndexFromSubtreeIdxAndPageIdx(pageIdx int, subTreeIdx int) int {
	pageSize := 1 << PageFixedDepth
	index := pageIdx*pageSize + subTreeIdx
	return index
}

func PageProofToFullJustification(pageProofByte []byte, pageIdx int, subTreeIdx int) (fullJustification []common.Hash, err error) {
	pageSize := 1 << PageFixedDepth
	index := pageIdx*pageSize + subTreeIdx
	// Decode the proof back to segments and verify
	decodedData, _, err := types.Decode(pageProofByte, reflect.TypeOf(types.PageProof{}))
	if err != nil {
		return nil, err
	}
	recoveredPageProof := decodedData.(types.PageProof)
	isFixedPadding := true
	if pageIdx == 0 && len(recoveredPageProof.LeafHashes) != pageSize {
		// degenerated case
		isFixedPadding = false
	}

	// extract J6(s,i) from the proof
	globalJustification := recoveredPageProof.JustificationX

	// rebuild subtree using L6(s,i) from the proof
	localHashedLeaves := recoveredPageProof.LeafHashes
	subTree := NewCDTSubtree(localHashedLeaves, isFixedPadding)
	localPageRoot := subTree.Root()

	// generate subtree justification using page specific idx
	subTreeJustification, err := subTree.GenerateCDTJustificationX(subTreeIdx, 0)
	if err != nil {
		return nil, err
	}

	// combine global and subtree justification
	fullJustification = append(subTreeJustification, globalJustification...)

	if debugCDT {
		fmt.Printf("!!! decoded pagedProof[%d]: %v\n", pageIdx, recoveredPageProof)
		fmt.Printf("PageProofToFullJustification: pageIdx=%d, index=%d, subTreeIdx=%d, isFixedPadding=%v\n", pageIdx, index, subTreeIdx, isFixedPadding)
		fmt.Printf("PageProofToFullJustification: pageIdx=%d, index=%d, subTreeIdx=%d, isFixedPadding=%v globalJustification=%v\n", pageIdx, index, subTreeIdx, isFixedPadding, recoveredPageProof.JustificationX)
		fmt.Printf("subTree[%d] localPageRoot: %x\n", pageIdx, localPageRoot)
		fmt.Printf("PageProofToFullJustification: pageIdx=%d, index=%d, subTreeIdx=%d, isFixedPadding=%v subTreeJustification=%v\n", pageIdx, index, subTreeIdx, isFixedPadding, subTreeJustification)
		fmt.Printf("PageProofToFullJustification: pageIdx=%d, index=%d, subTreeIdx=%d fullJustification=%v\n", pageIdx, index, subTreeIdx, fullJustification)
	}

	return fullJustification, nil
}

func FindPositions(nums [][]byte, target []byte) int {
	return findPositions(nums, target)
}

func findPositions(nums [][]byte, target []byte) int {
	positions := -1
	for i, num := range nums {
		if compareBytes(num, target) {
			return i
		}
	}
	return positions
}
