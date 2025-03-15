package node

import (
	"fmt"
	"reflect"

	//"time"
	"bytes"
	"encoding/json"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) StoreBlock(blk *types.Block, id uint16, debug bool) error {
	// from block, derive blockHash & headerHash
	s, err := n.GetStorage()
	if err != nil {
		fmt.Printf("Error getting storage: %v\n", err)
		return err
	}
	headerhash := blk.Header.Hash()
	blockHash := blk.Hash()

	// header_<headerhash> -> blockHash.
	headerPrefix := []byte("header_")
	storeKey := append(headerPrefix, headerhash[:]...)
	s.WriteRawKV(storeKey, blockHash[:])

	// blk_<blockHash> -> codec(block)
	blockPrefix := []byte("blk_")
	blkStoreKey := append(blockPrefix, blockHash[:]...)
	encodedblk, err := types.Encode(blk)
	if err != nil {
		fmt.Printf("Error encoding block: %v\n", err)
		return err
	}
	s.WriteRawKV(blkStoreKey, encodedblk)

	// child_<ParentHeaderHash>_headerhash -> blockHash and potentially use "seek"
	childPrefix := []byte("child_")

	childStoreKey := append(childPrefix, blk.Header.ParentHeaderHash[:]...)
	childStoreKey = append(childStoreKey, headerhash[:]...)

	s.WriteRawKV(childStoreKey, blockHash[:])
	return nil
}

func stripPrefix(key []byte, prefix []byte) ([]byte, error) {
	// Check if the key starts with the childPrefix
	if !bytes.HasPrefix(key, prefix) {
		return nil, fmt.Errorf("key does not start with the specified prefix")
	}

	// Strip the prefix by slicing
	return key[len(prefix):], nil
}

func (n *Node) GetAscendingBlockByHeader(headerHash common.Hash) (childBlks []*types.Block, err error) {

	// child_<parentHash>_headerhash -> blockHash and potentially use "seek"
	prefix := []byte("child_")
	childStoreKey := append(prefix, headerHash[:]...)

	s, _ := n.GetStorage()
	keyvals, rErr := s.ReadRawKVWithPrefix(childStoreKey)
	if rErr != nil {
		return nil, fmt.Errorf("Error reading childStoreKey: %v\n", rErr)
	}

	// childBlks may contain forks !!!
	childBlks = make([]*types.Block, 0)
	for _, keyval := range keyvals {
		strippedKey, err := stripPrefix(keyval[0], prefix)
		if err != nil && len(strippedKey) != 64 {
			fmt.Printf("Error stripping prefix: %v\n", err)
			return nil, err
		}
		//headerHash := strippedKey[:32]
		childHeaderHash := common.Hash(strippedKey[32:])
		childBlk, err := n.GetStoredBlockByHeader(childHeaderHash)
		if err != nil {
			fmt.Printf("Error getting child block: %v\n", err)
			return nil, err
		}
		childBlks = append(childBlks, childBlk)
	}

	return childBlks, nil
}

func (n *Node) GetStoredBlockByHeader(blkHeader common.Hash) (*types.Block, error) {
	//header_<headerhash> -> blockHash
	headerPrefix := []byte("header_")
	storeKey := append(headerPrefix, blkHeader[:]...)
	blockHash, ok, err := n.ReadRawKV(storeKey)
	if err != nil || !ok {
		return nil, err
	}
	//blk_<blockHash> -> codec(block)
	blockPrefix := []byte("blk_")
	blkStoreKey := append(blockPrefix, blockHash...)
	encodedblk, ok, err := n.ReadRawKV(blkStoreKey)
	if err != nil {
		fmt.Printf("Error reading block: %v\n", err)
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("Block not found")
	}
	blk, _, err := types.Decode(encodedblk, reflect.TypeOf(types.Block{}))
	if err != nil {
		fmt.Printf("Error decoding block: %v\n", err)
		return nil, err
	}
	b := blk.(types.Block)
	return &b, nil
}

