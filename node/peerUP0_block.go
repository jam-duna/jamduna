package node

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

type JAMSNP_BlockInfo struct {
	HeaderHash common.Hash `json:"header_hash"`
	Slot       uint32      `json:"slot"`
}

func (bi *JAMSNP_BlockInfo) ToBytes() []byte {
	blockinfo := *bi
	bytes, err := types.Encode(blockinfo)
	if err != nil {
		return nil
	}
	return bytes
}

func (bi *JAMSNP_BlockInfo) FromBytes(bytes []byte) error {
	blockinfo_interface, _, err := types.Decode(bytes, reflect.TypeOf(JAMSNP_BlockInfo{}))
	if err != nil {
		return err
	}
	*bi = blockinfo_interface.(JAMSNP_BlockInfo)
	return nil
}

type JAMSNP_Handshake struct {
	FinalizedBlock JAMSNP_BlockInfo   `json:"finalized_block"`
	Leaves         []JAMSNP_BlockInfo `json:"leaves"`
}

func (hs *JAMSNP_Handshake) ToBytes() []byte {
	handshake := *hs
	bytes, err := types.Encode(handshake)
	if err != nil {
		return nil
	}
	return bytes
}

func (hs *JAMSNP_Handshake) FromBytes(bytes []byte) error {
	handshake_interface, _, err := types.Decode(bytes, reflect.TypeOf(JAMSNP_Handshake{}))
	if err != nil {
		return err
	}
	*hs = handshake_interface.(JAMSNP_Handshake)

	return nil
}

type JAMSNP_BlockAnnounce struct {
	Header         types.BlockHeader
	FinalizedBlock JAMSNP_BlockInfo
}

func (ba *JAMSNP_BlockAnnounce) ToBytes() []byte {
	blockannounce := *ba
	bytes, err := types.Encode(blockannounce)
	if err != nil {
		return nil
	}
	return bytes
}

func (ba *JAMSNP_BlockAnnounce) FromBytes(bytes []byte) error {
	blockannounce_interface, _, err := types.Decode(bytes, reflect.TypeOf(JAMSNP_BlockAnnounce{}))
	if err != nil {
		return err
	}
	*ba = blockannounce_interface.(JAMSNP_BlockAnnounce)
	return nil
}

/*
Final = Header Hash ++ Slot
Leaf = Header Hash ++ Slot
Handshake = Final ++ len++[Leaf]
Announcement = Header ++ Final

Node -> Node

--> Handshake AND <-- Handshake (In parallel)

	loop {
		--> Announcement OR <-- Announcement (Either side may send)
	}
*/

func (n *Node) GetBlockAnnouncementBytes(block types.Block) ([]byte, error) {
	finalized := n.GetLatestFinalizedBlock()
	var finalized_block JAMSNP_BlockInfo
	if n.block_tree == nil {
		finalized_block.HeaderHash = common.Hash{}
		finalized_block.Slot = 0
	} else {
		finalized_block.HeaderHash = finalized.Header.Hash()
		finalized_block.Slot = finalized.Header.Slot
	}

	block_announcement := JAMSNP_BlockAnnounce{
		Header:         block.Header,
		FinalizedBlock: finalized_block,
	}
	block_announcement_bytes := block_announcement.ToBytes()
	if block_announcement_bytes == nil {
		return nil, fmt.Errorf("block_announcement_bytes is nil")
	}
	return block_announcement_bytes, nil
}

// this function is called by the node to send a block announcement to a peer
// it will either init a new stream or use an existing stream
func (p *Peer) GetOrInitBlockAnnouncementStream() (quic.Stream, error) {
	n := p.node
	validator_index := n.statedb.GetSafrole().GetCurrValidatorIndex(p.Validator.Ed25519)
	n.UP0_streamMu.Lock()
	if _, exist := n.UP0_stream[uint16(validator_index)]; exist {
		n.UP0_streamMu.Unlock()
		return n.UP0_stream[uint16(validator_index)], nil
	} else if _, exist := n.UP0_stream[uint16(p.PeerID)]; exist {
		n.UP0_streamMu.Unlock()
		return n.UP0_stream[uint16(p.PeerID)], nil

	}
	n.UP0_streamMu.Unlock()
	code := uint8(UP0_BlockAnnouncement)
	stream, err := p.openStream(code)
	if err != nil {

		return nil, err
	}
	n.UP0_streamMu.Lock()
	n.UP0_stream[uint16(validator_index)] = stream
	n.UP0_streamMu.Unlock()
	var wg sync.WaitGroup
	var errChan = make(chan error, 2)

	// send handshake and receive parallelly
	wg.Add(1)
	go func() {
		defer wg.Done()
		handshake := n.GetLatestHandshake()
		handshake_bytes := handshake.ToBytes()
		if handshake_bytes == nil {
			errChan <- fmt.Errorf("handshake_bytes is nil")
			return
		}
		err = sendQuicBytes(stream, handshake_bytes, p.PeerID, code)
		if err != nil {
			errChan <- err
			return
		}
	}()

	// receive handshake
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, err := receiveQuicBytes(stream, p.PeerID, code)
		if err != nil {
			errChan <- fmt.Errorf("receiveQuicBytes err: %v", err)
			return
		}
		handshake_peer := JAMSNP_Handshake{}
		err = handshake_peer.FromBytes(req)
		if err != nil {
			errChan <- fmt.Errorf("handshake_peer.FromBytes err: %v", err)
			return
		}
	}()

	wg.Wait()

	// check if there is any error
	select {
	case err = <-errChan:
		return nil, fmt.Errorf("GetOrInitBlockAnnouncementStream err: %v", err)
	default:
	}
	// TODO do something with the received handshake
	// persist the stream
	go n.runBlockAnnouncement(stream)
	return stream, nil
}

