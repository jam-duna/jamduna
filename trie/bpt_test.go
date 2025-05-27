package trie

import (
	//"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
)

const bptDebug = true

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
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

	for _, item := range data {
		tree.Insert(item[0], item[1])
	}
	if bptDebug {
		tree.printTree(tree.Root, 0)
	}
	getValue, _, err := tree.Get(hex2Bytes("0200000000000000000000000000000000000000000000000000000000000000"))
	if err != nil {
		t.Errorf("Key: %x not found, error: %s\n", hex2Bytes("0000"), err)
	}
	if bptDebug {
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
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

	for _, item := range data {
		tree.Insert(item[0], item[1])
	}
	if bptDebug {
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
	tree.Close()
	if bptDebug {
		fmt.Printf("GetPath %x\n", path)
		t.Logf("Trace path: %x\n", trace)
	}
	DeleteLevelDB()
}

func TestMerkleTree(t *testing.T) {
	// Read the JSON file
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
		if bptDebug {
			fmt.Printf("testCase %d: %v\n", i, testCase)
		}

		isSerialized := true
		var rootHash []byte

		// Create an empty Merkle Tree
		test_db, _ := initLevelDB()
		tree := NewMerkleTree(nil, test_db)

		if isSerialized {
			// Insert key-value pairs one by one
			for k, v := range testCase.Input {
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				tree.Insert(key, value)
				if bptDebug {
					fmt.Printf("computeHash:) %x\n", computeHash(leaf(key, value)))
				}
			}
			rootHash = tree.GetRootHash()
		}
		// Compare the computed root hash with the expected output

		if bptDebug {
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

			// Test Valid Proof
			if tree.Verify(key, value, expectedHash, path) {
				if bptDebug {
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
				if bptDebug {
					fmt.Printf("Key: %x, Value: %x\n", key, getValue)
				}
			}
		}
		// os.RemoveAll(dbPath) // Clean up the LevelDB folder(if needed)
		tree.Close()
	}
	DeleteLevelDB()
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
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)
	if bptDebug {
		fmt.Printf("initial rootHash: %x\n", tree.GetRootHash())
	}

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		if bptDebug {
			fmt.Printf("insert#%d, key=%x, value=%x\n", index, key, value)
		}
		tree.Insert(key, value)
	}

	if bptDebug {
		tree.printTree(tree.Root, 0)
	}
	wrongKey := hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522ff")
	_, err := tree.Trace(wrongKey)
	if err == nil {
		t.Errorf("Proof should not exist non arbitrary key [%x].\n", wrongKey)
	}

	key := hex2Bytes("f2a9fcaf8ae0ff770b0908ebdee1daf8457c0ef5e1106c89ad364236333c5fb3")

	path, err := tree.Trace(key)
	if bptDebug {
		fmt.Printf("Proof path for key %x: \n", key)
	}
	for i, p := range path {
		if bptDebug {
			fmt.Printf("  Node %d: %x\n", i, p)
		}
	}
	if err != nil {
		t.Errorf("Unable to generate Proof for key [%x].Err %v\n", key, err)
	}

	// Test Valid Proof
	value, _, _ := tree.Get(key)
	if bptDebug {
		fmt.Printf("levelDBGet key=%x, value=%x\n", key, value)
	}
	if tree.Verify(key, value, tree.GetRootHash(), path) {
		if bptDebug {
			fmt.Printf("Proof for key [%x] is valid.\n", key)
		}
	} else {
		t.Errorf("Proof for key [%x] is invalid.\n", key)
	}

	// Test Invalid Proof
	invalidValue := []byte("invalid")
	if tree.Verify(key, invalidValue, exceptedRootHash, path) {
		t.Errorf("Proof for key [%x] with invalid value is valid.\n", invalidValue)
	} else {
		if bptDebug {
			fmt.Printf("Proof for key [%x] with invalid value is invalid.\n", invalidValue)
		}
	}
	tree.Close()
	DeleteLevelDB()
}

