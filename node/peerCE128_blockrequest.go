package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
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

	// Telemetry: Sending block request (event 63)
	p.node.telemetryClient.SendingBlockRequest(p.PeerKey(), headerHash, direction, maximumBlocks)

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return blocks, err
	}

	err = sendQuicBytes(ctx, stream, reqBytes, p.Validator.Ed25519.String(), code)
	if err != nil {
		log.Error(log.Node, "CE128 SendBlockRequest:sendQuicBytes", "p", p.String(), "err", err)
		return blocks, err
	}
	stream.Close()

	// Telemetry: Block request sent (event 66) - note: SendingBlockRequest doesn't return eventID
	// We'll use a simple timestamp-based eventID
	eventID := p.node.telemetryClient.GetEventID()
	p.node.telemetryClient.BlockRequestSent(eventID)

	respBytes, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
	if err != nil {
		log.Error(log.Node, "CE128 SendBlockRequest:receiveQuicBytes", "peerKey", p.Validator.Ed25519.ShortString(), "err", err)
		// Telemetry: Block request failed (event 65)
		p.node.telemetryClient.BlockRequestFailed(eventID, err.Error())
		return blocks, err
	}

	decodedBlocks, err := types.DecodeBlocks(respBytes)
	if err != nil {
		log.Error(log.Node, "CE128 DecodeBlocks", "peerKey", p.Validator.Ed25519.ShortString(), "err", err)
		// Telemetry: Block request failed (event 65)
		p.node.telemetryClient.BlockRequestFailed(eventID, err.Error())
		return blocks, err
	}

	// Telemetry: Block transferred (event 68) for each block
	for i, block := range decodedBlocks {
		isLast := i == len(decodedBlocks)-1
		blockBytes, _ := types.Encode(block)
		blockOutline := telemetry.BlockOutline{
			SizeInBytes:          uint32(len(blockBytes)),
			HeaderHash:           block.Header.Hash(),
			NumTickets:           uint32(len(block.Extrinsic.Tickets)),
			NumPreimages:         uint32(len(block.Extrinsic.Preimages)),
			PreimagesSizeInBytes: 0, // TODO: calculate actual size
			NumGuarantees:        uint32(len(block.Extrinsic.Guarantees)),
			NumAssurances:        uint32(len(block.Extrinsic.Assurances)),
			NumDisputeVerdicts:   0, // TODO: get from block if available
		}
		p.node.telemetryClient.BlockTransferred(eventID, block.Header.Slot, blockOutline, isLast)
	}

	return decodedBlocks, nil
}
func (n *NodeContent) onBlockRequest(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) (err error) {
	var newReq JAMSNPBlockRequest
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	// Note: Builder/full nodes may not be in peersByPubKey initially, so we handle that case
	peer, ok := n.nodeSelf.peersByPubKey[peerKey]
	if !ok {
		// This could be a builder node connecting - still serve the block request
		log.Debug(log.Node, "onBlockRequest: peer not in peersByPubKey (may be builder/full node)", "peerKey", peerKey[:16]+"...")
	}

	// Telemetry: Receiving block request (event 64)
	eventID := n.telemetryClient.GetEventID()
	// Use peerKey directly for telemetry if peer is not in peersByPubKey
	var peerKeyBytes [32]byte
	if peer != nil {
		peerKeyBytes = PubkeyBytes(peer.Validator.Ed25519.String())
	} else {
		peerKeyBytes = PubkeyBytes(peerKey)
	}
	n.telemetryClient.ReceivingBlockRequest(peerKeyBytes)

	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Printf("%s onBlockRequest Error deserializing: %v\n", n.String(), err)
		// Telemetry: Block request failed (event 65)
		n.telemetryClient.BlockRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBlockRequest: failed to deserialize: %w", err)
	}

	// Telemetry: Block request received (event 67)
	n.telemetryClient.BlockRequestReceived(eventID, newReq.HeaderHash, newReq.Direction, newReq.MaximumBlocks)

	// read the request and response with a set of blocks
	blocks, ok, err := n.BlocksLookup(newReq.HeaderHash, newReq.Direction, newReq.MaximumBlocks)
	if err != nil {
		log.Error(log.Node, "onBlockRequest", "headerHash", newReq.HeaderHash, "err", err)
		stream.CancelWrite(ErrKeyNotFound)
		// Telemetry: Block request failed (event 65)
		n.telemetryClient.BlockRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBlockRequest: lookup failed: %w", err)
	}
	if !ok {
		log.Error(log.Node, "onBlockRequest NOT OK", "headerHash", newReq.HeaderHash, "direction", newReq.Direction)
		// Telemetry: Block request failed (event 65)
		n.telemetryClient.BlockRequestFailed(eventID, "blocks not found")
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
		log.Warn(log.Node, "***** onBlockRequest DecodeBlocks mismatch", "peerKey", peerKey, "ok", ok,
			"HeaderHash", newReq.HeaderHash, "Direction", newReq.Direction, "MaximumBlocks", newReq.MaximumBlocks,
			"len(blocks)", len(blocks), "len(blockBytes)", len(blockBytes), "len(decodedBlocks)", len(decodedBlocks))
	} else {
		for i, decodedBlock := range decodedBlocks {
			origBlock := blocks[i]
			if origBlock.Header.Hash() != decodedBlock.Header.Hash() {
				log.Warn(log.Node, "***** onBlockRequest decodedBlock != origBlock header hash", "i", i, "peerKey", peerKey,
					"ok", ok, "HeaderHash", newReq.HeaderHash, "Direction", newReq.Direction, "MaximumBlocks", newReq.MaximumBlocks,
					"len(blocks)", len(blocks))
			}
		}
	}

	err = sendQuicBytes(ctx, stream, blockBytes, n.GetEd25519Key().String(), CE128_BlockRequest)
	if err != nil {
		log.Warn(log.Node, "onBlockRequest sendQuicBytes", "headerHash", newReq.HeaderHash, "direction", newReq.Direction, "err", err)
		// Telemetry: Block request failed (event 65)
		n.telemetryClient.BlockRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBlockRequest: sendQuicBytes failed: %w", err)
	}

	// Telemetry: Block transferred (event 68) for each block
	for i, block := range blocks {
		isLast := i == len(blocks)-1
		blockBytes, _ := types.Encode(block)
		blockOutline := telemetry.BlockOutline{
			SizeInBytes:          uint32(len(blockBytes)),
			HeaderHash:           block.Header.Hash(),
			NumTickets:           uint32(len(block.Extrinsic.Tickets)),
			NumPreimages:         uint32(len(block.Extrinsic.Preimages)),
			PreimagesSizeInBytes: 0, // TODO: calculate actual size
			NumGuarantees:        uint32(len(block.Extrinsic.Guarantees)),
			NumAssurances:        uint32(len(block.Extrinsic.Assurances)),
			NumDisputeVerdicts:   0, // TODO: get from block if available
		}
		n.telemetryClient.BlockTransferred(eventID, block.Header.Slot, blockOutline, isLast)
	}

	// <-- FIN
	return nil
}