func (n *Node) GetMeta_Guarantor(erasureRoot common.Hash) (erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk, err error) {
	erasure_metaKey := fmt.Sprintf("erasureMeta-%v", erasureRoot)
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	erasure_metaKey_val, _, err := n.ReadRawKV([]byte(erasure_metaKey))
	erasure_bKey_val, _, err := n.ReadRawKV([]byte(erasure_bKey))
	erasure_sKey_val, ok, err := n.ReadRawKV([]byte(erasure_sKey)) // this has the segment shards AND proof page shards
	if err != nil || !ok {
		return erasureMeta, bECChunks, sECChunksArray, fmt.Errorf("Fail to find erasure_metaKey=%v", erasureRoot)
	}
	// TODO: figure out the codec later
	if err := erasureMeta.Unmarshal(erasure_metaKey_val); err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return erasureMeta, bECChunks, sECChunksArray, err
	}

	if err := json.Unmarshal(erasure_bKey_val, &bECChunks); err != nil {
		return erasureMeta, bECChunks, sECChunksArray, err
	}

	if err := json.Unmarshal(erasure_sKey_val, &sECChunksArray); err != nil {
		return erasureMeta, bECChunks, sECChunksArray, err
	}
	for i, sc := range sECChunksArray {
		l := 20
		if l > len(sc.Data) {
			l = len(sc.Data)
		}
		if l > 0 && false {
			fmt.Printf("GetMeta_Guarantor %d sc.Data[0:20]=%v h=%s len=%d\n", i, sc.Data[0:l], common.Blake2Hash(sc.Data), len(sc.Data))
		}
	}
	return erasureMeta, bECChunks, sECChunksArray, err
}

func (n *Node) StoreMeta_Guarantor(as *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk) {
	erasure_root_u := as.ErasureRoot
	erasure_metaKey := fmt.Sprintf("erasureMeta-%v", erasure_root_u)
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasure_root_u)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasure_root_u)
	packageHash_key := fmt.Sprintf("erasureSChunk-%v", as.WorkPackageHash)

	bChunkJson, _ := json.Marshal(bECChunks)
	sChunkJson, _ := json.Marshal(sECChunksArray)
	for i, sc := range sECChunksArray {
		l := 20
		if l > len(sc.Data) {
			l = len(sc.Data)
		}
		if l > 0 && false {
			fmt.Printf("StoreMeta_Guarantor %d sc.Data[0:20]=%v h=%s len=%d\n", i, sc.Data[0:l], common.Blake2Hash(sc.Data), len(sc.Data))
		}
	}
	n.WriteRawKV(erasure_metaKey, erasureMeta.Bytes())
	n.WriteRawKV(erasure_bKey, bChunkJson)
	n.WriteRawKV(erasure_sKey, sChunkJson) // this has the segments ***AND*** proof pages
	n.WriteRawKV(packageHash_key, erasure_root_u.Bytes())

}

// Helper function to generate a key using a prefix, erasureRoot, and shardIndex
func generateKey(prefix string, erasureRoot common.Hash, shardIndex uint16) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(prefix)
	buffer.Write(erasureRoot.Bytes())
	buffer.WriteByte(byte(shardIndex >> 8))   // high byte
	buffer.WriteByte(byte(shardIndex & 0xff)) // low byte
	return buffer.Bytes()
}

func generateErasureRootShardIdxKey(section string, erasureRoot common.Hash, shardIndex uint16) string {
	return fmt.Sprintf("%s_%v_%d", section, erasureRoot, shardIndex)
}

// used in CE139 GetSegmentShard_Assurer
func SplitToSegmentShards(concatenatedShards []byte) (segmentShards [][]byte, proofShards [][]byte) {
	fixedSegmentSize := types.NumECPiecesPerSegment * 2 // tiny 2052, full 12
	for i := 0; i < len(concatenatedShards); i += fixedSegmentSize {
		shard := concatenatedShards[i : i+fixedSegmentSize]
		segmentShards = append(segmentShards, shard)
	}
	proofPageSegments := len(segmentShards) / 65
	segmentShards = segmentShards[0 : len(segmentShards)-proofPageSegments] // this has just the segment shards now
	proofShards = segmentShards[proofPageSegments:]
	return segmentShards, proofShards
}

