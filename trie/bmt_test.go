package trie

import (
	//"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
)

// TestVector represents a test case in the JSON file
type TestVector struct {
	Input  map[string]string `json:"input"`
	Output string            `json:"output"`
}

func hex2Bytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}

func TestEdgeCases(t *testing.T) {
	data := [][2][]byte{
		{hex2Bytes("0200000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("0000000000000000000000000000000000000000000000000000")},
	}
	tree := NewMerkleTree(nil)

	for _, item := range data {
		tree.Insert(item[0], item[1])
	}
	if debugBPT {
		tree.printTree(tree.Root, 0)
	}
	getValue, _, err := tree.Get(hex2Bytes("0200000000000000000000000000000000000000000000000000000000000000"))
	if err != nil {
		t.Errorf("Key: %x not found, error: %s\n", hex2Bytes("0000"), err)
	}
	if debugBPT {
		fmt.Printf("getValue %x\n", getValue)
	}
}

// TestTrace tests the tracing functionality in the Merkle tree
func TestTrace(t *testing.T) {
	// t.Skip()
	// level_db_path := "../leveldb/BPT"
	data := [][2][]byte{
		{hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"), hex2Bytes("bb11c256876fe10442213dd78714793394d2016134c28a64eb27376ddc147fc6044df72bdea44d9ec66a3ea1e6d523f7de71db1d05a980e001e9fa")},
		{hex2Bytes("df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e7"), hex2Bytes("e46ddd41a5960807d528f5d9282568e622a023b94b72cb63f0353baff189257d")},
		{hex2Bytes("3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c"), hex2Bytes("9bc5d0")},
	}

	tree := NewMerkleTree(nil)

	for _, item := range data {
		tree.Insert(item[0], item[1])
	}
	_, _ = tree.Flush()
	if debugBPT {
		tree.PrintTree(tree.Root, 0)
	}
	trace, err := tree.Trace(hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"))

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(trace) == 0 {
		t.Error("Expected a non-empty trace, got empty")
	}
	path, _ := tree.GetPath(hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"))

	if debugBPT {
		fmt.Printf("GetPath %x\n", path)
		t.Logf("Trace path: %x\n", trace)
	}

}

func TestMerkleTree(t *testing.T) {
	t.Skip("Skip Merkele Tree Until we have 31bytes k,v version")
	// Read the JSON file
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "trie/trie.json")
	data, err := os.ReadFile(filePath)
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
		if debugBPT {
			fmt.Printf("testCase %d: %v\n", i, testCase)
		}

		isSerialized := true
		var rootHash []byte

		// Create an empty Merkle Tree
		tree := NewMerkleTree(nil)

		if isSerialized {
			// Insert key-value pairs one by one
			for k, v := range testCase.Input {
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				tree.Insert(key, value)
				if debugBPT {
					fmt.Printf("computeHash:) %x\n", computeHash(leafStandalone(key, value)))
				}
			}
			rootHash = tree.GetRoot().Bytes()
		}
		// Compare the computed root hash with the expected output

		if debugBPT {
			tree.printTree(tree.Root, 0)
		}
		expectedHash, _ := hex.DecodeString(testCase.Output)
		if !compareBytes(rootHash, expectedHash) {
			t.Errorf("Test case %d: Root hash mismatch for input %v: got %s, want %s", i, testCase.Input, common.Bytes2Hex(rootHash), testCase.Output)
		} else {
			t.Logf("Test case %d: Vector OK, rootHash=%x", i, expectedHash)
		}

		// Test Valid Proof for each key from the test case
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			// Get the proof path for the key
			path, err := tree.Trace(key)
			if err != nil {
				t.Errorf("Unable to generate Proof for key [%x].Err %v\n", key, err)
			}

			// Convert [][]byte to []common.Hash
			pathHashes := make([]common.Hash, len(path))
			for i, p := range path {
				copy(pathHashes[i][:], p)
			}

			// Test Valid Proof
			fmt.Printf("VerifyRaw key=%x, value=%x, expectedHash=%x, pathHashes=%x\n", key, value, expectedHash, pathHashes)
			if VerifyRaw(key, value, expectedHash, pathHashes) {
				if debugBPT {
					fmt.Printf("Proof for key [%x] is valid.\n", key)
				}
			} else {
				t.Errorf("Proof for key [%x] is invalid.\n", key)
			}

			// Test Get
			getValue, _, getErr := tree.Get(key)
			if getErr != nil {
				t.Errorf("Key: %x not found, error: %s\n", key, getErr)
			} else {
				if debugBPT {
					fmt.Printf("Key: %x, Value: %x\n", key, getValue)
				}
			}
		}
		// os.RemoveAll(dbPath) // Clean up the LevelDB folder(if needed)

	}

}

func TestBPTProofSimple(t *testing.T) {
	debugBPT := true
	data := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9"), hex2Bytes("965ac2547cacec18429e88553142a605d649fbcd6a40a0ae5d51a8f218c8fd5c")},
		{hex2Bytes("59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211"), hex2Bytes("72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b")},
		{hex2Bytes("a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2"), hex2Bytes("3f26db92922e86f6b538372608656a14762b3e93bd5d4f6a754d36f68ce0b28b")},
		{hex2Bytes("15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5"), hex2Bytes("be2a1eb0a1b961e9642c2e09c71d2f45aa653bb9a709bbc8cbad18022c9dcf2e")},
		{hex2Bytes("b05ff8a05bb23c0d7b177d47ce466ee58fd55c6a0351a3040cf3cbf5225aab19"), hex2Bytes("5c43fcf60000000000000000000000006ba080e1534c41f5d44615813a7d1b2b57c950390000000000000000000000008863786bebe8eb9659df00b49f8f1eeec7e2c8c1")},
		{hex2Bytes("df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c"), hex2Bytes("9bc5d0")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000200"), hex2Bytes("01")},
	}
	exceptedRootHash := hex2Bytes("511727325a0cd23890c21cda3c6f8b1c9fbdf37ed57b9a85ca77286356183dcf")

	// Create an empty Merkle Tree
	tree := NewMerkleTree(nil)
	if debugBPT {
		fmt.Printf("initial rootHash: %v\n", tree.GetRoot())
	}

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		if debugBPT {
			fmt.Printf("insert#%d, key=%x, value=%x\n", index, key, value)
		}
		tree.Insert(key, value)
		tree.Flush()
	}

	if debugBPT {
		//tree.printTree(tree.Root, 0)
		fmt.Printf("Actual rootHash: %v\n", tree.GetRoot())
	}
	wrongKey := hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522ff")
	_, err := tree.Trace(wrongKey)
	if err == nil {
		t.Errorf("Proof should not exist non arbitrary key [%x].\n", wrongKey)
	}

	key := hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3")

	path, err := tree.Trace(key)
	if debugBPT {
		fmt.Printf("Proof path for key %x: \n", key)
	}
	for i, p := range path {
		if debugBPT {
			fmt.Printf("  Node %d: %x\n", i, p)
		}
	}
	if err != nil {
		t.Errorf("Unable to generate Proof for key [%x].Err %v\n", key, err)
	}

	// Test Valid Proof
	value, _, _ := tree.Get(key)
	if debugBPT {
		fmt.Printf("levelDBGet key=%x, value=%x\n", key, value)
	}

	// Convert [][]byte to []common.Hash
	pathHashes := make([]common.Hash, len(path))
	for i, p := range path {
		copy(pathHashes[i][:], p)
	}

	fmt.Printf("VerifyRaw key=%x, value=%x, expectedHash=%x, pathHashes=%v\n", key, value, exceptedRootHash, pathHashes)
	if VerifyRaw(key, value, tree.GetRoot().Bytes(), pathHashes) {
		if debugBPT {
			fmt.Printf("Proof for key [%x] is valid.\n", key)
		}
	} else {
		t.Errorf("Proof for key [%x] is invalid.\n", key)
	}

	// Test Invalid Proof
	invalidValue := []byte("invalid")
	if VerifyRaw(key, invalidValue, exceptedRootHash, pathHashes) {
		t.Errorf("Proof for key [%x] with invalid value is valid.\n", invalidValue)
	} else {
		if debugBPT {
			fmt.Printf("Proof for key [%x] with invalid value is invalid.\n", invalidValue)
		}
	}
}

func TestBPTProof(t *testing.T) {
	data := [][2][]byte{
		{hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("22c62f84ee5775d1e75ba6519f6dfae571eb1888768f2a203281579656b6a29097f7c7e2cf44e38da9a541d9b4c773db8b71e1d3")},
		{hex2Bytes("f3a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3"), hex2Bytes("44d0b26211d9d4a44e375207")},
		{hex2Bytes("d7f99b746f23411983df92806725af8e5cb66eba9f200737accae4a1ab7f47b9"), hex2Bytes("24232437f5b3f2380ba9089bdbc45efaffbe386602cb1ecc2c17f1d0")},
		{hex2Bytes("59ee947b94bcc05634d95efb474742f6cd6531766e44670ec987270a6b5a4211"), hex2Bytes("72fdb0c99cf47feb85b2dad01ee163139ee6d34a8d893029a200aff76f4be5930b9000a1bbb2dc2b6c79f8f3c19906c94a3472349817af21181c3eef6b")},
		{hex2Bytes("a3dc3bed1b0727caf428961bed11c9998ae2476d8a97fad203171b628363d9a2"), hex2Bytes("8a0dafa9d6ae6177")},
		{hex2Bytes("15207c233b055f921701fc62b41a440d01dfa488016a97cc653a84afb5f94fd5"), hex2Bytes("157b6c821169dacabcf26690df")},
		{hex2Bytes("b05ff8a05bb23c0d7b177d47ce466ee58fd55c6a0351a3040cf3cbf5225aab19"), hex2Bytes("6a208734106f38b73880684b")},
	}
	exceptedRootHash := hex2Bytes("b9c99f66e5784879a178795b63ae178f8a49ee113652a122cd4b3b2a321418c1")
	// Create an empty Merkle Tree
	//level_db_path := "../leveldb/BPT"
	tree := NewMerkleTree(nil)
	if debugBPT {
		fmt.Printf("initial rootHash: %x\n", tree.GetRoot().Bytes())
	}

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		if debugBPT {
			fmt.Printf("insert#%d, key=%x, value=%x\n", index, key, value)
		}
		tree.Insert(key, value)
	}
	_, _ = tree.Flush()

	if debugBPT || true {
		tree.printTree(tree.Root, 0)
	}
	wrongKey := hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522ff")
	_, err := tree.Trace(wrongKey)
	if err == nil {
		t.Errorf("Proof should not exist non arbitrary key [%x].\n", wrongKey)
	}

	key := hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3")

	path, err := tree.Trace(key)
	if debugBPT {
		fmt.Printf("Proof path for key %x: \n", key)
	}
	for i, p := range path {
		if debugBPT {
			fmt.Printf("  Node %d: %x\n", i, p)
		}
	}
	if err != nil {
		t.Errorf("Unable to generate Proof for key [%x].Err %v\n", key, err)
	}

	// Test Valid Proof
	value, _, _ := tree.Get(key)
	if debugBPT {
		fmt.Printf("levelDBGet key=%x, value=%x\n", key, value)
	}

	// Convert [][]byte to []common.Hash
	pathHashes := make([]common.Hash, len(path))
	for i, p := range path {
		copy(pathHashes[i][:], p)
	}

	if VerifyRaw(key, value, tree.GetRoot().Bytes(), pathHashes) {
		if debugBPT {
			fmt.Printf("Proof for key [%x] is valid.\n", key)
		}
	} else {
		t.Errorf("Proof for key [%x] is invalid.\n", key)
	}

	// Test Invalid Proof
	invalidValue := []byte("invalid")
	if VerifyRaw(key, invalidValue, exceptedRootHash, pathHashes) {
		t.Errorf("Proof for key [%x] with invalid value is valid.\n", invalidValue)
	} else {
		if debugBPT {
			fmt.Printf("Proof for key [%x] with invalid value is invalid.\n", invalidValue)
		}
	}
}

