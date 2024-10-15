package node

import (
	"encoding/binary"
	"io"

	"bytes"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 129: State request

Request for a range of a block's posterior state.

A contiguous range of key/value pairs from the state trie should be returned, starting at the given start key and ending at or before the given end key.
The start and end keys are both "inclusive" but need not exist in the state trie. The returned key/value pairs should be sorted by key.

Additionally, a list of "boundary" nodes should be returned, covering the paths from the root to the given start key and to the last key/value pair included in the response.
The list should include only nodes on these paths, and should not include duplicate nodes.
If two nodes in the list have a parent-child relationship, the parent node must come first. Note that in the case where the given start key is not present in the state trie, the "path to the start key" should terminate either at a fork node with an all-zeroes hash in the branch that would be taken for the start key, or at a leaf node with a different key.

The total encoded length of the response should not exceed the given maximum size in bytes, unless the response contains only a single key/value pair. As such, the response may not cover the full requested range.

Note that the keys in the response are only 31 bytes, as the final key byte is ignored by the Merklization function.

Header Hash = [u8; 32]
Key = [u8; 31] (First 31 bytes of key only)
Maximum Size = u32
Boundary Node = As returned by B/L, defined in the State Merklization appendix of the GP
Value = len++[u8]

Node -> Node

--> Header Hash ++ Start Key ++ End Key ++ Maximum Size
--> FIN
<-- [Boundary Node]
<-- [Key ++ Value]
<-- FIN
*/

type JAMSNPStateRequest struct {
	HeaderHash  common.Hash `json:"headerHash"`
	StartKey    [31]byte    `json:"startKey"`
	EndKey      [31]byte    `json:"endKey"`
	MaximumSize uint32      `json:"maximumSize"`
}

// ToBytes serializes the JAMSNPStateRequest struct into a byte array
func (req *JAMSNPStateRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize HeaderHash (32 bytes for common.Hash)
	if _, err := buf.Write(req.HeaderHash[:]); err != nil {
		return nil, err
	}

	// Serialize StartKey (31 bytes)
	if _, err := buf.Write(req.StartKey[:]); err != nil {
		return nil, err
	}

	// Serialize EndKey (31 bytes)
	if _, err := buf.Write(req.EndKey[:]); err != nil {
		return nil, err
	}

	// Serialize MaximumSize (4 bytes)
	if err := binary.Write(buf, binary.BigEndian, req.MaximumSize); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPStateRequest struct
func (req *JAMSNPStateRequest) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize HeaderHash (32 bytes for common.Hash)
	if _, err := io.ReadFull(buf, req.HeaderHash[:]); err != nil {
		return err
	}

	// Deserialize StartKey (31 bytes)
	if _, err := io.ReadFull(buf, req.StartKey[:]); err != nil {
		return err
	}

	// Deserialize EndKey (31 bytes)
	if _, err := io.ReadFull(buf, req.EndKey[:]); err != nil {
		return err
	}

	// Deserialize MaximumSize (4 bytes)
	if err := binary.Read(buf, binary.BigEndian, &req.MaximumSize); err != nil {
		return err
	}

	return nil
}

type JAMSNPStateResponse struct {
	KeyValues []types.StateKeyValue `json:"boundary"`
}

func (p *Peer) SendStateRequest(headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (err error) {
	req := &JAMSNPStateRequest{
		HeaderHash:  headerHash,
		StartKey:    startKey,
		EndKey:      endKey,
		MaximumSize: maximumSize,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, err := p.openStream(CE129_StateRequest)
	// --> Header Hash ++ Start Key ++ End Key ++ Maximum Size
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}

	//<-- [Boundary Node]
	// TODO
	//<-- [Key ++ Value]
	// TODO

	return nil
}

func (n *Node) onStateRequest(stream quic.Stream, msg []byte) (err error) {
	var newReq JAMSNPStateRequest
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	boundarynodes, keyvalues, ok, err := n.GetState(newReq.HeaderHash, newReq.StartKey, newReq.EndKey, newReq.MaximumSize)
	if !ok {

	}
	//<-- [Boundary Node]
	err = sendQuicBytes(stream, common.ConcatenateByteSlices(boundarynodes))
	//<-- [Key ++ Value]
	kvbytes, err := keyvalues.ToBytes()
	err = sendQuicBytes(stream, kvbytes)

	// <-- FIN
	stream.Close()
	return
}