// Verification: CE137_FullShard
func VerifyFullShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, segmentShards [][]byte, justification []byte) (bool, error) {
	// verify its validity
	bClub := common.Blake2Hash(bundleShard)
	sClub := trie.NewWellBalancedTree(segmentShards, types.Blake2b).RootHash()
	bundle_segment_pair := append(bClub.Bytes(), sClub.Bytes()...)
	leafHash := common.ComputeLeafHash_WBT_Blake2B(bundle_segment_pair)
	path, err := common.ExpandPath(justification)
	if err != nil {
		return false, err
	}
	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), leafHash, path)
	if !verified {
		panic(9992)
		return false, fmt.Errorf("Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}
	//log.Info(module, "VerifyFullShard: Verified", "shardIndex", shardIndex, "erasureRoot", erasureRoot, "len(bs)", len(bundleShard), "len(ss)", len(segmentShards), "len(j)", len(justification))
	return true, nil
}

// Qns Source : CE137_FullShard -- By Assurer to Guarantor
// Ans Source : NOT SPECIFIED by Jam_np. Stored As is
func (n *Node) GetFullShard_Guarantor(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, segmentShards [][]byte, justification []byte, ok bool, err error) {
	erasureMeta, recoveredbECChunks, recoveredsECChunksArray, err := n.GetMeta_Guarantor(erasureRoot)
	if err != nil {
		return bundleShard, segmentShards, justification, false, err
	}
	shardJustifications, _ := ErasureRootDefaultJustification(erasureMeta.BClubs, erasureMeta.SClubs)
	shardJustification := shardJustifications[shardIndex]
	bundleShard = recoveredbECChunks[shardIndex].Data
	segmentShards, _ = SplitToSegmentShards(recoveredsECChunksArray[shardIndex].Data) //  this ONLY gets the segment shards, NOT the proof pages
	justification = shardJustification.CompactPath()

	verified, err := VerifyFullShard(erasureRoot, shardIndex, bundleShard, segmentShards, justification)
	if !verified {
		log.Crit(debugDA, "VerifyFullShard", "err", err)
	}
	return bundleShard, segmentShards, justification, true, nil
}

// used in assureData (by Assurer)
// Ans Source: CE137_FullShard Resp -- Stored By Assurer ONLY
// Allowing CE138_BundleShard  ANS via StoreAuditDA
// Allowing CE139_SegmentShard ANS via StoreImportDA
func (n *Node) StoreFullShard_Assurer(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, segmentShards [][]byte, full_justification []byte) error {
	verified, err := VerifyFullShard(erasureRoot, shardIndex, bundleShard, segmentShards, full_justification)
	if !verified || err != nil {
		return fmt.Errorf("Received Invalid FullShard! %v", err)
	}
	log.Trace(debugDA, "StoreFullShard_Assurer [CE137_ANS Verified]", "erasureRoot", erasureRoot, "shardIndex", shardIndex)
	// Store path to Erasure Root
	bClubH := common.Blake2Hash(bundleShard)
	sClubH := trie.NewWellBalancedTree(segmentShards, types.Blake2b).RootHash()

	n.StoreFullShardJustification(erasureRoot, shardIndex, bClubH, sClubH, full_justification)

	// Do not store justification for b,s . It should be generated on the fly

	// Short-term Audit DA (b)
	_errB := n.StoreAuditDA_Assurer(erasureRoot, shardIndex, bundleShard)
	if _errB != nil {
		return _errB
	}

	// Long-term ImportDA (s)

	_errS := n.StoreImportDA_Assurer(erasureRoot, shardIndex, segmentShards)
	if _errS != nil {
		return _errS
	}

	return nil
}

// USED
func (n *Node) StoreFullShardJustification(erasureRoot common.Hash, shardIndex uint16, bClubH common.Hash, sClubH common.Hash, justification []byte) {
	// levelDB key->Val (* Required for multi validator case or CE200s)
	// *f_erasureRoot_<erasureRoot> -> [f_erasureRoot_<shardIdx>]
	// f_erasureRoot_<erasureRoot>_<shardIdx> -> bClubHash++sClub ++ default_justification
	f_es_key := generateErasureRootShardIdxKey("f", erasureRoot, shardIndex)
	bundle_segment_pair := append(bClubH.Bytes(), sClubH.Bytes()...)
	f_es_val := append(bundle_segment_pair, justification...)
	n.WriteRawKV(f_es_key, f_es_val)
	// log.Info(debugDA, "StoreFullShardJustification", "n", n.id, "erasureRoot", erasureRoot, "shardIndex", shardIndex, "f_es_key", f_es_key, "len(f_es_val)", len(f_es_val), "h(val)", common.Blake2Hash(f_es_val))
}

// USED
func (n *Node) GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (bClubH common.Hash, sClubH common.Hash, justification []byte, err error) {
	f_es_key := generateErasureRootShardIdxKey("f", erasureRoot, shardIndex)
	log.Trace(debugDA, "GetFullShardJustification", "n", n.id, "f_es_key", f_es_key)

	data, ok, err := n.ReadRawKV([]byte(f_es_key))
	if err != nil || !ok {
		return
	}
	if len(data) < 64 {
		err = fmt.Errorf("GetFullShardJustification Bad data")
		return
	}
	bClubH = common.Hash(data[:32])
	sClubH = common.Hash(data[32:64])
	justification = data[64:]
	return bClubH, sClubH, justification, nil
}

// Short-term Audit DA -  Used to Store bClub(bundleShard) by Assurers (til finality)
func (n *Node) StoreAuditDA_Assurer(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte) (err error) {
	// *b_erasureRoot_<erasureRoot> -> [b_erasureRoot_<shardIdx>]
	// b_erasureRoot_<shardIdx> -> bundleShard
	b_es_key := generateErasureRootShardIdxKey("b", erasureRoot, shardIndex)
	n.WriteRawKV(b_es_key, bundleShard)
	log.Trace(debugDA, "StoreAuditDA", "b_es_key", b_es_key, "bundleShard", bundleShard)
	return nil
}

// requestHash (packageHash(wp) or SegmentRoot(e)) -> ErasureRoot(u)
func generateSpecKey(requestHash common.Hash) string {
	return fmt.Sprintf("rtou_%v", requestHash)
}

// Helper function to extract a segment from the concatenated segments
func extractSegment(segmentsConcat []byte, segmentIndex uint16, segmentSize int) []byte {
	start := int(segmentIndex) * segmentSize
	end := start + segmentSize
	return segmentsConcat[start:end]
}

func (n *Node) StoreSpec(spec types.AvailabilitySpecifier) error {
	erasureRoot := spec.ErasureRoot
	segementRoot := spec.ExportedSegmentRoot
	workpackageHash := spec.WorkPackageHash

	// write 3 mappings:
	specBytes, _ := spec.ToBytes()
	// (a) workpackageHash => spec
	// (b) segmentRoot => spec
	// (c) erasureRoot => spec
	n.WriteRawKV(generateSpecKey(workpackageHash), specBytes)
	n.WriteRawKV(generateSpecKey(segementRoot), specBytes)
	n.WriteRawKV(generateSpecKey(erasureRoot), specBytes)

	return nil
}

// Long-term ImportDA - Used to Store sClub (segmentShard) by Assurers (at least 672 epochs)
func (n *Node) StoreImportDA_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentShards [][]byte) (err error) {
	if len(segmentShards) == 0 {
		return nil
	}
	// *s_erasureRoot_<erasureRoot> -> [s_erasureRoot_<shardIdx>]
	// s_erasureRoot_<shardIdx> -> combinedSegmentShards

	/*
	   proof Step:
	   retrieve combinedSegmentShards via s_erasureRoot_<shardIdx>
	   combinedSegmentShards -> SegmentShards (via SplitToSegmentShards)
	   SegmentShards -> wbt tree
	   Derieve Path: T(s,i,H)
	   SegmentJustification: wbt_tree_root ++ path
	*/

	s_es_key := generateErasureRootShardIdxKey("s", erasureRoot, shardIndex)

	concatenatedShards := bytes.Join(segmentShards, nil)
	n.WriteRawKV(s_es_key, concatenatedShards) // this has segment shards AND proof page shards

	log.Info(debugDA, "StoreImportDA_Assurer concatenatedShards", "n", n.id, "s_es_key", s_es_key, "len(concat)", len(concatenatedShards), "h(concat)", common.Blake2Hash(concatenatedShards))
	return nil
}

