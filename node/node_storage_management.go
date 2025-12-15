package node

import (
	"fmt"
	"reflect"

	"bytes"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	storage "github.com/colorfulnotion/jam/storage"
	trie "github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *NodeContent) StoreBlock(blk *types.Block, id uint16, debug bool) error {
	timestamp := n.GetSlotTimestamp(blk.Header.Slot)
	return n.store.StoreBlock(blk, id, timestamp)
}

func (n *NodeContent) StoreFinalizedBlock(blk *types.Block) error {
	return n.store.StoreFinalizedBlock(blk)
}

func (n *NodeContent) GetFinalizedBlock() (*types.Block, error) {
	return n.store.GetFinalizedBlock()
}

func (n *NodeContent) GetFinalizedBlockInternal() (*types.Block, bool, error) {
	return n.store.GetFinalizedBlockInternal()
}

func stripPrefix(key []byte, prefix []byte) ([]byte, error) {
	// Check if the key starts with the childPrefix
	if !bytes.HasPrefix(key, prefix) {
		return nil, fmt.Errorf("key does not start with the specified prefix")
	}

	// Strip the prefix by slicing
	return key[len(prefix):], nil
}

func (n *NodeContent) GetAscendingBlockByHeader(headerHash common.Hash) (childBlks []*types.Block, err error) {
	keyvals, err := n.store.GetChildBlocks(headerHash)
	if err != nil {
		return nil, fmt.Errorf("error reading childStoreKey: %v", err)
	}

	prefix := []byte("child_")
	childStoreKey := append(prefix, headerHash[:]...)

	log.Trace(log.Node, "GetAscendingBlockByHeader", "headerHash", headerHash, "keyvals", fmt.Sprintf("%x", keyvals))
	// childBlks may contain forks !!!
	childBlks = make([]*types.Block, 0)
	for i, keyval := range keyvals {
		child_header_hash_byte, err := stripPrefix(keyval[0], childStoreKey)
		if err != nil {
			log.Error(log.Node, "GetAscendingBlockByHeader - Error stripping prefix", "err", err, "keyval", common.Bytes2Hex(keyval[0]), "prefix", common.Bytes2Hex(prefix))
			return nil, err
		}
		childHeaderHash := common.BytesToHash(child_header_hash_byte)
		log.Trace(log.Node, "GetAscendingBlockByHeader", "i", i, "strippedKey", common.Bytes2Hex(child_header_hash_byte), "childHeaderHash", childHeaderHash)
		childBlk, err := n.GetStoredBlockByHeader(childHeaderHash)
		if err != nil {
			log.Error(log.Node, "GetAscendingBlockByHeader: GetStoredBlockByHeader - Error getting child block", "err", err)
			return nil, err
		}
		log.Trace(log.Node, "GetAscendingBlockByHeader - found child", "i", "childHeaderHash", childHeaderHash, "blk", childBlk.String())
		childBlks = append(childBlks, &types.Block{
			Header:    childBlk.Header,
			Extrinsic: childBlk.Extrinsic,
		})
	}

	return childBlks, nil
}

func (n *NodeContent) GetSlotTimestamp(slot uint32) uint64 {
	// GP demands this, but for the testnet we should adjust not to something that relies on BRITTLE knowledge like "we are starting at the top of the hour",
	//  because we could be starting on the 22nd minute in MANUAL TESTING so we should have something else that is NOT BRITTLE that is cognizant of the JAM_START_TIME.
	// This might be related to:
	// (a) n.GetCurrJCE()
	// (b) n.epoch0Timestamp
	t := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	ts := t.Unix()
	return uint64(ts) + uint64(slot*types.SecondsPerSlot)
}

func (n *NodeContent) GetStoredBlockByHeader(blkHeader common.Hash) (*types.SBlock, error) {
	return n.store.GetBlockByHeader(blkHeader)
}

func (n *NodeContent) GetStoredBlockBySlot(slot uint32) (*types.SBlock, error) {
	return n.store.GetBlockBySlot(slot)
}

func (n *NodeContent) GetMeta_Guarantor(erasureRoot common.Hash) (bClubs []common.Hash, sClubs []common.Hash, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk, err error) {
	return n.store.GetGuarantorMetadata(erasureRoot)
}

