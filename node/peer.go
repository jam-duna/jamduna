package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
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
	CE146_WPbundlesubmission           = 146
	CE147_BundleRequest                = 147
	CE148_SegmentRequest               = 148

	CE149_GrandpaVote     = 149
	CE150_GrandpaCommit   = 150
	CE151_GrandpaState    = 151
	CE152_GrandpaCatchUp  = 152
	CE153_WarpSyncRequest = 153

	CE154_EpochFinalized          = 154
	CE155_EpochAggregateSignature = 155

	useQuicDeadline = false
)

const maxConsecutiveFailures = 3

type Peer struct {
	// these are initialized in NewPeer
	node      *NodeContent
	PeerID    uint16          `json:"peer_id"`
	PeerAddr  string          `json:"peer_addr"`
	Validator types.Validator `json:"validator"`

	knownHashes []common.Hash

	// these are established early on but may change
	connectionMu        sync.Mutex
	conn                quic.Connection
	consecutiveFailures int
	lastFailureTime     time.Time
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

func (p *Peer) PeerKey() [32]byte {
	return [32]byte(p.Validator.Ed25519)
}

func (p *Peer) SanKey() string {
	return common.ToSAN(p.Validator.Ed25519[:])
}

func (p *Peer) SanKeyWithIP() string {
	return fmt.Sprintf("%s@%s", common.ToSAN(p.Validator.Ed25519[:]), p.PeerAddr)
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

func (p *Peer) recordFailure() {
	p.consecutiveFailures++
	p.lastFailureTime = time.Now()
}

func (p *Peer) resetFailures() {
	p.consecutiveFailures = 0
}

func (p *Peer) shouldBootConnection() bool {
	return p.consecutiveFailures >= maxConsecutiveFailures
}

func (p *Peer) openStream(ctx context.Context, code uint8) (quic.Stream, error) {
	p.connectionMu.Lock()
	defer p.connectionMu.Unlock()

	if p.conn != nil && p.shouldBootConnection() {
		log.Warn(log.Node, "Booting stale connection after consecutive failures", "peerKey", p.SanKey(), "failures", p.consecutiveFailures)
		_ = p.conn.CloseWithError(0, "Too many consecutive failures")
		p.conn = nil
		p.resetFailures()
	}

	var err error
	if p.conn == nil {
		eventID := p.node.telemetryClient.GetEventID(p.PeerKey())
		telemetryConnecting := false
		host, port, splitErr := net.SplitHostPort(p.PeerAddr)
		if splitErr != nil {
			log.Warn(log.Node, "openStream: failed to split peer address", "peerAddr", p.PeerAddr, "err", splitErr)
		} else if addrBytes, portNum, parseErr := telemetry.ParseTelemetryAddress(host, port); parseErr != nil {
			log.Warn(log.Node, "openStream: failed to parse peer address for telemetry", "peerAddr", p.PeerAddr, "err", parseErr)
		} else {

			p.node.telemetryClient.ConnectingOut(p.PeerKey(), addrBytes, portNum)
			telemetryConnecting = true
		}

		dialCtx := ctx
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			dialCtx, cancel = context.WithTimeout(ctx, NormalTimeout)
			defer cancel()
		}

		conn, err := quic.DialAddr(dialCtx, p.PeerAddr, p.node.clientTLSConfig, GenerateQuicConfig())
		if err != nil {
			if err.Error() != "Context cancelled" {
				log.Error(log.Node, "DialAddr failed", "node", p.node.id, "peerKey", p.SanKey(), "err", err)
			}
			if telemetryConnecting {
				p.node.telemetryClient.ConnectOutFailed(eventID, err.Error())
			}
			p.recordFailure()
			return nil, fmt.Errorf("[%s] DialAddr failed: %w", p.SanKey(), err)
		} else {
			go p.node.nodeSelf.handleConnection(conn)
			p.conn = conn
			p.resetFailures()
			if telemetryConnecting {
				p.node.telemetryClient.ConnectedOut(eventID)
			}
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
		p.recordFailure()
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
		p.recordFailure()
		return nil, fmt.Errorf("failed to write code byte: %w", err)
	}

	p.resetFailures()
	return stream, nil
}
func sendQuicBytes(ctx context.Context, stream quic.Stream, msg []byte, peerKey string, code uint8) (err error) {
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
		stream.Close()
		return fmt.Errorf("sendQuicBytes: context cancelled before send: %w", ctx.Err())
	default:
	}
	if code == UP0_BlockAnnouncement {
		stream.SetWriteDeadline(time.Now().Add(2 * time.Second))
	}
	// First, write the message length to the stream
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Warn(log.Node, "sendQuicBytes-length", "peerKey", peerKey, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		stream.Close()
		return err
	} else if code == UP0_BlockAnnouncement {
		// remove the deadline
		stream.SetWriteDeadline(time.Time{})
	}

	// Respect context before sending
	select {
	case <-ctx.Done():
		stream.Close()
		return fmt.Errorf("sendQuicBytes: context cancelled before send: %w", ctx.Err())
	default:
	}

	// Then, write the actual message to the stream
	_, err = stream.Write(msg)
	if err != nil {
		stream.Close()
		log.Warn(log.Node, "sendQuicBytes-msg", "peerKey", peerKey, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		return err
	}

	return nil
}

// receiveMultiple reads `count` messages from a QUIC stream and returns a slice of received byte slices.
func receiveMultiple(ctx context.Context, stream quic.Stream, count int, peerKey string, code uint8) ([][]byte, error) {
	results := make([][]byte, 0, count)

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			data, err := receiveQuicBytes(ctx, stream, peerKey, code)
			if err != nil {
				log.Warn(log.Node, "receiveMultiple", "peerKey", peerKey, "err", err, "code", code)
				return nil, fmt.Errorf("receiveQuicBytes[%d/%d]: %w", i+1, count, err)
			}
			results = append(results, data)
		}
	}

	return results, nil
}

