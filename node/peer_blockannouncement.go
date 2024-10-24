package node

import (
	"fmt"
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
func (p *Peer) SendBlockAnnouncement(b types.Block, slot uint32) (err error) {
	req := types.BlockAnnouncement{
		Header:     b.Header,
		HeaderHash: b.Header.Hash(),
		Timeslot:   slot,
	}
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, err := p.openStream(UP0_BlockAnnouncement)
	if err != nil {
		fmt.Printf("SendBlockAnnouncment ERR %v\n", err)
		return err
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		fmt.Printf("SendBlockAnnouncement sendQuicBytes ERR %v\n", err)
		return err
	}
	//fmt.Printf("%s SendBlockAnnouncement (%v)\n", p.String(), req.HeaderHash)
	return nil
}

func (n *Node) onBlockAnnouncement(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()
	var newReq types.BlockAnnouncement
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// <-- FIN

	headerHash := newReq.HeaderHash
	_, found := n.cacheHeadersRead(headerHash)
	if !found {
		blockAnnouncement := types.BlockAnnouncement{
			ValidatorIndex: peerID,
			Header:         newReq.Header,
			HeaderHash:     headerHash,
			Timeslot:       newReq.Timeslot,
		}
		//fmt.Printf("[N%d] OnBlockAnnouncement headerHash: %v\n", n.id, headerHash)
		// initiate request to fetch block
		n.blockAnnouncementsCh <- blockAnnouncement
	}
	return nil
}
