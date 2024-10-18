package node

import (
	"fmt"
	"reflect"

	"encoding/binary"
	"encoding/json"

	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	// compile wp into b
	package_bundle := n.CompilePackageBundle(workPackage)
	package_bundle_byte := package_bundle.Bytes()
	recovered_package_bundle, _ := types.WorkPackageBundleFromBytes(package_bundle_byte)

	if debug {
		fmt.Printf("packageHash=%v, encodedPackage(Len=%v):%x\n", packageHash, len(package_bundle_byte), package_bundle_byte)
		fmt.Printf("raw=%v\n", package_bundle.String())
	}

	//recovered_b := n.decodeWorkPackage(package_bundle)

	//TODO: make sure b equals recovered_b
	if common.CompareBytes(package_bundle.Bytes(), recovered_package_bundle.Bytes()) {
		//fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are the same-------\n")
	} else {
		fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are different-------\n")
	}

	b := package_bundle.Bytes()

	// Length of `b`
	bLength := uint32(len(b))

	// Build b♣ and s♣
	blobRoot, bClubs, bEcChunks := n.buildBClub(b)
	combinedTreeRoot, sClubs, segmentsECRoots, segmentItemRoots, sEcChunksArr := n.buildSClub(segments)
	//treeRoot, sClubs, sEcChunksArr := n.buildSClub(segments)
	fmt.Printf("len(bEcChunks)=%v\n", len(bEcChunks))
	fmt.Printf("len(sEcChunksArr)=%v\n", len(sEcChunksArr))
	fmt.Printf("bClubs %v\n", bClubs)
	fmt.Printf("sClubs %v\n", sClubs)
	fmt.Printf("combinedTreeRoot %v\n", combinedTreeRoot)
	fmt.Printf("segmentsECRoots %v\n", segmentsECRoots)
	fmt.Printf("segmentItemRoots %v\n", segmentItemRoots)

	// u = (bClub, sClub)
	erasure_root_u := n.generateErasureRoot(bClubs, sClubs, blobRoot, combinedTreeRoot)

	// ExportedSegmentRoot = CDT(segments)
	exported_segment_root_e := generateExportedSegmentsRoot(segments)

	// Return the Availability Specifier
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:     packageHash,
		BundleLength:        bLength,
		ErasureRoot:         erasure_root_u,
		ExportedSegmentRoot: exported_segment_root_e,
	}

	erasureMeta = ECCErasureMap{
		ErasureRoot:         erasure_root_u,
		ExportedSegmentRoot: exported_segment_root_e,
		WorkPackageHash:     packageHash,
		BundleLength:        bLength,
		BClubs:              bClubs,
		SClubs:              sClubs,
		BlobRoot:            blobRoot,
		CombinedTreeRoot:    combinedTreeRoot,
		SegmentsECRoots:     segmentsECRoots,
		SegmentItemRoots:    segmentItemRoots,
	}

	return &availabilitySpecifier, erasureMeta, bEcChunks, sEcChunksArr
}

func (n *Node) GetMeta(erasureRoot common.Hash) (erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk, err error) {
	//TODO: should probably store erasureRoot -> pbH
	erasure_metaKey := fmt.Sprintf("erasureMeta-%v", erasureRoot)
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	erasure_metaKey_val, err := n.ReadRawKV([]byte(erasure_metaKey))
	erasure_bKey_val, err := n.ReadRawKV([]byte(erasure_bKey))
	erasure_sKey_val, err := n.ReadRawKV([]byte(erasure_sKey))
	if err != nil {
		return erasureMeta, bECChunks, sECChunksArray, fmt.Errorf("Fail to find erasure_metaKey=%v", erasureRoot)
	}
	// TODO: figure out the codec later
	if err := erasureMeta.Unmarshal(erasure_metaKey_val); err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return erasureMeta, bECChunks, sECChunksArray, err
	}

	// Variables to hold the unmarshalled data
	// var bECChunks []types.DistributeECChunk
	// var sECChunksArray [][]types.DistributeECChunk

	if err := json.Unmarshal(erasure_bKey_val, &bECChunks); err != nil {
		return erasureMeta, bECChunks, sECChunksArray, err
	}

	if err := json.Unmarshal(erasure_sKey_val, &sECChunksArray); err != nil {
		return erasureMeta, bECChunks, sECChunksArray, err
	}
	fmt.Printf("Recover Meta from levelDB. Ready for Building. %v, erasureMap=%v, bECChunks=%v, sECChunksArray=%v\n", erasureRoot, erasureMeta.String(), bECChunks, sECChunksArray)
	return erasureMeta, bECChunks, sECChunksArray, err
}

func (n *Node) StoreMeta(as *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	erasure_root_u := as.ErasureRoot
	erasure_metaKey := fmt.Sprintf("erasureMeta-%v", erasure_root_u)
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasure_root_u)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasure_root_u)
	packageHash_sey := fmt.Sprintf("erasureSChunk-%v", as.WorkPackageHash)

	bChunkJson, _ := json.Marshal(bECChunks)
	sChunkJson, _ := json.Marshal(sECChunksArray)

	fmt.Printf("erasure_metaKey=%v, val=%s\n", erasure_metaKey, string(erasureMeta.Bytes()))
	fmt.Printf("erasure_bKey=%v, val=%s\n", erasure_metaKey, string(bChunkJson))
	fmt.Printf("erasure_sKey=%v, val=%s\n", erasure_metaKey, string(sChunkJson))
	n.FakeWriteRawKV(erasure_metaKey, erasureMeta.Bytes())
	n.FakeWriteRawKV(erasure_bKey, bChunkJson)
	n.FakeWriteRawKV(erasure_sKey, sChunkJson)
	n.FakeWriteRawKV(packageHash_sey, erasure_root_u.Bytes())

}