// this function is for the accepting side of the block announcement
func (n *Node) onBlockAnnouncement(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	//don't close the stream here
	var newHandshake JAMSNP_Handshake
	// Deserialize byte array back into the struct
	var wg sync.WaitGroup
	var errChan = make(chan error, 2)
	code := uint8(UP0_BlockAnnouncement)
	// send and receive handshake parallelly
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = newHandshake.FromBytes(msg)
		if err != nil {
			errChan <- err
			return
		}
	}()
	// send the handshake back
	wg.Add(1)
	go func() {
		defer wg.Done()
		handshake := n.GetLatestHandshake()
		handshake_bytes := handshake.ToBytes()
		if handshake_bytes == nil {
			err = fmt.Errorf("handshake_bytes is nil")
			errChan <- err
			return
		}
		err = sendQuicBytes(stream, handshake_bytes, n.id, code)
		if err != nil {
			errChan <- err
			return
		}
	}()
	wg.Wait()
	if peerID == TestPeerID {
		log.Debug(debugBlock, "Received Handshake from peer", "peer", peerID)
	}
	// check if there is any error
	select {
	case err = <-errChan:
		return fmt.Errorf("block announcement handshake err: %v", err)

	default:
	}

	n.UP0_streamMu.Lock()
	n.UP0_stream[peerID] = stream
	n.UP0_streamMu.Unlock()

	// TODO do something with the received handshake
	go n.runBlockAnnouncement(stream)
	return nil
}

// this function will read the block announcement from the stream persistently
func (n *NodeContent) runBlockAnnouncement(stream quic.Stream) {
	code := uint8(UP0_BlockAnnouncement)
	for {
		time.Sleep(5 * time.Millisecond)
		// see if there is any stream error
		if stream == nil {
			return
		}
		req, err := receiveQuicBytes(stream, n.id, code)
		if err != nil {
			fmt.Println("Error receiving block announcement:", err)
			return
		}
		blockannounce := JAMSNP_BlockAnnounce{}
		err = blockannounce.FromBytes(req)
		if err != nil {
			fmt.Println("Error deserializing block announcement:", err)
			return
		}
		n.blockAnnouncementsCh <- blockannounce
	}
}

func (n *Node) GetLatestFinalizedBlock() *types.Block {
	if n.block_tree == nil {
		return nil
	}
	return n.block_tree.GetLastFinalizedBlock().Block
}

func (n *NodeContent) GetLatestHandshake() JAMSNP_Handshake {
	if n.block_tree == nil {
		return JAMSNP_Handshake{
			FinalizedBlock: JAMSNP_BlockInfo{
				HeaderHash: common.BytesToHash([]byte("no blocks yet")), // TODO: change this to a more meaningful value (genesis)
			},
			Leaves: []JAMSNP_BlockInfo{},
		}
	}
	finalized_block := n.block_tree.GetLastFinalizedBlock()
	if finalized_block == nil {
		return JAMSNP_Handshake{}
	}
	finalized_block_info := JAMSNP_BlockInfo{
		HeaderHash: finalized_block.Block.Header.Hash(),
		Slot:       finalized_block.Block.Header.Slot,
	}
	leaves := n.block_tree.GetLeafs()
	leaves_info := make([]JAMSNP_BlockInfo, 0)
	for _, leaf := range leaves {
		leaf_info := JAMSNP_BlockInfo{
			HeaderHash: leaf.Block.Header.Hash(),
			Slot:       leaf.Block.Header.Slot,
		}
		leaves_info = append(leaves_info, leaf_info)
	}
	handshake := JAMSNP_Handshake{
		FinalizedBlock: finalized_block_info,
		Leaves:         leaves_info,
	}
	return handshake
}
