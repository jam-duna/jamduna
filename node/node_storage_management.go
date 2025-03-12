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
	//TODO: should probably store erasureRoot -> pbH
	// Stanley: Need to check this logic
	erasure_metaKey := fmt.Sprintf("erasureMeta-%v", erasureRoot)
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	erasure_metaKey_val, _, err := n.ReadRawKV([]byte(erasure_metaKey))
	erasure_bKey_val, _, err := n.ReadRawKV([]byte(erasure_bKey))
	erasure_sKey_val, ok, err := n.ReadRawKV([]byte(erasure_sKey))
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
	log.Trace(debugDA, "erasure_metaKey", erasure_metaKey, "val", string(erasureMeta.Bytes()), "erasure_bKey", erasure_metaKey, "val", string(bChunkJson), "erasure_sKey", erasure_metaKey, "val", string(sChunkJson))
	n.WriteRawKV(erasure_metaKey, erasureMeta.Bytes())
	n.WriteRawKV(erasure_bKey, bChunkJson)
	n.WriteRawKV(erasure_sKey, sChunkJson)
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

func generateErasureRootShardIdxKey(erasureRoot common.Hash, shardIndex uint16) string {
	return fmt.Sprintf("es_%v_%d", erasureRoot, shardIndex)
}

// used in CE139 GetSegmentShard_Assurer
func SplitToSegmentShards(concatenatedShards []byte) (segmentShards [][]byte, err error) {
	fixedSegmentSize := types.NumECPiecesPerSegment * 2 // tiny 2056, full 12
	// if len(concatenatedShards)%(fixedSegmentSize) != 0 {
	// 	return nil, fmt.Errorf("Invalid SegmentShards Len:%v. MUST BE multiple of %v", len(concatenatedShards), fixedSegmentSize)
	// }

	for i := 0; i < len(concatenatedShards); i += fixedSegmentSize {
		shard := concatenatedShards[i : i+fixedSegmentSize]
		segmentShards = append(segmentShards, shard)
	}
	return segmentShards, nil
}

