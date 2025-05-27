package trie

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/crypto/blake2b"
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
	/*
		leavesHex := []string{
			"4c983c031df17976b42ae88506f5ed659e2657830c3d2f5b0fcae5aacd8c100e0000000000000000000000000000000000000000000000000000000000000000",
			"6b8d377ba694e1959fb1bd8e62dc6ef003d29f5062bbb14740fe9260d1dbc3db0000000000000000000000000000000000000000000000000000000000000000",
			"689448c578471a6e1a58da502a49fbddd99e6985a5d5c4b33dc8304ab79f41260000000000000000000000000000000000000000000000000000000000000000",
			"e478263d15b659f75135bd1f4d67c62315d516a080a6d079c5588281ebb585c00000000000000000000000000000000000000000000000000000000000000000",
			"90890d40266a075da42bbb8de2652a1f5d894e0b0adc6b54200221204a04c4410000000000000000000000000000000000000000000000000000000000000000",
			"2773dac2f5248da9ec88721ba092da5f4966f6f2a94b1d6112e3428c3d3130fd0000000000000000000000000000000000000000000000000000000000000000",
		}
	*/
	leavesHex := []string{
		"0x8fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f",
		"0x996dfd5a1bfbe7475e06c80b162964072c4568ec2ee542028c32369ac2425086",
		"0xe24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855",
		"0x80fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd",
		"0x05a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c57",
		"0x1b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3",
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

func TestMKWBTTrace(t *testing.T) {
	// all six of the failing shards from your log above
	tests := []struct {
		treeLen     int
		shardIdx    int
		erasureRoot common.Hash
		leafHash    []byte
		path        [][]byte
		encodedPath []byte
		bundleShard []byte
	}{
		{
			// shard 0
			treeLen:     6,
			shardIdx:    0,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0x8fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f0000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0xedce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded"),
				common.FromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
				common.FromHex("0xe24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855"),
				common.FromHex("0x996dfd5a1bfbe7475e06c80b162964072c4568ec2ee542028c32369ac2425086"),
			},
			encodedPath: common.FromHex("0x00edce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded010000000000000000000000000000000000000000000000000000000000000000e24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855010000000000000000000000000000000000000000000000000000000000000000996dfd5a1bfbe7475e06c80b162964072c4568ec2ee542028c32369ac2425086"),
			bundleShard: common.FromHex("0x0000000000fa002583136a79daec5ca5e802d27732517c3f7dc4970dc2d68abcaad891e99d001fd1664d60a77f32449f2ed9898f3eb0fef21ba0537b014276f7ff7042355c3a8f001f0b92def425b5cbd7c14118f67d99654e26894a76187f453631503f120cfee77aea940e38601cb828d2878308e32529f8382cf85f7a29b75f1c826985bc1db7"),
		},
		{
			// shard 1
			treeLen:     6,
			shardIdx:    1,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0x996dfd5a1bfbe7475e06c80b162964072c4568ec2ee542028c32369ac24250860000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0xedce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded"),
				common.FromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
				common.FromHex("0xe24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855"),
				common.FromHex("0x8fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f"),
			},
			encodedPath: common.FromHex("0x00edce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded00000000000000000000000000000000000000000000000000000000000000000e24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f98558fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f"),
			bundleShard: common.FromHex("0xf36f81fc164ee563dc27b8e940a40f8c4bc334fe5964678b1ed8cc848f6111451f0000010000000028a0e0c33fcc7cadbb6627bcc902064e89ba1d16c26ea88371a4f614942fb8372906e90be226abaccc638a0addae983f814190ba97367f8abc5e21642a1e48f23d57724100000000000000105e5f000000008096980000000000000000000000"),
		},
		{
			// shard 2
			treeLen:     6,
			shardIdx:    2,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0xe24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f98550000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0xedce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded"),
				common.FromHex("0xa1b718601179e4c6203872c53f4ff8adb2bd0976f9d95d8631f5881aaf335b3d"),
			},
			encodedPath: common.FromHex("0x00edce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded00a1b718601179e4c6203872c53f4ff8adb2bd0976f9d95d8631f5881aaf335b3d"),
			bundleShard: common.FromHex("0x5f8ebe44f503309e200d05b296518990e554ef9c404d8ec64aed5fe6f72ee5f1cfaec18a510018abba27b6260875c789132f069b04411e05cf8166f6ef7ec45ac1038e0e0b1ff81f8c0c2baf8024fe4b781c7837700473f77fa6b30b3d277315c3bfd3af6b2309c2a31f14b92deaa54460a92e4930145e1cce23277413d19d4f7835d9efc44bb74f"),
		},
		{
			// shard 3
			treeLen:     6,
			shardIdx:    3,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0x80fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd0000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0x87251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c596"),
				common.FromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
				common.FromHex("0x1b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3"),
				common.FromHex("0x5a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c57"),
			},
			encodedPath: common.FromHex("0x0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c59600000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a305a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c57"),
			bundleShard: common.FromHex("0xace13fb8e3b7d5d87f39d7220c19dab9469509152b78957229f1046fba997e087a765062cc00077af4ca3642488bffbb8690a8a8f3f3e6b95d9b289b2c521a2e4fd73a2fc30acf28ba01507a9627e02c63beb3255bd772adb0c1aafbdc0973da49d0a2f45331bfd7e4a2f2f6158ab9fc487ba9da66a87b35361b8b1ad4abb4f827295b8641f7aaf8"),
		},
		{
			// shard 4
			treeLen:     6,
			shardIdx:    4,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0x05a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c570000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0x87251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c596"),
				common.FromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
				common.FromHex("0x1b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3"),
				common.FromHex("0x80fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd"),
			},
			encodedPath: common.FromHex("0x0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c59600000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a380fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd"),
			bundleShard: common.FromHex("0x5ddbd3e3d0741142c4e07a4edc8178199211fd092da6a9b16e8f357366571a5d5b821e46af005488bfe02e7448da86610f5fda751b33537bb7e674ab17fcaa6e822b188ef17246fc3d0ea481ef2a9a17e3081159b92fef4938fa1ba823ad4e5cf7ea1217af4dc6affa67fe6a57b5b137c384e9bb6d6a939505caf4274ce95e36b8571dbfb2359936"),
		},
		{
			// shard 5
			treeLen:     6,
			shardIdx:    5,
			erasureRoot: common.HexToHash("0xe5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"),
			leafHash: common.FromHex(
				"0x1b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a30000000000000000000000000000000000000000000000000000000000000000",
			),
			path: [][]byte{
				common.FromHex("0x87251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c596"),
				common.FromHex("0xb7770152b42fe4495520c0f0c26fd4ad594df6acef3d4e8931613f5cd38c34c8"),
			},
			encodedPath: common.FromHex("0x0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c59600b7770152b42fe4495520c0f0c26fd4ad594df6acef3d4e8931613f5cd38c34c8"),
			bundleShard: common.FromHex("0xaeb4521fc6c0f4049bd4a8de46c92b3031d01b804693b2050d936efa2be081a4ee5a8fae32004b59f10dae100824be539ae07446ec81abc725fc3ac6d4d0741a0cffacaf396771cb0b03df54f9298470f8aada4b92fcee13f79d0258c2834e937d85634c975f70babdda18256fd5ad8feb566e283bd6b6bcfdf258498b937781e74b9fd637898481"),
		},
	}

	for _, tc := range tests {
		tc.leafHash = tc.leafHash[:32]
		recovered, ok, err := VerifyWBT(
			tc.treeLen,
			tc.shardIdx,
			tc.erasureRoot,
			tc.leafHash,
			tc.path,
		)
		if err != nil {
			t.Fatalf("VerifyWBT returned error for shard %d: %v", tc.shardIdx, err)
		}
		if !ok {
			bClub := common.Blake2Hash(tc.bundleShard)
			sClub := common.Hash{}
			bClub_sClub := append(bClub[:], sClub[:]...)
			h_bs := common.Blake2Hash(bClub_sClub)
			t.Errorf(
				"\nshard %d:\n\nexp %v\ngot %v\n\nbClub=%v\nsClub=%v\n(b,s)=%x\nH(b,s)=%v\n\nbundleShard=0x%x\n\npathLen=%d\npath=%x\n\n\n",
				tc.shardIdx,
				tc.erasureRoot,
				recovered,
				bClub,
				sClub,
				bClub_sClub,
				h_bs,
				tc.bundleShard,
				len(tc.path),
				tc.path,
				//tc.encodedPath,
			)
		}
	}
}

