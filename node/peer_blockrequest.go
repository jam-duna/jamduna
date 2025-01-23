package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/colorfulnotion/jam/common"
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
	stream, err := p.openStream(CE128_BlockRequest)
	if err != nil {
		return blocks, err
	}
	defer stream.Close()
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		fmt.Printf("%s SendBlockRequest ERR1 %v\n", p.String(), err)
		panic(0)
		return blocks, err
	}
	// fmt.Printf("%s SendBlockRequest %v\n", p.String(), headerHash)

	respBytes, err := receiveQuicBytes(stream)
	if err != nil {
		fmt.Printf("%s SendBlockRequest ERR2 %v\n", p.String(), err)
		return blocks, err
	}
	//fmt.Printf("%s SendBlockRequest received %d bytes: [%x]\n", p.String(), len(respBytes), respBytes)
	decodedBlocks, _, err := types.Decode(respBytes, reflect.TypeOf([]types.Block{}))
	if err != nil {
		return blocks, err
	}
	blocks = decodedBlocks.([]types.Block)
	if err != nil {
		fmt.Printf("%s SendBlockRequest ERR3 %v\n", p.String(), err)
		return blocks, err
	}
	return blocks, nil
}

func (n *Node) onBlockRequest(stream quic.Stream, msg []byte) (err error) {
	var newReq JAMSNPBlockRequest
	defer stream.Close()
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Printf("%s onBlockRequest Error deserializing: %v\n", n.String(), err)
		return
	}
	// read the request and response with a set of blocks
	blocks, ok, err := n.BlocksLookup(newReq.HeaderHash, newReq.Direction, newReq.MaximumBlocks)
	if err != nil {
		fmt.Printf("%s onBlockRequest (headerHash=%v)  BlocksLookup ERR %v\n", n.String(), newReq.HeaderHash, err)
		return err
	}
	if !ok {
		panic(314)
		return nil
	}
	var blockBytes []byte

	blockBytes, err = types.Encode(blocks)
	if err != nil {
		return err
	}
	// CHECK BLOCK if the blockbytes we sent are decodable and equal the headerhash
	_, _, err = types.Decode(blockBytes, reflect.TypeOf([]types.Block{}))
	if err != nil {
		fmt.Printf("%s SendBlockRequest ERR3 %v\n", n.String(), err)
		return err
	}
	/*
		checkBlock := c.(types.Block)
		checkheaderHash := checkBlock.Header.Hash()
		if checkheaderHash != newReq.HeaderHash {
			panic(9999)
		}
	*/
	// CHECK BLOCK if the blockbytes we sent are decodable and equal the headerhash
	//fmt.Printf("%s onBlockRequest(headerHash: %v) => sending 1 block (%d bytes)=[%v => %v]\n", n.String(), newReq.HeaderHash, len(blockBytes), checkheaderHash, blockBytes)
	err = sendQuicBytes(stream, blockBytes)
	if err != nil {
		fmt.Printf("%s onBlockRequest ERR %v", n.String(), err)
		return err
	}
	// <-- FIN
	return nil
}