func TestGet(t *testing.T) {
	data := [][2][]byte{
		{hex2Bytes("3dbc5f775f6156957139100c343bb5ae6589af7398db694ab6c60630a9ed0fcd"), hex2Bytes("bb11c256876fe10442213dd78714793394d2016134c28a64eb27376ddc147fc6044df72bdea44d9ec66a3ea1e6d523f7de71db1d05a980e001e9fa")},
		{hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e7"), hex2Bytes("e46ddd41a5960807d528f5d9282568e622a023b94b72cb63f0353baff189257d")},
	}

	// Create an empty Merkle Tree
	tree := NewMerkleTree(nil)
	if debugBPT {
		fmt.Printf("initial rootHash: %x\n", tree.GetRoot().Bytes())
	}

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		if debugBPT {
			fmt.Printf("insert#%d, key=%x, value=%x\n", index, key, value)
		}
		tree.Insert(key, value)
	}

	keys := [][]byte{
		hex2Bytes("3dbc5f775f6156957139100c343bb5ae6589af7398db694ab6c60630a9ed0fcd"),
		hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"),
		hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e7"),
	}
	validKeys := [][]byte{
		hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e6"), // The invalid Key
	}
	for _, key := range keys {
		value, _, err := tree.Get(key)
		if err != nil {
			t.Errorf("Key: %x not found, error: %s\n", key, err)
		} else {
			if debugBPT {
				fmt.Printf("Key: %x, Value: %x\n", key, value)
			}
		}
	}
	for _, key := range validKeys {
		value, ok, err := tree.Get(key)
		if err != nil || !ok {
			if debugBPT {
				fmt.Printf("Key: %x not found, error: %s\n", key, err)
			}
		} else {
			t.Errorf("Key: %x, Value: %x\n", key, value)
		}
	}

}