func TestGet(t *testing.T) {
	data := [][2][]byte{
		{hex2Bytes("3dbc5f775f6156957139100c343bb5ae6589af7398db694ab6c60630a9ed0fcd"), hex2Bytes("bb11c256876fe10442213dd78714793394d2016134c28a64eb27376ddc147fc6044df72bdea44d9ec66a3ea1e6d523f7de71db1d05a980e001e9fa")},
		{hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e7"), hex2Bytes("e46ddd41a5960807d528f5d9282568e622a023b94b72cb63f0353baff189257d")},
	}

	// Create an empty Merkle Tree
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)
	if bptDebug {
		fmt.Printf("initial rootHash: %x\n", tree.GetRootHash())
	}

	// Insert key-value pairs one by one
	for index, kv := range data {
		key := kv[0]
		value := kv[1]
		if bptDebug {
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
			if bptDebug {
				fmt.Printf("Key: %x, Value: %x\n", key, value)
			}
		}
	}
	for _, key := range validKeys {
		value, ok, err := tree.Get(key)
		if err != nil || !ok {
			if bptDebug {
				fmt.Printf("Key: %x not found, error: %s\n", key, err)
			}
		} else {
			t.Errorf("Key: %x, Value: %x\n", key, value)
		}
	}

	tree.Close()
	DeleteLevelDB()
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
			if bptDebug {
				fmt.Printf("testCase %d: %v\n", i, testCase)
			}
			var input [][2][]byte
			for k, v := range testCase.Input {
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				input = append(input, [2][]byte{key, value})
			}
			if bptDebug {
				fmt.Printf("input=%x (len=%v)\n", input, len(input))
			}

			var rootHash []byte

			// Create an empty Merkle Tree
			test_db, _ := initLevelDB()
			tree := NewMerkleTree(nil, test_db)

			if bptDebug {
				fmt.Printf("initial rootHash:%x \n", tree.GetRootHash())
			}
			// Insert key-value pairs one by one
			index := 0
			rand.Seed(time.Now().UnixNano())
			index = 0
			for k, v := range testCase.Input {
				if bptDebug {
					fmt.Printf("insert#%d, k=%v, v=%v\n", index, k, v)
				}
				key, _ := hex.DecodeString(k)
				value, _ := hex.DecodeString(v)
				tree.Insert(key, value)
				//tree.printTree(tree.Root, 0)
				index++
			}
			rootHash = tree.GetRootHash()

			// Compare the computed root hash with the expected output
			expectedHash, _ := hex.DecodeString(testCase.Output)
			if !compareBytes(rootHash, expectedHash) {
				if bptDebug {
					tree.PrintTree(tree.Root, 0)
				}
				t.Errorf("Test case %d in file %s: Root hash mismatch for input %v: got %s, want %s", i, filePath, testCase.Input, common.Bytes2Hex(rootHash), testCase.Output)
			} else {
				t.Logf("Test case %d in file %s: Vector OK, rootHash=%x", i, filePath, expectedHash)
			}
			//tree.PrintTree(tree.Root, 0)
			tree.Close()
		}
	}
	DeleteLevelDB()
}

