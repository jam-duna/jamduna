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
	bundleShards := [][]byte{
		common.FromHex("0x0000000000fa002583136a79daec5ca5e802d27732517c3f7dc4970dc2d68abcaad891e99d001fd1664d60a77f32449f2ed9898f3eb0fef21ba0537b014276f7ff7042355c3a8f001f0b92def425b5cbd7c14118f67d99654e26894a76187f453631503f120cfee77aea940e38601cb828d2878308e32529f8382cf85f7a29b75f1c826985bc1db7"),
		common.FromHex("0xf36f81fc164ee563dc27b8e940a40f8c4bc334fe5964678b1ed8cc848f6111451f0000010000000028a0e0c33fcc7cadbb6627bcc902064e89ba1d16c26ea88371a4f614942fb8372906e90be226abaccc638a0addae983f814190ba97367f8abc5e21642a1e48f23d57724100000000000000105e5f000000008096980000000000000000000000"),
		common.FromHex("0x5f8ebe44f503309e200d05b296518990e554ef9c404d8ec64aed5fe6f72ee5f1cfaec18a510018abba27b6260875c789132f069b04411e05cf8166f6ef7ec45ac1038e0e0b1ff81f8c0c2baf8024fe4b781c7837700473f77fa6b30b3d277315c3bfd3af6b2309c2a31f14b92deaa54460a92e4930145e1cce23277413d19d4f7835d9efc44bb74f"),
		common.FromHex("0xace13fb8e3b7d5d87f39d7220c19dab9469509152b78957229f1046fba997e087a765062cc00077af4ca3642488bffbb8690a8a8f3f3e6b95d9b289b2c521a2e4fd73a2fc30acf28ba01507a9627e02c63beb3255bd772adb0c1aafbdc0973da49d0a2f45331bfd7e4a2f2f6158ab9fc487ba9da66a87b35361b8b1ad4abb4f827295b8641f7aaf8"),
		common.FromHex("0x5ddbd3e3d0741142c4e07a4edc8178199211fd092da6a9b16e8f357366571a5d5b821e46af005488bfe02e7448da86610f5fda751b33537bb7e674ab17fcaa6e822b188ef17246fc3d0ea481ef2a9a17e3081159b92fef4938fa1ba823ad4e5cf7ea1217af4dc6affa67fe6a57b5b137c384e9bb6d6a939505caf4274ce95e36b8571dbfb2359936"),
		common.FromHex("0xaeb4521fc6c0f4049bd4a8de46c92b3031d01b804693b2050d936efa2be081a4ee5a8fae32004b59f10dae100824be539ae07446ec81abc725fc3ac6d4d0741a0cffacaf396771cb0b03df54f9298470f8aada4b92fcee13f79d0258c2834e937d85634c975f70babdda18256fd5ad8feb566e283bd6b6bcfdf258498b937781e74b9fd637898481"),
	}
	segmentsShards := [][]byte{
		common.FromHex("0x"),
		common.FromHex("0x"),
		common.FromHex("0x"),
		common.FromHex("0x"),
		common.FromHex("0x"),
		common.FromHex("0x"),
	}
	if len(bundleShards) != len(segmentsShards) {
		t.Fatalf("bundleShards and segmentsShards must have the same length")
	}
	leaves := make([][]byte, len(segmentsShards))
	for i := 0; i < len(segmentsShards); i++ {
		bClub := common.Blake2Hash(bundleShards[i])
		sClub := common.Hash{}
		if len(segmentsShards[i]) > 0 {
			sClub = common.Blake2Hash(segmentsShards[i])
		}
		leaves[i] = common.BuildBundleSegment(bClub, sClub)
	}

	leavesHex := []string{
		"00000000000000000000000000000000000000000000000000000000000000008fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f",
		"0000000000000000000000000000000000000000000000000000000000000000996dfd5a1bfbe7475e06c80b162964072c4568ec2ee542028c32369ac2425086",
		"0000000000000000000000000000000000000000000000000000000000000000e24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f9855",
		"000000000000000000000000000000000000000000000000000000000000000080fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd",
		"000000000000000000000000000000000000000000000000000000000000000005a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c57",
		"00000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3",
	}

	useLeavesHex := false
	numShards := len(leaves)
	values := make([][]byte, numShards)
	for i, sb := range leaves {
		if useLeavesHex {
			hx := leavesHex[i]
			decoded, _ := hex.DecodeString(hx)
			if !bytes.Equal(decoded, sb) {
				t.Fatalf("Decoded value %d does not match expected value: %x != %x", i, decoded, sb)
			}
			values[i] = decoded
		} else {
			values[i] = sb
		}
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

		encodedPath, err := common.EncodeJustification(path, numShards)
		if err != nil {
			t.Errorf("EncodeJustification error: %v", err)
		}
		decodedPath, err := common.DecodeJustification(encodedPath, numShards)
		if err != nil {
			t.Errorf("DecodeJustification error: %v", err)
		}

		derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, wbt.RootHash(), leafHash, decodedPath)
		if err != nil || verified == false {
			t.Errorf("VerifyWBT error: %v", err)
		}
		expectedHash := wbt.RootHash()
		if !bytes.Equal(derivedRoot[:], expectedHash[:]) {
			t.Errorf("\n\n\n ❌ shardIndex %d, expected hash %v | actual %v", shardIndex, expectedHash, derivedRoot)
		} else {
			fmt.Printf("\n\n\n ✅ shardIndex %d, erasureRoot(u) %v\n", shardIndex, expectedHash)
			fmt.Printf("treeLen=%v, leafHash=%x\nencodedPath(len=%d): %x\n", treeLen, leafHash, len(encodedPath), encodedPath)
			fmt.Printf("path(len=%d) %x\n", len(path), path)
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
			encodedPath: common.FromHex("00edce1f878be2218be5dcc32d25f551c81df9d8019f24ada124745236f2a7aded010000000000000000000000000000000000000000000000000000000000000000e24e633b13382a55ab59fbfb0ac537509a77b3d946f8b4eaf7dcae538a1f98550100000000000000000000000000000000000000000000000000000000000000008fa9ebbf17ebdacfc7c821dee0682b97f0ac9501912941e86aacb56c3ca1961f"),
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
			encodedPath: common.FromHex("0x0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c5960100000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a301000000000000000000000000000000000000000000000000000000000000000005a029717c4c021251f889cdccca5145fd7768427c92a4febbf5d3b6f7947c57"),
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
			encodedPath: common.FromHex("0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c5960100000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a301000000000000000000000000000000000000000000000000000000000000000080fff3be53cdb780b3dea9c24413191d55eb4bbf0c67d447f15841ca45e7fbcd"),
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
			encodedPath: common.FromHex("0x0087251429640e0091654ffe69abed7bbd1d7e429f2c76686b4dda8d7079f1c59600b7770152b42fe4495520c0f0c26fd4ad594df6acef3d4e8931613f5cd38c34c8"),
			bundleShard: common.FromHex("0xaeb4521fc6c0f4049bd4a8de46c92b3031d01b804693b2050d936efa2be081a4ee5a8fae32004b59f10dae100824be539ae07446ec81abc725fc3ac6d4d0741a0cffacaf396771cb0b03df54f9298470f8aada4b92fcee13f79d0258c2834e937d85634c975f70babdda18256fd5ad8feb566e283bd6b6bcfdf258498b937781e74b9fd637898481"),
		},
	}

	for _, tc := range tests {
		//t.Logf("leafHash=%x, path=%x, encodedPath=%x", tc.leafHash, tc.path, tc.encodedPath)
		decodedPath, _ := common.DecodeJustification(tc.encodedPath, 1026) // weired ... we are generating path in reverse order
		decodedPathReversed := common.ReversedByteArray(decodedPath)
		bClub := common.Blake2Hash(tc.bundleShard)
		sClub := common.Hash{}
		leafHash := common.BuildBundleSegment(bClub, sClub)
		recovered, ok, err := VerifyWBT(
			tc.treeLen,
			tc.shardIdx,
			tc.erasureRoot,
			leafHash,
			decodedPathReversed,
		)
		if err != nil {
			t.Fatalf("VerifyWBT returned error for shard %d: %v", tc.shardIdx, err)
		}

		if !ok {
			bClub := common.Blake2Hash(tc.bundleShard)
			sClub := common.Hash{}
			sClub_bClub := append(sClub[:], bClub[:]...)
			h_sb := common.Blake2Hash(sClub_bClub)
			t.Errorf(
				"\nshard %d:\n\nexp %v\ngot %v\n\nsClub=%v\nbClub=%v\n(s,b)=%x\nH(s,b)=%v\n\nbundleShard=0x%x\n\npathLen=%d\nencodedPath=0x%x\npath=%x\n\n\n",
				tc.shardIdx,
				tc.erasureRoot,
				recovered,
				sClub,
				bClub,
				sClub_bClub,
				h_sb,
				tc.bundleShard,
				len(decodedPath),
				tc.encodedPath,
				decodedPath,
			)
		} else {
			bClub := common.Blake2Hash(tc.bundleShard)
			sClub := common.Hash{}
			sClub_bClub := append(sClub[:], bClub[:]...)
			h_sb := common.Blake2Hash(sClub_bClub)
			t.Logf(
				"\nshard %d:\n\nexp %v\ngot %v\n\nsClub=%v\nbClub=%v\n(s,b)=%x\nH(s,b)=%v\n\nbundleShard=0x%x\n\npathLen=%d\nencodedPath=0x%x\npath=%x\n\n\n",
				tc.shardIdx,
				tc.erasureRoot,
				recovered,
				sClub,
				bClub,
				sClub_bClub,
				h_sb,
				tc.bundleShard,
				len(decodedPath),
				tc.encodedPath,
				decodedPath,
			)
		}
	}
}

func TestReverseRandom(t *testing.T) {
	a := common.FromHex("b7770152b42fe4495520c0f0c26fd4ad594df6acef3d4e8931613f5cd38c34c8")
	//b := common.Hash{}.Bytes()

	b := common.FromHex("0x00000000000000000000000000000000000000000000000000000000000000001b39865687e85c13b43c2166b07b918a7258332cc2d65a497be0f84c247e44a3")
	c := append(a, b...)
	hach_c := computeNode(c)
	d := append(b, a...)
	hach_d := computeNode(d)
	fmt.Printf("a + b : %x\n => %x\n\n", c, hach_c)
	fmt.Printf("b + a : %x\n => %x\n\n", d, hach_d)
}
