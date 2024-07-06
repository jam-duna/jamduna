package trie

import (
	"errors"
	"fmt"
	"strings"
)

// Node represents a node in the Merkle Tree
type Node struct {
	Hash  []byte
	Key   []byte
	Value []byte
	Left  *Node
	Right *Node
}

// MerkleTree represents the entire Merkle Tree
type MerkleTree struct {
	Root *Node
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func NewMerkleTree(data [][2][]byte) *MerkleTree {
	if data == nil || len(data) == 0 {
		return &MerkleTree{Root: nil}
	}
	root := buildMerkleTree(data, 0)
	return &MerkleTree{Root: root}
}


// buildMerkleTree constructs the Merkle tree from key-value pairs
func buildMerkleTree(kvs [][2][]byte, i int) *Node {
	//Base Case - Empty Data |d| = 0
	if len(kvs) == 0 {
		return &Node{Hash: make([]byte, 32)}
	}
	// V(d) = {(K,v)}
	if len(kvs) == 1 {
		encoded := leaf(kvs[0][0], kvs[0][1])
		return &Node{Hash: computeHash(encoded), Key: kvs[0][0], Value: kvs[0][1]}
	}
	//Recursive Case: B(M(l),M(r))
	var l, r [][2][]byte
	for _, kv := range kvs {
		if bit(kv[0], i) {
			r = append(r, kv)
		} else {
			l = append(l, kv)
		}
	}

	left := buildMerkleTree(l, i+1)
	right := buildMerkleTree(r, i+1)
	encoded := branch(left.Hash, right.Hash)
	return &Node{Hash: computeHash(encoded), Left: left, Right: right}
}

/*
Branch Node (64 bytes)
+-------------------------------------------------+
|    First 255 bits of left child node hash       |
+-------------------------------------------------+
|    Full 256 bits of right child node hash       |
+-------------------------------------------------+

Embedded-Value Leaf Node (64 bytes)
+--------+------------------------------------------+
|  2 bits | 6 bits (value size) | 31 bytes (key)    |
+--------+------------------------------------------+
|              32 bytes (embedded value)            |
+---------------------------------------------------+

Regular Leaf Node (64 bytes)
+--------+------------------------------------------+
|  2 bits | 6 bits (0s) | 31 bytes (key)            |
+--------+------------------------------------------+
|               32 bytes (hash of value)            |
+---------------------------------------------------+
*/

// branch concatenates the left and right node hashes with a modified head
func branch(left, right []byte) []byte {
	if len(left) != 32 || len(right) != 32 {
		panic("branch: input hashes must be 32 bytes")
	}
	head := left[0] & 0xfe // Set the LSB of the first byte of the left hash to 0
	left255bits := append([]byte{head}, left[1:]...) //Left: last 255 bits of
	concatenated := append(left255bits, right...)    //(l,r): 512 bits
	return concatenated
}

// leaf encodes a key-value pair into a leaf node
func leaf(k, v []byte) []byte {
	//Embedded-value leaf node
    if len(v) <= 32 {
        head := byte(0b01 | (len(v) << 2))
        if len(k) > 31 {
            k = k[:31]
        } else {
            k = append(k, make([]byte, 31-len(k))...)
        }
        value := append(v, make([]byte, 32-len(v))...)
        return append([]byte{head}, append(k, value...)...)
    } else {
		//Regular leaf node
        head := byte(0b11)
        if len(k) > 31 {
            k = k[:31]
        } else {
            k = append(k, make([]byte, 31-len(k))...)
        }
        hash := computeHash(v)
        return append([]byte{head}, append(k, hash...)...)
    }
}

func bit(k []byte, i int) bool {
    byteIndex := i / 8 // the byte index in the array where the bit is located
	if byteIndex >= len(k) {
		return false // return false if index is out of range
	}
    bitIndex := i % 8  // the bit position within the byte
    b := k[byteIndex] // target byte
    mask := byte(1 << (bitIndex)) //least significant bit first [Not sure about this]
	//mask := byte(1 << (7 - bitIndex)) // most significant bit first [Not sure]
    return (b & mask) != 0 //return  set (1) or not (0)
}

// GetRootHash returns the root hash of the Merkle Tree
func (t *MerkleTree) GetRootHash() []byte {
	if t.Root == nil {
		return make([]byte, 32)
	}
	return t.Root.Hash
}

// Insert inserts a new key-value pair into the Merkle Tree
func (t *MerkleTree) Insert(key, value []byte) {
	// To insert a new key-value pair, we need to rebuild the tree with the new data
	extracted_data := t.extractData(t.Root, 0)
	fmt.Printf("(before insert; Len=%v) extracted_data=%x\n", len(extracted_data), extracted_data)

	// Filter out empty key-value pairs
	var filtered_data [][2][]byte
	for _, kv := range extracted_data {
		if len(kv[0]) > 0 && len(kv[1]) > 0 {
			filtered_data = append(filtered_data, kv)
		}
	}

	// Add the new key-value pair
	filtered_data = append(filtered_data, [2][]byte{key, value})

	fmt.Printf("(new to insert; Len=%v) extracted_data=%x\n", len(filtered_data), filtered_data)

	newTree := NewMerkleTree(filtered_data)
	t.Root = newTree.Root
}

func (t *MerkleTree) printTree(node *Node, level int) {
    if level == 0 {
        fmt.Printf("Root Hash: %x\n", t.Root.Hash)
    }
    if node == nil {
        fmt.Printf("%snode empty\n", strings.Repeat("  ", level))
        return
    }
    nodeType := "Branch"
    if node.Left == nil && node.Right == nil {
        nodeType = "Leaf"
    }
    fmt.Printf("%s[%s Node] Key: %x, Hash: %x\n", strings.Repeat("  ", level), nodeType, node.Key, node.Hash)
    if node.Value != nil {
        fmt.Printf("%s  [Leaf Node] Value: %x\n", strings.Repeat("  ", level), node.Value)
    }
    if node.Left != nil || node.Right != nil {
        fmt.Printf("%s  Left:\n", strings.Repeat("  ", level))
        t.printTree(node.Left, level+1)
        fmt.Printf("%s  Right:\n", strings.Repeat("  ", level))
        t.printTree(node.Right, level+1)
    }
}

func (t *MerkleTree) Insert2(key, value []byte) {
    fmt.Printf("Insert called for key: %x, value: %x\n", key, value)
    if t.Root == nil {
        fmt.Printf("Root is nil, creating new root node.\n")
        t.Root = &Node{
            Hash:  computeHash(leaf(key, value)),
            Key:   key,
            Value: value,
        }
    } else {
        fmt.Printf("Inserting into existing tree.\n")
        t.Root = insertNode(t.Root, key, value, 0)
    }
    fmt.Printf("Root hash after insertion: %x\n", t.Root.Hash)
    //t.printTree(t.Root, 0)
}

func insertNode(node *Node, key, value []byte, depth int) *Node {
    fmt.Printf("Inserting key: %x, value: %x, at depth: %d\n", key, value, depth)

    if node == nil {
        fmt.Printf("Creating new node for key: %x, value: %x\n", key, value)
        return &Node{
            Hash:  computeHash(leaf(key, value)),
            Key:   key,
            Value: value,
        }
    }

    if compareBytes(node.Key, key) {
        fmt.Printf("Updating existing node with key: %x, new value: %x\n", key, value)
        node.Value = value
        node.Hash = computeHash(leaf(key, value))
        return node
    }

    if bit(key, depth) {
        fmt.Printf("Going right at depth: %d for key: %x\n", depth, key)
        node.Right = insertNode(node.Right, key, value, depth+1)
    } else {
        fmt.Printf("Going left at depth: %d for key: %x\n", depth, key)
        node.Left = insertNode(node.Left, key, value, depth+1)
    }

    leftHash := make([]byte, 32)
    rightHash := make([]byte, 32)
    if node.Left != nil {
        leftHash = node.Left.Hash
    }
    if node.Right != nil {
        rightHash = node.Right.Hash
    }
    node.Hash = computeHash(branch(leftHash, rightHash))
    fmt.Printf("Updated node hash: %x at depth: %d\n", node.Hash, depth)
    return node
}

// extractData extracts all key-value pairs from the current tree
func (t *MerkleTree) extractData(node *Node, i int) [][2][]byte {
	if node == nil {
		return nil
	}
	if node.Left == nil && node.Right == nil {
		// Leaf node
		return [][2][]byte{{node.Key, node.Value}}
	}
	var data [][2][]byte
	if node.Left != nil {
		data = append(data, t.extractData(node.Left, i+1)...)
	}
	if node.Right != nil {
		data = append(data, t.extractData(node.Right, i+1)...)
	}
	return data
}

func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	if t.Root == nil {
		return nil, errors.New("empty tree")
	}
	return t.getValue(t.Root, key, 0)
}

