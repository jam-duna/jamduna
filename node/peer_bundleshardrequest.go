package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
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

// TODO: separate them
func (p *Peer) SendBundleShardRequest(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, justification []byte, err error) {
	code := uint8(CE138_BundleShardRequest)
	stream, err := p.openStream(code)
	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s SendBundleShardRequest(erasureRoot=%v, shardIndex=%d)\n", p.String(), req.ErasureRoot, req.ShardIndex)
	}
	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s SendBundleShardRequest received %d bytes for bundleShard\n", p.String(), len(bundleShard))
	}
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
	if debugA {
		fmt.Printf("%s onBundleShardRequest(erasureRoot=%v, shardIndex=%d)\n", n.String(), req.ErasureRoot, req.ShardIndex)
	}
	//TODO: this should be bundleshard call
	bundleShard, justification, ok, err := n.GetBundleShard(req.ErasureRoot, req.ShardIndex)
	//bundleShard, justification, ok, err := n.GetBundleShard(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onBundleShardRequest ERR0 %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}
	// <-- Bundle Shard
	err = sendQuicBytes(stream, bundleShard)
	if err != nil {
		fmt.Printf("onFullShardRequest ERR1 %v\n", err)
		return err
	}

	// <-- Justification
	err = sendQuicBytes(stream, justification)
	if err != nil {
		return err
	}

	// <-- FIN
	return nil
}
