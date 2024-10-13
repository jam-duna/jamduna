package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"io"
	"reflect"
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
	HeaderHash    common.Hash `json:"headerHash"`
	Direction     uint8       `json:"direction"`
	MaximumBlocks uint32      `json:"maximumBlocks"`
}

// Serialize function to convert the struct into a byte array
func (req *JAMSNPBlockRequest) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write HeaderHash (32 bytes for common.Hash)
	if _, err := buf.Write(req.HeaderHash[:]); err != nil {
		return nil, err
	}

	// Write Direction (1 byte)
	if err := binary.Write(buf, binary.BigEndian, req.Direction); err != nil {
		return nil, err
	}

	// Write MaximumBlocks (4 bytes)
	if err := binary.Write(buf, binary.BigEndian, req.MaximumBlocks); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &req.Direction); err != nil {
		return err
	}

	// Read MaximumBlocks (4 bytes)
	if err := binary.Read(buf, binary.BigEndian, &req.MaximumBlocks); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendBlockRequest(headerHash common.Hash, direction uint8, maximumBlocks uint32) (blocks []types.Block, err error) {
	req := &JAMSNPBlockRequest{
		HeaderHash:    headerHash,
		Direction:     direction,
		MaximumBlocks: maximumBlocks,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return blocks, err
	}
	p.sendCode(CE128_BlockRequest)
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return blocks, err
	}
	respBytes, err := p.receiveQuicBytes()
	if err != nil {
		return blocks, err
	}
	blocksT, _, err := types.Decode(respBytes, reflect.TypeOf([]types.Block(nil)))
	blocks = blocksT.([]types.Block)
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return blocks, err
}

func (p *Peer) processBlockRequest(msg []byte) (err error) {
	var newReq JAMSNPBlockRequest
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// read the request and response with a set of blocks
	blocks, ok, err := p.node.GetBlocks(newReq.HeaderHash, newReq.Direction, newReq.MaximumBlocks)
	if err != nil {
		return err
	}
	if !ok {
		return nil // TODO
	}
	blocksBytes, err := types.Encode(blocks)
	if err != nil {
		return err
	}
	err = p.sendQuicBytes(blocksBytes)
	if err != nil {
		return err
	}

	// <-- FIN
	p.receiveFIN()
	// --> FIN
	p.sendFIN()
	return nil
}
