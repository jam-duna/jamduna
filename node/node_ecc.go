package node

import (
	//"bytes"
	//"context"

	//"encoding/binary"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"

	//"time"

	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
)

func encodeBlobMeta(segmentRootsFlattened []byte) []byte {
	// Need hashes here...
	extraByteLen := 4
	extraBytes := make([]byte, extraByteLen)
	metadataBytes := append(extraBytes, segmentRootsFlattened...)
	return metadataBytes
}

// TODO: length shouldn't be stored
func decodeBlobMeta(value []byte) ([]byte, error) {

	extraByteLen := 4
	//extraBytes := value[:extraByteLen]

	// The rest of the value is the segment roots
	segmentRootsFlattened := value[extraByteLen:]

	return segmentRootsFlattened, nil
}

func (n *Node) GetStorage() (*storage.StateDBStorage, error) {
	if n == nil {
		return nil, fmt.Errorf("Node Not initiated")
	}
	if n.store == nil {
		return nil, fmt.Errorf("Node Store Not initiated")
	}
	return n.store, nil
}

func (n *Node) FakeReadKV(key common.Hash) ([]byte, error) {
	return n.ReadKV(key)
}

func (n *Node) FakeWriteKV(key common.Hash, val []byte) error {
	//fmt.Printf("FAKE local write detected! key=%v\n", key)
	return n.WriteKV(key, val)
}

func (n *Node) FakeWriteRawKV(key string, val []byte) error {
	//fmt.Printf("FAKE local write detected! key=%v\n", key)
	return n.WriteRawKV(key, val)
}

func (n *Node) ReadKV(key common.Hash) ([]byte, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, err
	}
	val, err := store.ReadKV(key)
	if err != nil {
		return []byte{}, fmt.Errorf("ReadKV K=%v not found\n", key)
	}
	return val, nil
}