func (n *NodeContent) StoreBundleSpecSegments(as *types.AvailabilitySpecifier, d types.AvailabilitySpecifierDerivation, b types.WorkPackageBundle, segments [][]byte) {
	encodedSegments, err := types.Encode(segments)
	if err != nil {
		log.Error(log.Node, "StoreBundleSpecSegments: failed to encode segments", "erasureRoot", as.ErasureRoot, "err", err)
		return
	}

	err = n.store.StoreBundleSpecSegments(
		as.ErasureRoot,
		as.ExportedSegmentRoot,
		d.BundleChunks,
		d.SegmentChunks,
		d.BClubs,
		d.SClubs,
		b.Bytes(),
		encodedSegments,
	)
	if err != nil {
		log.Error(log.Node, "StoreBundleSpecSegments: failed to persist", "erasureRoot", as.ErasureRoot, "err", err)
	}
}

// Verification: CE137_FullShard
func VerifyFullShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, exported_segments_and_proofpageShards []byte, encodedPath []byte) (bool, error) {
	bClub := common.Blake2Hash(bundleShard)
	shards := storage.SplitBytes(exported_segments_and_proofpageShards)
	sClub := trie.NewWellBalancedTree(shards, types.Blake2b).RootHash()
	bundle_segment_pair := common.BuildBundleSegment(bClub, sClub)
	path, err := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
	if err != nil {
		return false, err
	}
	verified, recovered_erasureRoot := statedb.VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, path, "VerifyFullShard")
	if !verified {
		log.Warn(log.Node, "VerifyFullShard VerifyWBTJustification FAILED", "erasureRoot", erasureRoot, "shardIdx", shardIndex, "treeLen", types.TotalValidators,
			"bundle_segment_pair", fmt.Sprintf("%x", bundle_segment_pair), "path", fmt.Sprintf("%x", path))
		return false, fmt.Errorf("justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}
	log.Trace(log.DA, "VerifyFullShard VerifyWBTJustification VERIFIED", "erasureRoot", erasureRoot, "shardIdx", shardIndex, "treeLen", types.TotalValidators,
		"bundle_segment_pair", fmt.Sprintf("%x", bundle_segment_pair), "path", fmt.Sprintf("%x", path))
	return true, nil
}

// Qns Source : CE137_FullShard -- By Assurer to Guarantor
// Ans Source : NOT SPECIFIED by Jam_np. Stored As is
func (n *Node) GetFullShard_Guarantor(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, exported_segments_and_proofpageShards []byte, justification []byte, ok bool, err error) {
	bClubs, sClubs, recoveredbECChunks, recoveredsECChunksArray, err := n.GetMeta_Guarantor(erasureRoot)
	if err != nil {
		return bundleShard, exported_segments_and_proofpageShards, justification, false, err
	}
	bundle_segment_pairs := common.BuildBundleSegmentPairs(bClubs, sClubs)
	treeLen, leafHash, path, isFound := statedb.GenerateWBTJustification(erasureRoot, uint16(shardIndex), bundle_segment_pairs)
	if !isFound {
		return bundleShard, exported_segments_and_proofpageShards, justification, false, fmt.Errorf("not found")
	}
	encodedPath, _ := common.EncodeJustification(path, types.NumECPiecesPerSegment)
	if paranoidVerification {
		decodedPath, _ := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
		verified, _ := statedb.VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIndex), leafHash, decodedPath, "GetFullShard_Guarantor")
		if !verified {
			log.Crit(log.DA, "GetFullShard_Guarantor")
		}
		if !reflect.DeepEqual(path, decodedPath) {
			log.Crit(log.DA, "GenerateErasureRoot:JustificationsPath mismatch")
		}
	}
	return recoveredbECChunks[shardIndex].Data, recoveredsECChunksArray[shardIndex].Data, encodedPath, true, nil
}

// used in assureData (by Assurer)
// Ans Source: CE137_FullShard Resp -- Stored By Assurer ONLY
// Allowing CE138_BundleShard  ANS via StoreAuditDA
// Allowing CE139_SegmentShard ANS via StoreImportDA
// NOTE: everyone who calls this must VERIFY first
func (n *NodeContent) StoreFullShard_Assurer(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, exported_segments_and_proofpageShards []byte, encodedPath []byte) error {
	// Store path to Erasure Root

	bClubH := common.Blake2Hash(bundleShard)
	shards := storage.SplitBytes(exported_segments_and_proofpageShards)
	sClubH := trie.NewWellBalancedTree(shards, types.Blake2b).RootHash()

	n.StoreFullShardJustification(erasureRoot, shardIndex, bClubH, sClubH, encodedPath)

	// Do not store justification for b,s . It should be generated on the fly

	// Short-term Audit DA (b)
	_errB := n.StoreAuditDA_Assurer(erasureRoot, shardIndex, bundleShard)
	if _errB != nil {
		return _errB
	}

	// Long-term ImportDA (s)
	_errS := n.StoreImportDA_Assurer(erasureRoot, shardIndex, exported_segments_and_proofpageShards)
	if _errS != nil {
		return _errS
	}

	return nil
}