// used in StoreImportDA_Assurer
func CombineSegmentShards(segmentShards [][]byte) (concatenatedShards []byte, err error) {
	// Loop through each segment shard and append it to the combined slice
	fixedSegmentSize := types.NumECPiecesPerSegment * 2 // tiny 2056, full 12
	for _, shard := range segmentShards {
		concatenatedShards = append(concatenatedShards, shard...)
	}
	if len(concatenatedShards)%(fixedSegmentSize) != 0 {
		return nil, fmt.Errorf("Invalid SegmentShards Len:%v. MUST BE multiple of %v", len(concatenatedShards), fixedSegmentSize)
	}
	return concatenatedShards, nil
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

func splitBytes(data []byte, n int) [][]byte {
	var result [][]byte
	for i := 0; i < len(data); i += n {
		end := i + n
		if end > len(data) {
			end = len(data)
		}
		result = append(result, data[i:end])
	}
	return result
}

// Qns Source : CE137_FullShard -- By Assurer to Guarantor
// Ans Source : NOT SPECIFIED by Jam_np. Stored As is
func (n *Node) GetFullShard_Guarantor(erasureRoot common.Hash, shardIndex uint16) (erasure_root common.Hash, shardIdx uint16, bundleShard []byte, segmentShards [][]byte, justification []byte, ok bool, err error) {
	erasureMeta, recoveredbECChunks, recoveredsECChunksArray, err := n.GetMeta_Guarantor(erasureRoot)
	if err != nil {
		return erasureRoot, shardIndex, bundleShard, segmentShards, justification, false, err
	}
	shardJustifications, _ := ErasureRootDefaultJustification(erasureMeta.BClubs, erasureMeta.SClubs)
	shardJustification := shardJustifications[shardIdx]
	bundleShard = recoveredbECChunks[shardIdx].Data
	segmentShards = splitBytes(recoveredsECChunksArray[shardIdx].Data, 2056)
	justification = shardJustification.CompactPath()

	verified, err := VerifyFullShard(erasureRoot, shardIndex, bundleShard, segmentShards, justification)
	if !verified {
		log.Crit(debugDA, "VerifyFullShard", "err", err)
	}
	return erasureRoot, shardIndex, bundleShard, segmentShards, justification, true, nil
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
	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	f_es_key := fmt.Sprintf("f_%v", esKey)
	log.Trace(debugDA, "StoreFullShardJustification", "n", n.id, "f_es_key", f_es_key)
	bundle_segment_pair := append(bClubH.Bytes(), sClubH.Bytes()...)
	f_es_val := append(bundle_segment_pair, justification...)
	n.WriteRawKV(f_es_key, f_es_val)
	log.Trace(debugDA, "StoreFullShardJustification", "f_es_key", f_es_key, "f_es_val", f_es_val)
}

// USED
func (n *Node) GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (bClubH common.Hash, sClubH common.Hash, justification []byte, err error) {
	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	f_es_key := fmt.Sprintf("f_%v", esKey)
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
	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	b_es_key := fmt.Sprintf("b_%s", esKey)
	n.WriteRawKV(b_es_key, bundleShard)
	log.Trace(debugDA, "StoreAuditDA", "b_es_key", b_es_key, "bundleShard", bundleShard)
	return nil
}

// requestHash (packageHash(wp) or SegmentRoot(e)) -> ErasureRoot(u)
func generateRequestedHashToErasureRootKey(requestHash common.Hash) string {
	return fmt.Sprintf("rtou_%v", requestHash)
}

// ErasureRoot(u) -> SegmentsRoot(e)
func generateErasureRootToSegmentsKey(erasureRoot common.Hash) string {
	return fmt.Sprintf("utoe_%v", erasureRoot)
}

// ErasureRoot(u) -> requestHash SegmentsRoot(u) ++ WorkPackageHash(wp)
func generateErasureRootToRequestedHash(erasureRoot common.Hash) string {
	return fmt.Sprintf("utor2_%v", erasureRoot)
}

// h is a WorkPackageHash or ExportSegmentRoot
func (n *Node) getErasureRootFromHash(h common.Hash) (erasureRoot common.Hash, err error) {
	// Retrieve ErasureRoot from LevelDB
	erasureRootRaw, ok, err0 := n.ReadRawKV([]byte(generateRequestedHashToErasureRootKey(h)))
	if err0 != nil || !ok {
		return erasureRoot, err0
	}
	return common.Hash(erasureRootRaw), nil
}

// reqHash -> erasureRoot -> exportedSegmentsRoot
func (n *Node) getExportedSegmenstRootFromHash(requestedHash common.Hash) (exportedSegmentsRoot common.Hash, packageHash common.Hash, err error) {
	// requestedHash -> erasureRoot
	erasureRootRaw, ok, err0 := n.ReadRawKV([]byte(generateRequestedHashToErasureRootKey(requestedHash)))
	if err0 != nil || !ok {
		log.Error("getExportedSegmenstRootFromHash not found", "requestedHash", requestedHash, "err", err0)
		return exportedSegmentsRoot, packageHash, err0
	}
	erasureRoot := common.Hash(erasureRootRaw)

	// erasureRoot -> exportedSegmentsRoot
	segmentRoot_packageHash, ok2, err2 := n.ReadRawKV([]byte(generateErasureRootToRequestedHash(erasureRoot)))
	if err2 != nil || len(segmentRoot_packageHash) != 64 || !ok2 {
		return exportedSegmentsRoot, packageHash, err2
	}
	exportedSegmentsRoot = common.Hash(segmentRoot_packageHash[:32])
	packageHash = common.Hash(segmentRoot_packageHash[32:64])
	return exportedSegmentsRoot, packageHash, nil
}

// h is a WorkPackageHash or ExportSegmentRoot
func (n *Node) getImportSegment(h common.Hash, segmentIndex uint16) ([]byte, bool) {
	panic("dont call!")

	// Retrieve ErasureRoot from LevelDB
	erasureRoot, err := n.getErasureRootFromHash(h)
	if err != nil {
		return nil, false
	}

	// Retrieve the concatenated segments using the ErasureRoot
	segmentsConcat, ok, err := n.ReadRawKV([]byte(generateErasureRootToSegmentsKey(erasureRoot)))
	if err != nil || !ok {
		return nil, false
	}

	// Extract and return the requested segment
	return extractSegment(segmentsConcat, segmentIndex, types.SegmentSize), true
}

// Helper function to extract a segment from the concatenated segments
func extractSegment(segmentsConcat []byte, segmentIndex uint16, segmentSize int) []byte {
	start := int(segmentIndex) * segmentSize
	end := start + segmentSize
	return segmentsConcat[start:end]
}

func (n *Node) StoreImportDAWorkReportMap(spec types.AvailabilitySpecifier) error {
	erasureRoot := spec.ErasureRoot
	segementRoot := spec.ExportedSegmentRoot
	workpackageHash := spec.WorkPackageHash

	// write 3 mappings:

	// (a) workpackageHash => erasureRoot
	n.WriteRawKV(generateRequestedHashToErasureRootKey(workpackageHash), erasureRoot.Bytes())
	// (b) segementRoot => erasureRoot
	n.WriteRawKV(generateRequestedHashToErasureRootKey(segementRoot), erasureRoot.Bytes())

	// (c) erasureRoot => segementRoot ++ workpackageHash
	segmentRoot_packageHash := append(segementRoot[:], workpackageHash.Bytes()...)
	n.WriteRawKV(generateErasureRootToRequestedHash(erasureRoot), segmentRoot_packageHash)

	return nil
}

// spec.ErasureRoot => segments
// and be able to retrieve the ith segment by either (a) spec.WorkPackageHash or (b) spec.ExportedSegmentRoot using (c) in response to
func (n *Node) StoreImportDAErasureRootToSegments(spec *types.AvailabilitySpecifier, segments []byte) error {
	panic("DONT CALL!")
	// this is cheating/not-required
	//n.WriteRawKV(generateErasureRootToSegmentsKey(spec.ErasureRoot), segments)
	return nil
}

// Long-term ImportDA - Used to Store sClub (segmentShard) by Assurers (at least 672 epochs)
func (n *Node) StoreImportDA_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentShards [][]byte) (err error) {
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

	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	s_es_key := fmt.Sprintf("s_%s", esKey)

	log.Trace(debugDA, "StoreImportDA_Assurer concatenatedShards", "n", n.id, "s_es_key", s_es_key)
	concatenatedShards, err := CombineSegmentShards(segmentShards) // REVIEW
	if err != nil {
		return err
	}
	n.WriteRawKV(s_es_key, concatenatedShards)

	//testing
	var testShards []uint16
	for idx, _ := range segmentShards {
		testShards = append(testShards, uint16(idx))
	}
	log.Trace(debugDA, "setting StoreImportDA", "testShards", testShards)

	n.GetSegmentShard_Assurer(erasureRoot, shardIndex, testShards)
	log.Trace(debugDA, "StoreAuditDA", "s_es_key", s_es_key, "segmentShards", segmentShards, "concatenatedShards", concatenatedShards)
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

	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	b_es_key := fmt.Sprintf("b_%s", esKey)

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

// Qns Source : CE140/CE139_SegmentShard Question -- By Guarantor to Assurer
// Ans Source : CE137_FullShard ANS (via StoreImportDA)
func (n *Node) GetSegmentShard_Assurer(erasureRoot common.Hash, shardIndex uint16, segmentIndices []uint16) (erasure_root common.Hash, shard_index uint16, segment_Indices []uint16, selected_segments [][]byte, selected_full_justifications [][]byte, selected_segments_justifications [][]byte, exportedSegmentAndPageProofLens int, ok bool, err error) {
	//j⌢[b] <--- CE_137 shared
	//segmentshards = []byte{}
	//segment_justifications = [][]byte{}

	esKey := generateErasureRootShardIdxKey(erasureRoot, shardIndex)
	s_es_key := fmt.Sprintf("s_%s", esKey)
	log.Trace(debugDA, "N%d GetSegmentShard_Assurer s_es_key %v\n", n.id, s_es_key)
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

// Look up the erasureRoot for  [exportedSegmentRoot, erasureRoot and WorkPackageHash]
func (n *Node) ErasureRootLookUP(h common.Hash) (erasureRoot common.Hash, err error) {
	erasureRootRaw, ok, err := n.ReadRawKV([]byte(generateRequestedHashToErasureRootKey(h)))
	if err != nil || !ok {
		log.Error(debugDA, "ErasureRootLookUP", "err", err)
		return h, nil
	}
	return common.Hash(erasureRootRaw), nil
}