func TestModifyGenesis(t *testing.T) {
	return
	dirPath := "../chainspecs/tiny-00000000.json"
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() || file.Name()[len(file.Name())-5:] != ".json" {
			continue
		}

		filePath := dirPath + file.Name()
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read JSON file %s: %v", filePath, err)
		}

		// Parse the JSON file
		var testVectors []TestVector
		err = json.Unmarshal(data, &testVectors)
		if err != nil {
			t.Fatalf("Failed to parse JSON file %s: %v", filePath, err)
		}

		// Run each test case
		for i, testCase := range testVectors {
			// Create the Merkle Tree input from the test case
			if debugBPT {
				fmt.Printf("testCase %d: %v\n", i, testCase)
			}
			var input [][2][]byte
			for k, v := range testCase.Input {
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				input = append(input, [2][]byte{key, value})
			}
			if debugBPT {
				fmt.Printf("input=%x (len=%v)\n", input, len(input))
			}

			var rootHash []byte

			// Create an empty Merkle Tree
			tree := NewMerkleTree(nil)
			if debugBPT {
				fmt.Printf("initial rootHash:%x \n", tree.GetRoot().Bytes())
			}
			// Insert key-value pairs one by one
			index := 0
			rand.Seed(time.Now().UnixNano())
			index = 0
			for k, v := range testCase.Input {
				if debugBPT {
					fmt.Printf("insert#%d, k=%v, v=%v\n", index, k, v)
				}
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				tree.Insert(key, value)
				//tree.printTree(tree.Root, 0)
				index++
			}
			rootHash = tree.GetRoot().Bytes()

			// Compare the computed root hash with the expected output
			expectedHash, _ := hex.DecodeString(testCase.Output)
			if !compareBytes(rootHash, expectedHash) {
				if debugBPT {
					tree.PrintTree(tree.Root, 0)
				}
				t.Errorf("Test case %d in file %s: Root hash mismatch for input %v: got %s, want %s", i, filePath, testCase.Input, common.Bytes2Hex(rootHash), testCase.Output)
			} else {
				t.Logf("Test case %d in file %s: Vector OK, rootHash=%x", i, filePath, expectedHash)
			}
			//tree.PrintTree(tree.Root, 0)

		}
	}
}

