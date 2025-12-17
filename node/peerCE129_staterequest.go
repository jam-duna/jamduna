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

const DefaultMaxStateSize = 10 * 1024 * 1024 // 50MB - large enough for full state responses

// FetchStateRange fetches a single page of state. Returns truncated=true if more data available.
func (p *Peer) FetchStateRange(ctx context.Context, headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (resp *JAMSNPStateResponse, truncated bool, err error) {
	log.Trace(log.Node, "FetchStateRange: sending request",
		"peer", p.Validator.Ed25519.ShortString(),
		"headerHash", headerHash.Hex(),
		"startKey", fmt.Sprintf("%x", startKey[:]),
		"endKey", fmt.Sprintf("%x", endKey[:]),
		"maxSize", maximumSize)

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
		return nil, false, fmt.Errorf("FetchStateRange: failed to open stream: %w", err)
	}

	if err = sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code); err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: failed to send request: %w", err)
	}

	log.Debug(log.Node, "FetchStateRange: request sent, closing write side",
		"peer", p.Validator.Ed25519.ShortString(),
		"reqBytesLen", len(reqBytes))

	stream.Close()

	log.Debug(log.Node, "FetchStateRange: waiting for response",
		"peer", p.Validator.Ed25519.ShortString())

	// <-- [Boundary Node]
	// <-- [Key ++ Value]
	parts, err := receiveMultiple(ctx, stream, 2, p.Validator.Ed25519.String(), code)
	if err != nil {
		log.Trace(log.Node, "FetchStateRange: failed to receive response",
			"peer", p.Validator.Ed25519.ShortString(),
			"headerHash", headerHash.Hex(),
			"err", err)
		return nil, false, fmt.Errorf("FetchStateRange: failed to receive response: %w", err)
	}

	boundaryBytes := parts[0]
	kvBytes := parts[1]

	resp = &JAMSNPStateResponse{Boundary: boundaryBytes}
	if err = resp.KeyValues.FromBytes(kvBytes); err != nil {
		return nil, false, fmt.Errorf("FetchStateRange: %w", err)
	}

	// Determine if response was truncated due to MaximumSize limit
	// The server truncates when response would exceed maximumSize (unless only 1 KV)
	// We consider it truncated if lastKey < endKey (more keys could exist)
	if len(resp.KeyValues.Items) > 0 {
		lastKey := resp.KeyValues.LastKey()
		cmpResult := compareKeys(*lastKey, endKey)
		truncated = cmpResult < 0
		log.Debug(log.Node, "FetchStateRange: truncation check",
			"lastKey", fmt.Sprintf("%x", (*lastKey)[:]),
			"endKey", fmt.Sprintf("%x", endKey[:]),
			"cmpResult", cmpResult,
			"truncated", truncated)
	}

	// Log first and last key if we have items
	var firstKeyStr, lastKeyStr string
	if len(resp.KeyValues.Items) > 0 {
		firstKeyStr = fmt.Sprintf("%x", resp.KeyValues.Items[0].Key[:])
		lastKeyStr = fmt.Sprintf("%x", resp.KeyValues.Items[len(resp.KeyValues.Items)-1].Key[:])
	}

	log.Trace(log.Node, "FetchStateRange: success",
		"peer", p.Validator.Ed25519.ShortString(),
		"headerHash", headerHash.Hex(),
		"startKey", fmt.Sprintf("%x", startKey[:]),
		"endKey", fmt.Sprintf("%x", endKey[:]),
		"boundaryLen", len(resp.Boundary),
		"numKVs", len(resp.KeyValues.Items),
		"firstKey", firstKeyStr,
		"lastKey", lastKeyStr,
		"truncated", truncated)

	return resp, truncated, nil
}

func (p *Peer) SendStateRequest(ctx context.Context, headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) error {
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
	code := uint8(CE129_StateRequest)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return err
	}
	// --> Header Hash ++ Start Key ++ End Key ++ Maximum Size
	err = sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code)
	if err != nil {
		return err
	}
	// --> FIN
	stream.Close()

	// <-- [Boundary Node]
	// <-- [Key ++ Value]
	parts, err := receiveMultiple(ctx, stream, 2, p.Validator.Ed25519.String(), code)
	if err != nil {
		return err
	}

	boundaryNode := parts[0]
	keyVal := parts[1]
	log.Info(log.Node, "SendStateRequest: received boundary node and key-value pairs",
		"peer", p.Validator.Ed25519.ShortString(),
		"boundaryNodeLength", len(boundaryNode), "keyValLength", len(keyVal))
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
// When used with narrow key ranges (like single first-byte prefixes), empty responses
// simply mean no keys exist in that range - we don't jump ahead.
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
			// Empty response - no keys in range [currentStart, endKey]
			// Since we use narrow ranges (per first-byte prefix), this just means no keys exist here
			log.Debug(log.Node, "FetchStatePaginated: empty response, range complete",
				"startKey", fmt.Sprintf("%x", startKey[:]),
				"endKey", fmt.Sprintf("%x", endKey[:]))
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

		log.Debug(log.Node, "FetchStatePaginated: advancing to next page",
			"lastKey", fmt.Sprintf("%x", (*lastKey)[:]),
			"nextStart", fmt.Sprintf("%x", nextStart[:]))

		if compareKeys(nextStart, currentStart) <= 0 {
			return totalKVs, fmt.Errorf("FetchStatePaginated: no progress at key %x", currentStart[:])
		}

		currentStart = nextStart
	}

	return totalKVs, nil
}

func (n *NodeContent) onStateRequest(ctx context.Context, stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()

	var req JAMSNPStateRequest
	if err = req.FromBytes(msg); err != nil {
		log.Warn(log.Node, "onStateRequest: FromBytes failed", "err", err)
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onStateRequest: %w", err)
	}

	log.Debug(log.Node, "onStateRequest: received request",
		"headerHash", req.HeaderHash.Hex(),
		"startKey", fmt.Sprintf("%x", req.StartKey[:4]),
		"endKey", fmt.Sprintf("%x", req.EndKey[:4]),
		"maxSize", req.MaximumSize)

	boundarynodes, keyvalues, ok, err := n.GetState(req.HeaderHash, req.StartKey, req.EndKey, req.MaximumSize)
	if err != nil {
		log.Warn(log.Node, "onStateRequest: GetState error", "headerHash", req.HeaderHash.Hex(), "err", err)
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onStateRequest: GetState error: %w", err)
	}
	if !ok {
		log.Warn(log.Node, "onStateRequest: state not found", "headerHash", req.HeaderHash.Hex())
		stream.CancelWrite(ErrKeyNotFound)
		return nil
	}

	log.Debug(log.Node, "onStateRequest: GetState success",
		"headerHash", req.HeaderHash.Hex(),
		"numBoundaryNodes", len(boundarynodes),
		"numKeyValues", len(keyvalues.Items))

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

	log.Info(log.Node, "onStateRequest: success",
		"headerHash", req.HeaderHash.Hex(),
		"numKeyValues", len(keyvalues.Items),
		"kvBytesLen", len(kvbytes))

	return nil
}
