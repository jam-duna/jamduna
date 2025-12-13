package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 138: Audit shard request
Request for a work-package bundle shard (a.k.a. "audit" shard).

This protocol should be used by auditors to request work-package bundle shards from assurers in order to reconstruct work-package bundles for auditing. In addition to the requested shard, the response should include a justification, proving the correctness of the shard.

The justification should be the co-path of the path from the erasure root to the shard. The assurer should construct this by appending the corresponding segment shard root to the justification received via CE 137. The last-but-one entry in the justification may consist of a pair of hashes.

Erasure Root = [u8; 32]
Shard Index = u16
Bundle Shard = [u8]
Hash = [u8; 32]
Justification = [Hash OR (Hash ++ Hash)]

Auditor -> Assurer

--> Erasure Root ++ Shard Index
--> FIN
<-- Bundle Shard
<-- Justification
<-- FIN
*/

func (p *Peer) SendBundleShardRequest(
	ctx context.Context,
	erasureRoot common.Hash,
	shardIndex uint16,
	eventID uint64,
) (bundleShard []byte, sClub common.Hash, encodedPath []byte, err error) {

	code := uint8(CE138_BundleShardRequest)

	// Telemetry: Sending bundle shard request (event 140)
	// Using 0 for eventID since we don't have auditing event context here
	p.node.telemetryClient.SendingBundleShardRequest(eventID, p.GetPeer32(), shardIndex)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Bundle shard request failed (event 142)
		p.node.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return nil, common.Hash{}, nil, fmt.Errorf("openStream[CE138]: %w", err)
	}

	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		stream.Close()
		// Telemetry: Bundle shard request failed (event 142)
		p.node.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return nil, common.Hash{}, nil, fmt.Errorf("ToBytes[ShardRequest]: %w", err)
	}

	// --> Erasure Root ++ Shard Index
	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		log.Trace(log.DA, "SendBundleShardRequest - sending error", "p", p.String(), "erasureRoot", erasureRoot, "shardIndex", shardIndex, "ERR", err)
		// Telemetry: Bundle shard request failed (event 142)
		p.node.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return nil, common.Hash{}, nil, fmt.Errorf("sendQuicBytes[CE138]: %w", err)
	}
	//--> FIN
	stream.Close()

	// Telemetry: Bundle shard request sent (event 143)
	p.node.telemetryClient.BundleShardRequestSent(eventID)

	parts, err := receiveMultiple(ctx, stream, 2, p.PeerID, code)
	if err != nil {
		log.Trace(log.DA, "SendBundleShardRequest - receive error", "p", p.String(), "erasureRoot", erasureRoot, "shardIndex", shardIndex, "ERR", err)
		// Telemetry: Bundle shard request failed (event 142)
		p.node.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return nil, common.Hash{}, nil, fmt.Errorf("receiveMultiple[CE138]: %w", err)
	}

	bundleShard = parts[0]
	encodedJustification := parts[1]

	sclubJustification, decodeErr := common.DecodeJustification(encodedJustification, types.NumECPiecesPerSegment)
	if decodeErr != nil || len(sclubJustification) < 1 {
		log.Error(log.DA, "SendBundleShardRequest - justification decode error",
			"p", p.String(), "erasureRoot", erasureRoot, "shardIndex", shardIndex, "ERR", decodeErr)
		// Telemetry: Bundle shard request failed (event 142)
		errMsg := "DecodeJustification failed or too short"
		if decodeErr != nil {
			errMsg = decodeErr.Error()
		}
		p.node.telemetryClient.BundleShardRequestFailed(eventID, errMsg)
		return nil, common.Hash{}, nil, fmt.Errorf("DecodeJustification failed or too short")
	}

	sClub = common.BytesToHash(sclubJustification[len(sclubJustification)-1]) // last element in the CE138 justification is the SClub
	pathJustification := sclubJustification[:len(sclubJustification)-1]       // the rest is the path justification without the last element
	encodedPath, err = common.EncodeJustification(pathJustification, types.NumECPiecesPerSegment)
	if err != nil {
		// Telemetry: Bundle shard request failed (event 142)
		p.node.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return nil, common.Hash{}, nil, fmt.Errorf("EncodeJustification[path] failed: %w", err)
	}

	// Telemetry: Bundle shard transferred (event 145)
	p.node.telemetryClient.BundleShardTransferred(eventID)

	log.Trace(log.DA, "SendBundleShardRequest DONE",
		"p", p.String(), "erasureRoot", erasureRoot, "shardIndex", shardIndex,
		"bundleShardLen", len(bundleShard), "encodedPathLen", len(encodedPath),
		"sClub", sClub, "encodedPath", fmt.Sprintf("%x", encodedPath))

	return bundleShard, sClub, encodedPath, nil
}

func (n *Node) onBundleShardRequest(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	// Telemetry: Receiving bundle shard request (event 141)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingBundleShardRequest(n.PeerID32(peerID))

	var req JAMSNPShardRequest
	if err := req.FromBytes(msg); err != nil {
		// Telemetry: Bundle shard request failed (event 142)
		n.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBundleShardRequest: failed to deserialize: %w", err)
	}

	// Telemetry: Bundle shard request received (event 144)
	n.telemetryClient.BundleShardRequestReceived(eventID, req.ErasureRoot, req.ShardIndex)

	log.Trace(log.A, "onBundleShardRequest", "n", n.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex)

	code := uint8(CE138_BundleShardRequest)
	bundleShard, sClub, encodedPath, ok, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		// Telemetry: Bundle shard request failed (event 142)
		n.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBundleShardRequest: GetBundleShard_Assurer error: %w", err)
	}
	if !ok {
		stream.CancelWrite(ErrKeyNotFound)
		// Telemetry: Bundle shard request failed (event 142)
		n.telemetryClient.BundleShardRequestFailed(eventID, "bundle shard not found")
		return fmt.Errorf("onBundleShardRequest: bundle shard not found for root=%v shardIndex=%d", req.ErasureRoot, req.ShardIndex)
	}

	// Respect cancellation before starting any response
	select {
	case <-ctx.Done():
		return fmt.Errorf("onBundleShardRequest: context cancelled before sending: %w", ctx.Err())
	default:
	}

	// Send bundle shard
	if err := sendQuicBytes(ctx, stream, bundleShard, n.id, code); err != nil {
		stream.CancelWrite(ErrCECode)
		return fmt.Errorf("onBundleShardRequest: send bundleShard failed: %w", err)
	}

	// Build and send justification
	pathJustification, _ := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
	sclubJustification := append(pathJustification, [][]byte{sClub.Bytes()}...) // last element in the CE138 justification is the SClub

	encodedJustification, err := common.EncodeJustification(sclubJustification, types.NumECPiecesPerSegment)
	if err != nil {
		stream.CancelWrite(ErrInvalidData)
		return fmt.Errorf("onBundleShardRequest: encode justification failed: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, encodedJustification, n.id, code); err != nil {
		stream.CancelWrite(ErrCECode)
		// Telemetry: Bundle shard request failed (event 142)
		n.telemetryClient.BundleShardRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBundleShardRequest: send justification failed: %w", err)
	}

	// Telemetry: Bundle shard transferred (event 145)
	n.telemetryClient.BundleShardTransferred(eventID)

	log.Trace(log.A, "onBundleShardRequest sent", "n", n.String(), "bundleShardLen", len(bundleShard), "justificationLen", len(encodedJustification))
	return nil
}