func receiveQuicBytes(ctx context.Context, stream quic.Stream, peerKey string, code uint8) ([]byte, error) {
	var lengthPrefix [4]byte

	// Set read deadline from context if present
	if useQuicDeadline {
		if deadline, ok := ctx.Deadline(); ok {
			if err := stream.SetReadDeadline(deadline); err != nil {
				log.Error(log.Node, "SetReadDeadline failed", "peerKey", peerKey, "err", err, "code", code)
				return nil, err
			}
		}
	}

	// Read length prefix
	if _, err := io.ReadFull(stream, lengthPrefix[:]); err != nil {
		log.Error(log.Node, "receiveQuicBytes-length prefix", "peerKey", peerKey, "err", err, "code", code)
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lengthPrefix[:])
	buf := make([]byte, msgLen)

	/*  do we really need to do this AGA IN?? */
	if useQuicDeadline {
		if deadline, ok := ctx.Deadline(); ok {
			if err := stream.SetReadDeadline(deadline); err != nil {
				log.Error(log.Node, "SetReadDeadline failed", "peerKey", peerKey, "err", err, "code", code)
				return nil, err
			}
		}
	}

	// Read message body
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Error(log.Node, "receiveQuicBytes-message body", "peerKey", peerKey, "err", err, "code", code, "msgLen", msgLen)
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

// CECodeName returns a human-readable name for CE protocol codes
func CECodeName(code uint8) string {
	switch code {
	case UP0_BlockAnnouncement:
		return "UP0_BlockAnnouncement"
	case CE128_BlockRequest:
		return "CE128_BlockRequest"
	case CE129_StateRequest:
		return "CE129_StateRequest"
	case CE131_TicketDistribution:
		return "CE131_TicketDistribution"
	case CE132_TicketDistribution:
		return "CE132_TicketDistribution"
	case CE133_WorkPackageSubmission:
		return "CE133_WorkPackageSubmission"
	case CE134_WorkPackageShare:
		return "CE134_WorkPackageShare"
	case CE135_WorkReportDistribution:
		return "CE135_WorkReportDistribution"
	case CE136_WorkReportRequest:
		return "CE136_WorkReportRequest"
	case CE137_FullShardRequest:
		return "CE137_FullShardRequest"
	case CE138_BundleShardRequest:
		return "CE138_BundleShardRequest"
	case CE139_SegmentShardRequest:
		return "CE139_SegmentShardRequest"
	case CE140_SegmentShardRequestP:
		return "CE140_SegmentShardRequestP"
	case CE141_AssuranceDistribution:
		return "CE141_AssuranceDistribution"
	case CE142_PreimageAnnouncement:
		return "CE142_PreimageAnnouncement"
	case CE143_PreimageRequest:
		return "CE143_PreimageRequest"
	case CE144_AuditAnnouncement:
		return "CE144_AuditAnnouncement"
	case CE145_JudgmentPublication:
		return "CE145_JudgmentPublication"
	case CE146_WPbundlesubmission:
		return "CE146_WPbundlesubmission"
	case CE147_BundleRequest:
		return "CE147_BundleRequest"
	case CE148_SegmentRequest:
		return "CE148_SegmentRequest"
	case CE149_GrandpaVote:
		return "CE149_GrandpaVote"
	case CE150_GrandpaCommit:
		return "CE150_GrandpaCommit"
	case CE151_GrandpaState:
		return "CE151_GrandpaState"
	case CE152_GrandpaCatchUp:
		return "CE152_GrandpaCatchUp"
	case CE153_WarpSyncRequest:
		return "CE153_WarpSyncRequest"
	case CE154_EpochFinalized:
		return "CE154_EpochFinalized"
	case CE155_EpochAggregateSignature:
		return "CE155_EpochAggregateSignature"
	default:
		return fmt.Sprintf("Unknown(%d)", code)
	}
}

// DispatchIncomingQUICStream reads from QUIC and dispatches based on message type
// peerKey is the Ed25519 pubKey hex string of the peer
func (n *Node) DispatchIncomingQUICStream(ctx context.Context, stream quic.Stream, peerKey string) error {
	// Respect context by setting read deadline if present
	/*if deadline, ok := ctx.Deadline(); ok {
		if err := stream.SetReadDeadline(deadline); err != nil {
			log.Warn(log.Node, "SetReadDeadline failed", "err", err)
			return err
		}
	}*/
	// Read message type (1 byte)
	msgTypeBytes := make([]byte, 1)
	if _, err := io.ReadFull(stream, msgTypeBytes); err != nil {
		log.Trace(log.Node, "DispatchIncomingQUICStream - code", "err", err)
		_ = stream.Close()
		return err
	}
	msgType := msgTypeBytes[0]

	// Read message length (4 bytes)
	msgLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, msgLenBytes); err != nil {
		log.Trace(log.Node, "DispatchIncomingQUICStream - length prefix", "err", err)
		_ = stream.Close()
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)

	// Read message body
	msg := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, msg); err != nil {
		log.Trace(log.Node, "DispatchIncomingQUICStream - message body", "peerKey", peerKey, "code", msgType, "err", err)
		stream.CancelRead(ErrCECode)
		_ = stream.Close()
		return err
	}

	// Dispatch based on msgType
	var err error
	switch msgType {
	case UP0_BlockAnnouncement:
		return n.onBlockAnnouncement(stream, msg, peerKey)
	case CE128_BlockRequest:
		return n.onBlockRequest(ctx, stream, msg, peerKey)
	case CE129_StateRequest:
		err = n.onStateRequest(ctx, stream, msg)
	case CE131_TicketDistribution, CE132_TicketDistribution:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		return n.onTicketDistribution(ctx, stream, msg, peerKey, msgType)
	case CE133_WorkPackageSubmission:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onWorkPackageSubmission(ctx, stream, msg, peerKey)
	case CE146_WPbundlesubmission:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onBundleSubmission(ctx, stream, msg, peerKey)
	case CE134_WorkPackageShare:
		err = n.onWorkPackageShare(ctx, stream, msg, peerKey)
	case CE135_WorkReportDistribution:
		err = n.onWorkReportDistribution(ctx, stream, msg, peerKey)
	case CE136_WorkReportRequest:
		err = n.onWorkReportRequest(ctx, stream, msg)
	case CE137_FullShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onFullShardRequest(ctx, stream, msg, peerKey)
	case CE138_BundleShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onBundleShardRequest(ctx, stream, msg, peerKey)
	case CE139_SegmentShardRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onSegmentShardRequest(ctx, stream, msg, false, peerKey)
	case CE140_SegmentShardRequestP:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onSegmentShardRequest(ctx, stream, msg, true, peerKey)
	case CE141_AssuranceDistribution:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onAssuranceDistribution(ctx, stream, msg, peerKey)
	case CE142_PreimageAnnouncement:
		err = n.onPreimageAnnouncement(ctx, stream, msg, peerKey)
	case CE143_PreimageRequest:
		err = n.onPreimageRequest(ctx, stream, msg)
	case CE144_AuditAnnouncement:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onAuditAnnouncement(ctx, stream, msg, peerKey)
	case CE145_JudgmentPublication:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onJudgmentPublication(ctx, stream, msg, peerKey)
	case CE147_BundleRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onBundleRequest(ctx, stream, msg, peerKey)
	case CE148_SegmentRequest:
		if !n.GetIsSync() {
			n.AbortStream(stream, ErrStateNotSynced)
			return nil
		}
		err = n.onSegmentRequest(ctx, stream, msg, peerKey)

	case CE149_GrandpaVote:
		err = n.onGrandpaVote(ctx, stream, msg)
	case CE150_GrandpaCommit:
		err = n.onGrandpaCommit(ctx, stream, msg)
	case CE151_GrandpaState:
		err = n.onGrandpaState(ctx, stream, msg, peerKey)
	case CE152_GrandpaCatchUp:
		err = n.onGrandpaCatchUp(ctx, stream, msg, peerKey)
	case CE153_WarpSyncRequest:
		err = n.onWarpSyncRequest(ctx, stream, msg, peerKey)
	case CE154_EpochFinalized:
		err = n.onEpochFinalized(ctx, stream, msg, peerKey)
	case CE155_EpochAggregateSignature:
		err = n.onEpochAggregateSignature(ctx, stream, msg, peerKey)
	default:
		return fmt.Errorf("unknown message Type msgType=%d", msgType)
	}

	// Wrap error with message type for better debugging
	if err != nil {
		return fmt.Errorf("[%s] %w", CECodeName(msgType), err)
	}
	return nil
}

func (n *Node) AbortStream(stream quic.Stream, code uint64) {
	if stream != nil {
		stream.CancelRead(quic.StreamErrorCode(code))
		stream.CancelWrite(quic.StreamErrorCode(code))
		_ = stream.Close()
	}
	log.Trace(log.Node, "AbortStream", "code", code, "stream", stream)
}