func TestModify(t *testing.T) {
	return
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
		if bptDebug {
			fmt.Printf("testCase %d: %v\n", i, testCase)
		}
		var input [][2][]byte
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			input = append(input, [2][]byte{key, value})
		}
		if bptDebug {
			fmt.Printf("input=%x (len=%v)\n", input, len(input))
		}

		var rootHash []byte

		// Create an empty Merkle Tree
		test_db, _ := initLevelDB()
		tree := NewMerkleTree(nil, test_db)

		if bptDebug {
			fmt.Printf("initial rootHash:%x \n", tree.GetRootHash())
		}
		// Insert key-value pairs one by one
		index := 0
		rand.Seed(time.Now().UnixNano())
		index = 0
		for k, v := range testCase.Input {
			if bptDebug {
				fmt.Printf("insert#%d, k=%v, v=%v\n", index, k, v)
			}
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			tree.Insert(key, value)
			//tree.printTree(tree.Root, 0)
			index++
		}
		rootHash = tree.GetRootHash()

		// Compare the computed root hash with the expected output
		expectedHash, _ := hex.DecodeString(testCase.Output)
		if !compareBytes(rootHash, expectedHash) {
			if bptDebug {
				tree.PrintTree(tree.Root, 0)
			}
			t.Errorf("Test case %d: Root hash mismatch for input %v: got %s, want %s", i, testCase.Input, common.Bytes2Hex(rootHash), testCase.Output)
		} else {
			t.Logf("Test case %d: Vector OK, rootHash=%x", i, expectedHash)
		}
		tree.Close()
	}
	DeleteLevelDB()
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
	bptDebug := true
	filePath := "../jamtestvectors/trie/trie.json"
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
		if bptDebug {
			fmt.Printf("==== testCase %d ====\n", i)
			//fmt.Printf("kv:%v\n", testCase)
		}
		var data [][2][]byte
		for k, v := range testCase.Input {
			key, _ := hex.DecodeString(k)
			value, _ := hex.DecodeString(v)
			data = append(data, [2][]byte{key, value})
		}
		if bptDebug {
			//fmt.Printf("data=%x (len=%v)\n", data, len(data))
			for idx, kv := range data {
				key := kv[0]
				value := kv[1]
				fmt.Printf("key[%d]: %x, value(len=%d): %v\n", idx, key, len(value), truncateValue(value))
			}
		}

		test_db, _ := initLevelDB()
		tree := NewMerkleTree(nil, test_db)

		// Record root hashes after each insertion
		var rootHashes [][]byte

		if bptDebug {
			fmt.Printf("-------- Testing Insert ---------- \n")
		}
		for idx, kv := range data {
			tree.Insert(kv[0], kv[1])
			rootHash := tree.GetRootHash()
			rootHashes = append(rootHashes, rootHash)
			if bptDebug || true {
				fmt.Printf("H'=%x, Inserted key[%d]: %x, value(len=%d): %v\n", rootHash, idx, kv[0], len(kv[1]), truncateValue(kv[1]))
			}
		}

		// Check root hashes after each deletion in reverse order
		if bptDebug {
			fmt.Printf("-------- Testing Deletion -------- \n")
		}
		isFailure := false
		for i := len(data) - 1; i >= 0; i-- {
			kv := data[i]
			err := tree.Delete(kv[0])
			if err != nil {
				t.Fatalf("Failed to delete key: %x, error: %v", kv[0], err)
			}
			rootHash := tree.GetRootHash()
			expectedRootHash := make([]byte, 32)
			if i > 0 {
				expectedRootHash = rootHashes[i-1]
			}
			if bptDebug || true {
				fmt.Printf("H':%x, Deleted key[%d]: %x\n", rootHash, i, kv[0])
			}
			if !compareBytes(rootHash, expectedRootHash) {
				isFailure = true
				t.Fatalf("RootHash mismatch after deleting key: %x. Got: %x, Expected: %x", kv[0], rootHash, expectedRootHash)
			}
		}
		if bptDebug && !isFailure {
			fmt.Printf("testCase %d Success\n\n", i)
		}
		tree.Close()
	}

	DeleteLevelDB()
}

func TestStateKey(t *testing.T) {
	test_db, _ := initLevelDB()
	rootHash, tree, err := Initial_bpt(test_db)
	if err != nil {
		t.Errorf("Failed to initial BPT %v", err)
	}
	if bptDebug {
		fmt.Printf("Root Hash=%x \n", rootHash)
	}

	value, ok, err := tree.GetPreImageBlob(0, common.Hash(hex2Bytes("e6f0db7107765905cfdc1f19af6eb8ff07d89626f47429556d9a52b4e8b001d7")))

	if bptDebug {
		fmt.Printf("get value=%x, err=%v\n", value, err)
		if !ok {
			fmt.Printf("get key not found: %v\n", err)
		}
		//tree.printTree(tree.Root, 0)
	}
	tree.Close()
	DeleteLevelDB()
}

