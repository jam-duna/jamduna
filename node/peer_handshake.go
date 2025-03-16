package node

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
UP 0 UP 0: Block announcement

Header Hash = [u8; 32]
Slot = u32
Final = Header Hash ++ Slot
Leaf = Header Hash ++ Slot
Handshake = Final ++ len++[Leaf]
Header = As in GP
Announcement = Header ++ Final

Node -> Node

--> Handshake AND <-- Handshake (In parallel)
loop {
    --> Announcement OR <-- Announcement (Either side may send)
}
*/

type JAMSNPHandshake struct {
	HeaderHash common.Hash       `json:"headerHash"`
	Timeslot   uint32            `json:"slot"`
	Len        uint16            `json:"len"`
	Leaves     []types.ChainLeaf `json:"leaves"`
}

// ToBytes serializes the JAMSNPHandshake struct to bytes
func (h *JAMSNPHandshake) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write HeaderHash (32 bytes)
	if _, err := buf.Write(h.HeaderHash.Bytes()); err != nil {
		return nil, fmt.Errorf("failed to write HeaderHash: %w", err)
	}

	// Write Timeslot (4 bytes, uint32)
	if err := binary.Write(buf, binary.LittleEndian, h.Timeslot); err != nil {
		return nil, fmt.Errorf("failed to write Timeslot: %w", err)
	}

	// Write Len (2 bytes, uint16)
	if err := binary.Write(buf, binary.LittleEndian, h.Len); err != nil {
		return nil, fmt.Errorf("failed to write Len: %w", err)
	}

	// Write each Leaf
	for _, leaf := range h.Leaves {
		// Write Leaf HeaderHash (32 bytes)
		if _, err := buf.Write(leaf.HeaderHash.Bytes()); err != nil {
			return nil, fmt.Errorf("failed to write Leaf HeaderHash: %w", err)
		}
		// Write Leaf Timeslot (4 bytes, uint32)
		if err := binary.Write(buf, binary.LittleEndian, leaf.Timeslot); err != nil {
			return nil, fmt.Errorf("failed to write Leaf Timeslot: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes the given bytes into a JAMSNPHandshake struct
func (h *JAMSNPHandshake) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Read HeaderHash (32 bytes)
	if _, err := buf.Read(h.HeaderHash.Bytes()[:]); err != nil {
		return fmt.Errorf("failed to read HeaderHash: %w", err)
	}

	// Read Timeslot (4 bytes, uint32)
	if err := binary.Read(buf, binary.LittleEndian, &h.Timeslot); err != nil {
		return fmt.Errorf("failed to read Timeslot: %w", err)
	}

	// Read Len (2 bytes, uint16)
	if err := binary.Read(buf, binary.LittleEndian, &h.Len); err != nil {
		return fmt.Errorf("failed to read Len: %w", err)
	}

	// Read each Leaf based on the Len
	h.Leaves = make([]types.ChainLeaf, h.Len)
	for i := range h.Leaves {
		// Read Leaf HeaderHash (32 bytes)
		if _, err := buf.Read(h.Leaves[i].HeaderHash[:]); err != nil {
			return fmt.Errorf("failed to read Leaf HeaderHash: %w", err)
		}
		// Read Leaf Timeslot (4 bytes, uint32)
		if err := binary.Read(buf, binary.LittleEndian, &h.Leaves[i].Timeslot); err != nil {
			return fmt.Errorf("failed to read Leaf Timeslot: %w", err)
		}
	}

	return nil
}

func (p *Peer) SendHandshake(stream quic.Stream, b types.Block, slot uint32, leaves []types.ChainLeaf) (err error) {
	req := JAMSNPHandshake{
		HeaderHash: b.Header.Hash(),
		Len:        uint16(len(leaves)),
		Timeslot:   slot,
		Leaves:     leaves,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) onHandshake(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	var newReq JAMSNPHandshake
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	n.OnHandshake(peerID, newReq.HeaderHash, newReq.Timeslot, newReq.Leaves)
	return nil
}
