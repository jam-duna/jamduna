package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/quic-go/quic-go"
	"io"
)

/*
CE 137: Shard distribution
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
	if err := binary.Write(buf, binary.BigEndian, req.ShardIndex); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &req.ShardIndex); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendShardRequest(erasureRoot common.Hash, shardIndex uint16, isAudit bool) (bundleShard []byte, segmentShards []byte, justification []byte, err error) {
	code := uint8(CE137_ShardRequest)
	if isAudit {
		code = CE138_ShardRequest
	}
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
		fmt.Printf("%s SendShardRequest(erasureRoot=%v, shardIndex=%d)\n", p.String(), req.ErasureRoot, req.ShardIndex)
	}
	// <-- Bundle Shard
	bundleShard, err = receiveQuicBytes(stream)
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s SendShardRequest received %d bytes for bundleShard\n", p.String(), len(bundleShard))
	}
	if false {
		// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
		segmentShards, err = receiveQuicBytes(stream)
		if err != nil {
			return
		}
		// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
		justification, err = receiveQuicBytes(stream)
		if err != nil {
			return
		}
	}
	return
}

func (n *Node) onShardRequest(stream quic.Stream, msg []byte, isAudit bool) (err error) {
	defer stream.Close()
	var req JAMSNPShardRequest
	// Deserialize byte array back into the struct
	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	if debugA {
		fmt.Printf("%s onShardRequest(erasureRoot=%v, shardIndex=%d)\n", n.String(), req.ErasureRoot, req.ShardIndex)
	}
	bundleShard, segmentShards, justification, ok, err := n.store.GetShard(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		fmt.Printf("onShardRequest ERR0 %v\n", err)
		return err
	}
	if !ok {
		return fmt.Errorf("Not found")
	}
	// <-- Bundle Shard
	err = sendQuicBytes(stream, bundleShard)
	if err != nil {
		fmt.Printf("onShardRequest ERR1 %v\n", err)
		return err
	}
	if debugA {
		fmt.Printf("%s onShardRequest sent %d bytes\n", n.String(), len(bundleShard))
	}
	if false {
		// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)
		err = sendQuicBytes(stream, segmentShards)
		if err != nil {
			return err
		}

		// <-- Justification
		err = sendQuicBytes(stream, justification)
		if err != nil {
			return err
		}
	}
	// <-- FIN
	return nil
}

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
// basically the same as the above without segments
func (p *Peer) SendAuditShardRequest(erasureRoot common.Hash, shardIndex uint16) (bundleShard []byte, segmentShards []byte, justification []byte, err error) {
	return p.SendShardRequest(erasureRoot, shardIndex, true)
}

func (n *Node) onAuditShardRequest(stream quic.Stream, msg []byte, isAudit bool) error {
	return n.onShardRequest(stream, msg, true)
}
