package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 128: Block request
Header Hash = [u8; 32]
Direction = { 0 (Ascending exclusive), 1 (Descending inclusive) } (Single byte)
Maximum Blocks = u32
Block = As in GP

Node -> Node

--> Header Hash ++ Direction ++ Maximum Blocks
--> FIN
<-- [Block]
<-- FIN
*/
type JAMSNPBlockRequest struct {
	HeaderHash    common.Hash `json:"header_hash"`
	Direction     uint8       `json:"direction"`
	MaximumBlocks uint32      `json:"maximum_blocks"`
}

// Serialize function to convert the struct into a byte array
func (req *JAMSNPBlockRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write HeaderHash (32 bytes for common.Hash)
	if _, err := buf.Write(req.HeaderHash[:]); err != nil {
		return nil, err
	}

	// Write Direction (1 byte)
	if err := binary.Write(buf, binary.LittleEndian, req.Direction); err != nil {
		return nil, err
	}

	// Write MaximumBlocks (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.MaximumBlocks); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Deserialize function to convert a byte array back into a JAMSNPBlockRequest struct
func (req *JAMSNPBlockRequest) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Read HeaderHash (32 bytes)
	if _, err := io.ReadFull(buf, req.HeaderHash[:]); err != nil {
		return err
	}

	// Read Direction (1 byte)
	if err := binary.Read(buf, binary.LittleEndian, &req.Direction); err != nil {
		return err
	}

	// Read MaximumBlocks (4 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.MaximumBlocks); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendBlockRequest(ctx context.Context, headerHash common.Hash, direction uint8, maximumBlocks uint32) (blocks []types.Block, err error) {
	// span for block request => response here
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(p.node.store.BlockAnnouncementContext, fmt.Sprintf("[N%d] SendBlockRequest", p.node.store.NodeID))
		// p.node.UpdateBlockContext(ctx)
		defer span.End()
	}

	req := &JAMSNPBlockRequest{
		HeaderHash:    headerHash,
		Direction:     direction,
		MaximumBlocks: maximumBlocks,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return blocks, err
	}
	code := uint8(CE128_BlockRequest)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return blocks, err
	}
	defer stream.Close()

	err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code)
	if err != nil {
		log.Trace(module, "CE128 SendBlockRequest", "p", p.String(), "err", err)
		return blocks, err
	}

	respBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		log.Trace(module, "CE128 SendBlockRequest", "peerID", p.String(), "err", err)
		return blocks, err
	}

	decodedBlocks, err := types.DecodeBlocks(respBytes)
	if err != nil {
		log.Error(module, "CE128 DecodeBlocks", "peerID", p.String(), "err", err)
		return blocks, err
	}
	return decodedBlocks, nil
}
func (n *NodeContent) onBlockRequest(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) (err error) {
	var newReq JAMSNPBlockRequest
	defer stream.Close()

	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Printf("%s onBlockRequest Error deserializing: %v\n", n.String(), err)
		return fmt.Errorf("onBlockRequest: failed to deserialize: %w", err)
	}

	// read the request and response with a set of blocks
	blocks, ok, err := n.BlocksLookup(newReq.HeaderHash, newReq.Direction, newReq.MaximumBlocks)
	if err != nil {
		log.Error(module, "onBlockRequest", "headerHash", newReq.HeaderHash, "err", err)
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onBlockRequest: lookup failed: %w", err)
	}
	if !ok {
		log.Error(module, "onBlockRequest NOT OK", "headerHash", newReq.HeaderHash, "direction", newReq.Direction)
		return nil
	}
	var blockBytes []byte
	for _, b := range blocks {
		bBytes, err := types.Encode(b)
		if err != nil {
			return fmt.Errorf("onBlockRequest: block encode failed: %w", err)
		}
		blockBytes = append(blockBytes, bBytes...)
	}

	// check BLOCK if the blockbytes we sent are decodable
	decodedBlocks, err := types.DecodeBlocks(blockBytes)
	if len(decodedBlocks) != len(blocks) {
		log.Warn(module, "***** onBlockRequest DecodeBlocks mismatch", "peerID", peerID, "ok", ok,
			"HeaderHash", newReq.HeaderHash, "Direction", newReq.Direction, "MaximumBlocks", newReq.MaximumBlocks,
			"len(blocks)", len(blocks), "len(blockBytes)", len(blockBytes), "len(decodedBlocks)", len(decodedBlocks))
	} else {
		for i, decodedBlock := range decodedBlocks {
			origBlock := blocks[i]
			if origBlock.Header.Hash() != decodedBlock.Header.Hash() {
				log.Warn(module, "***** onBlockRequest decodedBlock != origBlock header hash", "i", i, "peerID", peerID,
					"ok", ok, "HeaderHash", newReq.HeaderHash, "Direction", newReq.Direction, "MaximumBlocks", newReq.MaximumBlocks,
					"len(blocks)", len(blocks))
			}
		}
	}

	err = sendQuicBytes(ctx, stream, blockBytes, n.id, CE128_BlockRequest)
	if err != nil {
		log.Warn(module, "onBlockRequest sendQuicBytes", "headerHash", newReq.HeaderHash, "direction", newReq.Direction, "err", err)
		return fmt.Errorf("onBlockRequest: sendQuicBytes failed: %w", err)
	}

	// <-- FIN
	return nil
}
