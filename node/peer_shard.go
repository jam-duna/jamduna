package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
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

func (p *Peer) SendShardRequest(erasureRoot common.Hash, shardIndex uint16, isAudit bool) (err error) {
	if isAudit {
		p.sendCode(CE138_ShardRequest)
	} else {
		p.sendCode(CE137_ShardRequest)
	}
	req := &JAMSNPShardRequest{
		ErasureRoot: erasureRoot,
		ShardIndex:  shardIndex,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return err
	}
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return nil
}

func (p *Peer) processShardRequest(msg []byte, isAudit bool) (err error) {
	var req JAMSNPShardRequest
	// Deserialize byte array back into the struct
	err = req.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	bundleShard, justification, ok, err := p.node.GetShard(req.ErasureRoot, req.ShardIndex)
	if err != nil {
		return err
	}
	if !ok {
		// TODO
	}
	// <-- Bundle Shard
	err = p.sendQuicBytes(bundleShard)
	if err != nil {
		return err
	}

	// <-- [Segment Shard] (Should include all exported and proof segment shards with the given index)

	// <-- Justification
	err = p.sendQuicBytes(justification)
	if err != nil {
		return err
	}

	// --> FIN
	p.receiveFIN()
	// <-- FIN
	p.sendFIN()

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
func (p *Peer) SendAuditShardRequest(erasureRoot common.Hash, shardIndex uint16) (err error) {
	return p.SendShardRequest(erasureRoot, shardIndex, true)
}

func (p *Peer) processAuditShardRequest(msg []byte, isAudit bool) error {
	return p.processShardRequest(msg, true)
}