func TestModify(t *testing.T) {
	return
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "trie/trie.json")
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
		if debugBPT {
			fmt.Printf("testCase %d: %v\n", i, testCase)
		}
		var input [][2][]byte
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			input = append(input, [2][]byte{key, value})
		}
		if debugBPT {
			fmt.Printf("input=%x (len=%v)\n", input, len(input))
		}

		var rootHash []byte

		// Create an empty Merkle Tree
		tree := NewMerkleTree(nil)

		if debugBPT {
			fmt.Printf("initial rootHash:%x \n", tree.GetRoot().Bytes())
		}
		// Insert key-value pairs one by one
		index := 0
		rand.Seed(time.Now().UnixNano())
		index = 0
		for k, v := range testCase.Input {
			if debugBPT {
				fmt.Printf("insert#%d, k=%v, v=%v\n", index, k, v)
			}
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			tree.Insert(key, value)
			//tree.printTree(tree.Root, 0)
			index++
		}
		rootHash = tree.GetRoot().Bytes()

		// Compare the computed root hash with the expected output
		expectedHash, _ := hex.DecodeString(testCase.Output)
		if !compareBytes(rootHash, expectedHash) {
			if debugBPT {
				tree.PrintTree(tree.Root, 0)
			}
			t.Errorf("Test case %d: Root hash mismatch for input %v: got %s, want %s", i, testCase.Input, common.Bytes2Hex(rootHash), testCase.Output)
		} else {
			t.Logf("Test case %d: Vector OK, rootHash=%x", i, expectedHash)
		}

	}

}

