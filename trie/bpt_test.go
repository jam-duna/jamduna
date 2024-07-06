package trie

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"testing"
	"fmt"
)

// TestVector represents a test case in the JSON file
type TestVector struct {
	Input  map[string]string `json:"input"`
	Output string            `json:"output"`
}


func hexToBytes(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}

func hex2Bytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}


// TestNewMerkleTree tests the creation of a new Merkle tree
func TestNewMerkleTreeInit(t *testing.T) {
	t.Skip()
	input := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), []byte("value_b")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), []byte("value_c")},
	}

	tree := NewMerkleTree(input)
	rootHash := tree.GetRootHash()
	extract_data := tree.extractData(tree.Root, 0)
	fmt.Printf("rootHash=%x, extract_data=%x\n", rootHash, extract_data)
	tree.printTree(tree.Root, 0)
	if tree.Root == nil {
		t.Error("Expected a valid tree with a root, got nil")
	}
}

func TestGetValue(t *testing.T) {
	t.Skip()
	data := [][2][]byte{
		{[]byte("a"), []byte("value_a")},
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), []byte("value_b")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), []byte("value_c")},
	}

    tree := NewMerkleTree(data)
	extracted_data := tree.extractData(tree.Root, 0)
	fmt.Printf("(after insert; Len=%v) extracted_data=%x\n", len(extracted_data), extracted_data)

    value, err := tree.Get(hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"))

    if err != nil {
        t.Errorf("Unexpected error: %v", err)
    }

    if !compareBytes(value, []byte("value_c")) {
        t.Errorf("Expected value 'value_c', got %s", value)
    }

    t.Logf("Retrieved value: %s\n", value)
}

// TestTrace tests the tracing functionality in the Merkle tree
func TestTrace(t *testing.T) {
	t.Skip()
	data := [][2][]byte{
		{[]byte("a"), []byte("value_a")},
		{[]byte("b"), []byte("value_b")},
		{[]byte("c"), []byte("value_c")},
		{[]byte("d"), []byte("value_d")},
	}

	tree := NewMerkleTree(data)
	trace, err := tree.Trace([]byte("b"))

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(trace) == 0 {
		t.Error("Expected a non-empty trace, got empty")
	}

	t.Logf("Trace path: %x\n", trace)
}

func TestMerkleTree(t *testing.T) {
	// Read the JSON file
	t.Skip()
	filePath := "../jamtestvectors/trie/trie.json"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse the JSON file
	var testVectors []TestVector
	err = json.Unmarshal(data, &testVectors)
	if err != nil {
		t.Fatalf("Failed to parse JSON file: %v", err)
	}

	// Run each test case
	for i, testCase := range testVectors {
		// Create the Merkle Tree input from the test case
		if (i != 6) {
			continue
		}
		fmt.Printf("testCase %d: %v\n", i, testCase)
		var input [][2][]byte
		for k, v := range testCase.Input {
			fmt.Printf("k=%v, v=%v\n", k, v)
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			input = append(input, [2][]byte{key, value})
		}
		fmt.Printf("input=%x (len=%v)\n", input, len(input))

		isSerialized := true

		var rootHash []byte
		if isSerialized {
			// Create an empty Merkle Tree
			tree := NewMerkleTree(nil)
			fmt.Printf("initial rootHash:%x \n", tree.GetRootHash())
			// Insert key-value pairs one by one
			index := 0
			for k, v := range testCase.Input {
				fmt.Printf("insert#%d, k=%v, v=%v\n", index, k, v)
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				//tree.Insert2(key, value)
				tree.Insert(key, value)
				tree.printTree(tree.Root, 0)
				index++
			}
			rootHash = tree.GetRootHash()
		} else {
			// Test 1: insert all at once
			tree := NewMerkleTree(input)
			rootHash = tree.GetRootHash()
			extract_data := tree.extractData(tree.Root, 0)
			fmt.Printf("[%v] extract_data=%x\n", i, extract_data)
			tree.printTree(tree.Root, 0)
		}

		// Compare the computed root hash with the expected output
		expectedHash, _ := hex.DecodeString(testCase.Output)
		if !compareBytes(rootHash, expectedHash) {
			t.Errorf("Test case %d: Root hash mismatch for input %v: got %s, want %s", i, testCase.Input, hex.EncodeToString(rootHash), testCase.Output)
		} else {
			t.Logf("Test case %d: Vector OK, rootHash=%x", i, expectedHash)
		}
	}
}