func blake2bHashHex(hexStrings ...string) (string, error) {
	var dataBytes []byte

	// Decode and concatenate all provided hex strings
	for _, hx := range hexStrings {
		bytes, err := hex.DecodeString(hx)
		if err != nil {
			return "", fmt.Errorf("failed to decode hex string %s: %w", hx, err)
		}
		dataBytes = append(dataBytes, bytes...)
	}

	// Calculate Blake2b-256 (32 bytes)
	hash := blake2b.Sum256(dataBytes)

	// Return as a hex string
	return hex.EncodeToString(hash[:]), nil
}

func TestReverseMKWBTTrace(t *testing.T) {
	// --- Inputs ---
	h2 := "4990224dfaf7c36e618a2c6b6a3b407e5901d5a35dbf553adc53c835b207a58e"
	p2_1 := "edce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded"
	p2_2 := "a1b718601179e4c6203872c53f4ff8adb2bd0976f9d95d8631f5881aaf335b3d"

	h5 := "7884591fac059ddd997960c31da7843c83e7be4a5bc310bbd38d8984c3eeefff"
	p5_1 := "87251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c596"
	p5_2 := "b7770152b42fe4495520c0f0c26fd4ad594df6acef3d4e8931613f5cd38c34c8"

	expectedRoot := "e5d85280921add9a96aa5de510274f820b64088deac6e7b7b84a06fa93a933e6"

	fmt.Println("--- Testing Hypotheses ---")

	// Helper to run and print hash results
	runTest := func(name string, hex1 string, hex2 string) string {
		parent, err := blake2bHashHex(hex1, hex2)
		if err != nil {
			log.Fatalf("Error in %s: %v", name, err)
		}
		// fmt.Printf("%s Parent: %s\n", name, parent) // Optional: print intermediate hash
		return parent
	}

	runRootTest := func(name string, hex1 string, hex2 string) string {
		root, err := blake2bHashHex(hex1, hex2)
		if err != nil {
			log.Fatalf("Error in %s: %v", name, err)
		}
		fmt.Printf("%s: %s\n", name, root)
		return root
	}

	// --- Hypothesis Test for Shard 2 ---
	// H2 is idx 2 (left), P2_2 is idx 3 (right) -> Parent is idx 1 (right)
	// P2_1 is uncle (idx 0) (left)
	parent_2_A := runTest("Shard 2 (H2+P2_2)", h2, p2_2)
	runRootTest("Root 2 (P2_1+N)", p2_1, parent_2_A)

	// H2 right, P2_2 left -> Parent right
	// P2_1 left
	parent_2_B := runTest("Shard 2 (P2_2+H2)", p2_2, h2)
	runRootTest("Root 2 (P2_1+N)", p2_1, parent_2_B)

	// H2 left, P2_2 right -> Parent left
	// P2_1 right
	parent_2_C := runTest("Shard 2 (H2+P2_2)", h2, p2_2)
	runRootTest("Root 2 (N+P2_1)", parent_2_C, p2_1)

	// --- Hypothesis Test for Shard 5 ---
	// Assuming H5 is right, P5_2 left -> Parent right
	// P5_1 left
	parent_5_A := runTest("Shard 5 (P5_2+H5)", p5_2, h5)
	runRootTest("Root 5 (P5_1+N)", p5_1, parent_5_A)

	// Assuming H5 is left, P5_2 right -> Parent left
	// P5_1 right
	parent_5_B := runTest("Shard 5 (H5+P5_2)", h5, p5_2)
	runRootTest("Root 5 (N+P5_1)", parent_5_B, p5_1)

	// --- Test if P5_1 and P2_1 are children of root ---
	runRootTest("Root H(P5_1 + P2_1)", p5_1, p2_1) // P5_1 left, P2_1 right
	runRootTest("Root H(P2_1 + P5_1)", p2_1, p5_1) // P2_1 left, P5_1 right

	fmt.Println("-------------------------")
	fmt.Printf("Expected Root: %s\n", expectedRoot)
}

func TestReverseRandom(t *testing.T) {
	a := common.FromHex("0xe24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855")
	//b := common.Hash{}.Bytes()

	b := common.FromHex("1b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3")
	c := append(a, b...)
	hach_c := common.Blake2Hash(c)
	d := append(b, a...)
	hach_d := common.Blake2Hash(d)
	fmt.Printf("a + b : %x\n => %x\n\n", c, hach_c.Bytes())
	fmt.Printf("b + a : %x\n => %x\n\n", d, hach_d.Bytes())
}