// TestInitial tests the initialization of a Merkle tree from a root hash
func TestInitial(t *testing.T) {

	data := [][2][]byte{
		{hex2Bytes("5dffe0e2c9f089d30e50b04ee562445cf2c0e7e7d677580ef0ccf2c6fa3522dd"), hex2Bytes("bb11c256876fe10442213dd78714793394d2016134c28a64eb27376ddc147fc6044df72bdea44d9ec66a3ea1e6d523f7de71db1d05a980e001e9fa")},
		{hex2Bytes("df08871e8a54fde4834d83851469e635713615ab1037128df138a6cd223f1242"), hex2Bytes("b8bded4e1c")},
		{hex2Bytes("7723a8383e43a1713eb920bae44880b2ae9225ea2d38c031cf3b22434b4507e7"), hex2Bytes("e46ddd41a5960807d528f5d9282568e622a023b94b72cb63f0353baff189257d")},
		{hex2Bytes("3e7d409b9037b1fd870120de92ebb7285219ce4526c54701b888c5a13995f73c"), hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
		{hex2Bytes("0200000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
		{hex2Bytes("0300000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
		{hex2Bytes("0D00000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
		{hex2Bytes("0100000000000000000000000000000000000000000000000000000000000000"), hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
	}

	// Build the initial tree and insert the data
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)
	for _, item := range data {
		tree.Insert(item[0], item[1])
	}

	if bptDebug {
		tree.printTree(tree.Root, 0)
	}
	// Get the root hash of the tree
	rootHash := tree.GetRootHash()

	// Rebuild the recovered tree (rt) from the root hash
	rt, err := InitMerkleTreeFromHash(rootHash, test_db)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if bptDebug {
		rt.printTree(rt.Root, 0)
	}
	// Compare the initial tree with the reconstructed tree
	if !compareTrees(tree.Root, rt.Root) {
		t.Error("The reconstructed tree does not match the initial tree")
	}
	DeleteLevelDB()
}

// Test service
func TestService(t *testing.T) {

	// Build the initial tree and insert the data
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)
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

	if bptDebug {
		fmt.Println("s1:", s1)
		fmt.Println("s2:", s2)
		fmt.Println("s3:", s3)
		fmt.Println("s4:", s4)
	}
}

// Test PreImage_lookup
func TestServicePreImage_lookup(t *testing.T) {

	// Build the initial tree and insert the data
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

	case_a := []byte{1}
	case_b := []byte{1, 2}
	case_c := []byte{1, 2, 3}

	tree.SetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)), []uint32{100})
	tree.SetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)), []uint32{100, 200})
	tree.SetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)), []uint32{100, 200, 300})

	if bptDebug {
		tree.printTree(tree.Root, 0)
	}

	ts1, _, _ := tree.GetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	ts2, _, _ := tree.GetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	ts3, _, _ := tree.GetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))
	ts4, _, _ := tree.GetPreImageLookup(45, common.Blake2Hash(case_c), uint32(len(case_c)))

	if bptDebug {
		fmt.Println("ts1:", ts1)
		fmt.Println("ts2:", ts2)
		fmt.Println("ts3:", ts3)
		fmt.Println("ts4:", ts4)
	}
	_ = tree.DeletePreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	_ = tree.DeletePreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	_ = tree.DeletePreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))

	ts1, _, _ = tree.GetPreImageLookup(42, common.Blake2Hash(case_a), uint32(len(case_a)))
	ts2, _, _ = tree.GetPreImageLookup(43, common.Blake2Hash(case_b), uint32(len(case_b)))
	ts3, _, _ = tree.GetPreImageLookup(44, common.Blake2Hash(case_c), uint32(len(case_c)))
	if bptDebug {
		fmt.Println("ts1:", ts1)
		fmt.Println("ts2:", ts2)
		fmt.Println("ts3:", ts3)
	}
}