// Verification: CE138_BundleShard_FullShard
func VerifyBundleShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, sclub_justification []byte) (bool, error) {
	// verify its validity
	bClub := common.Blake2Hash(bundleShard)
	sclub_path, err := common.ExpandPath(sclub_justification)
	if err != nil {
		return false, err
	}

	sClub := sclub_path[0]
	path := sclub_path[1:]
	bundle_segment_pair := append(bClub.Bytes(), sClub.Bytes()...)
	leafHash := common.ComputeLeafHash_WBT_Blake2B(bundle_segment_pair)
	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), leafHash, path)
	if !verified {
		return false, fmt.Errorf("Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}
	return true, nil
}

// Qns Source : CE138_BundleShard --  Ask to Assurer From Auditor
// Ans Source : CE137_FullShard (via StoreAuditDA)
func (n *Node) GetBundleShard_Assurer(erasureRoot common.Hash, shardIndex uint16) (erasure_root common.Hash, shard_idx uint16, bundleShard []byte, justification []byte, ok bool, err error) {
	bundleShard = []byte{}
	justification = []byte{}

	b_es_key := generateErasureRootShardIdxKey("b", erasureRoot, shardIndex)

	bundleShard, _, err = n.ReadRawKV([]byte(b_es_key))
	bClubH, sClubH, justification, err := n.GetFullShardJustification(erasureRoot, shardIndex)
	if err != nil {
		return erasureRoot, shardIndex, nil, nil, false, err
	}
	log.Trace(debugDA, "GetBundleShard_Assurer", b_es_key, bClubH, sClubH)
	var sclub_justification []byte
	sclub_justification = append(sClubH[:], justification...)

	// proof: retrieve b_erasureRoot_<shardIdx> ++ SClub ++ default_justification
	verified, err := VerifyBundleShard(erasureRoot, shardIndex, bundleShard, sclub_justification) // this is needed
	if !verified {
		log.Crit(debugDA, "VerifyBundleShard not verified")
		return erasureRoot, shardIndex, nil, nil, false, err
	}
	log.Trace(debugDA, "[CE138_QNS Verified] erasureRoot-shardIndex: %v-%d\n", erasureRoot, shardIndex)
	return erasureRoot, shardIndex, bundleShard, sclub_justification, true, nil
}

// Verification: CE140_SegmentShard
func VerifySegmentShard(erasureRoot common.Hash, shardIndex uint16, segmentShard []byte, segmentIndex uint16, full_justification []byte, bclub_sclub_justification []byte, exportedSegmentLen int) (bool, error) {
	log.Trace(debugDA, "VerifySegmentShard Step 0", "shardIndex", shardIndex, "segmentShard", segmentShard)

	//full_path & s_path MUST NEED saparation. Not sure why

	// verify its validity
	fPath, err := common.ExpandPath(full_justification)
	if err != nil {
		return false, err
	}

	// verify its validity
	bclub_path, err := common.ExpandPath(bclub_sclub_justification)
	if err != nil {
		return false, err
	}

	bClub := bclub_path[0]
	bPath := bclub_path[1:]
	log.Trace(debugDA, "bClub", bClub, "bPath", bPath)
	segmentLeafHash := common.ComputeLeafHash_WBT_Blake2B(segmentShard)

	_, recovered_sClub := VerifyWBTJustification(exportedSegmentLen, erasureRoot, segmentIndex, segmentLeafHash, bPath)

	bundle_segment_pair := append(bClub.Bytes(), recovered_sClub.Bytes()...)
	log.Trace(debugDA, "VerifySegmentShard Step 2: shardIndex", shardIndex, "bundle_segment_pair", bundle_segment_pair)

	erasureLeafHash := common.ComputeLeafHash_WBT_Blake2B(bundle_segment_pair)

	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), erasureLeafHash, fPath)
	log.Trace(debugDA, "VerifySegmentShard Step 3: shardIndex", "verified", verified, shardIndex, "recovered_sClub", recovered_sClub, "erasureRoot", erasureRoot)
	if !verified {
		return false, fmt.Errorf("Segment Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}

	return true, nil
}

// Qns Source : CE139_SegmentShard
// Ans Source : CE137_FullShard
func (n *Node) GetSegmentShard_AssurerSimple(erasureRoot common.Hash, shardIndex uint16, segmentIndices []uint16) (selected_segments [][]byte, ok bool, err error) {
	s_es_key := generateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	concatenatedShards, ok, err := n.ReadRawKV([]byte(s_es_key))
	if err != nil {
		return selected_segments, false, err
	}
	if !ok {
		return selected_segments, false, nil
	}
	segmentShards, _ := SplitToSegmentShards(concatenatedShards) // this has segment shards AND proof page shards

	selected_segments = make([][]byte, len(segmentIndices))
	for i, segmentIndex := range segmentIndices {
		selected_segments[i] = segmentShards[segmentIndex]
	}
	return selected_segments, true, nil
}

// Qns Source : CE140
// Ans Source : CE137_FullShard
func (n *Node) GetSegmentShard_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentIndices []uint16) (erasure_root common.Hash, shard_index uint16, segment_Indices []uint16, selected_segments [][]byte, selected_full_justifications [][]byte, selected_segments_justifications [][]byte, exportedSegmentAndPageProofLens int, ok bool, err error) {
	//j⌢[b] <--- CE_137 shared
	//segmentshards = []byte{}
	//segment_justifications = [][]byte{}

	s_es_key := generateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	concatenatedShards, _, err := n.ReadRawKV([]byte(s_es_key))
	segmentShards, _ := SplitToSegmentShards(concatenatedShards) // REVIEW

	segmentTree := trie.NewWellBalancedTree(segmentShards, types.Blake2b)
	recoveredSclubH := segmentTree.RootHash()

	bClubH, sClubH, full_justification, err := n.GetFullShardJustification(erasureRoot, shardIndex)
	if err != nil {
		return erasureRoot, shardIndex, segmentIndices, nil, nil, nil, 0, false, err
	}
	if recoveredSclubH != sClubH {
		// TODO: add log.Warn() -- do not panic
		return erasureRoot, shardIndex, segmentIndices, nil, nil, nil, 0, false, fmt.Errorf("GetSegmentShard_Assurer: Invalid GetSegmentShard ERROR")
	}

	log.Trace(debugDA, "GetSegmentShard_Assurer", "len(full_justification)", len(full_justification), segmentIndices, s_es_key, bClubH, sClubH, full_justification)

	exportedSegmentAndPageProofLen := len(segmentShards)
	for _, segmentIndex := range segmentIndices {
		selected_segment := segmentShards[segmentIndex]
		selected_segments = append(selected_segments, selected_segment)
		_, segmentLeafHash, segmentPath, isFound, err := segmentTree.Trace(int(segmentIndex))
		if err != nil || !isFound {
			return erasureRoot, shardIndex, segmentIndices, nil, nil, nil, 0, false, err
		}
		log.Trace(debugDA, "GetSegmentShard_Assurer", "segmentLeafHash", segmentLeafHash)

		var s_justification []byte
		for _, segmentHash := range segmentPath {
			s_justification = append(s_justification, segmentHash.Bytes()...)
		}

		bclub_sclub_sj := make([]byte, 0)
		bclub_sclub_sj = append(bclub_sclub_sj, bClubH.Bytes()...)
		//bclub_sclub_sj = append(bclub_sclub_sj, recoveredSclubH.Bytes()...)
		bclub_sclub_sj = append(bclub_sclub_sj, s_justification...)
		selected_segments_justifications = append(selected_segments_justifications, bclub_sclub_sj)
		selected_full_justifications = append(selected_segments_justifications, full_justification)

		verified, err := VerifySegmentShard(erasureRoot, shardIndex, segmentShards[segmentIndex], uint16(segmentIndex), full_justification, bclub_sclub_sj, exportedSegmentAndPageProofLen)
		if err != nil || !verified {
			log.Crit(debugDA, "VerifySegmentShard", "err", err)
			return erasureRoot, shardIndex, segmentIndices, nil, nil, nil, 0, false, err
		}
	}

	return erasureRoot, shardIndex, segmentIndices, selected_segments, selected_full_justifications, selected_segments_justifications, exportedSegmentAndPageProofLens, true, nil
}

