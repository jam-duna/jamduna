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

func (n *NodeContent) StoreBlock(blk *types.Block, id uint16, debug bool) error {
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

	// store block by slot
	// "blk_"+slot uint32 to []byte
	slotPrefix := []byte("blk_")
	slotStoreKey := append(slotPrefix, common.Uint32ToBytes(blk.Header.Slot)...)
	s.WriteRawKV(slotStoreKey, encodedblk)

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

func (n *NodeContent) GetAscendingBlockByHeader(headerHash common.Hash) (childBlks []*types.Block, err error) {

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

func (n *NodeContent) GetStoredBlockByHeader(blkHeader common.Hash) (*types.Block, error) {
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
func (n *NodeContent) GetStoredBlockBySlot(slot uint32) (*types.Block, error) {
	// "blk_"+slot uint32 to []byte
	slotPrefix := []byte("blk_")
	slotStoreKey := append(slotPrefix, common.Uint32ToBytes(slot)...)
	encodedblk, ok, err := n.ReadRawKV(slotStoreKey)
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
func (n *Node) GetMeta_Guarantor(erasureRoot common.Hash) (bClubs []common.Hash, sClubs []common.Hash, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk, err error) {
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	erasure_bKey_val, ok, err := n.ReadRawKV([]byte(erasure_bKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_bKey_val, &bECChunks); err != nil {
		return
	}

	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	erasure_sKey_val, _, err := n.ReadRawKV([]byte(erasure_sKey)) // this has the segment shards AND proof page shards
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_sKey_val, &sECChunksArray); err != nil {
		return
	}

	erasure_bClubsKey := fmt.Sprintf("erasureBClubs-%v", erasureRoot)
	erasure_bClubs_val, ok, err := n.ReadRawKV([]byte(erasure_bClubsKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_bClubs_val, &bClubs); err != nil {
		return
	}

	erasure_sClubsKey := fmt.Sprintf("erasureSClubs-%v", erasureRoot)
	erasure_sClubs_val, ok, err := n.ReadRawKV([]byte(erasure_sClubsKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_sClubs_val, &sClubs); err != nil {
		return
	}
	return
}

func (n *Node) StoreMeta_Guarantor(as *types.AvailabilitySpecifier, bClubs []common.Hash, sClubs []common.Hash, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk) {
	erasure_root_u := as.ErasureRoot
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasure_root_u)
	bChunkJson, _ := json.Marshal(bECChunks)
	n.WriteRawKV(erasure_bKey, bChunkJson)

	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasure_root_u)
	sChunkJson, _ := json.Marshal(sECChunksArray)
	n.WriteRawKV(erasure_sKey, sChunkJson) // this has the segments ***AND*** proof pages

	erasure_bClubsKey := fmt.Sprintf("erasureBClubs-%v", erasure_root_u)
	bClubsJson, _ := json.Marshal(bClubs)
	n.WriteRawKV(erasure_bClubsKey, bClubsJson)

	erasure_sClubsKey := fmt.Sprintf("erasureSClubs-%v", erasure_root_u)
	sClubsJson, _ := json.Marshal(sClubs)

	n.WriteRawKV(erasure_sClubsKey, sClubsJson)
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

func SplitHashes(data []byte) []common.Hash {
	var chunks []common.Hash
	for i := 0; i < len(data); i += 32 {
		end := i + 32
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, common.BytesToHash(data[i:end]))
	}
	return chunks
}

func SplitBytes(data []byte) [][]byte {
	chunkSize := types.NumECPiecesPerSegment * 2
	var chunks [][]byte
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	return chunks
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
func VerifyFullShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, exported_segments_and_proofpageShards []byte, encodedPath []byte) (bool, error) {
	bClub := common.Blake2Hash(bundleShard)
	shards := SplitBytes(exported_segments_and_proofpageShards)
	sClub := trie.NewWellBalancedTree(shards, types.Blake2b).RootHash()
	bundle_segment_pair := append(bClub.Bytes(), sClub.Bytes()...)
	path, err := common.DecodeJustification(encodedPath)
	if err != nil {
		return false, err
	}
	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, path)
	if !verified {
		log.Error(debugDA, "VerifyFullShard VerifyWBTJustification FAILED", "erasureRoot", erasureRoot, "shardIdx", shardIndex, "treeLen", types.TotalValidators,
			"bundle_segment_pair", fmt.Sprintf("%x", bundle_segment_pair), "path", fmt.Sprintf("%x", path))
		return false, fmt.Errorf("Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}
	log.Trace(debugDA, "VerifyFullShard VerifyWBTJustification VERIFIED", "erasureRoot", erasureRoot, "shardIdx", shardIndex, "treeLen", types.TotalValidators,
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
	bundle_segment_pairs := zipPairs(bClubs, sClubs)
	treeLen, leafHash, path, isFound := GenerateWBTJustification(erasureRoot, uint16(shardIndex), bundle_segment_pairs)
	if !isFound {
		return bundleShard, exported_segments_and_proofpageShards, justification, false, fmt.Errorf("Not found")
	}
	encodedPath, _ := common.EncodeJustification(path)
	if paranoidVerification {
		decodedPath, _ := common.DecodeJustification(encodedPath)
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIndex), leafHash, decodedPath)
		if !verified {
			log.Crit(debugDA, "GetFullShard_Guarantor")
		}
		if !reflect.DeepEqual(path, decodedPath) {
			log.Crit(debugDA, "generateErasureRoot:JustificationsPath mismatch")
		}
	}
	return recoveredbECChunks[shardIndex].Data, recoveredsECChunksArray[shardIndex].Data, encodedPath, true, nil
}

// used in assureData (by Assurer)
// Ans Source: CE137_FullShard Resp -- Stored By Assurer ONLY
// Allowing CE138_BundleShard  ANS via StoreAuditDA
// Allowing CE139_SegmentShard ANS via StoreImportDA
// NOTE: everyone who calls this must VERIFY first
func (n *Node) StoreFullShard_Assurer(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, exported_segments_and_proofpageShards []byte, encodedPath []byte) error {
	// Store path to Erasure Root

	bClubH := common.Blake2Hash(bundleShard)
	shards := SplitBytes(exported_segments_and_proofpageShards)
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
func (n *Node) StoreFullShardJustification(erasureRoot common.Hash, shardIndex uint16, bClubH common.Hash, sClubH common.Hash, encodedPath []byte) {
	// levelDB key->Val (* Required for multi validator case or CE200s)
	// *f_erasureRoot_<erasureRoot> -> [f_erasureRoot_<shardIdx>]
	// f_erasureRoot_<erasureRoot>_<shardIdx> -> bClubHash++sClub ++ default_justification
	f_es_key := generateErasureRootShardIdxKey("f", erasureRoot, shardIndex)
	bundle_segment_pair := append(bClubH.Bytes(), sClubH.Bytes()...)
	f_es_val := append(bundle_segment_pair, encodedPath...)
	n.WriteRawKV(f_es_key, f_es_val)
	// log.Info(debugDA, "StoreFullShardJustification", "n", n.id, "erasureRoot", erasureRoot, "shardIndex", shardIndex, "f_es_key", f_es_key, "len(f_es_val)", len(f_es_val), "h(val)", common.Blake2Hash(f_es_val))
}

// USED
func (n *Node) GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (bClubH common.Hash, sClubH common.Hash, encodedPath []byte, err error) {
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
	encodedPath = data[64:]
	return bClubH, sClubH, encodedPath, nil
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
func (n *Node) StoreImportDA_Assurer(erasureRoot common.Hash, shardIndex uint16, concatenatedShards []byte) (err error) {
	s_es_key := generateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	n.WriteRawKV(s_es_key, concatenatedShards) // this has segment shards AND proof page shards
	return nil
}

// Verification: CE138_BundleShard_FullShard
func VerifyBundleShard(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte, sclub_justification []byte) (bool, error) {
	// verify its validity
	bClub := common.Blake2Hash(bundleShard)
	sClub := sclub_justification[0:32]
	encodedPath := sclub_justification[32:]
	decodedPath, err := common.DecodeJustification(encodedPath)
	if err != nil {
		return false, err
	}

	bundle_segment_pair := append(bClub.Bytes(), sClub...)
	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, decodedPath)
	if !verified {
		return false, fmt.Errorf("Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}
	return true, nil
}

// Qns Source : CE138_BundleShard --  Ask to Assurer From Auditor
// Ans Source : CE137_FullShard (via StoreAuditDA)
func (n *Node) GetBundleShard_Assurer(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, sClub common.Hash, justification []byte, ok bool, err error) {
	bundleShard = []byte{}
	justification = []byte{}

	b_es_key := generateErasureRootShardIdxKey("b", erasureRoot, shardIndex)

	bundleShard, _, err = n.ReadRawKV([]byte(b_es_key))
	_, sClub, encodedPath, err := n.GetFullShardJustification(erasureRoot, shardIndex)
	if err != nil {
		return nil, sClub, nil, false, err
	}

	// proof: retrieve b_erasureRoot_<shardIdx> ++ SClub ++ default_justification
	if paranoidVerification {
		verified, err := VerifyBundleShard(erasureRoot, shardIndex, bundleShard, encodedPath) // this is needed
		if !verified {
			log.Crit(debugDA, "VerifyBundleShard not verified")
			return nil, sClub, nil, false, err
		}
	}
	return bundleShard, sClub, encodedPath, true, nil
}

// Verification: CE140_SegmentShard
func VerifySegmentShard(erasureRoot common.Hash, shardIndex uint16, segmentShard []byte, segmentIndex uint16, full_justification []byte, bclub_sclub_justification []byte, exportedSegmentLen int) (bool, error) {
	log.Debug(debugDA, "VerifySegmentShard Step 0", "shardIndex", shardIndex, "segmentShard", fmt.Sprintf("%x", segmentShard), "full_justification", fmt.Sprintf("%x", full_justification), "bclub_sclub_justification", fmt.Sprintf("%x", bclub_sclub_justification), "exportedSegmentLen", exportedSegmentLen)

	//full_path & s_path MUST NEED saparation. Not sure why

	// verify its validity
	fPath, err := common.DecodeJustification(full_justification)
	if err != nil {
		fmt.Printf("DecodeJustification full_justification Error: %v\n", err)
		return false, err
	}

	// verify its validity
	bclub_path, err := common.DecodeJustification(bclub_sclub_justification)
	if err != nil {
		fmt.Printf("DecodeJustification bclub_sclub_justification Error: %v\n", err)
		return false, err
	}

	bClub := bclub_path[0]
	bPath := bclub_path[1:]
	log.Debug(debugDA, "VerifySegmentShard", "bClub", bClub, "bPath", bPath)
	//segmentLeafHash := common.ComputeLeafHash_WBT_Blake2B(segmentShard)

	// Michael: please review
	_, recovered_sClub := VerifyWBTJustification(exportedSegmentLen, erasureRoot, segmentIndex, segmentShard, bPath)

	bundle_segment_pair := append(bClub, recovered_sClub.Bytes()...)
	log.Debug(debugDA, "VerifySegmentShard Step 2", "shardIndex", shardIndex, "bundle_segment_pair", fmt.Sprintf("%x", bundle_segment_pair))

	verified, recovered_erasureRoot := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(shardIndex), bundle_segment_pair, fPath)
	log.Debug(debugDA, "VerifySegmentShard Step 3", "shardIndex", shardIndex, "verified", verified, shardIndex, "recovered_sClub", recovered_sClub, "erasureRoot", erasureRoot)
	if !verified {
		return false, fmt.Errorf("Segment Justification Error: expected=%v | recovered=%v", erasureRoot, recovered_erasureRoot)
	}

	return true, nil
}

// Qns Source : CE139_SegmentShard / CE140
// Ans Source : CE137_FullShard
func (n *Node) GetSegmentShard_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentIndices []uint16, withJustification bool) (selected_segments [][]byte, selected_justifications [][]byte, ok bool, err error) {
	s_es_key := generateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	concatenatedShards, ok, err := n.ReadRawKV([]byte(s_es_key))
	if err != nil {
		return selected_segments, selected_justifications, false, err
	}
	if !ok {
		return selected_segments, selected_justifications, false, nil
	}
	segmentShards, _ := SplitToSegmentShards(concatenatedShards) // this has segment shards AND proof page shards

	selected_segments = make([][]byte, len(segmentIndices))
	for i, segmentIndex := range segmentIndices {
		selected_segments[i] = segmentShards[segmentIndex]
	}
	if withJustification {
		// return selected_justifications originally stored by assureData (in response to a guarantee)
		_, _, encodedPath, err := n.GetFullShardJustification(erasureRoot, shardIndex)
		if err != nil {
		}
		selected_justifications = append(selected_justifications, encodedPath)
	}
	return selected_segments, selected_justifications, true, nil
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

	// scan through recentblocks

	specBytes, ok, err := n.ReadRawKV([]byte(generateSpecKey(h)))
	if err != nil || !ok {
		log.Error(debugDA, "ErasureRootLookUP", "err", err, "state", n.statedb.JamState.RecentBlocks)
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