func truncateValue(value []byte) string {
	maxLen := 8
	if len(value) > maxLen {
		half := maxLen / 2
		start := value[:half]
		end := value[len(value)-half:]
		return fmt.Sprintf("0x%x...%x", start, end)
	}
	return fmt.Sprintf("0x%x", value)
}

// TestDelete tests the deletion of key-value pairs from the Merkle tree
func TestDelete(t *testing.T) {
	t.Skip()
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "trie/trie.json")
	testData, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	// Parse the JSON file
	var testVectors []TestVector
	err = json.Unmarshal(testData, &testVectors)
	if err != nil {
		t.Fatalf("Failed to parse JSON file: %v", err)
	}

	// Run each test case
	for i, testCase := range testVectors {
		// Create the Merkle Tree input from the test case
		if debugBPT {
			fmt.Printf("==== testCase %d ====\n", i)
			//fmt.Printf("kv:%v\n", testCase)
		}
		var data [][2][]byte
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			data = append(data, [2][]byte{key, value})
		}
		if debugBPT {
			//fmt.Printf("data=%x (len=%v)\n", data, len(data))
			for idx, kv := range data {
				key := kv[0]
				value := kv[1]
				fmt.Printf("key[%d]: %x, value(len=%d): %v\n", idx, key, len(value), truncateValue(value))
			}
		}

		tree := NewMerkleTree(nil)

		// Record root hashes after each insertion
		var rootHashes [][]byte

		if debugBPT {
			fmt.Printf("-------- Testing Insert ---------- \n")
		}
		for idx, kv := range data {
			tree.Insert(kv[0], kv[1])
			rootHash := tree.GetRoot().Bytes()
			rootHashes = append(rootHashes, rootHash)
			if debugBPT {
				fmt.Printf("H'=%x, Inserted key[%d]: %x, value(len=%d): %v\n", rootHash, idx, kv[0], len(kv[1]), truncateValue(kv[1]))
			}
		}

		// Check root hashes after each deletion in reverse order
		if debugBPT {
			fmt.Printf("-------- Testing Deletion -------- \n")
		}
		isFailure := false
		for i := len(data) - 1; i >= 0; i-- {
			kv := data[i]
			err := tree.Delete(kv[0])
			if err != nil {
				t.Fatalf("Failed to delete key: %x, error: %v", kv[0], err)
			}
			_, err = tree.Flush()
			if err != nil {
				t.Fatalf("Failed to flush delete for key: %x, error: %v", kv[0], err)
			}
			rootHash := tree.GetRoot().Bytes()
			expectedRootHash := make([]byte, 32)
			if i > 0 {
				expectedRootHash = rootHashes[i-1]
			}
			if debugBPT {
				fmt.Printf("H':%x, Deleted key[%d]: %x\n", rootHash, i, kv[0])
			}
			if !compareBytes(rootHash, expectedRootHash) {
				isFailure = true
				t.Fatalf("RootHash mismatch after deleting key: %x. Got: %x, Expected: %x", kv[0], rootHash, expectedRootHash)
			}
		}
		if debugBPT && !isFailure {
			fmt.Printf("testCase %d Success\n\n", i)
		}
	}
}

