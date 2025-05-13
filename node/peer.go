package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"

	"sync"
)

const (
	UP0_BlockAnnouncement        uint8 = iota
	CE128_BlockRequest                 = 128
	CE129_StateRequest                 = 129
	CE131_TicketDistribution           = 131
	CE132_TicketDistribution           = 132
	CE133_WorkPackageSubmission        = 133
	CE134_WorkPackageShare             = 134
	CE135_WorkReportDistribution       = 135
	CE136_WorkReportRequest            = 136
	CE137_FullShardRequest             = 137
	CE138_BundleShardRequest           = 138
	CE139_SegmentShardRequest          = 139
	CE140_SegmentShardRequestP         = 140
	CE141_AssuranceDistribution        = 141
	CE142_PreimageAnnouncement         = 142
	CE143_PreimageRequest              = 143
	CE144_AuditAnnouncement            = 144
	CE145_JudgmentPublication          = 145
	CE192_VoteMessage                  = 192
	CE193_CommitMessage                = 193
	CE194_NeighborMessage              = 194
	CE195_Catchup                      = 195
	CE224_BLSSignature                 = 224
	CE225_BLSAggregateSignature        = 225

	useQuicDeadline = false
)

type Peer struct {
	// these are initialized in NewPeer
	node      *NodeContent
	PeerID    uint16          `json:"peer_id"`
	PeerAddr  string          `json:"peer_addr"`
	Validator types.Validator `json:"validator"`

	knownHashes []common.Hash

	// these are established early on but may change
	connectionMu sync.Mutex
	conn         quic.Connection
}

type PeerInfo struct {
	PeerID    uint16          `json:"peer_id"`
	PeerAddr  string          `json:"peer_addr"`
	Validator types.Validator `json:"validator"`
}

func (p *Peer) Clone() *Peer {
	return &Peer{
		node:      p.node,
		PeerID:    p.PeerID,
		PeerAddr:  p.PeerAddr,
		Validator: p.Validator,
		conn:      p.conn,
	}
}

func NewPeer(n *Node, validatorIndex uint16, validator types.Validator, peerAddr string) (p *Peer) {
	p = &Peer{
		node:        &n.NodeContent,
		PeerAddr:    peerAddr,
		Validator:   validator,
		PeerID:      validatorIndex,
		knownHashes: make([]common.Hash, 0),
	}
	return p
}

func (p *Peer) AddKnownHash(h common.Hash) {
	p.knownHashes = append(p.knownHashes, h)
	if len(p.knownHashes) > 128 {
		p.knownHashes = p.knownHashes[1:]
	}
}

func (p *Peer) IsKnownHash(h common.Hash) bool {
	for _, k := range p.knownHashes {
		if k == h {
			return true
		}
	}
	return false
}

func (p *Peer) String() string {
	return fmt.Sprintf("[N%d => %d]", p.node.id, p.PeerID)
}

