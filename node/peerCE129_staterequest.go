package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
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
	if err := binary.Write(buf, binary.LittleEndian, req.MaximumSize); err != nil {
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
	if err := binary.Read(buf, binary.LittleEndian, &req.MaximumSize); err != nil {
		return err
	}

	return nil
}

type JAMSNPStateResponse struct {
	Boundary  []byte                  `json:"boundary"`
	KeyValues types.StateKeyValueList `json:"keyValues"`
}

const DefaultMaxStateSize = 5 * 1024 * 1024

// FetchStateRange fetches a single page of state. Returns truncated=true if more data available.
func (p *Peer) FetchStateRange(ctx context.Context, headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (resp *JAMSNPStateResponse, truncated bool, err error) {
	req := &JAMSNPStateRequest{
		HeaderHash:  headerHash,
		StartKey:    startKey,
		EndKey:      endKey,
		MaximumSize: maximumSize,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: %w", err)
	}

	code := uint8(CE129_StateRequest)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: %w", err)
	}

	if err = sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code); err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: failed to send request: %w", err)
	}
	stream.Close()

	// <-- [Boundary Node]
	boundaryBytes, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: failed to receive boundary nodes: %w", err)
	}

	// <-- [Key ++ Value]
	kvBytes, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: failed to receive key-values: %w", err)
	}

	resp = &JAMSNPStateResponse{Boundary: boundaryBytes}
	if err = resp.KeyValues.FromBytes(kvBytes); err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: %w", err)
	}

	if len(resp.KeyValues.Items) > 0 {
		lastKey := resp.KeyValues.LastKey()
		truncated = compareKeys(*lastKey, endKey) < 0
	}

	log.Debug(log.Node, "FetchStateRange",
		"startKey", fmt.Sprintf("%x", startKey[:4]),
		"endKey", fmt.Sprintf("%x", endKey[:4]),
		"numKVs", len(resp.KeyValues.Items),
		"truncated", truncated)

	return resp, truncated, nil
}

func (p *Peer) SendStateRequest(ctx context.Context, headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) error {
	resp, _, err := p.FetchStateRange(ctx, headerHash, startKey, endKey, maximumSize)
	if err != nil {
		return err
	}
	log.Info(log.Node, "SendStateRequest",
		"boundaryLength", len(resp.Boundary),
		"numKeyValues", len(resp.KeyValues.Items))
	return nil
}

func incrementKey(key [31]byte) ([31]byte, bool) {
	result := key
	for i := 30; i >= 0; i-- {
		if result[i] < 0xFF {
			result[i]++
			return result, true
		}
		result[i] = 0
	}
	return result, false
}

func compareKeys(a, b [31]byte) int {
	return bytes.Compare(a[:], b[:])
}

// StatePageHandler is called for each page. Return false to stop.
type StatePageHandler func(boundary []byte, keyValues types.StateKeyValueList) bool

// FetchStatePaginated fetches state with automatic pagination.
func (p *Peer) FetchStatePaginated(ctx context.Context, headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32, handler StatePageHandler) (totalKVs int, err error) {
	currentStart := startKey

	for {
		if ctx.Err() != nil {
			return totalKVs, ctx.Err()
		}

		resp, truncated, err := p.FetchStateRange(ctx, headerHash, currentStart, endKey, maximumSize)
		if err != nil {
			return totalKVs, err
		}

		if len(resp.KeyValues.Items) == 0 {
			break
		}

		totalKVs += len(resp.KeyValues.Items)

		if handler != nil && !handler(resp.Boundary, resp.KeyValues) {
			break
		}

		if !truncated {
			break
		}

		lastKey := resp.KeyValues.LastKey()
		nextStart, ok := incrementKey(*lastKey)
		if !ok {
			break
		}

		if compareKeys(nextStart, currentStart) <= 0 {
			return totalKVs, fmt.Errorf("FetchStatePaginated: no progress at key %x", currentStart[:4])
		}

		currentStart = nextStart
	}

	return totalKVs, nil
}

func (n *NodeContent) onStateRequest(ctx context.Context, stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()

	var req JAMSNPStateRequest
	if err = req.FromBytes(msg); err != nil {
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onStateRequest: %w", err)
	}

	boundarynodes, keyvalues, ok, err := n.GetState(req.HeaderHash, req.StartKey, req.EndKey, req.MaximumSize)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onStateRequest: GetState error: %w", err)
	}
	if !ok {
		log.Warn(log.Node, "onStateRequest: state not found", "headerHash", req.HeaderHash)
		stream.CancelWrite(ErrKeyNotFound)
		return nil
	}

	// <-- [Boundary Node]
	if err = sendQuicBytes(ctx, stream, common.ConcatenateByteSlices(boundarynodes), n.GetEd25519Key().String(), CE129_StateRequest); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onStateRequest: failed to send boundarynodes: %w", err)
	}

	select {
	case <-ctx.Done():
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onStateRequest: context cancelled before sending keyvalues: %w", ctx.Err())
	default:
	}

	// <-- [Key ++ Value]
	kvbytes, err := keyvalues.ToBytes()
	if err != nil {
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onStateRequest: failed to encode keyvalues: %w", err)
	}
	if err = sendQuicBytes(ctx, stream, kvbytes, n.GetEd25519Key().String(), CE129_StateRequest); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onStateRequest: failed to send keyvalues: %w", err)
	}

	return nil
}