func TestStateKey(t *testing.T) {
	tree := NewMerkleTree(nil)
	var rootHash common.Hash
	var err error
	if err != nil {
		t.Errorf("Failed to initial BPT %v", err)
	}
	if debugBPT {
		fmt.Printf("Root Hash=%x \n", rootHash)
	}

	value, ok, err := tree.GetPreImageBlob(0, common.Hash(hex2Bytes("e6f0db7107765905cfdc1f19af6eb8ff07d89626f47429556d9a52b4e8b001d7")))

	if debugBPT {
		fmt.Printf("get value=%x, err=%v\n", value, err)
		if !ok {
			fmt.Printf("get key not found: %v\n", err)
		}
		//tree.printTree(tree.Root, 0)
	}
}

// Test service
func TestService(t *testing.T) {

	// Build the initial tree and insert the data

	tree := NewMerkleTree(nil)
	value1 := PadToMultipleOfN([]byte{1}, 68)
	value2 := PadToMultipleOfN([]byte{1, 2}, 68)
	value3 := PadToMultipleOfN([]byte{1, 2, 3}, 68)
	tree.SetService(42, value1)
	tree.SetService(43, value2)
	tree.SetService(44, value3)

	s1, _, _ := tree.GetService(42)
	s2, _, _ := tree.GetService(43)
	s3, _, _ := tree.GetService(44)
	s4, _, _ := tree.GetService(45)

	if debugBPT {
		fmt.Println("s1:", s1)
		fmt.Println("s2:", s2)
		fmt.Println("s3:", s3)
		fmt.Println("s4:", s4)
	}
}

// Test PreImage_lookup
func TestServicePreImage_lookup(t *testing.T) {

	// Build the initial tree and insert the data

	tree := NewMerkleTree(nil)

	case_a := []byte{1}
	case_b := []byte{1, 2}
	case_c := []byte{1, 2, 3}

	tree.SetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)), []uint32{100})
	tree.SetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)), []uint32{100, 200})
	tree.SetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)), []uint32{100, 200, 300})

	if debugBPT {
		tree.printTree(tree.Root, 0)
	}

	ts1, _, _ := tree.GetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	ts2, _, _ := tree.GetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	ts3, _, _ := tree.GetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))
	ts4, _, _ := tree.GetPreImageLookup(45, common.Blake2Hash(case_c), uint32(len(case_c)))

	if debugBPT {
		fmt.Println("ts1:", ts1)
		fmt.Println("ts2:", ts2)
		fmt.Println("ts3:", ts3)
		fmt.Println("ts4:", ts4)
	}
	tree.DeletePreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	tree.DeletePreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	tree.DeletePreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))
	_, _ = tree.Flush()

	ts1, _, _ = tree.GetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	ts2, _, _ = tree.GetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	ts3, _, _ = tree.GetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))
	if debugBPT {
		fmt.Println("ts1:", ts1)
		fmt.Println("ts2:", ts2)
		fmt.Println("ts3:", ts3)
	}
}

// Test PreImage_blob
func TestServicePreImage_blob(t *testing.T) {
	t.Skip("Skipping PreImage_blob test - not robust yet")
	// Build the initial tree and insert the data

	tree := NewMerkleTree(nil)

	tree.SetPreImageBlob(42, []byte{1})
	tree.SetPreImageBlob(43, []byte{1, 2})
	tree.SetPreImageBlob(44, []byte{1, 2, 3})

	if debugBPT {
		tree.printTree(tree.Root, 0)
	}

	blob1, ok1, err1 := tree.GetPreImageBlob(42, common.BytesToHash([]byte{1}))
	blob2, ok2, err2 := tree.GetPreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	blob3, ok3, err3 := tree.GetPreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))
	blob4, ok4, err4 := tree.GetPreImageBlob(45, common.BytesToHash([]byte{1, 2, 3}))
	if !ok1 || !ok2 || !ok3 || ok4 {
		t.Errorf("GetPreImageBlob failed: ok1=%v, ok2=%v, ok3=%v, ok4=%v", ok1, ok2, ok3, ok4)
	}
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		t.Errorf("GetPreImageBlob failed with error: err1=%v, err2=%v, err3=%v, err4=%v", err1, err2, err3, err4)
	}

	if debugBPT {
		fmt.Println("blob1:", blob1)
		fmt.Println("blob2:", blob2)
		fmt.Println("blob3:", blob3)
		fmt.Println("blob4:", blob4)
	}
	tree.DeletePreImageBlob(42, common.BytesToHash([]byte{1}))
	tree.DeletePreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	tree.DeletePreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))
	_, flush_err := tree.Flush()
	if flush_err != nil {
		t.Errorf("Flush after DeletePreImageBlob failed: %v", flush_err)
	}

	blob1, ok1, err1 = tree.GetPreImageBlob(42, common.BytesToHash([]byte{1}))
	blob2, ok2, err2 = tree.GetPreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	blob3, ok3, err3 = tree.GetPreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))
	if ok1 || ok2 || ok3 {
		t.Errorf("GetPreImageBlob after deletion should be empty: ok1=%v, ok2=%v, ok3=%v", ok1, ok2, ok3)
	}
	if err1 != nil || err2 != nil || err3 != nil {
		t.Errorf("GetPreImageBlob after deletion failed with error: err1=%v, err2=%v, err3=%v", err1, err2, err3)
	}
	if debugBPT {
		fmt.Println("blob1:", blob1)
		fmt.Println("blob2:", blob2)
		fmt.Println("blob3:", blob3)
	}

}