// USED
func (n *NodeContent) StoreFullShardJustification(erasureRoot common.Hash, shardIndex uint16, bClub common.Hash, sClub common.Hash, encodedPath []byte) {
	n.store.StoreFullShardJustification(erasureRoot, shardIndex, bClub, sClub, encodedPath)
}

// USED
func (n *NodeContent) GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (bClubH common.Hash, sClubH common.Hash, encodedPath []byte, err error) {
	return n.store.GetFullShardJustification(erasureRoot, shardIndex)
}

// Short-term Audit DA -  Used to Store bClub(bundleShard) by Assurers (til finality)
func (n *NodeContent) StoreAuditDA_Assurer(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte) (err error) {
	return n.store.StoreAuditDA(erasureRoot, shardIndex, bundleShard)
}

func (n *NodeContent) StoreWorkReport(wr types.WorkReport) error {
	// Once availability info exists, drop any builder-cached segments for this work package
	n.builderSegmentsMu.Lock()
	delete(n.builderSegments, wr.AvailabilitySpec.WorkPackageHash)
	n.builderSegmentsMu.Unlock()

	return n.store.StoreWorkReport(&wr)
}

// Long-term ImportDA - Used to Store sClub (segmentShard) by Assurers (at least 672 epochs)
func (n *NodeContent) StoreImportDA_Assurer(erasureRoot common.Hash, shardIndex uint16, concatenatedShards []byte) (err error) {
	return n.store.StoreImportDA(erasureRoot, shardIndex, concatenatedShards)
}

// Verification: CE138_BundleShard_FullShard
func VerifyBundleShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, sClub common.Hash, encodedPath []byte) (bool, error) {
	// verify its validity
	bClub := common.Blake2Hash(bundleShard)
	decodedPath, err := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
	if err != nil {
		return false, err
	}

	bundle_segment_pair := common.BuildBundleSegment(bClub, sClub)
	verified, recovered_erasureRoot := statedb.VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, decodedPath, "VerifyBundleShard")
	if !verified {
		log.Crit(log.Node, "VerifyBundleShard:VerifyWBTJustification VERIFICATION FAILURE", "erasureRoot", erasureRoot, "shardIndex", shardIndex, "bundle_segment_pair", common.Bytes2Hex(bundle_segment_pair), "decodedPath", fmt.Sprintf("%x", decodedPath))
		return false, fmt.Errorf("justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	} else {
		log.Info(log.Node, "VerifyBundleShard:VerifyWBTJustification VERIFIED", "erasureRoot", erasureRoot, "shardIndex", shardIndex, "bundle_segment_pair", common.Bytes2Hex(bundle_segment_pair), "decodedPath", fmt.Sprintf("%x", decodedPath))
	}
	return true, nil
}

// Qns Source : CE138_BundleShard --  Ask to Assurer From Auditor
// Ans Source : CE137_FullShard (via StoreAuditDA)
func (n *NodeContent) GetBundleShard_Assurer(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, sClub common.Hash, justification []byte, ok bool, err error) {
	bundleShard, sClub, encodedPath, ok, err := n.store.GetBundleShard(erasureRoot, shardIndex)
	if err != nil || !ok {
		return bundleShard, sClub, encodedPath, ok, err
	}

	// proof: retrieve b_erasureRoot_<shardIdx> ++ SClub ++ default_justification
	if paranoidVerification {
		verified, err := VerifyBundleShard(erasureRoot, shardIndex, bundleShard, sClub, encodedPath) // this is needed
		if err != nil {
			log.Error(log.Node, "[GetBundleShard_Assurer:VerifyBundleShard] err", "err", err, "erasureRoot", erasureRoot, "shardIndex", shardIndex, "bundleShard", fmt.Sprintf("%x", bundleShard), "encodedPath", "s_club", sClub, fmt.Sprintf("%x", encodedPath))
		} else if !verified {
			log.Warn(log.Node, "[GetBundleShard_Assurer:VerifyBundleShard] not verified", "erasureRoot", erasureRoot, "shardIndex", shardIndex, "bundleShard", fmt.Sprintf("%x", bundleShard), "encodedPath", "s_club", sClub, fmt.Sprintf("%x", encodedPath))
		} else {
			log.Trace(log.Node, "[GetBundleShard_Assurer:VerifyBundleShard] VERIFIED", "erasureRoot", erasureRoot, "shardIndex", shardIndex, "bundleShard", fmt.Sprintf("%x", bundleShard), "encodedPath", "s_club", sClub, fmt.Sprintf("%x", encodedPath))
		}
	}
	return bundleShard, sClub, encodedPath, true, nil
}

