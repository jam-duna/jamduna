package trie

import (
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

//"golang.org/x/crypto/blake2b"

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
	emptyHash := make([]byte, 32)
	if len(values) == 0 {
		return &CDMerkleTree{
			root: &CDTNode{Hash: emptyHash},
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
		if !compareBytes(value, emptyHash) {
			leaves = append(leaves, &CDTNode{Hash: computeLeaf(value), Value: value})
		} else {
			leaves = append(leaves, &CDTNode{Hash: emptyHash, Value: emptyHash})
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

	// Padding with zero hashes
	for len(values) < nextPowerOfTwo {
		values = append(values, make([]byte, 32))
	}

	return values
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
func (tree *CDMerkleTree) PrintTree() {
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

// Equation(E.5) in GP 0.6.2
// Equation(E.2) in GP 0.6.2(E.5 use E.2 to generate the proof)
// GenerateCDTJustificationX returns the justification for a given index and size x (function J_x)
func (mt *CDMerkleTree) GenerateCDTJustificationX(index int, x int) ([][]byte, error) {
	if _, err := mt.IsOutOfRange(index); err != nil {
		return nil, err
	}
	if len(mt.leaves) == 1 {
		return [][]byte{}, nil
	}

	justification := make([][]byte, 0)
	currentNode := mt.leaves[index]

	for currentNode != mt.root {
		parent := findParent(mt.root, currentNode)
		sibling := findSibling(parent, currentNode)
		if sibling != nil {
			justification = append(justification, sibling.Hash)
		} else {
			justification = append(justification, make([]byte, 32))
		}
		currentNode = parent
		if x > 0 {
			x--
			if x == 0 {
				break
			}
		}
	}
	return justification, nil
}

func verifyCDTJustificationX(leafHash []byte, index int, justification [][]byte, x int) []byte {
	currentHash := leafHash
	maxSteps := x
	if x == 0 {
		maxSteps = len(justification)
	}
	for i := 0; i < maxSteps && i < len(justification); i++ {
		siblingHash := justification[i]
		if index%2 == 0 {
			currentHash = computeNode(append(currentHash, siblingHash...))
		} else {
			currentHash = computeNode(append(siblingHash, currentHash...))
		}
		index /= 2
	}
	return currentHash
}

func VerifyCDTJustificationX(leafHash []byte, index int, justification [][]byte, x int) []byte {
	return verifyCDTJustificationX(leafHash, index, justification, x)
}

func (mt *CDMerkleTree) LocalRootX(pageIndex int, x int) ([]byte, error) {
	if _, err := mt.IsOutOfRange(pageIndex); err != nil {
		return nil, err
	}

	if len(mt.leaves) == 1 {
		return mt.Root(), nil
	}

	index := (1 << pageIndex) * pageIndex
	currentNode := mt.leaves[index]
	var parent *CDTNode

	for currentNode != mt.root && x > 0 {
		parent = findParent(mt.root, currentNode)
		currentNode = parent
		x--
	}

	return parent.Hash, nil
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
func GeneratePageProof(segments [][]byte) ([][]byte, error) {
	return generatePageProof(segments)
}

// generatePageProof creates paged proofs from segments
// Equation(14.10) in GP 0.6.2
func generatePageProof(segments [][]byte) ([][]byte, error) {
	// Build the Merkle tree
	tree := NewCDMerkleTree(segments)
	leaves := make([][]byte, len(tree.leaves))
	for i, leaf := range tree.leaves {
		leaves[i] = leaf.Value
	}
	// Count the number of pages
	pageSize := 64
	numPages := (len(leaves) + pageSize - 1) / pageSize // ceiling function
	results := make([][]byte, 0)
	for page := 0; page < numPages; page++ {
		// The start and end index of the current page
		start := page * 64
		end := start + 64
		if end > len(leaves) {
			end = len(leaves)
		}

		// Calculate the index of the leaf node
		i := page * 64

		// Get the justification for the leaf node
		tracePath, err := tree.GenerateCDTJustificationX(i, 6)
		if err != nil {
			return results, err
		}
		leafHashes := make([][]byte, len(leaves[start:end]))

		// Equation(E.6) in GP 0.6.2
		for j := range leafHashes {
			hash := common.ComputeLeafHash_WBT_Blake2B(leaves[j+i])
			leafHashes[j] = hash[:]
		}
		// Encode the trace path and the leaves
		combinedData := append(tracePath, leafHashes...)
		encoded, err := types.Encode(combinedData)

		if err != nil {
			return results, err
		}

		// Pad the encoded data to W_E * W_S
		// paddingSize := types.W_G
		// paddedOutput := make([]byte, paddingSize)
		// copy(paddedOutput, encoded) // If the encoded data is larger than the padding size, it will be truncated
		results = append(results, encoded)
	}

	return results, nil
}

// splitPageProof splits the page proof into justificationX(1-5) and leafHash(range: 1-64)
func splitPageProof(pageProof [][]byte) (justificationX [][]byte, leafHash [][]byte) {
	proofLen := len(pageProof)
	switch {
	case proofLen == 1:
		// 0 justificationX + 1 leafHash
		return [][]byte{}, pageProof[0:]
	case proofLen == 3:
		// 1 justificationX + 2 leafHash
		return pageProof[0:1], pageProof[1:]
	case proofLen >= 5 && proofLen <= 6:
		// 2 justificationX & 3-4 leafHash.
		return pageProof[0:2], pageProof[2:]
	case proofLen >= 8 && proofLen <= 11:
		// 3 justificationX & 5-8 leafHash.
		return pageProof[0:3], pageProof[3:]
	case proofLen >= 13 && proofLen <= 20:
		// 4 justificationX & 9-16 leafHash.
		return pageProof[0:4], pageProof[4:]
	case proofLen >= 22 && proofLen <= 37:
		// 5 justificationX & 17-32 leafHash.
		return pageProof[0:5], pageProof[5:]
	default:
		// 6 justificationX & 33-64 leafHash.
		return pageProof[0:6], pageProof[6:]
	}
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