// Test Storage
func TestServiceStorage(t *testing.T) {

	// Build the initial tree and insert the data
	tree := NewMerkleTree(nil)

	k42 := common.ServiceStorageKey(42, []byte{1})
	k43 := common.ServiceStorageKey(43, []byte{1, 2})
	k44 := common.ServiceStorageKey(44, []byte{1, 2, 3})
	k45 := common.ServiceStorageKey(45, []byte{1, 2, 3})
	tree.SetServiceStorage(42, k42, []byte{1})
	tree.SetServiceStorage(43, k43, []byte{1, 2})
	tree.SetServiceStorage(44, k44, []byte{1, 2, 3})

	if debugBPT {
		tree.printTree(tree.Root, 0)
	}

	Storage1, _, _ := tree.GetServiceStorage(42, k42)
	Storage2, _, _ := tree.GetServiceStorage(43, k43)
	Storage3, _, _ := tree.GetServiceStorage(44, k44)
	Storage4, _, _ := tree.GetServiceStorage(45, k45)

	if debugBPT {
		fmt.Println("Storage1", Storage1)
		fmt.Println("Storage2", Storage2)
		fmt.Println("Storage3", Storage3)
		fmt.Println("Storage4", Storage4)
	}
	tree.DeleteServiceStorage(42, k42)
	tree.DeleteServiceStorage(43, k43)
	tree.DeleteServiceStorage(44, k44)
	_, _ = tree.Flush()

	Storage1, _, _ = tree.GetServiceStorage(42, k42)
	Storage2, _, _ = tree.GetServiceStorage(43, k43)
	Storage3, _, _ = tree.GetServiceStorage(44, k44)
	if debugBPT {
		fmt.Println("Storage1", Storage1)
		fmt.Println("Storage2", Storage2)
		fmt.Println("Storage3", Storage3)
	}

}

// Test TestLevelDB
func TestLevelDB(t *testing.T) {
	// Initialize the LevelDB
	tree := NewMerkleTree(nil)

	// Test data
	data := [][2][]byte{
		{hex2Bytes("A1"), []byte{1}},
		{hex2Bytes("A2"), []byte{1, 2}},
		{hex2Bytes("A3"), []byte{1, 2, 3}},
	}

	// Insert key-value pairs into the LevelDB
	for _, kv := range data {
		err := tree.levelDBSet(kv[0], kv[1])
		if err != nil {
			t.Fatalf("Failed to set key: %x, error: %v", kv[0], err)
		}
	}

	// Retrieve and verify key-value pairs from the LevelDB
	for _, kv := range data {
		value, ok, err := tree.levelDBGet(kv[0])
		if err != nil {
			t.Fatalf("Failed to get key: %x, error: %v", kv[0], err)
		}
		if !ok {
			t.Fatalf("Key: %x not found", kv[0])
		}
		if !compareBytes(value, kv[1]) {
			t.Fatalf("Value mismatch for key: %x, got: %x, want: %x", kv[0], value, kv[1])
		}
	}

	// Test non-existent key
	nonExistentKey := hex2Bytes("a411")
	_, ok, err := tree.levelDBGet(nonExistentKey)
	if err != nil {
		t.Fatalf("Error while getting non-existent key: %v", err)
	}
	if ok {
		t.Fatalf("Expected key: %x to be non-existent", nonExistentKey)
	}

}