// Verification: CE140_SegmentShard
func VerifySegmentShard(erasureRoot common.Hash, shardIndex uint16, segmentShard []byte, segmentIndex uint16, full_justification []byte, bclub_sclub_justification []byte, exportedSegmentLen int) (bool, error) {
	log.Trace(log.DA, "VerifySegmentShard Step 0", "shardIndex", shardIndex, "segmentShard", fmt.Sprintf("%x", segmentShard), "full_justification", fmt.Sprintf("%x", full_justification), "bclub_sclub_justification", fmt.Sprintf("%x", bclub_sclub_justification), "exportedSegmentLen", exportedSegmentLen)

	//full_path & s_path MUST NEED saparation. Not sure why

	// verify its validity
	fPath, err := common.DecodeJustification(full_justification, types.NumECPiecesPerSegment)
	if err != nil {
		fmt.Printf("DecodeJustification full_justification Error: %v\n", err)
		return false, err
	}

	// verify its validity
	bclub_path, err := common.DecodeJustification(bclub_sclub_justification, types.NumECPiecesPerSegment)
	if err != nil {
		fmt.Printf("DecodeJustification bclub_sclub_justification Error: %v\n", err)
		return false, err
	}

	bClub := common.Hash(bclub_path[0])
	bPath := bclub_path[1:]
	log.Trace(log.DA, "VerifySegmentShard", "bClub", bClub, "bPath", fmt.Sprintf("%x", bPath))
	//segmentLeafHash := common.ComputeLeafHash_WBT_Blake2B(segmentShard)

	_, recovered_sClub := statedb.VerifyWBTJustification(exportedSegmentLen, erasureRoot, segmentIndex, segmentShard, bPath, "VerifySegmentShard_call1")

	bundle_segment_pair := common.BuildBundleSegment(bClub, recovered_sClub)
	verified, recovered_erasureRoot := statedb.VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, fPath, "VerifySegmentShard_call2")
	if !verified {
		return false, fmt.Errorf("Segment Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}

	return true, nil
}

// Qns Source : CE139_SegmentShard / CE140
// Ans Source : CE137_FullShard
func (n *NodeContent) GetSegmentShard_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentIndices []uint16, withJustification bool) (selected_segments [][]byte, selected_justifications [][]byte, ok bool, err error) {
	concatenatedShards, ok, err := n.store.GetSegmentShard(erasureRoot, shardIndex)
	if err != nil {
		return selected_segments, selected_justifications, false, err
	}
	if !ok {
		return selected_segments, selected_justifications, false, nil
	}
	segmentShards := storage.SplitToSegmentShards(concatenatedShards)

	selected_segments = make([][]byte, len(segmentIndices))
	for i, segmentIndex := range segmentIndices {
		// segmentShards & proofShards are all accessible by its raw segmentIndex
		//fmt.Printf("GetSegmentShard_Assurer: selected_segments[%d] segmentIndex: %v\n", i, segmentIndex)
		selected_segments[i] = segmentShards[segmentIndex]
	}
	if withJustification {
		// return selected_justifications originally stored by assureData (in response to a guarantee)
		_, _, encodedPath, err := n.GetFullShardJustification(erasureRoot, shardIndex)
		if err != nil {
			fmt.Printf("GetSegmentShard_Assurer GetFullShardJustification Error: %v\n", err)
		}
		selected_justifications = append(selected_justifications, encodedPath)
	}
	return selected_segments, selected_justifications, true, nil
}

// Look up the erasureRoot, exportedSegmentRoot, workpackageHash for either kind of hash: segment root OR workPackageHash
func (n *NodeContent) WorkReportSearch(h common.Hash) (si *storage.SpecIndex) {
	wr, ok := n.store.WorkReportSearch(h)
	if !ok || wr == nil {
		return nil
	}
	return &storage.SpecIndex{
		WorkReport: *wr,
		Indices:    make([]uint16, 0),
	}
}