func (p *Peer) openStream(ctx context.Context, code uint8) (quic.Stream, error) {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()

	var err error
	if p.conn == nil {
		dialCtx := ctx
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			dialCtx, cancel = context.WithTimeout(ctx, NormalTimeout)
			defer cancel()
		}

		p.conn, err = quic.DialAddr(dialCtx, p.PeerAddr, p.node.clientTLSConfig, GenerateQuicConfig())
		if err != nil {
			log.Error(module, "DialAddr failed", "node", p.node.id, "peerID", p.PeerID, "err", err)
			return nil, fmt.Errorf("[P%d] DialAddr failed: %w", p.PeerID, err)
		}
	}

	// Open stream with context
	stream, err := p.conn.OpenStreamSync(ctx)
	if err != nil {
		//fmt.Printf("[%s] OpenStreamSync ERR %v\n", p.PeerAddr, err)
		_ = p.conn.CloseWithError(0, "stream open failed")
		p.conn = nil
		if stream != nil {
			_ = stream.Close()
		}
		return nil, fmt.Errorf("OpenStreamSync failed: %w", err)
	}

	// Send code byte to identify stream purpose
	_, err = stream.Write([]byte{code})
	if err != nil {
		if err == io.EOF {
			fmt.Println("EOF encountered during Write()")
		}
		_ = stream.Close()
		_ = p.conn.CloseWithError(0, "write failed")
		p.conn = nil
		return nil, fmt.Errorf("failed to write code byte: %w", err)
	}

	return stream, nil
}
func sendQuicBytes(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16, code uint8) (err error) {
	// Create a buffer to hold the length of the message (big-endian uint32)
	if stream == nil {
		return errors.New("stream is nil")
	}
	msgLen := uint32(len(msg))
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, msgLen)
	// Respect context before sending
	select {
	case <-ctx.Done():
		return fmt.Errorf("sendQuicBytes: context cancelled before send: %w", ctx.Err())
	default:
	}
	if code == UP0_BlockAnnouncement {
		stream.SetWriteDeadline(time.Now().Add(2 * time.Second))
	}
	// First, write the message length to the stream
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Warn(module, "sendQuicBytes-length", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		return err
	} else if code == UP0_BlockAnnouncement {
		// remove the deadline
		stream.SetWriteDeadline(time.Time{})
	}

	// Respect context before sending
	select {
	case <-ctx.Done():
		return fmt.Errorf("onPreimageRequest: context cancelled before send: %w", ctx.Err())
	default:
	}

	// Then, write the actual message to the stream
	_, err = stream.Write(msg)
	if err != nil {
		log.Warn(module, "sendQuicBytes-msg", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		return err
	}

	return nil
}

// receiveMultiple reads `count` messages from a QUIC stream and returns a slice of received byte slices.
func receiveMultiple(ctx context.Context, stream quic.Stream, count int, peerID uint16, code uint8) ([][]byte, error) {
	results := make([][]byte, 0, count)

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			data, err := receiveQuicBytes(ctx, stream, peerID, code)
			if err != nil {
				return nil, fmt.Errorf("receiveQuicBytes[%d/%d]: %w", i+1, count, err)
			}
			results = append(results, data)
		}
	}

	return results, nil
}

func receiveQuicBytes(ctx context.Context, stream quic.Stream, peerID uint16, code uint8) ([]byte, error) {
	var lengthPrefix [4]byte

	// Set read deadline from context if present
	if useQuicDeadline {
		if deadline, ok := ctx.Deadline(); ok {
			if err := stream.SetReadDeadline(deadline); err != nil {
				log.Trace(module, "SetReadDeadline failed", "peerID", peerID, "err", err, "code", code)
				return nil, err
			}
		}
	}

	// Read length prefix
	if _, err := io.ReadFull(stream, lengthPrefix[:]); err != nil {
		log.Trace(module, "receiveQuicBytes-length prefix", "peerID", peerID, "err", err, "code", code)
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lengthPrefix[:])
	buf := make([]byte, msgLen)

	/*  do we really need to do this AGAIN?? */
	if useQuicDeadline {
		if deadline, ok := ctx.Deadline(); ok {
			if err := stream.SetReadDeadline(deadline); err != nil {
				log.Trace(module, "SetReadDeadline failed", "peerID", peerID, "err", err, "code", code)
				return nil, err
			}
		}
	}

	// Read message body
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Trace(module, "receiveQuicBytes-message body", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen)
		return nil, err
	}

	return buf, nil
}

const (
	ErrStateNotSynced = 0x1000
	ErrCECode         = 0x1001
	ErrKeyNotFound    = 0x1002
	ErrInvalidData    = 0x1003
)