// Test PreImage_blob
func TestServicePreImage_blob(t *testing.T) {

	// Build the initial tree and insert the data
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

	tree.SetPreImageBlob(42, []byte{1})
	tree.SetPreImageBlob(43, []byte{1, 2})
	tree.SetPreImageBlob(44, []byte{1, 2, 3})

	if bptDebug {
		tree.printTree(tree.Root, 0)
	}

	blob1, _, _ := tree.GetPreImageBlob(42, common.BytesToHash([]byte{1}))
	blob2, _, _ := tree.GetPreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	blob3, _, _ := tree.GetPreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))
	blob4, _, _ := tree.GetPreImageBlob(45, common.BytesToHash([]byte{1, 2, 3}))

	if bptDebug {
		fmt.Println("blob1:", blob1)
		fmt.Println("blob2:", blob2)
		fmt.Println("blob3:", blob3)
		fmt.Println("blob4:", blob4)
	}
	_ = tree.DeletePreImageBlob(42, common.BytesToHash([]byte{1}))
	_ = tree.DeletePreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	_ = tree.DeletePreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))

	blob1, _, _ = tree.GetPreImageBlob(42, common.BytesToHash([]byte{1}))
	blob2, _, _ = tree.GetPreImageBlob(43, common.BytesToHash([]byte{1, 2}))
	blob3, _, _ = tree.GetPreImageBlob(44, common.BytesToHash([]byte{1, 2, 3}))
	if bptDebug {
		fmt.Println("blob1:", blob1)
		fmt.Println("blob2:", blob2)
		fmt.Println("blob3:", blob3)
	}

}

// Test Storage
func TestServiceStorage(t *testing.T) {

	// Build the initial tree and insert the data
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

	k42 := common.ServiceStorageKey(42, []byte{1})
	k43 := common.ServiceStorageKey(43, []byte{1, 2})
	k44 := common.ServiceStorageKey(44, []byte{1, 2, 3})
	k45 := common.ServiceStorageKey(45, []byte{1, 2, 3})
	tree.SetServiceStorage(42, k42, []byte{1})
	tree.SetServiceStorage(43, k43, []byte{1, 2})
	tree.SetServiceStorage(44, k44, []byte{1, 2, 3})

	if bptDebug {
		tree.printTree(tree.Root, 0)
	}

	Storage1, _, _ := tree.GetServiceStorage(42, k42)
	Storage2, _, _ := tree.GetServiceStorage(43, k43)
	Storage3, _, _ := tree.GetServiceStorage(44, k44)
	Storage4, _, _ := tree.GetServiceStorage(45, k45)

	if bptDebug {
		fmt.Println("Storage1", Storage1)
		fmt.Println("Storage2", Storage2)
		fmt.Println("Storage3", Storage3)
		fmt.Println("Storage4", Storage4)
	}
	_ = tree.DeleteServiceStorage(42, k42)
	_ = tree.DeleteServiceStorage(43, k43)
	_ = tree.DeleteServiceStorage(44, k44)

	Storage1, _, _ = tree.GetServiceStorage(42, k42)
	Storage2, _, _ = tree.GetServiceStorage(43, k43)
	Storage3, _, _ = tree.GetServiceStorage(44, k44)
	if bptDebug {
		fmt.Println("Storage1", Storage1)
		fmt.Println("Storage2", Storage2)
		fmt.Println("Storage3", Storage3)
	}
	tree.Close()
	DeleteLevelDB()
}

// Test TestLevelDB
func TestLevelDB(t *testing.T) {
	// Initialize the LevelDB
	test_db, _ := initLevelDB()
	tree := NewMerkleTree(nil, test_db)

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
	fmt.Printf("ok %v, err %v\n", ok, err)
	if err != nil {
		t.Fatalf("Error while getting non-existent key: %v", err)
	}
	if ok {
		t.Fatalf("Expected key: %x to be non-existent", nonExistentKey)
	}

}