func (n *Node) WriteKV(key common.Hash, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}
	err = store.WriteKV(key, val)
	if err != nil {
		return fmt.Errorf("WriteKV K=%v|V=%x err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) WriteRawKV(key string, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}
	err = store.WriteRawKV([]byte(key), val)
	if err != nil {
		return fmt.Errorf("ReadRawKV K=%v|V=%x Err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) WriteKVByte(key []byte, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}
	err = store.WriteRawKV(key, val)
	if err != nil {
		return fmt.Errorf("ReadRawKV K=%v|V=%x Err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) ReadRawKV(key []byte) ([]byte, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, err
	}
	val, err := store.ReadRawKV([]byte(key))
	if err != nil {
		return []byte{}, fmt.Errorf("ReadRawKV K=%v not found\n", key)
	}
	//fmt.Printf("Read key %s | val %x\n", key, val)
	return val, nil
}

func (n *Node) ReadKVByte(key []byte) ([]byte, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, err
	}
	val, err := store.ReadRawKV(key)
	if err != nil {
		return []byte{}, fmt.Errorf("ReadRawKV K=%v not found\n", key)
	}
	//fmt.Printf("Read key %s | val %x\n", key, val)
	return val, nil
}

func (n *Node) encode(data []byte, isFixed bool, data_len int) ([][][]byte, error) {
	// Load the file and encode them into segments of chunks. (3D byte array)
	c_base := 6
	if !isFixed {
		// get the veriable c_base by computing roundup(|b|/Wc)
		c_base = types.ComputeC_Base(int(data_len))
	}
	// encode the data
	encoded_data, err := erasurecoding.Encode(data, c_base)
	if err != nil {
		return nil, err
	}
	// return the encoded data
	return encoded_data, nil
}

func (n *Node) decode(data [][][]byte, isFixed bool, data_len int) ([]byte, error) {
	// Load the file and encode them into segments of chunks. (3D byte array)
	c_base := 6
	if !isFixed {
		// get the veriable c_base by computing roundup(|b|/Wc)
		c_base = types.ComputeC_Base(int(data_len))
	}
	// encode the data
	encoded_data, err := erasurecoding.Decode(data, c_base)
	if err != nil {
		return nil, err
	}
	// return the encoded data
	return encoded_data, nil

}

//	func (n *Node) packChunks(segments [][][]byte, segmentRoots [][]byte) ([]types.DistributeECChunk, error) {
//		// TODO: Pack the chunks into DistributeECChunk objects
//		chunks := make([]types.DistributeECChunk, 0)
//		for i := range segments {
//			for j := range segments[i] {
//				// Pack the DistributeECChunk object
//				// TODO: Modify this as needed.
//				chunk := types.DistributeECChunk{
//					SegmentRoot: segmentRoots[i],
//					Data:        segments[i][j],
//				}
//				chunks = append(chunks, chunk)
//			}
//		}
//		// Return the DistributeECChunk objects
//		return chunks, nil
//	}

func (n *Node) packChunks(segments [][][]byte, segmentRoots [][]byte, blobHash common.Hash, blobMeta []byte) ([]types.DistributeECChunk, error) {
	// TODO: Pack the chunks into DistributeECChunk objects
	chunks := make([]types.DistributeECChunk, 0)
	for i := range segments {
		for j := range segments[i] {
			// Pack the DistributeECChunk object
			// TODO: Modify this as needed.
			chunk := types.DistributeECChunk{
				SegmentRoot: segmentRoots[i],
				Data:        segments[i][j],
				RootHash:    blobHash,
				BlobMeta:    blobMeta,
			}
			chunks = append(chunks, chunk)
		}
	}
	// Return the DistributeECChunk objects
	return chunks, nil
}

// -----Custom methods for tiny QUIC EC experiment-----
func (n *Node) processDistributeECChunk(chunk types.DistributeECChunk) error {
	// Serialize the chunk
	serialized, err := types.Encode(types.ECChunkResponse{
		SegmentRoot: chunk.SegmentRoot,
		Data:        chunk.Data,
	})
	if err != nil {
		return err
	}

	key := fmt.Sprintf("DA-%v", common.Hash(chunk.SegmentRoot))
	fmt.Printf("ECChunkReceive Store key %s | val %x\n", key, serialized)

	// Save the chunk to the local storage
	err = n.WriteKVByte([]byte(key), serialized)
	if err != nil {
		return err
	}
	err = n.store.WriteKV(common.Hash(chunk.RootHash), chunk.BlobMeta)
	if err != nil {
		return err
	}

	fmt.Printf(" -- [N%d] saved DistributeECChunk common.Hash(chunk.RootHash)=%s\nchunk.BlobMeta=%x\nchunk.SegmentRoot=%s\nchunk.Data=%x\n", n.id, common.Hash(chunk.RootHash), chunk.BlobMeta, common.Hash(chunk.SegmentRoot), chunk.Data)
	return nil
}

func (n *Node) processECChunkQuery(ecChunkQuery types.ECChunkQuery) (types.ECChunkResponse, error) {
	key := fmt.Sprintf("DA-%v", ecChunkQuery.SegmentRoot)
	// fmt.Printf("[N%v] processECChunkQuery key=%s\n", n.id, key)
	data, err := n.ReadKVByte([]byte(key))
	fmt.Printf("ECChunkQuery Fetch key %s | val %x\n", key, data)
	if err != nil {
		return types.ECChunkResponse{}, err
	}
	// Deserialize the chunk
	var chunk types.ECChunkResponse
	// err = json.Unmarshal(data, &chunk)
	decodedData, _, err := types.Decode(data, reflect.TypeOf(types.ECChunkResponse{}))
	if err != nil {
		return types.ECChunkResponse{}, err
	}
	chunk = decodedData.(types.ECChunkResponse)

	if err != nil {
		return types.ECChunkResponse{}, err
	}
	return chunk, nil
}

func (n *Node) DistributeExportedEcChunkArray(ecChunksArr [][]types.DistributeECChunk) error {
	for _, ecChunks := range ecChunksArr {
		err := n.DistributeEcChunks(ecChunks)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) DistributeEcChunks(ecChunks []types.DistributeECChunk) error {
/*
	numNodes := types.TotalValidators
	peerIdentifiers := make([]string, len(ecChunks))
	requestObjs := make([]interface{}, len(ecChunks))

	for i, ecChunk := range ecChunks {
		peerIdx := uint32(i % numNodes)
		peerIdentifier, err := n.getPeerByIndex(peerIdx)
		if err != nil {
			return err
		}
		peerIdentifiers[i] = peerIdentifier
		requestObjs[i] = ecChunk
	}
	responses, err := n.makeRequests(peerIdentifiers, requestObjs, types.TotalValidators, types.QuicIndividualTimeout,  types.QuicOverallTimeout)
	if err != nil {
		fmt.Printf("[N%v] DistributeEcChunks MakeRequests Errors %v\n", n.id, err)
		return err
	}

	fmt.Printf("DistributeEcChunks resp=%v\n", len(responses))
*/
	return nil
}

// was Distribute_SegmentData
func (n *Node) BuildExportedSegmentChunks(erasureCodingSegments [][][]byte, segmentRoots [][]byte) (ecChunks []types.DistributeECChunk, err error) {

	// Generate the blob hash by hashing the original data
	blobTree := trie.NewCDMerkleTree(segmentRoots)
	segmentsECRoot := blobTree.Root()

	// Flatten the segment roots
	segmentRootsFlattened := make([]byte, 0)
	for i := range segmentRoots {
		segmentRootsFlattened = append(segmentRootsFlattened, segmentRoots[i]...)
	}

	// Store the blob meta data, and return an error if it fails
	segment_meta := encodeBlobMeta(segmentRootsFlattened)
	err = n.store.WriteKV(common.Hash(segmentsECRoot), segment_meta)
	if err != nil {
		return ecChunks, err
	}

	// Pack the chunks into DistributeECChunk objects
	ecChunks, err = n.packChunks(erasureCodingSegments, segmentRoots, common.Hash(segmentsECRoot), segment_meta)
	if err != nil {
		return ecChunks, err
	}

	//n.DistributeEcChunks(ecChunks)
	return ecChunks, nil
}

// Compute ecChunks without distribution for arbitrary dara
// Renamed from DistributeArbitraryData -- to remove distribution
func (n *Node) BuildArbitraryDataChunks(chunks [][][]byte, blobHash common.Hash, blobLen int) (ecChunks []types.DistributeECChunk, err error) {

	// if len(segments) != 1 {
	// 	panic("Expected only one segment")
	// }
	// fmt.Printf("Number of segments: %d for data size %d\n", len(segments), blobLen)

	// Build segment roots
	chunksRoots := make([][]byte, 0)
	for i := range chunks {
		leaves := chunks[i]
		tree := trie.NewCDMerkleTree(leaves)
		chunksRoots = append(chunksRoots, tree.Root())
	}

	segmentRootsFlattened := make([]byte, 0)
	for i := range chunksRoots {
		segmentRootsFlattened = append(segmentRootsFlattened, chunksRoots[i]...)
	}
	/// storer needs to know the size of original byte in order to eliminate any segment padding
	blob_meta := encodeBlobMeta(segmentRootsFlattened)
	// n.store.WriteKV(blobHash, blob_meta)

	// Pack the chunks into DistributeECChunk objects
	ecChunks, err = n.packChunks(chunks, chunksRoots, blobHash, blob_meta)
	if err != nil {
		return ecChunks, err
	}

	//n.DistributeEcChunks(ecChunks)
	fmt.Printf("allHash encode %x\n", chunksRoots)
	return ecChunks, nil
}

/*
func (n *Node) FetchAndReconstructSegmentData(treeRoot common.Hash, segmentIndex uint32) ([]byte, []byte, error) {
	var outputData [][]byte
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	segmentsECRoots, err := n.store.ReadKV(treeRoot)
	if err != nil {
		return nil, nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	// segmentRoots, pageProohRoots := splitHashes(allHash)

	for _, segmentRoot := range allHash {
		segmentMeta, err := n.store.ReadKV(segmentRoot)
		if err != nil {
			return nil, nil, err
		}
		segmentRootsFlattened, _ := decodeBlobMeta(segmentMeta)
		erasurCodingSegmentRoots := make([][]byte, 0)
		fmt.Printf("len(segmentRootsFlattened): %d\n", len(segmentRootsFlattened))
		for i := 0; i < len(segmentRootsFlattened); i += 32 {
			erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
		}
		// Fetch the chunks from peers
		decoderInputSegments := make([][][]byte, 0)
		for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
			_ = i
			ecChunkResponses := make([]types.ECChunkResponse, 0)
			fetchedChunks := 0
			for j := 0; j < numNodes; j++ {
				peerIdentifier, err := n.getPeerByIndex(uint32(j))
				if err != nil {
					return nil, nil, err
				}
				ecChunkQuery := types.ECChunkQuery{
					SegmentRoot: common.Hash(erasurCodingSegmentRoot),
				}
				//response, err := n.makeRequest(peerIdentifier, ecChunkQuery, types.QuicIndividualTimeout)
				var ecChunkResponse types.ECChunkResponse


				fetchedChunks++
				if fetchedChunks >= K {
					break
				}
			}

			// debug
			fmt.Printf("Fetched %d chunks\n", fetchedChunks)
			for j := 0; j < fetchedChunks; j++ {
				fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
			}

			// build decoder input
			// _ = decoderInputSegments
			decoderInputSegment := make([][]byte, 0)
			for j := 0; j < N; j++ {
				if j >= len(ecChunkResponses) {
					decoderInputSegment = append(decoderInputSegment, nil)
					continue
				}
				if len(ecChunkResponses[j].Data) == 0 {
					decoderInputSegment = append(decoderInputSegment, nil)
					continue
				}
				decoderInputSegment = append(decoderInputSegment, ecChunkResponses[j].Data)
			}
			decoderInputSegments = append(decoderInputSegments, decoderInputSegment)
		}

		// Reconstruct the data
		reconstructedData, err := n.decode(decoderInputSegments, true, 24)
		if err != nil {
			return nil, nil, err
		}

		outputData = append(outputData, reconstructedData)
	}
	segmentData, pageProofData := SplitDataIntoSegmentAndPageProofByIndex(outputData, segmentIndex)
	return segmentData, pageProofData, nil
}
*/
/*
func (n *Node) FetchAndReconstructAllSegmentsData(treeRoot common.Hash) ([][]byte, [][]byte, []common.Hash, error) {
	var outputData [][]byte
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	segmentsECRoots, err := n.store.ReadKV(treeRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	fmt.Printf("FetchAndReconstructAllSegmentsData root:%v, allHash=%v\n", treeRoot, allHash)
	// segmentRoots, pageProohRoots := splitHashes(allHash)
	segmentRoots, _ := splitHashes(allHash)

	for _, segmentRoot := range allHash {
		segmentMeta, err := n.store.ReadKV(segmentRoot)
		if err != nil {
			return nil, nil, nil, err
		}
		segmentRootsFlattened, _ := decodeBlobMeta(segmentMeta)
		erasurCodingSegmentRoots := make([][]byte, 0)
		fmt.Printf("len(segmentRootsFlattened): %d\n", len(segmentRootsFlattened))
		for i := 0; i < len(segmentRootsFlattened); i += 32 {
			erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
		}
		// Fetch the chunks from peers
		decoderInputSegments := make([][][]byte, 0)
		for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
			_ = i
			ecChunkResponses := make([]types.ECChunkResponse, 0)
			fetchedChunks := 0
			for j := 0; j < numNodes; j++ {
				peerIdentifier, err := n.getPeerByIndex(uint32(j))
				if err != nil {
					return nil, nil, nil, err
				}
				ecChunkQuery := types.ECChunkQuery{
					SegmentRoot: common.Hash(erasurCodingSegmentRoot),
				}
				//response, err := n.makeRequest(peerIdentifier, ecChunkQuery, types.QuicIndividualTimeout)
				// fmt.Printf("[DEBUG] Received response: %s\n", response)
				if err != nil {
					fmt.Printf("Failed to make request from N%d to N%d (%s): %v\n", n.id, j, peerIdentifier, err)
					ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
				}

				var ecChunkResponse types.ECChunkResponse
				if response == nil {
					ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
				} else {
					decodedResponse, _, err := types.Decode(response, reflect.TypeOf(types.ECChunkResponse{}))
					if err != nil {
						return nil, nil, nil, err
					}
					ecChunkResponse = decodedResponse.(types.ECChunkResponse)
					if err != nil {
						return nil, nil, nil, err
					}
					ecChunkResponses = append(ecChunkResponses, ecChunkResponse)
				}

				fetchedChunks++
				if fetchedChunks >= K {
					break
				}
			}

			// debug
			fmt.Printf("Fetched %d chunks\n", fetchedChunks)
			for j := 0; j < fetchedChunks; j++ {
				fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
			}

			// build decoder input
			// _ = decoderInputSegments
			decoderInputSegment := make([][]byte, 0)
			for j := 0; j < N; j++ {
				if j >= len(ecChunkResponses) {
					decoderInputSegment = append(decoderInputSegment, nil)
					continue
				}
				if len(ecChunkResponses[j].Data) == 0 {
					decoderInputSegment = append(decoderInputSegment, nil)
					continue
				}
				decoderInputSegment = append(decoderInputSegment, ecChunkResponses[j].Data)
			}
			decoderInputSegments = append(decoderInputSegments, decoderInputSegment)
		}

		// Reconstruct the data
		reconstructedData, err := n.decode(decoderInputSegments, true, 24)
		if err != nil {
			return nil, nil, nil, err
		}

		outputData = append(outputData, reconstructedData)
	}
	segmentData, pageProofData := SplitDataIntoSegmentAndPageProof(outputData)

	return segmentData, pageProofData, segmentRoots, nil
}
func (n *Node) FetchAndReconstructSpecificSegmentData(segmentRoot common.Hash) ([]byte, error) {
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	segmentMeta, err := n.store.ReadKV(segmentRoot)
	if err != nil {
		return nil, err
	}
	segmentRootsFlattened, _ := decodeBlobMeta(segmentMeta)
	erasurCodingSegmentRoots := make([][]byte, 0)
	fmt.Printf("len(segmentRootsFlattened): %d\n", len(segmentRootsFlattened))
	for i := 0; i < len(segmentRootsFlattened); i += 32 {
		erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
	}
	// Fetch the chunks from peers
	decoderInputSegments := make([][][]byte, 0)
	for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
		_ = i
		ecChunkResponses := make([]types.ECChunkResponse, 0)
		fetchedChunks := 0
		for j := 0; j < numNodes; j++ {
			peerIdentifier, err := n.getPeerByIndex(uint32(j))
			if err != nil {
				return nil, err
			}
			ecChunkQuery := types.ECChunkQuery{
				SegmentRoot: common.Hash(erasurCodingSegmentRoot),
			}
			//response, err := n.makeRequest(peerIdentifier, ecChunkQuery, types.QuicIndividualTimeout)
			// fmt.Printf("[DEBUG] Received response: %s\n", response)
			if err != nil {

				ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			}
			var ecChunkResponse types.ECChunkResponse

			decodedResponse, _, err := types.Decode(response, reflect.TypeOf(types.ECChunkResponse{}))
			if err != nil {
				return nil, err
			}
			ecChunkResponse = decodedResponse.(types.ECChunkResponse)

			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

			fetchedChunks++
			if fetchedChunks >= K {
				break
			}
		}

		// debug
		fmt.Printf("Fetched %d chunks\n", fetchedChunks)
		for j := 0; j < fetchedChunks; j++ {
			fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
		}

		// build decoder input
		// _ = decoderInputSegments
		decoderInputSegment := make([][]byte, 0)
		for j := 0; j < N; j++ {
			if j >= len(ecChunkResponses) {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			if len(ecChunkResponses[j].Data) == 0 {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			decoderInputSegment = append(decoderInputSegment, ecChunkResponses[j].Data)
		}
		decoderInputSegments = append(decoderInputSegments, decoderInputSegment)
	}

	// Reconstruct the data
	reconstructedData, err := n.decode(decoderInputSegments, true, 24)
	if err != nil {
		return nil, err
	}

	return reconstructedData, nil
}

func (n *Node) FetchAndReconstructArbitraryData(blobHash common.Hash, blobLen int) ([]byte, error) {
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d for data size %d\n", K, N, blobLen)
	fmt.Printf("blobHash %s\n", blobHash)
	blob_meta, err := n.store.ReadKV(blobHash)
	if err != nil {
		fmt.Printf("Failed to read blob meta data: %v", err)
		return nil, err
	}
	segmentRootsFlattened, _ := decodeBlobMeta(blob_meta)
	segmentRoots := make([][]byte, 0)
	for i := 0; i < len(segmentRootsFlattened); i += 32 {
		segmentRoots = append(segmentRoots, segmentRootsFlattened[i:i+32])
	}

	// Fetch the chunks from peers
	decoderInputSegments := make([][][]byte, 0)
	for i, segmentRoot := range segmentRoots {
		_ = i
		ecChunkResponses := make([]types.ECChunkResponse, 0)
		fetchedChunks := 0
		for j := 0; j < numNodes; j++ {
			peerIdentifier, err := n.getPeerByIndex(uint32(j))
			if err != nil {
				return nil, err
			}
			ecChunkQuery := types.ECChunkQuery{
				SegmentRoot: common.Hash(segmentRoot),
			}
			//response, err := n.makeRequest(peerIdentifier, ecChunkQuery, types.QuicIndividualTimeout)
			//if err != nil {
			//	fmt.Printf("Failed to make request from N%d to N%d (%s): %v", n.id, j, peerIdentifier, err)
			//	ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			//}
			var ecChunkResponse types.ECChunkResponse
			fmt.Printf("types.Decode response %v\n", response)
			decodedResponse, _, err := types.Decode(response, reflect.TypeOf(types.ECChunkResponse{}))
			if err != nil {
				return nil, err
			}
			ecChunkResponse = decodedResponse.(types.ECChunkResponse)

			if err != nil {
				return nil, err
			}
			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)
			fmt.Printf("fetchedChunks %d\n", fetchedChunks)
			fetchedChunks++
			if fetchedChunks >= K {
				break
			}
		}

		// debug
		fmt.Printf("Fetched %d chunks\n", fetchedChunks)
		for j := 0; j < fetchedChunks; j++ {
			fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
		}

		// build decoder input
		_ = decoderInputSegments
		decoderInputSegment := make([][]byte, 0)
		for j := 0; j < N; j++ {
			if j >= len(ecChunkResponses) {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			if len(ecChunkResponses[j].Data) == 0 {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			decoderInputSegment = append(decoderInputSegment, ecChunkResponses[j].Data)
		}
		decoderInputSegments = append(decoderInputSegments, decoderInputSegment)
	}

	// Reconstruct the data
	reconstructedData, err := n.decode(decoderInputSegments, false, int(blobLen))
	if err != nil {
		return nil, err
	}

	// Trim any padding using the original length
	if len(reconstructedData) > int(blobLen) {
		reconstructedData = reconstructedData[:blobLen]
	}

	return reconstructedData, nil
}
*/
