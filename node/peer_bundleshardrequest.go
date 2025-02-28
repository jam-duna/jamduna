package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

func (p *Peer) SendBundleShardRequest(erasureRoot common.Hash, shardIndex uint16) (erasure_root common.Hash, shard_index uint16, bundleShard []byte, justification []byte, err error) {
	// TODO: add span for SendBundleShardRequest => get Bundle Shard/Justification back here
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] SendBundleShardRequest", p.node.store.NodeID))
		// p.node.UpdateBundleShardContext(ctx)
		defer span.End()
	}

	isSelfRequesting := false
	if isSelfRequesting {

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

	erasure_root = req.ErasureRoot
	shard_index = req.ShardIndex

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	p.jamnp_test_vector("CE138", "BundleShardRequest", reqBytes, req)
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return
	}
	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}
	log.Debug(debugDA, "SendBundleShardRequest", "p", p.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex, "len", len(bundleShard))
	// <-- Justification
	justification, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}
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
	log.Trace(debugA, "onBundleShardRequest", "n", n.String(), "erasureRoot", req.ErasureRoot, "shardIndex", req.ShardIndex)

	_, _, bundleShard, b_justification, ok, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onBundleShardRequest ERR0 %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}
	// <-- Bundle Shard
	n.jamnp_test_vector("CE138", "BundleShard", bundleShard, nil)
	err = sendQuicBytes(stream, bundleShard)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR1 %v\n", err)
		return err
	}

	// <-- Justification
	n.jamnp_test_vector("CE138", "Justification", b_justification, nil)
	err = sendQuicBytes(stream, b_justification)
	if err != nil {
		return err
	}
	// TODO: Verify justification
	// <-- FIN
	return nil
}
