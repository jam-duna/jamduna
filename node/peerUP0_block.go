package node

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
process UP0 iff announced slot is within reasonable bounds
currentJCE-UP0LowerBound<= currentJCE <=currentJCE+UP0UpperBound
6s delay <= now() <= 30s delay
*/
const (
	UP0LowerBound = 5
	UP0UpperBound = 15
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

func (n *Node) GetJAMSNPBlockAnnouncementFromHeader(header types.BlockHeader) (JAMSNP_BlockAnnounce, error) {
	finalized := n.GetLatestFinalizedBlock()
	var finalized_block JAMSNP_BlockInfo
	if n.block_tree == nil {
		finalized_block.HeaderHash = common.Hash{}
		finalized_block.Slot = 0
		log.Warn(log.B, "GetLatestHandshake: block_tree is nil")
	} else {
		finalized_block.HeaderHash = finalized.Header.Hash()
		finalized_block.Slot = finalized.Header.Slot
	}
	block_announcement := JAMSNP_BlockAnnounce{
		Header:         header,
		FinalizedBlock: finalized_block,
	}
	return block_announcement, nil
}

// this function is called by the node to send a block announcement to a peer
// it will either init a new stream or use an existing stream
func (p *Peer) GetOrInitBlockAnnouncementStream(ctx context.Context) (quic.Stream, error) {
	n := p.node
	peerKeyStr := p.Validator.Ed25519.String()
	peerKey := p.Validator.Ed25519.ShortString()
	selfKey := n.GetEd25519Key().ShortString()

	// First check if we have an existing stream (outbound or inbound)
	n.UP0_streamMu.Lock()
	if existingStream, exist := n.UP0_stream[peerKeyStr]; exist && existingStream != nil {
		log.Trace(log.B, "UP0_stream reuse outbound", "node", n.id, "self", selfKey, "peerKey", peerKey)
		n.UP0_streamMu.Unlock()
		return existingStream, nil
	}
	if existingStream, exist := n.UP0_inbound_stream[peerKeyStr]; exist && existingStream != nil {
		log.Trace(log.B, "UP0_stream reuse inbound", "node", n.id, "self", selfKey, "peerKey", peerKey)
		n.UP0_streamMu.Unlock()
		return existingStream, nil
	}
	n.UP0_streamMu.Unlock()

	// No existing stream, need to dial and create one
	p.connectionMu.Lock()
	conn := p.conn
	p.connectionMu.Unlock()
	var err error
	if conn == nil {
		p.conn, err = quic.DialAddr(ctx, p.PeerAddr, p.node.clientTLSConfig, GenerateQuicConfig())

		log.Trace(log.B, "Dial From Up0", "peerKey", p.Validator.Ed25519.ShortString(), "err", err)
		if p.conn != nil {
			negotiatedProto := p.conn.ConnectionState().TLS.NegotiatedProtocol
			log.Trace(log.Quic, "Client connected", "protocol", negotiatedProto)
		}
		if err != nil {
			log.Error(log.Node, "GetOrInitBlockAnnouncementStream", "err", err)
			return nil, fmt.Errorf("peer %s connection is nil", p.Validator.Ed25519.ShortString())
		} else {
			conn = p.conn
			go p.node.nodeSelf.handleConnection(conn)
		}
	}

	// Double-check after dialing - maybe inbound stream was established while we were dialing
	n.UP0_streamMu.Lock()
	if existingStream, exist := n.UP0_stream[peerKeyStr]; exist && existingStream != nil {
		log.Trace(log.B, "UP0_stream rejected - already exists (after dial)", "node", n.id, "self", selfKey, "peerKey", peerKey)
		n.UP0_streamMu.Unlock()
		return existingStream, nil
	}
	if existingStream, exist := n.UP0_inbound_stream[peerKeyStr]; exist && existingStream != nil {
		log.Trace(log.B, "UP0_stream reuse inbound (after dial)", "node", n.id, "self", selfKey, "peerKey", peerKey)
		n.UP0_streamMu.Unlock()
		return existingStream, nil
	}
	n.UP0_streamMu.Unlock()

	code := uint8(UP0_BlockAnnouncement)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, err
	}

	n.UP0_streamMu.Lock()
	if existingStream, exist := n.UP0_stream[peerKeyStr]; exist && existingStream != nil {
		log.Warn(log.B, "UP0_stream rejected - race condition", "node", n.id, "self", selfKey, "peerKey", peerKey)
		n.UP0_streamMu.Unlock()
		stream.Close()
		return existingStream, nil
	}
	n.UP0_stream[peerKeyStr] = stream
	log.Trace(log.B, "UP0_stream stored", "node", n.id, "self", selfKey, "peerKey", peerKey)
	log.Trace(log.B, "InitBlockAnnouncementStream", "node", n.id, "->peer", peerKey)

	// Telemetry: BlockAnnouncementStreamOpened (event 60) - Local side opened stream
	connectionSide := byte(0) // 0 = Local side opened the stream
	peerIDBytes := PubkeyBytes(p.Validator.Ed25519.String())
	n.telemetryClient.BlockAnnouncementStreamOpened(peerIDBytes, connectionSide)

	n.UP0_streamMu.Unlock()
	var wg sync.WaitGroup
	var errChan = make(chan error, 2)

	wg.Add(2)
	// send handshake and receive parallelly
	go func() {
		defer wg.Done()
		handshake := n.GetLatestHandshake()
		handshake_bytes := handshake.ToBytes()
		if handshake_bytes == nil {
			errChan <- fmt.Errorf("handshake_bytes is nil")
			return
		}
		err = sendQuicBytes(ctx, stream, handshake_bytes, p.Validator.Ed25519.String(), code)
		if err != nil {
			errChan <- err
		} else {
			log.Debug(log.B, "sendQuicBytes", "peerKey", peerKey, "handshake", handshake)
		}
	}()

	// receive handshake
	go func() {
		defer wg.Done()
		req, err := receiveQuicBytes(ctx, stream, p.Validator.Ed25519.String(), code)
		if err != nil {
			errChan <- fmt.Errorf("receiveQuicBytes err: %v", err)
			return
		}
		handshake_peer := JAMSNP_Handshake{}
		err = handshake_peer.FromBytes(req)
		if err != nil {
			errChan <- fmt.Errorf("handshake_peer.FromBytes err: %v", err)
		} else {
			log.Debug(log.B, "receiveQuicBytes", "peerKey", peerKey, "handshake", handshake_peer)
			n := p.node.nodeSelf
			if n == nil {
				return
			}
			sync := n.GetIsSync()
			if !sync {
				latest_block_info := n.GetLatestBlockInfo()
				for _, leaf := range handshake_peer.Leaves {
					if latest_block_info != nil {
						if leaf.Slot > latest_block_info.Slot {
							newinfo := leaf
							n.SetLatestBlockInfo(&newinfo, "GetOrInitBlockAnnouncementStream")
						}
					}
				}
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("handshake timeout")
	case err := <-errChan:
		return nil, fmt.Errorf("handshake failed: %v", err)
	case <-done:
		// successful
		log.Debug(log.B, "Handshake completed (initiator)", "peerKey", peerKey)
	}
	// Start reading announcements from this peer.
	// This is safe because handshake exchange is complete - we've sent our handshake
	// AND received peer's handshake response, so next message will be BlockAnnounce.
	go n.nodeSelf.runBlockAnnouncement(stream, peerKeyStr, peerKey)
	return stream, nil
}

// onBlockAnnouncement handles the incoming handshake (in msg) and replies in parallel,
// then registers the stream and spins up runBlockAnnouncement.
func (n *Node) onBlockAnnouncement(stream quic.Stream, msg []byte, peerKey string) error {
	code := uint8(UP0_BlockAnnouncement)
	selfKey := n.GetEd25519Key().ShortString()
	peerKeyShort := peerKey[:10] // short version for logging

	// Verify peer exists
	if _, ok := n.peersByPubKey[peerKey]; !ok {
		return fmt.Errorf("onBlockAnnouncement: could not find peer for key %s", peerKey)
	}

	// Early check - reject if stream already exists before doing expensive handshake
	n.UP0_streamMu.Lock()
	if _, exist := n.UP0_inbound_stream[peerKey]; exist {
		log.Warn(log.B, "UP0_inbound_stream rejected - already exists", "node", n.id, "self", selfKey, "peerKey", peerKeyShort)
		n.UP0_streamMu.Unlock()
		stream.Close()
		return nil
	}
	n.UP0_streamMu.Unlock()

	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	// (1) Decode peer's handshake
	wg.Add(1)
	go func() {
		defer wg.Done()
		var peerHS JAMSNP_Handshake
		if decodeErr := peerHS.FromBytes(msg); decodeErr != nil {
			errCh <- fmt.Errorf("decode handshake failed: %w", decodeErr)
			return
		}

		// If we're not synced, update latest info from their leaves
		if !n.GetIsSync() {
			if latest := n.GetLatestBlockInfo(); latest != nil {
				for _, leaf := range peerHS.Leaves {
					if leaf.Slot > latest.Slot {
						n.SetLatestBlockInfo(&leaf, "onBlockAnnouncement")
					}
				}
			}
		}
	}()

	// (2) Send our handshake back
	wg.Add(1)
	go func() {
		defer wg.Done()
		hs := n.GetLatestHandshake()
		data := hs.ToBytes()
		if data == nil {
			errCh <- fmt.Errorf("handshake bytes nil")
			return
		}
		if sendErr := sendQuicBytes(context.Background(), stream, data, n.GetEd25519Key().String(), code); sendErr != nil {
			errCh <- fmt.Errorf("send handshake failed: %w", sendErr)
		}
	}()

	// wait for both send & decode (or first error)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case e := <-errCh:
		return fmt.Errorf("block announcement handshake err: %v", e)
	case <-done:
		// both succeeded
		log.Debug(log.B, "Handshake completed (receiver)", "peerKey", peerKeyShort)
	}

	// Final check after handshake - detect race condition
	n.UP0_streamMu.Lock()
	if existingStream, exist := n.UP0_inbound_stream[peerKey]; exist && existingStream != nil {
		log.Warn(log.B, "UP0_inbound_stream rejected - race condition", "node", n.id, "self", selfKey, "peerKey", peerKeyShort)
		n.UP0_streamMu.Unlock()
		stream.Close()
		return nil
	}
	n.UP0_inbound_stream[peerKey] = stream
	log.Trace(log.B, "UP0_inbound_stream stored", "node", n.id, "self", selfKey, "peerKey", peerKeyShort)
	n.UP0_streamMu.Unlock()

	// Telemetry: BlockAnnouncementStreamOpened (event 60) - Remote side opened stream
	connectionSide := byte(1) // 1 = Remote side opened the stream
	peerIDBytes := PubkeyBytes(peerKey)
	n.telemetryClient.BlockAnnouncementStreamOpened(peerIDBytes, connectionSide)

	go n.runBlockAnnouncementInbound(stream, peerKey, peerKeyShort)
	return nil
}

// runBlockAnnouncementInbound loops reading announcements on inbound streams.
func (n *Node) runBlockAnnouncementInbound(stream quic.Stream, peerKeyStr string, peerKeyShort string) {
	if stream == nil {
		log.Warn(log.B, "runBlockAnnouncementInbound", "peerKey", peerKeyShort, "err", "nil stream")
		return
	}
	selfKey := n.GetEd25519Key().ShortString()
	defer func() {
		n.UP0_streamMu.Lock()
		delete(n.UP0_inbound_stream, peerKeyStr)
		n.UP0_streamMu.Unlock()
		log.Trace(log.B, "UP0_inbound_stream deleted", "node", n.id, "self", selfKey, "peerKey", peerKeyShort)
		peerIDBytes := PubkeyBytes(peerKeyStr)
		n.telemetryClient.BlockAnnouncementStreamClosed(peerIDBytes, 0, "inbound stream cleanup")
	}()
	n.runBlockAnnouncementLoop(stream, peerKeyStr)
}

// runBlockAnnouncement loops reading announcements on outbound streams.
func (n *Node) runBlockAnnouncement(stream quic.Stream, peerKeyStr string, peerKeyShort string) {
	if stream == nil {
		log.Warn(log.B, "runBlockAnnouncement", "peerKey", peerKeyShort, "err", "nil stream")
		return
	}
	selfKey := n.GetEd25519Key().ShortString()
	defer func() {
		n.UP0_streamMu.Lock()
		delete(n.UP0_stream, peerKeyStr)
		n.UP0_streamMu.Unlock()
		log.Trace(log.B, "UP0_stream deleted", "node", n.id, "self", selfKey, "peerKey", peerKeyShort)
		peerIDBytes := PubkeyBytes(peerKeyStr)
		n.telemetryClient.BlockAnnouncementStreamClosed(peerIDBytes, 0, "outbound stream cleanup")
	}()
	n.runBlockAnnouncementLoop(stream, peerKeyStr)
}

// runBlockAnnouncementLoop is the shared receive loop for both inbound and outbound streams.
func (n *Node) runBlockAnnouncementLoop(stream quic.Stream, peerKeyStr string) {

	code := uint8(UP0_BlockAnnouncement)
	ctx := context.Background()
	receivedFirstValid := false
	peerKeyShort := peerKeyStr[:10]

	for {
		raw, err := receiveQuicBytes(ctx, stream, peerKeyStr, code)
		if err != nil {
			log.Error(log.Node, "runBlockAnnouncement receive error", "peerKey", peerKeyShort, "err", err)
			return
		}

		// Skip empty messages silently (likely handshake drain)
		if len(raw) == 0 {
			continue
		}

		var ann JAMSNP_BlockAnnounce
		if err := ann.FromBytes(raw); err != nil {
			if !receivedFirstValid {
				log.Trace(log.Node, "runBlockAnnouncement skipping invalid frame (pre-first-valid)", "peerKey", peerKeyShort, "rawLen", len(raw))
			} else {
				log.Warn(log.Node, "runBlockAnnouncement decode error", "peerKey", peerKeyShort, "err", err, "rawLen", len(raw))
				peerIDBytes := PubkeyBytes(peerKeyStr)
				n.telemetryClient.BlockAnnouncementMalformed(peerIDBytes, err.Error())
			}
			continue
		}

		// Skip slot=0 messages silently (likely handshake or genesis-related)
		if ann.Header.Slot == 0 {
			continue
		}

		receivedFirstValid = true

		h := ann.Header.Hash()
		peer, ok := n.peersByPubKey[peerKeyStr]
		if ok {
			peer.AddKnownHash(h)
		} else {
			log.Warn(log.B, "runBlockAnnouncement: peer not found in peersByPubKey", "peerKey", peerKeyShort)
		}

		if _, exists := n.block_tree.GetBlockNode(h); exists {
			continue
		}
		received_blk_slot := ann.Header.Slot
		if !n.ValidateJCE(received_blk_slot) {
			// Block announcement is outside of reasonable bound
			continue
		}
		select {
		case n.blockAnnouncementsCh <- ann:
			n.ba_checker.Set(h, peerKeyStr)
			// Emit telemetry event for block announced (received from peer)
			peerIDBytes := PubkeyBytes(peerKeyStr)
			n.telemetryClient.BlockAnnounced(peerIDBytes, 1, ann.Header.Slot, h)
		default:
			log.Warn(log.Node, "runBlockAnnouncement: channel full",
				"peerKey", peerKeyStr,
				"headerHash", h.String_short(),
			)
		}
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
		log.Warn(log.B, "GetLatestHandshake: block_tree is nil")
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
