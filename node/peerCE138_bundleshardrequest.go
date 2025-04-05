package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

func (p *Peer) SendBundleShardRequest(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, sClub common.Hash, encodedPath []byte, err error) {
	// add span for SendBundleShardRequest => get Bundle Shard/Justification back here
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] SendBundleShardRequest", p.node.store.NodeID))
		// p.node.UpdateBundleShardContext(ctx)
		defer span.End()
	}

	code := uint8(CE138_BundleShardRequest)
	stream, err := p.openStream(code)
	if err != nil {
		return
	}
	defer stream.Close()
	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	err = sendQuicBytes(stream, reqBytes, p.PeerID, code)
	if err != nil {
		log.Error(debugDA, "SendBundleShardRequest - sedning Err", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
		return
	}
	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream, p.PeerID, code)
	if err != nil {
		log.Error(debugDA, "SendBundleShardRequest - bundleShard Stream Err", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
		return
	}
	// <-- JAM NP Justification
	encoded_sclub_justification, err := receiveQuicBytes(stream, p.PeerID, code)
	if err != nil {
		log.Error(debugDA, "SendBundleShardRequest - encoded_sclub_justification Stream Err", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
		return
	}
	sclub_justification, sclub_justification_err := common.DecodeJustification(encoded_sclub_justification, types.NumECPiecesPerSegment)
	if sclub_justification_err != nil || len(sclub_justification) < 1 {
		log.Error(debugDA, "SendBundleShardRequest - encoded_sclub_justification Decoding Err", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
		return
	}
	sClub = common.BytesToHash(sclub_justification[0])
	path_justification := sclub_justification[1:]
	encodedPath, _ = common.EncodeJustification(path_justification, types.NumECPiecesPerSegment)
	log.Trace(debugDA, "SendBundleShardRequest DONE", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "bundleShardLen", len(bundleShard), "ecodedPathLen", len(encodedPath), "sClub", sClub, "encodedPath", fmt.Sprintf("%x", encodedPath))
	return
}

func (n *Node) onBundleShardRequest(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	var req JAMSNPShardRequest
	// Deserialize byte array back into the struct
	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	log.Debug(debugA, "!! onBundleShardRequest", "n", n.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex)

	code := uint8(CE138_BundleShardRequest)
	bundleShard, sClub, encodedPath, ok, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onBundleShardRequest ERR0 %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}
	// <-- Bundle Shard
	err = sendQuicBytes(stream, bundleShard, n.id, code)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR1 %v\n", err)
		return err
	}

	// <-- Justification USDED BY JAM NP = sClub++path
	path_justification, _ := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
	sclub_justification := make([][]byte, 0)
	sclub_justification = append(sclub_justification, sClub.Bytes())
	sclub_justification = append(sclub_justification, path_justification...)
	// <-- Justification
	encoded_sclub_justification, _ := common.EncodeJustification(sclub_justification, types.NumECPiecesPerSegment)
	err = sendQuicBytes(stream, encoded_sclub_justification, n.id, code)
	if err != nil {
		return err
	}
	// <-- FIN
	return nil
}