// this is the default justification from (b,s) to erasureRoot
func ErasureRootDefaultJustification(b []common.Hash, s []common.Hash) (shardJustifications []types.Justification, err error) {
	shardJustifications = make([]types.Justification, types.TotalValidators)
	erasureTree, bundle_segment_pairs := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leafHash, path, isFound, _ := erasureTree.Trace(shardIdx)
		verified := VerifyJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		if !verified {
			return shardJustifications, fmt.Errorf("Justification Failure")
		}
		if verified {
			shardJustifications[shardIdx] = types.Justification{
				Root:     erasureRoot,
				ShardIdx: shardIdx,
				TreeLen:  types.TotalValidators,
				Leaf:     bundle_segment_pairs[shardIdx],
				LeafHash: leafHash,
				Path:     path,
			}
		}
		fmt.Printf("ErasureRootPath shardIdx=%v, treeLen=%v leafHash=%v, path=%v, isFound=%v | verified=%v\n", shardIdx, treeLen, leafHash, path, isFound, verified)
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash common.Hash, path []common.Hash) bool {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	if root != recoveredRoot {
		fmt.Sprintf("VerifyJustification Failure! Expected:%v | Recovered: %v\n", root, recoveredRoot)
		//panic("VerifyJustification")
	}
	return verified
}

// Generating co-path for T(s,i,H)
// s: [(b♣T,s♣T)...] -  sequence of (work-package bundle shard hash, segment shard root) pairs satisfying u = MB(s)
// i: shardIdx or ChunkIdx
// H: Blake2b
func GenerateJustification(root common.Hash, shardIndex uint16, leaves [][]byte) (treeLen int, leafHash common.Hash, path []common.Hash, isFound bool) {
	wbt := trie.NewWellBalancedTree(leaves)
	//treeLen, leafHash, path, isFound, nil
	treeLen, leafHash, path, isFound, _ = wbt.Trace(int(shardIndex))
	//fmt.Printf("[shardIndex=%v] erasureRoot=%v, leafHash=%v, path=%v, found=%v\n", shardIndex, erasureRoot, leafHash, path, isFound)
	return treeLen, leafHash, path, isFound
}

func GetOrderedChunks(erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) (shardJustifications []types.Justification, orderedBundleShards []types.ConformantECChunk, orderedSegmentShards [][]types.ConformantECChunk) {
	shardJustifications, _ = ErasureRootDefaultJustification(erasureMeta.BClubs, erasureMeta.SClubs)
	fmt.Printf("shardJustifications: %v\n", shardJustifications[0].String())
	orderedBundleShards = ComputeOrderedNPBundleChunks(bECChunks)
	fmt.Printf("orderedBundleShards %x\v\n", orderedBundleShards)
	orderedSegmentShards = ComputeOrderedExportedNPChunks(sECChunksArray)
	fmt.Printf("orderedSegmentShards %x\v\n", orderedSegmentShards)
	return shardJustifications, orderedBundleShards, orderedSegmentShards
}

func GetShardSpecificOrderedChunks(shardIdx uint16, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) (shardJustification types.Justification, bundleShard types.ConformantECChunk, segmentShards []types.ConformantECChunk) {
	idx := int(shardIdx)
	shardJustifications, orderedBundleShards, orderedSegmentShards := GetOrderedChunks(erasureMeta, bECChunks, sECChunksArray)
	return shardJustifications[idx], orderedBundleShards[idx], orderedSegmentShards[idx]
}

func (n *Node) FakeDistributeChunks(erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	//cheating .. remove after correctness
	debug := true
	if debug {
		// Distribute b♣ Chunks and s♣ Chunks
		n.DistributeEcChunks(bECChunks)
		n.DistributeExportedEcChunkArray(sECChunksArray)
	}
}

type ECCErasureMap struct {
	ErasureRoot         common.Hash
	ExportedSegmentRoot common.Hash
	WorkPackageHash     common.Hash
	BundleLength        uint32
	BClubs              []common.Hash
	SClubs              []common.Hash
	BlobRoot            common.Hash
	CombinedTreeRoot    common.Hash
	SegmentsECRoots     []common.Hash
	SegmentItemRoots    [][]common.Hash
}

// Marshal marshals ECCErasureMap into JSON
func (e *ECCErasureMap) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *ECCErasureMap) Unmarshal(data []byte) error {
	return json.Unmarshal(data, e)
}

// TODO: use codec ..
func (e *ECCErasureMap) Bytes() []byte {
	jsonData, _ := e.Marshal()
	return jsonData
}

// TODO: use codec ..
func (e *ECCErasureMap) String() string {
	return string(e.Bytes())
}

func (n *Node) PrepareArbitaryData(b []byte) ([][][]byte, common.Hash, int) {
	// Padding b to the length of W_E
	paddedB := common.PadToMultipleOfN(b, types.W_E)
	bLength := len(b)

	chunks, err := n.encode(paddedB, false, bLength)
	if err != nil {
		fmt.Println("Error in prepareArbitaryData:", err)
	}
	blobHash := common.Blake2Hash(paddedB)
	return chunks, blobHash, bLength
}

