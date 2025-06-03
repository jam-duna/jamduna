package trie

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const debugWBT = false

func TestZeroLeaves(t *testing.T) {
	var values [][]byte
	tree := NewWellBalancedTree(values, types.Keccak)
	if debugWBT {
		fmt.Printf("%x\n", tree.Root())
	}

}

// TestWellBalancedTree tests the MerkleB method of the WellBalancedTree

func TestWBTTrace(t *testing.T) {
	// List of different network setting
	//numShardsList := []int{6, 12, 18, 36, 108, 342, 684, 1023}
	numShardsList := []int{6}
	for _, numShards := range numShardsList {
		shardJustifications := make([]types.Justification, numShards)
		if debugWBT {
			t.Logf("Testing with %d shards", numShards)
		}
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

		// Build the tree using Blake2b.
		wbt := NewWellBalancedTree(values, types.Blake2b)
		erasureRoot := wbt.RootHash()
		if debugWBT {
			t.Logf("Values: %x", values)
			t.Log("Tree structure:")
			wbt.PrintTree()
			t.Logf("ErasureRoot: %v", erasureRoot)
		}

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
			} else if debugWBT {
				t.Logf("shardIndex=%v justificationsPath match!", shardIndex)
			}

			if !bytes.Equal(derivedRoot[:], erasureRoot[:]) {
				t.Errorf("ShardIndex %d: ErasureRoot=%v, got %v", shardIndex, erasureRoot, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			} else if debugWBT {
				t.Logf("ShardIndex %d verified. DerivedRoot=%v", shardIndex, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			}
		}
	}
}