// DispatchIncomingQUICStream reads from QUIC and dispatches based on message type
func (n *Node) DispatchIncomingQUICStream(ctx context.Context, stream quic.Stream, peerID uint16) error {
	// Respect context by setting read deadline if present
	/*if deadline, ok := ctx.Deadline(); ok {
		if err := stream.SetReadDeadline(deadline); err != nil {
			log.Warn(module, "SetReadDeadline failed", "err", err)
			return err
		}
	}*/
	// Read message type (1 byte)
	msgTypeBytes := make([]byte, 1)
	if _, err := io.ReadFull(stream, msgTypeBytes); err != nil {
		log.Trace(module, "DispatchIncomingQUICStream - code", "err", err)
		_ = stream.Close()
		return err
	}
	msgType := msgTypeBytes[0]

	// Read message length (4 bytes)
	msgLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, msgLenBytes); err != nil {
		log.Trace(module, "DispatchIncomingQUICStream - length prefix", "err", err)
		_ = stream.Close()
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)

	// Read message body
	msg := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, msg); err != nil {
		log.Trace(module, "DispatchIncomingQUICStream - message body", "peerID", peerID, "code", msgType, "err", err)
		stream.CancelRead(ErrCECode)
		_ = stream.Close()
		return err
	}

	// Dispatch based on msgType
	switch msgType {
	case UP0_BlockAnnouncement:
		return n.onBlockAnnouncement(stream, msg, peerID)
	case CE128_BlockRequest:
		return n.onBlockRequest(ctx, stream, msg, peerID)
	case CE129_StateRequest:
		return n.onStateRequest(ctx, stream, msg)
	case CE131_TicketDistribution, CE132_TicketDistribution:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onTicketDistribution(ctx, stream, msg, peerID)
	case CE133_WorkPackageSubmission:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onWorkPackageSubmission(ctx, stream, msg)
	case CE134_WorkPackageShare:
		return n.onWorkPackageShare(ctx, stream, msg)
	case CE135_WorkReportDistribution:
		return n.onWorkReportDistribution(ctx, stream, msg)
	case CE136_WorkReportRequest:
		return n.onWorkReportRequest(ctx, stream, msg)
	case CE137_FullShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onFullShardRequest(ctx, stream, msg)
	case CE138_BundleShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onBundleShardRequest(ctx, stream, msg)
	case CE139_SegmentShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onSegmentShardRequest(ctx, stream, msg, false)
	case CE140_SegmentShardRequestP:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onSegmentShardRequest(ctx, stream, msg, true)
	case CE141_AssuranceDistribution:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onAssuranceDistribution(ctx, stream, msg, peerID)
	case CE142_PreimageAnnouncement:
		return n.onPreimageAnnouncement(ctx, stream, msg, peerID)
	case CE143_PreimageRequest:
		return n.onPreimageRequest(ctx, stream, msg)
	case CE144_AuditAnnouncement:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onAuditAnnouncement(ctx, stream, msg, peerID)
	case CE145_JudgmentPublication:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onJudgmentPublication(ctx, stream, msg, peerID)
	case CE192_VoteMessage:
		return n.onVoteMessage(ctx, stream, msg)
	case CE193_CommitMessage:
		return n.onCommitMessage(ctx, stream, msg)
	case CE194_NeighborMessage:
		//return n.onNeighborMessage(ctx, stream, msg)
	case CE195_Catchup:
		//return n.onCatchup(ctx, stream, msg)
	case CE224_BLSSignature:
		//return n.onBLSSignature(ctx, stream, msg)
	case CE225_BLSAggregateSignature:
		//return n.onBLSAggregateSignature(ctx, stream, msg)
	default:
		return errors.New("unknown message type")
	}
	return nil
}

func (n *Node) AbortStream(stream quic.Stream, code uint64) {
	if stream != nil {
		stream.CancelRead(quic.StreamErrorCode(code))
		stream.CancelWrite(quic.StreamErrorCode(code))
		_ = stream.Close()
	}
	log.Trace(module, "AbortStream", "code", code, "stream", stream)
}