func (t *MerkleTree) getValue(node *Node, key []byte, depth int) ([]byte, error) {
	if node == nil {
		return nil, errors.New("key not found")
	}

	fmt.Printf("Searching key: %x at node key: %x at depth: %d\n", key, node.Key, depth)

	if compareBytes(node.Key, key) {
		fmt.Printf("Found key: %x with value: %x\n", key, node.Value)
		return node.Value, nil
	}

	if bit(key, depth) {
		return t.getValue(node.Right, key, depth+1)
	} else {
		return t.getValue(node.Left, key, depth+1)
	}
}

func (t *MerkleTree) Trace(key []byte) ([][]byte, error) {
	if t.Root == nil {
		return nil, errors.New("empty tree")
	}

	var tracePath [][]byte
	found, err := t.trace(t.Root, key, &tracePath, 0)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("key not found in the tree")
	}
	return tracePath, nil
}

func (t *MerkleTree) trace(node *Node, key []byte, tracePath *[][]byte, depth int) (bool, error) {
	if node == nil {
		return false, nil
	}

	// If we found the node with the matching key, we return true
	if compareBytes(node.Key, key) {
		return true, nil
	}

	if bit(key, depth) {
		// Add the left node's hash to the trace path
		if node.Left != nil {
			*tracePath = append(*tracePath, node.Left.Hash)
		} else {
			*tracePath = append(*tracePath, make([]byte, 32)) // empty hash if the left node is nil
		}
		return t.trace(node.Right, key, tracePath, depth+1)
	} else {
		// Add the right node's hash to the trace path
		if node.Right != nil {
			*tracePath = append(*tracePath, node.Right.Hash)
		} else {
			*tracePath = append(*tracePath, make([]byte, 32)) // empty hash if the right node is nil
		}
		return t.trace(node.Left, key, tracePath, depth+1)
	}
}
