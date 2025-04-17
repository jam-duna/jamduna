package trie

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestZeroLeaves(t *testing.T) {
	var values [][]byte
	tree := NewWellBalancedTree(values, types.Keccak)
	fmt.Printf("%x\n", tree.Root())
}

// TestWellBalancedTree tests the MerkleB method of the WellBalancedTree
func TestWBMerkleTree(t *testing.T) {
	values := [][]byte{
		common.Hex2Bytes("03f9883f0100000001000000000000000000000000000000000000000000000000000000"),
		common.Hex2Bytes("e9ac0c500100000001000000000000000000000000000000000000000000000000000000"),
		//common.Hex2Bytes("d15f17c30100000002000000000000000000000000000000000000000000000000000000"),
	}
	tree := NewWellBalancedTree(values, types.Keccak)
	tree.PrintTree()
	fmt.Printf("RESULT: %x\n", tree.Root())
}

func TestWBTTrace(t *testing.T) {
	// List of different network setting
	//numShardsList := []int{6, 12, 18, 36, 108, 342, 684, 1023}
	numShardsList := []int{6}
	for _, numShards := range numShardsList {
		shardJustifications := make([]types.Justification, numShards)
		t.Logf("Testing with %d shards", numShards)
		values := make([][]byte, 0, numShards)
		for i := 0; i < numShards; i++ {
			bclub_Raw := []byte(fmt.Sprintf("b_club_value%d", i))
			sclub_Raw := []byte(fmt.Sprintf("s_club_value%d", i))
			bclub_h := common.Blake2Hash(bclub_Raw)
			sclub_h := common.Blake2Hash(sclub_Raw)
			// Each value is the concatenation of two 32-byte hashes, making 64 bytes.
			value := append(bclub_h[:], sclub_h[:]...)
			values = append(values, value)
		}
		t.Logf("Values: %x", values)

		// Build the tree using Blake2b.
		wbt := NewWellBalancedTree(values, types.Blake2b)
		t.Log("Tree structure:")
		wbt.PrintTree()

		erasureRoot := wbt.RootHash()
		t.Logf("ErasureRoot: %v", erasureRoot)

		// For each shard, get the proof and verify.
		for shardIndex := 0; shardIndex < numShards; shardIndex++ {
			treeLen, leafHash, path, isFound, err := wbt.Trace(shardIndex)
			if err != nil || !isFound {
				t.Errorf("Trace error for shard %d: %v", shardIndex, err)
				continue
			}

			derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, erasureRoot, leafHash, path)
			if err != nil || !verified {
				t.Errorf("VerifyWBT error for shard %d: %v", shardIndex, err)
				continue
			}

			shardJustification := types.Justification{
				Root:     erasureRoot,
				ShardIdx: shardIndex,
				TreeLen:  treeLen,
				LeafHash: leafHash,
				Path:     path,
			}
			shardJustifications[shardIndex] = shardJustification

			// encode the justification
			rawPath := shardJustification.Path
			encodedPath, err := common.EncodeJustification(rawPath, 6)
			if err != nil {
				t.Fatalf("EncodeJustification error: %v", err)
			}

			// decode the justification
			decodedPath, err := common.DecodeJustification(encodedPath, 6)
			if err != nil {
				t.Fatalf("DecodeJustification error: %v", err)
			}
			if !reflect.DeepEqual(rawPath, decodedPath) {
				t.Logf("ShardIndex=%v     RawPath: %x\n", shardIndex, rawPath)
				t.Logf("ShardIndex=%v DecodedPath: %x\n", shardIndex, decodedPath)
				t.Fatalf("shardIndex=%v justificationsPath mismatch!", shardIndex)
			} else {
				t.Logf("shardIndex=%v justificationsPath match!", shardIndex)
			}

			if !bytes.Equal(derivedRoot[:], erasureRoot[:]) {
				t.Errorf("ShardIndex %d: ErasureRoot=%v, got %v", shardIndex, erasureRoot, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			} else {
				t.Logf("ShardIndex %d verified. DerivedRoot=%v", shardIndex, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			}
		}
	}
}

func TestGenerateWBTJustification(t *testing.T) {
	// Initialize the tree with some values
	leavesHex := []string{
		"4c983c031df17976b42ae88506f5ed659e2657830c3d2f5b0fcae5aacd8c100e0000000000000000000000000000000000000000000000000000000000000000",
		"6b8d377ba694e1959fb1bd8e62dc6ef003d29f5062bbb14740fe9260d1dbc3db0000000000000000000000000000000000000000000000000000000000000000",
		"689448c578471a6e1a58da502a49fbddd99e6985a5d5c4b33dc8304ab79f41260000000000000000000000000000000000000000000000000000000000000000",
		"e478263d15b659f75135bd1f4d67c62315d516a080a6d079c5588281ebb585c00000000000000000000000000000000000000000000000000000000000000000",
		"90890d40266a075da42bbb8de2652a1f5d894e0b0adc6b54200221204a04c4410000000000000000000000000000000000000000000000000000000000000000",
		"2773dac2f5248da9ec88721ba092da5f4966f6f2a94b1d6112e3428c3d3130fd0000000000000000000000000000000000000000000000000000000000000000",
	}

	// Decode leaves
	numShards := len(leavesHex)
	values := make([][]byte, numShards)
	for i, hx := range leavesHex {
		decoded, _ := hex.DecodeString(hx)
		values[i] = decoded
	}
	fmt.Printf("TestGenerateWBTJustification values: %x\n", values)
	wbt := NewWellBalancedTree(values, types.Blake2b)
	wbt.PrintTree()

	// Test the Trace method to get the proof path for a given index
	for shardIndex := 0; shardIndex < numShards; shardIndex++ {
		treeLen, leafHash, path, isFound, err := wbt.Trace(int(shardIndex))
		if err != nil || !isFound {
			t.Errorf("Trace error: %v", err)
		}

		derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, wbt.RootHash(), leafHash, path)

		if err != nil || verified == false {
			t.Errorf("VerifyWBT error: %v", err)
		}
		expectedHash := wbt.RootHash()
		if !bytes.Equal(derivedRoot[:], expectedHash[:]) {
			t.Errorf("shardIndex %d, expected hash %v, got %v", shardIndex, expectedHash, derivedRoot)
		} else {
			fmt.Printf("shardIndex %d, expected hash %v, got %v\n", shardIndex, expectedHash, derivedRoot)
			fmt.Printf("treeLen=%v, leafHash=%x path: %x\n", treeLen, leafHash, path)
		}
	}
}

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// TODO
func TestWBTTrace2(t *testing.T) {
	// List of different network setting
	numShardsList := []int{6, 12, 18, 36, 108, 342, 684, 1023}
	ecShardSizeList := []int{2052, 1026, 684, 342, 114, 36, 18, 12}
	numECPiecesPerSegmentList := []int{1026, 513, 342, 171, 57, 18, 9, 6}
	networkList := []string{"tiny", "small", "medium", "large", "xlarge", "2xlarge", "3xlarge", "full"}
	for listIdx, numShards := range numShardsList {
		ecShardSize := ecShardSizeList[listIdx]
		numECPieceSize := numECPiecesPerSegmentList[listIdx]
		network := networkList[listIdx]
		shardJustifications := make([]types.Justification, numShards)
		t.Logf("Testing NetWork: %v with %d shards (V=%d) | ShardByteSize=%d (WP=%d)\n", network, numShards, numShards, ecShardSize, numECPieceSize)
		values := make([][]byte, 0, numShards)
		for i := 0; i < numShards; i++ {
			bclub_Raw := []byte(fmt.Sprintf("b_club_value%d", i))
			sclub_Raw := fmt.Appendf(nil, "s_club_value%d", i)
			bclub_h := common.Blake2Hash(bclub_Raw)
			sclub_h := common.Blake2Hash(sclub_Raw)
			// Each value is the concatenation of two 32-byte hashes, making 64 bytes.
			value := append(bclub_h[:], sclub_h[:]...)
			values = append(values, value)
		}
		bclub_sclub_len := len(values[0])
		//t.Logf("Values: %x", values)

		// Build the tree using Blake2b.
		wbt := NewWellBalancedTree(values, types.Blake2b)
		t.Log("Tree structure:")
		//wbt.PrintTree()

		erasureRoot := wbt.RootHash()
		t.Logf("ErasureRoot: %v", erasureRoot)

		// For each shard, get the proof and verify.
		for shardIndex := 0; shardIndex < numShards; shardIndex++ {
			treeLen, leafHash, path, isFound, err := wbt.Trace(shardIndex)
			if err != nil || !isFound {
				t.Errorf("Trace error for shard %d: %v", shardIndex, err)
				continue
			}

			derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, erasureRoot, leafHash, path)
			if err != nil || !verified {
				t.Errorf("VerifyWBT error for shard %d: %v", shardIndex, err)
				continue
			}

			shardJustification := types.Justification{
				Root:     erasureRoot,
				ShardIdx: shardIndex,
				TreeLen:  treeLen,
				LeafHash: leafHash,
				Path:     path,
			}
			shardJustifications[shardIndex] = shardJustification

			// encode the justification
			rawPath := shardJustification.Path
			encodedPath, err := common.EncodeJustification(rawPath, numECPieceSize)
			if err != nil {
				t.Fatalf("EncodeJustification error: %v", err)
			}
			encodedPathLen := len(encodedPath)
			upperPathLen := ComputeEncodedProofSize(shardIndex, numShards, bclub_sclub_len)
			segPathLen := ComputeEncodedProofSize(shardIndex, numShards, numECPieceSize*2)
			segPathLen2 := ComputeEncodedProofSize(shardIndex, numShards, numECPieceSize*2*1000)
			fmt.Printf("ShardIdx=%d, numShards=%d, EncodedPath Len: %d | UpperProofLen=%d | SegProofLen=%d | %d\n", shardIndex, numShards, encodedPathLen, upperPathLen, segPathLen, segPathLen2)

			if upperPathLen != encodedPathLen {
				t.Fatalf("EncodedPath length mismatch: %d != %d", upperPathLen, encodedPathLen)
			}

			// decode the justification
			decodedPath, err := common.DecodeJustification(encodedPath, numECPieceSize)
			if err != nil {
				t.Fatalf("DecodeJustification error: %v", err)
			}
			if !reflect.DeepEqual(rawPath, decodedPath) {
				t.Logf("ShardIndex=%v     RawPath: %x\n", shardIndex, rawPath)
				t.Logf("ShardIndex=%v DecodedPath: %x\n", shardIndex, decodedPath)
				t.Fatalf("shardIndex=%v justificationsPath mismatch!", shardIndex)
			} else {
				//t.Logf("shardIndex=%v justificationsPath match!", shardIndex)
			}

			if !bytes.Equal(derivedRoot[:], erasureRoot[:]) {
				t.Errorf("ShardIndex %d: ErasureRoot=%v, got %v", shardIndex, erasureRoot, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			} else {
				//t.Logf("ShardIndex %d verified. DerivedRoot=%v", shardIndex, derivedRoot)
				//t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			}
		}
	}
}
