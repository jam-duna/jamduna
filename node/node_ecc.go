package node

import (
	//"bytes"
	//"context"

	//"encoding/binary"
	"fmt"

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

func (n *Node) ReadRawKV(key []byte) ([]byte, bool, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, false, err
	}
	val, ok, err := store.ReadRawKV([]byte(key))
	if err != nil {
		return []byte{}, false, fmt.Errorf("ReadRawKV Err:%v\n", err)
	} else if !ok {
		return []byte{}, false, fmt.Errorf("ReadRawKV K=%v not found\n", key)
	}
	//fmt.Printf("Read key %s | val %x\n", key, val)
	return val, true, nil
}

func (n *Node) ReadKVByte(key []byte) ([]byte, bool, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, false, err
	}
	val, ok, err := store.ReadRawKV(key)
	if err != nil {
		return []byte{}, false, fmt.Errorf("ReadKVByte Err:%v\n", err)
	} else if !ok {
		return []byte{}, false, fmt.Errorf("ReadKVByte K=%v not found\n", key)
	}
	//fmt.Printf("Read key %s | val %x\n", key, val)
	return val, true, nil
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

func (n *Node) packChunks(segments [][][]byte, segmentRoots [][]byte, blobMeta []byte) ([]types.DistributeECChunk, error) {
	// TODO: Pack the chunks into DistributeECChunk objects
	chunks := make([]types.DistributeECChunk, 0)
	for i := range segments {
		for j := range segments[i] {
			// Pack the DistributeECChunk object
			// TODO: Modify this as needed.
			chunk := types.DistributeECChunk{
				SegmentRoot: segmentRoots[i],
				Data:        segments[i][j],
				BlobMeta:    blobMeta,
			}
			chunks = append(chunks, chunk)
		}
	}
	// Return the DistributeECChunk objects
	return chunks, nil
}

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
	ecChunks, err = n.packChunks(erasureCodingSegments, segmentRoots, segment_meta)
	if err != nil {
		return ecChunks, err
	}

	return ecChunks, nil
}

// Compute ecChunks without distribution for arbitrary dara
func (n *Node) BuildArbitraryDataChunks(chunks [][][]byte, blobLen int) (ecChunks []types.DistributeECChunk, err error) {
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

	// Pack the chunks into DistributeECChunk objects
	ecChunks, err = n.packChunks(chunks, chunksRoots, blob_meta)
	if err != nil {
		return ecChunks, err
	}

	if debugDA {
		fmt.Printf("allHash encode %x\n", chunksRoots)
	}
	return ecChunks, nil
}