func TestGenerateWBTJustification(t *testing.T) {
	// Initialize the tree with some values
	debugWBT := true
	bundleShards := [][]byte{
		common.FromHex("0000000000a649a61ab2617164cb87e2ed05859dba1fa352ca4ac9b4aa13bf36fb42708e04001aa048689e38d46d787e1a3f285d08946cd4f3a5cc0243229f90ba3c7b1bdb3170dd1f5448b163e23e64b980c95578a4f8e06f2c4a6b1963dd34665c4168d5a52fc9a372a65e2341f5e92ca21944a87986a3ff07b5c5cbabd6f36187ff1436f3"),
		common.FromHex("1aa048689e38d46d787e1a3f285d08946cd4f3a5cc0243229f90ba3c7b1bdb315aad2100000100000000f7a896797fbfab3d4e770a72fd8bfe3aa2011adc32e639142641834f2b0e70b724080f033802100d4a080edc172c4ca476afcfef203a40bed4415ff3db8288c2e59821000000127a000000000040420f000000000000000000000000"),
		common.FromHex("32decd84076bb6fdcfe3e2c46608374c277f528667343df45d3ff95dae1d513cd12bda6fc002287e85ec0cf3a64aa028753cb1694cf1bd4ff16a10031465153ed80b0b89679150a42638984fc48d98e60c60a8cd5b0ccdb1c0c57834c18145322f32c85d9632bf757b80a9021bf0eb88a1d4f2fc3ad3decf7409a398ad7aecb36f6a783361b3"),
		common.FromHex("287e85ec99f52b36ad2f998a2a9eb83aa6ae24be1129dd8408e58ad57f15353b70c48be1c40332decd846563e45ea7e9c43ed7434e172c10fcf57e004d9bb8485b2356d33fef0b7749dbf4f6a86c9e80a5ed2b902d74227de34d44f0170db83c09d05d741c644b3e5030eac419b11e619f0cebb892aa582cc901165d66d13a400eed87275740"),
		common.FromHex("c7c22fd2fd9be4b9a9a4921110f188709b5c0f6f4a502ecd641b78ce2c149e3f73e42cf9a904dd6267ba0e0fa321c66d6134a7ee393128fa9d9c4f0d3dca240268842c375fa4a08ffbf3d13a7f4ef7bc5cf83b70103c33f6fe560c7ac61fbc3e40ff7fd4a788aaab2dfa8644612f0242a8f81c283ce5c904ad0026d0d661011ebcf30558491e"),
		common.FromHex("dd6267ba63057972cb68e95f5c6707061a8d79573c4dcebd31c10b46fd1cfa38d20b7d77ad05c7c22fd2679fe135c1acd036c1c43bd7b9a59003210e64348974ebac716d07dafb5c9410bd8313aff1daf575b82d6644dc3addde30be10934130661deafd2dde5ee0064ac582636ef7ab9620056c949c4fe7100893151dcad7eddd74fa4c7fed"),
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

	numShards := len(leaves)
	values := make([][]byte, numShards)
	for i, sb := range leaves {
		values[i] = sb
	}

	wbt := NewWellBalancedTree(values, types.Blake2b)
	if debugWBT {
		t.Logf("TestGenerateWBTJustification values: %x\n", values)
		wbt.PrintTree()
	}

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
		} else if debugWBT {
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

func TestWBTTraceAllSetting(t *testing.T) {
	//t.Skip(("manual"))
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
		if debugWBT {
			t.Logf("Testing NetWork: %v with %d shards (V=%d) | ShardByteSize=%d (WP=%d)\n", network, numShards, numShards, ecShardSize, numECPieceSize)
		}
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
		erasureRoot := wbt.RootHash()
		if debugWBT {
			t.Log("Tree structure:")
			//wbt.PrintTree()
			t.Logf("ErasureRoot: %v", erasureRoot)
		}

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
			if debugWBT {
				t.Logf("ShardIdx=%d, numShards=%d, EncodedPath Len: %d | UpperProofLen=%d | SegProofLen=%d | %d\n", shardIndex, numShards, encodedPathLen, upperPathLen, segPathLen, segPathLen2)
			}

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
			} else if debugWBT {
				t.Logf("shardIndex=%v justificationsPath match!", shardIndex)
			}

			if !bytes.Equal(derivedRoot[:], erasureRoot[:]) {
				t.Errorf("ShardIndex %d: ErasureRoot=%v, got %v", shardIndex, erasureRoot, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
			} else if debugWBT {
				t.Logf("ShardIndex %d verified. DerivedRoot=%v", shardIndex, derivedRoot)
				t.Logf("treeLen=%v, leafHash=%x, path=%x", treeLen, leafHash, path)
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
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("0000000000a649a61ab2617164cb87e2ed05859dba1fa352ca4ac9b4aa13bf36fb42708e04001aa048689e38d46d787e1a3f285d08946cd4f3a5cc0243229f90ba3c7b1bdb3170dd1f5448b163e23e64b980c95578a4f8e06f2c4a6b1963dd34665c4168d5a52fc9a372a65e2341f5e92ca21944a87986a3ff07b5c5cbabd6f36187ff1436f3"),
			encodedPath: common.FromHex("015c2215eaae7dd520cc796a407df40f0604deeafc01a09fe90b95f42331deee2a00000000000000000000000000000000000000000000000000000000000000000168b28c8520cee24201e98ff12f2884bfcc131667f0981144e908e9ee4c19039d0000000000000000000000000000000000000000000000000000000000000000001845daf704c5a9b0b50c763ac336c2de1aeb786adb002fe8c87a7f63db782027"),
		},
		{
			// shard 1
			treeLen:     6,
			shardIdx:    1,
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("1aa048689e38d46d787e1a3f285d08946cd4f3a5cc0243229f90ba3c7b1bdb315aad2100000100000000f7a896797fbfab3d4e770a72fd8bfe3aa2011adc32e639142641834f2b0e70b724080f033802100d4a080edc172c4ca476afcfef203a40bed4415ff3db8288c2e59821000000127a000000000040420f000000000000000000000000"),
			encodedPath: common.FromHex("01142372415c6c976a01f0579924ed0207b157629c13233eab92a09496d137bd7b00000000000000000000000000000000000000000000000000000000000000000168b28c8520cee24201e98ff12f2884bfcc131667f0981144e908e9ee4c19039d0000000000000000000000000000000000000000000000000000000000000000001845daf704c5a9b0b50c763ac336c2de1aeb786adb002fe8c87a7f63db782027"),
		},
		{
			// shard 2
			treeLen:     6,
			shardIdx:    2,
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("32decd84076bb6fdcfe3e2c46608374c277f528667343df45d3ff95dae1d513cd12bda6fc002287e85ec0cf3a64aa028753cb1694cf1bd4ff16a10031465153ed80b0b89679150a42638984fc48d98e60c60a8cd5b0ccdb1c0c57834c18145322f32c85d9632bf757b80a9021bf0eb88a1d4f2fc3ad3decf7409a398ad7aecb36f6a783361b3"),
			encodedPath: common.FromHex("0072469ac3ad0e74a39bbca2b24f573516b01727f185e4e812e71479b56daa12ff001845daf704c5a9b0b50c763ac336c2de1aeb786adb002fe8c87a7f63db782027"),
		},
		{
			// shard 3
			treeLen:     6,
			shardIdx:    3,
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("287e85ec99f52b36ad2f998a2a9eb83aa6ae24be1129dd8408e58ad57f15353b70c48be1c40332decd846563e45ea7e9c43ed7434e172c10fcf57e004d9bb8485b2356d33fef0b7749dbf4f6a86c9e80a5ed2b902d74227de34d44f0170db83c09d05d741c644b3e5030eac419b11e619f0cebb892aa582cc901165d66d13a400eed87275740"),
			encodedPath: common.FromHex("01bade851d8e84f6a1adc55eb561ba285f75d197d619ca5acc0b109e4efb1873cf0000000000000000000000000000000000000000000000000000000000000000013df461211184d8afd715b4978bda2c0ccc03b7da325e93c282c86c430d27da750000000000000000000000000000000000000000000000000000000000000000008b2f3b3bf4bb5905bcd52e09f820fe191412b0d637800ce7845fd58802f21680"),
		},
		{
			// shard 4
			treeLen:     6,
			shardIdx:    4,
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("c7c22fd2fd9be4b9a9a4921110f188709b5c0f6f4a502ecd641b78ce2c149e3f73e42cf9a904dd6267ba0e0fa321c66d6134a7ee393128fa9d9c4f0d3dca240268842c375fa4a08ffbf3d13a7f4ef7bc5cf83b70103c33f6fe560c7ac61fbc3e40ff7fd4a788aaab2dfa8644612f0242a8f81c283ce5c904ad0026d0d661011ebcf30558491e"),
			encodedPath: common.FromHex("0117c7b8e2a5dcd8c431cb8f8678fe2a18d4e9253c9ffee151d1ec5745e554c3900000000000000000000000000000000000000000000000000000000000000000013df461211184d8afd715b4978bda2c0ccc03b7da325e93c282c86c430d27da750000000000000000000000000000000000000000000000000000000000000000008b2f3b3bf4bb5905bcd52e09f820fe191412b0d637800ce7845fd58802f21680"),
		},
		{
			// shard 5
			treeLen:     6,
			shardIdx:    5,
			erasureRoot: common.HexToHash("0xf0f413c5e373921f22a24e7c9f13320e1458cacf4d15b362f0d6667e211cf57c"),
			bundleShard: common.FromHex("dd6267ba63057972cb68e95f5c6707061a8d79573c4dcebd31c10b46fd1cfa38d20b7d77ad05c7c22fd2679fe135c1acd036c1c43bd7b9a59003210e64348974ebac716d07dafb5c9410bd8313aff1daf575b82d6644dc3addde30be10934130661deafd2dde5ee0064ac582636ef7ab9620056c949c4fe7100893151dcad7eddd74fa4c7fed"),
			encodedPath: common.FromHex("00afd6a25e8f92f7ac664e41dc25eee8db9998dd5c752a78cb5be9a0802d599188008b2f3b3bf4bb5905bcd52e09f820fe191412b0d637800ce7845fd58802f21680"),
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