// Compute b♣ using the EncodeWorkPackage function
func (n *Node) buildBClub(b []byte) (common.Hash, []common.Hash, []types.DistributeECChunk) {

	chunks, blobHash, bLength := n.PrepareArbitaryData(b)
	ecChunks, err := n.BuildArbitraryDataChunks(chunks, blobHash, bLength)
	if err != nil {
		fmt.Println("Error in DistributeSegmentData:", err)
	}

	//var tmpbClub []common.Hash
	// for _, block := range chunks {
	// 	for _, b := range block {
	// 		tmpbClub = append(tmpbClub, common.Hash(common.PadToMultipleOfN(b, 32)))
	// 	}
	// 	bClub = tmpbClub
	// }

	// Hash each element of the encoded data
	bClubs := make([]common.Hash, types.TotalValidators)
	bundleShards := chunks[0] // this should be of size 1
	for shardIdx, shard := range bundleShards {
		//tmpbClub = append(tmpbClub, common.Hash(common.PadToMultipleOfN(shard, 32)))
		bClubs[shardIdx] = common.Blake2Hash(shard)
	}
	return blobHash, bClubs, ecChunks
}

func (n *Node) buildSClub(segments [][]byte) (common.Hash, []common.Hash, []common.Hash, [][]common.Hash, [][]types.DistributeECChunk) {

	ecChunksArr := make([][]types.DistributeECChunk, 0)
	pageProofs, _ := trie.GeneratePageProof(segments)
	exportedSegmentLen := len(segments)
	pageProofLen := len(pageProofs)

	// array of segmentsRoot ++ pageProofRoot
	var segmentsECRoots []common.Hash

	// this is the raw roots at ith_item level
	segmentItemRoots := make([][]common.Hash, exportedSegmentLen+pageProofLen)

	// logic a transpose
	sequentialTranspose := make([][][]byte, types.TotalValidators)

	// gathering root per segment or pageProof
	for segmentIdx, segmentData := range segments {
		// Encode the data into segments
		erasureCodingSegments, err := n.encode(segmentData, true, len(segmentData)) // Set to false for variable size segments
		if err != nil {
			fmt.Printf("Error in buildSClub segment#%v: %v\n", segmentIdx, err)
		}

		// Build segment roots
		fmt.Printf("segment#%v len=%v\n", segmentIdx, len(erasureCodingSegments))
		if len(erasureCodingSegments) != 1 {
			panic("Invalid segment implementation! NOT OK")
		}

		segmentLeaves := erasureCodingSegments[0]
		segmentTree := trie.NewCDMerkleTree(segmentLeaves)
		segmentRoot := segmentTree.RootHash()
		segmentRoots := make([][]byte, 1)
		segmentRoots[0] = segmentRoot.Bytes()

		segmentItemRoots[segmentIdx] = []common.Hash{segmentRoot}

		// Generate the blob hash by hashing the original data
		// blobTree := trie.NewCDMerkleTree(segmentRoots)
		// segmentsECRoot := blobTree.Root()
		// fmt.Printf("segment#%v: segmentsECRoot=%x, segmentRoots=%x\n", segmentIdx, segmentsECRoot, segmentRoots)

		blobTree := trie.NewCDMerkleTree(segmentRoots)
		segmentsECRoot := blobTree.RootHash()
		segmentsECRoots = append(segmentsECRoots, segmentsECRoot)

		// Distribute the segments
		ecChunks, err := n.BuildExportedSegmentChunks(erasureCodingSegments, segmentRoots)
		if err != nil {
			fmt.Printf("Error in buildSClub segment#%v: %v\n", segmentIdx, err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}

		fmt.Printf("buildSClub segment#%v: len(ecChunks)=%v\n", segmentIdx, len(ecChunks))
		ecChunksArr = append(ecChunksArr, ecChunks)
	}

	// gathering root per pagrProof, each pageProof can be more than G per our implementation
	for pageIdx, pageData := range pageProofs {
		// Encode the data into segments
		erasureCodingPageSegments, err := n.encode(pageData, true, len(pageData)) // Set to false for variable size segments
		if err != nil {
			fmt.Printf("Error in buildSClub pageProof#%v: %v\n", pageIdx, err)
		}

		// Build segment roots -- page can be longer than G long
		fmt.Printf("!!! pageProof#%v len=%v\n", pageIdx, len(erasureCodingPageSegments))
		pageSegmentRoots := make([][]byte, 0)
		ith_pageProof := make([]common.Hash, 0)
		for i := range erasureCodingPageSegments {
			pageProofleaves := erasureCodingPageSegments[i]
			pageProofSubtree := trie.NewCDMerkleTree(pageProofleaves)
			pageProofSubtreeRoot := pageProofSubtree.RootHash()
			ith_pageProof = append(ith_pageProof, pageProofSubtreeRoot)
			pageSegmentRoots = append(pageSegmentRoots, pageProofSubtreeRoot.Bytes())
		}

		segmentItemRoots[exportedSegmentLen+pageIdx] = ith_pageProof
		pageProofTree := trie.NewCDMerkleTree(pageSegmentRoots)
		pageProofTreeRoot := pageProofTree.RootHash()

		// Append the segment root to the list of segment roots
		segmentsECRoots = append(segmentsECRoots, pageProofTreeRoot)
		// Distribute the segments
		ecChunks, err := n.BuildExportedSegmentChunks(erasureCodingPageSegments, pageSegmentRoots)
		if err != nil {
			fmt.Printf("Error in buildSClub pageProof#%v: %v\n", pageIdx, err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}
		fmt.Printf("len(ecChunks)=%v\n", len(ecChunks)) // this is multiple of totalValidators

		fmt.Printf("buildSClub pageProof#%v: len(ecChunks)=%v\n", pageIdx, len(ecChunks))
		ecChunksArr = append(ecChunksArr, ecChunks)
	}
	fmt.Printf("len(ecChunksArr)=%v\n", len(ecChunksArr))

	var combinedData [][][]byte

	//s++P(s)
	combinedSegmentAndPageProofs := append(segments, pageProofs...)

	// Encode the combined data: s++P(s)
	combinedTree := trie.NewCDMerkleTree(combinedSegmentAndPageProofs)
	combinedTreeRoot := common.Hash(combinedTree.Root())

	// Flatten the combined data
	var FlattenData []byte
	for _, singleData := range combinedSegmentAndPageProofs {
		FlattenData = append(FlattenData, singleData...)
	}
	// Erasure code the combined data
	encodedSegment, _ := erasurecoding.Encode(FlattenData, 6)

	var flattenSegmentsECRoots []byte
	for _, segmentsECRoot := range segmentsECRoots {
		flattenSegmentsECRoots = append(flattenSegmentsECRoots, segmentsECRoot[:]...)
	}

	n.FakeWriteKV(combinedTreeRoot, flattenSegmentsECRoots)
	//fmt.Printf("combinedTreeRoot: %x -> %v\n", combinedTreeRoot[:], segmentsECRoots)
	//fmt.Printf("combinedTreeRoot: %x -> items %v\n", combinedTreeRoot[:], segmentItemRoots)

	// Append the encoded segment to the combined data
	combinedData = encodedSegment

	// fmt.Printf("Before Transpose Size: %d, %d, %d\n", len(combinedData), len(combinedData[0]), len(combinedData[0][0]))

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	fmt.Printf("After Transpose Size: %d, %d, %d\n", len(transposedData), len(transposedData[0]), len(transposedData[0][0]))

	sClub := make([]common.Hash, types.TotalValidators)
	for shardIdx, shardData := range sequentialTranspose {
		shard_wbt := trie.NewWellBalancedTree(shardData)
		//fmt.Printf("!!!sClub verifying shardIdx=%v, shardData%x Root=%v\n", shardIdx, shardData, shard_wbt.RootHash())
		sClub[shardIdx] = shard_wbt.RootHash()
	}
	return combinedTreeRoot, sClub, segmentsECRoots, segmentItemRoots, ecChunksArr
}

func GenerateErasureTree(b []common.Hash, s []common.Hash) (*trie.WellBalancedTree, [][]byte) {
	// Combine b♣ and s♣ into a matrix and transpose it
	bundle_segment_pairs := make([][]byte, types.TotalValidators)

	// Transpose the b♣ array and s♣ array
	for i := 0; i < types.TotalValidators; i++ {
		//(work-package bundle shard hash, segment shard root) pairs
		pair := append(b[i].Bytes(), s[i].Bytes()...)
		bundle_segment_pairs[i] = pair
	}

	// Generate WBT from the hashed elements and return the root (u)
	wbt := trie.NewWellBalancedTree(bundle_segment_pairs)
	// do I get same root for wbt and wbt1??
	return wbt, bundle_segment_pairs
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func (n *Node) generateErasureRoot(b []common.Hash, s []common.Hash, blobHash common.Hash, treeRoot common.Hash) common.Hash {
	erasureTree, bundle_segment_pairs := GenerateErasureTree(b, s)
	erasureRoot := common.Hash(erasureTree.Root())
	fmt.Printf("Len(bundle_segment_pairs), bundle_segment_pairs: %d, %x\n", len(bundle_segment_pairs), bundle_segment_pairs)
	fmt.Printf("Len(blobHash), blobHash: %d, %x\n", len(blobHash), blobHash[:])
	fmt.Printf("Len(treeRoot), treeRoot: %d, %x\n", len(treeRoot), treeRoot[:])

	//STANLEY TODO: this has to be part of metadata
	n.FakeWriteKV(erasureRoot, append(blobHash[:], treeRoot[:]...))
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leafHash, path, isFound := GenerateJustification(erasureRoot, uint16(shardIdx), bundle_segment_pairs)
		verified := VerifyJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		fmt.Printf("ErasureRootPath shardIdx=%v, treeLen=%v leafHash=%v, path=%v, isFound=%v | verified=%v\n", shardIdx, treeLen, leafHash, path, isFound, verified)
	}
	fmt.Printf("Len(ErasureRoot), ErasureRoot: %d, %v\n", len(erasureRoot), erasureRoot)
	return erasureRoot
}

// M(s) - CDT of exportedSegment
func generateExportedSegmentsRoot(segments [][]byte) common.Hash {
	var segmentData [][]byte
	for _, segment := range segments {
		segmentData = append(segmentData, segment)
	}

	cdt := trie.NewCDMerkleTree(segmentData)
	return common.Hash(cdt.Root())
}

// Validate the availability specifier
func (n *Node) IsValidAvailabilitySpecifier(bClubBlobHash common.Hash, bLength int, sClubBlobHash common.Hash, originalAS *types.AvailabilitySpecifier) (bool, error) {
	return true, nil
	/*
	   // Fetch and reconstruct the data for b♣ and s♣
	   bClubData, err := n.FetchAndReconstructArbitraryData(bClubBlobHash, bLength)

	   	if err != nil {
	   		return false, err
	   	}

	   sClubData, _, _, err := n.FetchAndReconstructAllSegmentsData(sClubBlobHash)

	   	if err != nil {
	   		return false, err
	   	}

	   // Recalculate the AvailabilitySpecifier
	   reconstructbClub := n.recomputeBClub(bClubData)
	   reconstructsClub := n.recomputeSClub(sClubData)

	   //bClub, sClub, blobRoot, treeRoot
	   erasureRoot_u := n.generateErasureRoot(reconstructbClub, reconstructsClub, bClubBlobHash, sClubBlobHash)

	   // compare the recalculated AvailabilitySpecifier with the original

	   	if originalAS.ErasureRoot != erasureRoot_u {
	   		fmt.Printf("ErasureRoot mismatch (%x, %x)\n", originalAS.ErasureRoot, erasureRoot_u)
	   		return false, nil
	   	}

	   return true, nil
	*/
}

// Recompute b♣ using the EncodeWorkPackage function
func (n *Node) recomputeBClub(paddedB []byte) []common.Hash {
	// Process the padded data using erasure coding
	c_base := types.ComputeC_Base(len(paddedB))

	encodedB, err := erasurecoding.Encode(paddedB, c_base)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	// Hash each element of the encoded data
	var tmpbClub []common.Hash
	var bClub []common.Hash

	for _, block := range encodedB {
		for _, b := range block {
			tmpbClub = append(tmpbClub, common.Hash(common.PadToMultipleOfN(b, 32)))
		}
		//bClub = append(bClub, tmpbClub)
		bClub = tmpbClub
	}

	return bClub
}

func (n *Node) recomputeSClub(combinedSegmentAndPageProofs [][]byte) []common.Hash {
	var combinedData [][][]byte

	// Flatten the combined data
	var FlattenData []byte
	for _, singleData := range combinedSegmentAndPageProofs {
		FlattenData = append(FlattenData, singleData...)
	}
	// Erasure code the combined data
	encodedSegment, err := erasurecoding.Encode(FlattenData, 6)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	// Append the encoded segment to the combined data
	combinedData = append(combinedData, encodedSegment...)
	// fmt.Printf("Before Transpose Size: %d, %d, %d\n", len(combinedData), len(combinedData[0]), len(combinedData[0][0]))

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	// fmt.Printf("After Transpose Size: %d, %d, %d\n", len(transposedData), len(transposedData[0]), len(transposedData[0][0]))

	var sClub []common.Hash
	var tmpsClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		tmpsClub = append(tmpsClub, common.Hash(root))
	}
	sClub = tmpsClub
	//sClub = append(sClub, tmpsClub)

	return sClub
}

// Helper function to transpose a matrix
func transpose(matrix [][]byte) [][]byte {
	if len(matrix) == 0 {
		return nil
	}

	transposed := make([][]byte, len(matrix[0]))
	for i := range transposed {
		transposed[i] = make([]byte, len(matrix))
		for j := range matrix {
			transposed[i][j] = matrix[j][i]
		}
	}
	return transposed
}

func transpose3D(data [][][]byte) [][][]byte {
	if len(data) == 0 || len(data[0]) == 0 {
		return [][][]byte{}
	}

	// Building a new 3D array to store the transposed data
	transposed := make([][][]byte, len(data[0]))
	for i := range transposed {
		transposed[i] = make([][]byte, len(data))
		for j := range transposed[i] {
			transposed[i][j] = make([]byte, len(data[0][0]))
		}
	}

	// Transposing the data
	for i := 0; i < len(data[0]); i++ {
		for j := 0; j < len(data); j++ {
			for k := 0; k < len(data[0][0]); k++ {
				transposed[i][j][k] = data[j][i][k]
			}
		}
	}

	return transposed
}

// TODO: Sean to encode & decode properly
// The E(p,x,s,j) function is a function that takes a package and its segments and returns a result, in EQ(186)

func (n *Node) CompilePackageBundle(p types.WorkPackage) types.WorkPackageBundle {

	workItems := p.WorkItems
	workItemCnt := len(workItems)

	// p - workPackage

	// x - [extrinsic data] for some workitem argument w
	extrinsicData := make([]types.ExtrinsicsBlobs, workItemCnt)
	for workIdx, workItem := range workItems {
		extrinsicData[workIdx] = workItem.ExtrinsicsBlobs
	}

	// s - [ImportSegmentData] should be size of G = W_E * W_S
	importedSegmentData := make([][][]byte, workItemCnt)
	importedSegmentIdx := make([][]uint16, workItemCnt)

	for workIdx, workItem := range workItems {
		// not sure what does idx mean inside of ImportedSegments
		segmentIdxMap := make([]uint16, len(workItem.ImportedSegments))
		workItemIdx_importedSegmentData, err := n.getImportSegments(workItem.ImportedSegments)
		for i, workItem_importedSegment := range workItem.ImportedSegments {
			segmentIdxMap[i] = workItem_importedSegment.Index
		}
		if err != nil {
			//TODO
		}
		importedSegmentIdx[workIdx] = segmentIdxMap
		importedSegmentData[workIdx] = workItemIdx_importedSegmentData
	}

	// j - justifications
	justification := make([][][]common.Hash, workItemCnt)

	for workIdx, item_segmentData := range importedSegmentData {
		workIdx_segment_tree := trie.NewCDMerkleTree(item_segmentData)
		var item_Justification [][]common.Hash
		for segment_idx, _ := range item_segmentData {
			// not sure what does index mean here...
			importedSegments_Index := importedSegmentIdx[workIdx][segment_idx]
			// not sure if we should use segment_idx or Index from ImportedSegments
			segment_Justification, _ := workIdx_segment_tree.Justify(int(importedSegments_Index))
			segment_Justification_hash := make([]common.Hash, len(segment_Justification))
			for hash_idx, byte := range segment_Justification {
				segment_Justification_hash[hash_idx] = common.Hash(byte)
			}
			item_Justification = append(item_Justification, segment_Justification_hash)
			justification[workIdx] = item_Justification
		}
	}

	workPackageBundle := types.WorkPackageBundle{
		WorkPackage:       p,
		ExtrinsicData:     extrinsicData,
		ImportSegmentData: importedSegmentData,
		Justification:     justification,
	}
	return workPackageBundle
}

func (n *Node) encodeWorkPackage_delete(wp types.WorkPackage) []byte {
	//fmt.Println("encodeWorkPackage")
	output := make([]byte, 0)
	// 1. Encode the package (p)
	//fmt.Println("wp:", wp)
	encodedPackage, err := types.Encode(wp)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	output = append(output, encodedPackage...)

	// 2. Encode the extrinsic (x)
	x := wp.WorkItems
	extrinsics := make([][]byte, 0)
	for _, WorkItem := range x {
		extrinsics = append(extrinsics, WorkItem.ExtrinsicsBlobs...)
	}
	if debug {
		fmt.Println("extrinsics:", extrinsics)
	}
	encodedExtrinsic, err := types.Encode(extrinsics)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	output = append(output, encodedExtrinsic...)

	// 3. Encode the segments (i)
	var encodedSegments []byte
	var segments [][]byte
	for _, WorkItem := range x {
		segments, _ = n.getImportSegments(WorkItem.ImportedSegments)
	}
	//fmt.Println("segments:", segments)
	encodedSegments, err = types.Encode(segments)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	output = append(output, encodedSegments...)

	// 4. Encode the justifications (j)
	var encodedJustifications []byte
	var justification [][]byte
	for _, WorkItem := range x {
		tree := trie.NewCDMerkleTree(segments)
		for i, _ := range WorkItem.ImportedSegments {
			justifies, _ := tree.Justify(i)
			var tmpJustification [][]byte
			for _, justify := range justifies {
				tmpJustification = append(tmpJustification, justify)
			}
			justification = append(justification, tmpJustification...)
		}
	}

	encodedJustification, err := types.Encode(justification)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	encodedJustifications = append(encodedJustifications, encodedJustification...)

	output = append(output, encodedJustifications...)

	// Combine all encoded parts: e(p,x,i,j)
	return output
}

func (n *Node) decodeWorkPackage_delete(encodedWorkPackage []byte) types.WorkPackage {
	//fmt.Println("decodeWorkPackage")
	// length := uint32(0)

	// // Decode the package (p)
	wp, _, err := types.Decode(encodedWorkPackage, reflect.TypeOf(types.WorkPackage{}))
	if err != nil {
		fmt.Println("Error in decodeWorkPackage:", err)
	}
	decodedPackage := wp.(types.WorkPackage)
	//fmt.Println("decodedPackage:", decodedPackage.String())
	// length += l
	/*
		// Decode the extrinsic (x)
		extrinsics, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("extrinsics:", extrinsics)
		decodedPackage.WorkItems = make([]types.WorkItem, 0)
		for _, extrinsic := range extrinsics.([][]byte) {
			decodedPackage.WorkItems = append(decodedPackage.WorkItems, types.WorkItem{
				ExtrinsicsBlobs: [][]byte{extrinsic},
			})
		}
		length += l

		// Decode the segments (i)
		segments, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("segments:", segments)
		// setImportSegments
		length += l

		// Decode the justifications (j)
		justifications, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("justifications:", justifications)
		// setJustifications
		length += l
	*/

	return decodedPackage
	// return decodedPackage, decodedPackage.WorkItems, segments,justifications
}

func compareWorkPackages(wp1, wp2 types.WorkPackage) bool {
	// Compare Authorization
	if !common.CompareBytes(wp1.Authorization, wp2.Authorization) {
		fmt.Printf("Authorization mismatch (%x, %x)\n", wp1.Authorization, wp2.Authorization)
		fmt.Println("Authorization mismatch")
		return false
	}

	// Compare AuthCodeHost
	if wp1.AuthCodeHost != wp2.AuthCodeHost {
		return false
	}

	// Compare Authorizer struct
	if !common.CompareBytes(wp1.Authorizer.CodeHash[:], wp2.Authorizer.CodeHash[:]) {
		return false
	}
	if !common.CompareBytes(wp1.Authorizer.Params, wp2.Authorizer.Params) {
		return false
	}

	// Compare RefineContext struct
	if !common.CompareBytes(wp1.RefineContext.Anchor[:], wp2.RefineContext.Anchor[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.StateRoot[:], wp2.RefineContext.StateRoot[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.BeefyRoot[:], wp2.RefineContext.BeefyRoot[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.LookupAnchor[:], wp2.RefineContext.LookupAnchor[:]) {
		return false
	}
	if wp1.RefineContext.LookupAnchorSlot != wp2.RefineContext.LookupAnchorSlot {
		return false
	}

	// Compare WorkItems
	if len(wp1.WorkItems) != len(wp2.WorkItems) {
		return false
	}
	for i := range wp1.WorkItems {
		if wp1.WorkItems[i].CodeHash != wp2.WorkItems[i].CodeHash {
			return false
		}
	}
	return true
}

// Verify WorkPackage by comparing the original and the decoded WorkPackage
func (n *Node) VerifyWorkPackageBundle(package_bundle types.WorkPackageBundle) bool {

	package_bundle_byte := package_bundle.Bytes()
	recovered_package_bundle, _ := types.WorkPackageBundleFromBytes(package_bundle_byte)
	if common.CompareBytes(package_bundle.Bytes(), recovered_package_bundle.Bytes()) {
		//fmt.Printf("----------Original WorkPackageBundle and Decoded WorkPackageBundle are the same-------\n")
		return true
	} else {
		//fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are different-------\n")
		return false
	}
	// if compareWorkPackageBundle(b, recovered_package_bundle) {
	// 	return true
	// } else {
	// 	return false
	// }
}

// After FetchExportedSegments and quick verify exported segments
func (n *Node) FetchWorkPackage(erasureRoot common.Hash, lengthB int) (workpackage types.WorkPackage, bClubHash common.Hash, err error) {
	/*
		// Fetch the WorkPackage and the exported segments
		// erasureRoot = B^Club + S^Club
		allHash, err := n.store.ReadKV(erasureRoot)
		if err != nil {
			fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
		}
		fmt.Printf("erasureRoot Val %x\n", allHash)
		bClubRoot := allHash[:32]
		bClubHash := common.Hash(bClubRoot)

		encodedB, err := n.FetchAndReconstructArbitraryData(bClubHash, lengthB)
		if err != nil {
			fmt.Printf("\nError in FetchWorkPackage: %v\n", err)
			return types.WorkPackage{}, common.Hash{}, err
		}
		fmt.Printf("FetchWorkPackage erasureRoot=%v, bClubHash=%v, encodedB=%v\n", erasureRoot, bClubHash, encodedB)
		encodedB = encodedB[:lengthB]
		workpackage := n.decodeWorkPackage(encodedB)
		// TODO:Do something like audit
	*/
	return workpackage, bClubHash, err
}
func (n *Node) FetchExportedSegments(erasureRoot common.Hash) (exportedSegments [][]byte, pageProofs [][]byte, treeRoots []common.Hash, sClubHash common.Hash, err error) {
	/*
		// Fetch the WorkPackage and the exported segments
		// TODO: use broadcast to fetch the data
		// erasureRoot = B^Club + S^Club
		allHash, err := n.store.ReadKV(erasureRoot)
		if err != nil {
			fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
		}
		fmt.Printf("allHash: %x\n", allHash)
		sClubRoot := allHash[32:]
		sClubHash := common.Hash(sClubRoot)
		fmt.Printf("sClubHash: %v\n", sClubHash)
		exportedSegments, pageProofs, treeRoots, err := n.FetchAndReconstructAllSegmentsData(sClubHash)
		if err != nil {
			return [][]byte{}, [][]byte{}, []common.Hash{}, common.Hash{}, err
		}
	*/
	return exportedSegments, pageProofs, treeRoots, sClubHash, nil
}

// func (n *Node) FetchWorkPackageAndExportedSegments(erasureRoot common.Hash) (workPackage types.WorkPackage, exportedSegments [][]byte, err error) {
// 	exportedSegments, treeRoots, sClubHash, segment_err := n.FetchExportedSegments(erasureRoot)
// 	if (segment_err != nil){
// 		return workPackage, exportedSegments, segment_err
// 	}
// 	workPackage, bClubHash, wp_err := n.FetchWorkPackage(erasureRoot)
// 	if (wp_err != nil){
// 		return types.WorkPackage{}, exportedSegments, wp_err
// 	}
// 	//b♣,s♣
// 	fmt.Printf("b♣Hash=%v, s♣Hash=%v, treeRoots=%v\n", bClubHash, sClubHash, treeRoots)
// 	return workPackage, exportedSegments, nil
// }

// work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error
func (n *Node) executeWorkPackage(workPackage types.WorkPackage) (guarantee types.Guarantee, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {

	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	service_index := uint32(workPackage.AuthCodeHost)
	packageHash := workPackage.Hash()
	fmt.Printf("%s executeWorkPackage workPackageHash=%v\n", n.String(), packageHash)

	//TODO: do we still need audit friendly work WorkPackage?

	segments := make([][]byte, 0)
	for _, workItem := range workPackage.WorkItems {
		// recover code from the bpt. NOT from DA
		code := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if len(code) == 0 {
			err = fmt.Errorf("code not found in bpt. C(%v, %v)", service_index, workItem.CodeHash)
			fmt.Println(err)
			return
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			fmt.Printf("Code and CodeHash Mismatch\n")
			panic(0)
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		// set malicious mode here
		vm.IsMalicious = false
		imports, err0 := n.getImportSegments(workItem.ImportedSegments)
		if err0 != nil {
			// return spec, common.Hash{}, err
			imports = make([][]byte, 0)
		}
		// Decode Import Segments to FIB fromat
		//fmt.Printf("Import Segments: %v\n", imports)
		if len(imports) > 0 {
			if debug {
				fib_imported_result := imports[0][:12]
				n := binary.LittleEndian.Uint32(fib_imported_result[0:4])
				Fib_n := binary.LittleEndian.Uint32(fib_imported_result[4:8])
				Fib_n_1 := binary.LittleEndian.Uint32(fib_imported_result[8:12])
				fmt.Printf("Imported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n\n", n, Fib_n, Fib_n_1)
			}
		}
		vm.SetImports(imports)
		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		err = vm.Execute(types.EntryPointRefine)
		if err0 != nil {
			return
		}
		output, _ := vm.GetArgumentOutputs()
		// The workitem is an ordered collection of segments
		asWorkItem := types.ASWorkItem{
			Segments:   make([]types.Segment, 0),
			Extrinsics: make([]types.WorkItemExtrinsic, 0),
		}
		for _, i := range vm.Imports {
			asWorkItem.Segments = append(asWorkItem.Segments, types.Segment{Data: i})
		}
		for _, extrinsicblob := range workItem.ExtrinsicsBlobs {
			asWorkItem.Extrinsics = append(asWorkItem.Extrinsics, types.WorkItemExtrinsic{Hash: common.BytesToHash(extrinsicblob), Len: uint32(len(extrinsicblob))})
		}

		// 1. NOTE: We do NOT need to erasure code import data
		// 2. TODO: We DO need to erasure encode extrinsics into "Audit DA"
		// ******TODO******

		// 3. We DO need to erasure code exports from refine execution into "Import DA"
		//fmt.Printf("VM Exports: %v\n", vm.Exports)
		for _, e := range vm.Exports {
			s := e
			segments = append(segments, s) // this is used in NewAvailabilitySpecifier
		}

		// Decode the Exports Segments to FIB format
		//fmt.Printf("Exports Segments: %v\n", segments)
		if len(segments) > 0 {
			if debug {
				fib_exported_result := segments[0][:12]
				n := binary.LittleEndian.Uint32(fib_exported_result[0:4])
				Fib_n := binary.LittleEndian.Uint32(fib_exported_result[4:8])
				Fib_n_1 := binary.LittleEndian.Uint32(fib_exported_result[8:12])
				fmt.Printf("Exported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n\n", n, Fib_n, Fib_n_1)
			}
		}

		//pageProofs, _ := trie.GeneratePageProof(segments)
		//combinedSegmentAndPageProofs := append(segments, pageProofs...)

		// setup work results
		// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
		result := types.WorkResult{
			Service:     workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			GasRatio:    0,
			Result:      output,
		}
		//fmt.Printf("[node_da:executeWorkPackage] WorkResult %s output: %s\n", result.String(), output.String())
		results = append(results, result)
	}

	// treeRoot, err = n.EncodeAndDistributeSegmentData(combinedSegmentAndPageProofs, &wg)
	// TODO: need to figure out where distribution is happening

	// Step 2:  Now create a WorkReport with AvailabilitySpecification and RefinementContext
	spec, erasureMeta, bECChunks, sECChunksArray := n.NewAvailabilitySpecifier(packageHash, workPackage, segments)
	prerequisite_hash := common.HexToHash("0x")
	refinementContext := types.RefineContext{
		Anchor:           n.statedb.ParentHash,                      // TODO  common.HexToHash("0x123abc")
		StateRoot:        n.statedb.Block.Header.ParentStateRoot,    // TODO, common.HexToHash("0x")
		BeefyRoot:        common.HexToHash("0x"),                    // SKIP
		LookupAnchor:     n.statedb.ParentHash,                      // TODO
		LookupAnchorSlot: n.statedb.Block.Header.Slot,               //TODO: uint32(0)
		Prerequisite:     (*types.Prerequisite)(&prerequisite_hash), //common.HexToHash("0x"), // SKIP
	}

	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return
	}

	workReport := types.WorkReport{
		AvailabilitySpec: *spec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		CoreIndex:        core,
		//	Output:               result.Output,
		RefineContext: refinementContext,
		Results:       results,
	}

	n.StoreMeta(spec, erasureMeta, bECChunks, sECChunksArray)
	n.FakeDistributeChunks(erasureMeta, bECChunks, sECChunksArray)

	/*
		var bundleShards [][]byte
		bundleShards, err = erasurecoding.EncodeBundle(workPackage.Bytes(), types.TotalValidators)
		if err != nil {
			return
		}
		for shardIndex, bundleShard := range bundleShards {
			segmentShards := make([]byte, 0)
			justification := []byte{}
			err = n.store.StoreAuditDA(spec.ErasureRoot, uint16(shardIndex), bundleShard, segmentShards, justification)
			if err != nil {
				fmt.Printf("[executeWorkPackage:StoreAuditDA] Err %v\n", err)
			}
		}
	*/
	//workReport.Print()
	//work = n.MakeGuaranteeReport(workReport)
	//work.Sign(n.GetEd25519Secret())

	gc := workReport.Sign(n.GetEd25519Secret(), uint16(n.GetCurrValidatorIndex()))
	guarantee = types.Guarantee{
		Report:     workReport,
		Signatures: []types.GuaranteeCredential{gc},
	}

	return
}
