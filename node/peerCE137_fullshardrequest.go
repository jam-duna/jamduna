package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/quic-go/quic-go"
)

/*
CE 137: Full Shard distribution
This protocol should be used by assurers to request their EC shards from the guarantors of a work-report.
The response should include a work-package bundle shard and a sequence of exported/proof segment shards.
The response should also include a "justification", proving the correctness of the shards.

The justification should be the co-path of the path from the erasure root to the shards

Erasure Root = [u8; 32]
Shard Index = u16
Bundle Shard = [u8]
Segment Shard = [u8; 12]
Hash = [u8; 32]
Justification = [Hash OR (Hash ++ Hash)]

Assurer -> Guarantor

--> Erasure Root ++ Shard Index
--> FIN
<-- Bundle Shard
<-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
<-- Justification
<-- FIN
*/

type JAMSNPShardRequest struct {
	ErasureRoot common.Hash `json:"erasure_root"`
	ShardIndex  uint16      `json:"shard_index"`
}

// ToBytes serializes the JAMSNPShardRequest struct into a byte array
func (req *JAMSNPShardRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ErasureRoot (32 bytes for common.Hash)
	if _, err := buf.Write(req.ErasureRoot[:]); err != nil {
		return nil, err
	}

	// Serialize ShardIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.ShardIndex); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPShardRequest struct
func (req *JAMSNPShardRequest) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ErasureRoot (32 bytes for common.Hash)
	if _, err := io.ReadFull(buf, req.ErasureRoot[:]); err != nil {
		return err
	}

	// Deserialize ShardIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.ShardIndex); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendFullShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
) (bundleShard []byte, concatSegmentShards []byte, encodedPath []byte, err error) {

	if ctx.Err() != nil {
		return nil, nil, nil, ctx.Err()
	}

	code := uint8(CE137_FullShardRequest)

	// Telemetry: Sending shard request (event 120)
	eventID := p.node.telemetryClient.GetEventID()
	p.node.telemetryClient.SendingShardRequest(p.PeerKey(), erasureRoot, shardIndex)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Shard request failed (event 122)
		p.node.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return nil, nil, nil, fmt.Errorf("openStream[CE137]: %w", err)
	}

	// --> Erasure Root ++ Shard Index
	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		stream.Close()
		// Telemetry: Shard request failed (event 122)
		p.node.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return nil, nil, nil, fmt.Errorf("ToBytes[ShardRequest]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code); err != nil {
		// Telemetry: Shard request failed (event 122)
		p.node.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return nil, nil, nil, fmt.Errorf("sendQuicBytes[CE137]: %w", err)
	}
	//--> FIN
	stream.Close()

	// Telemetry: Shard request sent (event 123)
	p.node.telemetryClient.ShardRequestSent(eventID)

	// <-- Bundle Shard
	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	// <-- Justification
	parts, err := receiveMultiple(ctx, stream, 3, p.Validator.Ed25519.String(), code)
	if err != nil {
		// Telemetry: Shard request failed (event 122)
		p.node.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return nil, nil, nil, fmt.Errorf("receiveMultiple[CE137]: %w", err)
	}

	// Telemetry: Shards transferred (event 125)
	p.node.telemetryClient.ShardsTransferred(eventID)

	return parts[0], parts[1], parts[2], nil
}

// guarantor receives CE137 request from assurer by erasureRoot and shard index
func (n *Node) onFullShardRequest(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onFullShardRequest: peer not found for key %s", peerKey)
	}

	// Telemetry: Receiving shard request (event 121)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingShardRequest(PubkeyBytes(peer.Validator.Ed25519.String()))

	var req JAMSNPShardRequest
	if err := req.FromBytes(msg); err != nil {
		log.Error(log.DA, "onFullShardRequest: failed to deserialize", "err", err)
		// Telemetry: Shard request failed (event 122)
		n.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onFullShardRequest: decode failed: %w", err)
	}

	// Telemetry: Shard request received (event 124)
	n.telemetryClient.ShardRequestReceived(eventID, req.ErasureRoot, req.ShardIndex)

	bundleShard, exportedSegmentsAndProofShards, encodedPath, ok, err := n.GetFullShard_Guarantor(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		// Telemetry: Shard request failed (event 122)
		n.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onFullShardRequest: GetFullShard_Guarantor failed: %w", err)
	}
	if !ok {
		stream.CancelWrite(ErrKeyNotFound)
		// Telemetry: Shard request failed (event 122)
		n.telemetryClient.ShardRequestFailed(eventID, "shard not found")
		return fmt.Errorf("onFullShardRequest: shard not found for root=%v, shardIndex=%d", req.ErasureRoot, req.ShardIndex)
	}

	code := uint8(CE137_FullShardRequest)

	// <-- Bundle Shard
	if err := sendQuicBytes(ctx, stream, bundleShard, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onFullShardRequest: send bundleShard failed: %w", err)
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
	if err := sendQuicBytes(ctx, stream, exportedSegmentsAndProofShards, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onFullShardRequest: send exportedSegments failed: %w", err)
	}

	// <-- Justification
	if err := sendQuicBytes(ctx, stream, encodedPath, n.GetEd25519Key().String(), code); err != nil {
		stream.CancelWrite(ErrCECode)
		// Telemetry: Shard request failed (event 122)
		n.telemetryClient.ShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onFullShardRequest: send justification failed: %w", err)
	}

	// Telemetry: Shards transferred (event 125)
	n.telemetryClient.ShardsTransferred(eventID)

	log.Trace(log.DA, "onFullShardRequest completed", "n", n.String(),
		"erasureRoot", req.ErasureRoot,
		"shardIndex", req.ShardIndex,
		"bundleShardLen", len(bundleShard),
		"segmentsLen", len(exportedSegmentsAndProofShards),
		"encodedPathLen", len(encodedPath),
	)

	return nil
}