type SpecIndex struct {
	Spec    types.AvailabilitySpecifier `json:"spec"`
	Indices []uint16                    `json:"indices"`
}

// Look up the erasureRoot, exportedSegmentRoot, workpackageHash for either kind of hash: segment root OR workPackageHash
func (si *SpecIndex) String() string {
	// print JSON
	jsonBytes, err := json.Marshal(si)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(jsonBytes)
}

// Look up the erasureRoot, exportedSegmentRoot, workpackageHash for either kind of hash: segment root OR workPackageHash
func (si *SpecIndex) AddIndex(idx uint16) bool {
	if !common.Uint16Contains(si.Indices, idx) {
		si.Indices = append(si.Indices, idx)
		return true
	}
	return false
}

// Look up the erasureRoot, exportedSegmentRoot, workpackageHash for either kind of hash: segment root OR workPackageHash
func (n *Node) SpecSearch(h common.Hash) (si *SpecIndex) {
	specBytes, ok, err := n.ReadRawKV([]byte(generateSpecKey(h)))
	if err != nil || !ok {
		log.Error(debugDA, "ErasureRootLookUP", "err", err)
		return nil
	}

	var spec types.AvailabilitySpecifier
	err = spec.FromBytes(specBytes)
	if err != nil {
		panic(1234431)
	}
	return &SpecIndex{
		Spec:    spec,
		Indices: make([]uint16, 0),
	}
}
