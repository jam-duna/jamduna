package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"

	//"bytes"
	"sync"
)

const (
	UP0_BlockAnnouncement        uint8 = iota
	CE101_VoteMessage                  = 101
	CE102_CommitMessage                = 102
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
)

type Peer struct {
	// these are initialized in NewPeer
	node      *NodeContent
	PeerID    uint16          `json:"peer_id"`
	PeerAddr  string          `json:"peer_addr"`
	Validator types.Validator `json:"validator"`

	// these are established early on but may change
	connectionMu sync.Mutex
	conn         quic.Connection

	// TODO: UP0 will keep this
	//stream quic.Stream
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
		node:      &n.NodeContent,
		PeerAddr:  peerAddr,
		Validator: validator,
		PeerID:    validatorIndex,
	}
	return p
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
			return nil, fmt.Errorf("DialAddr failed: %w", err)
		}
	}

	// Open stream with context
	stream, err := p.conn.OpenStreamSync(ctx)
	if err != nil {
		fmt.Printf("[%s] OpenStreamSync ERR %v\n", p.PeerAddr, err)
		_ = p.conn.CloseWithError(0, "stream open failed")
		p.conn = nil
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
		return fmt.Errorf("onPreimageRequest: context cancelled before send: %w", ctx.Err())
	default:
	}

	// First, write the message length to the stream
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Error(module, "!!! sendQuicBytes-length", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		return err
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
		log.Error(module, "!!! sendQuicBytes-msg", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen, "msg", common.Bytes2Hex(msg))
		return err
	}

	return nil
}

func receiveMultiple(ctx context.Context, stream quic.Stream, count int, peerID uint16, code uint8) ([][]byte, error) {
	results := make([][]byte, 0, count)

	for i := 0; i < count; i++ {
		data, err := receiveQuicBytes(ctx, stream, peerID, code)
		if err != nil {
			return nil, fmt.Errorf("receiveQuicBytes[%d/%d]: %w", i+1, count, err)
		}
		results = append(results, data)
	}

	return results, nil
}
func receiveQuicBytes(ctx context.Context, stream quic.Stream, peerID uint16, code uint8) ([]byte, error) {
	var lengthPrefix [4]byte

	// Apply context deadline if it exists
	/*if deadline, ok := ctx.Deadline(); ok {
		//log.Info(module, "receiveQuicBytes: applying deadline", "peerID", peerID, "deadline", deadline, "now", time.Now())
		_ = stream.SetReadDeadline(deadline)
		defer stream.SetReadDeadline(time.Time{}) // clear deadline afterward
	}*/

	// Read length prefix
	if _, err := io.ReadFull(stream, lengthPrefix[:]); err != nil {
		//log.Error(module, "!!! receiveQuicBytes-length", "peerID", peerID, "err", err, "code", code)
		return nil, err
	}

	msgLen := binary.LittleEndian.Uint32(lengthPrefix[:])
	buf := make([]byte, msgLen)

	// Read message body
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Error(module, "!!! receiveQuicBytes-msg", "peerID", peerID, "err", err, "code", code, "msgLen", msgLen)
		return nil, err
	}
	return buf, nil
}

// jamsnp_dispatch reads from QUIC and dispatches based on message type
// TODO: use ctx and support cancellation/timeouts!
func (n *Node) DispatchIncomingQUICStream(ctx context.Context, stream quic.Stream, peerID uint16) error {
	var msgType byte

	msgTypeBytes := make([]byte, 1) // code

	_, err := io.ReadFull(stream, msgTypeBytes)
	if err != nil {
		log.Error(module, "DispatchIncomingQUICStream - code", "err", err)
		return err
	}
	msgType = msgTypeBytes[0]
	msgLenBytes := make([]byte, 4)
	_, err = io.ReadFull(stream, msgLenBytes)
	if err != nil {
		log.Error(module, "DispatchIncomingQUICStream - msgLen", "err", err)
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)

	msg := make([]byte, msgLen)
	_, err = io.ReadFull(stream, msg)
	if err != nil {
		log.Error(module, "DispatchIncomingQUICStream - msg", "peerID", peerID, "code", msgType)
		return err
	}

	// Dispatch based on msgType
	switch msgType {
	case UP0_BlockAnnouncement:
		err := n.onBlockAnnouncement(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "UP0_BlockAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE101_VoteMessage:
		err := n.onVoteMessage(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE101_VoteMessage", "n", n.id, "p", peerID, "err", err)
		}
	case CE102_CommitMessage:
		err := n.onCommitMessage(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE102_CommitMessage", "n", n.id, "p", peerID, "err", err)
		}
	case CE128_BlockRequest:
		err := n.onBlockRequest(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE128_BlockRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE129_StateRequest:
		err := n.onStateRequest(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE129_StateRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE131_TicketDistribution, CE132_TicketDistribution:
		err := n.onTicketDistribution(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE131_TicketDistribution, CE132_TicketDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE133_WorkPackageSubmission:
		err := n.onWorkPackageSubmission(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE133_WorkPackageSubmission", "n", n.id, "p", peerID, "err", err)
		}
	case CE134_WorkPackageShare:
		err := n.onWorkPackageShare(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE134_WorkPackageShare", "n", n.id, "p", peerID, "err", err)
		}
	case CE135_WorkReportDistribution:
		err := n.onWorkReportDistribution(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE135_WorkReportDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE136_WorkReportRequest:
		err := n.onWorkReportRequest(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE136_WorkReportRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE137_FullShardRequest:
		err := n.onFullShardRequest(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE137_FullShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE138_BundleShardRequest:
		err := n.onBundleShardRequest(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE138_BundleShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE139_SegmentShardRequest:
		err := n.onSegmentShardRequest(ctx, stream, msg, false)
		if err != nil {
			log.Warn(debugStream, "CE139_SegmentShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE140_SegmentShardRequestP:
		err := n.onSegmentShardRequest(ctx, stream, msg, true)
		if err != nil {
			log.Warn(debugStream, "CE140_SegmentShardRequestP", "n", n.id, "p", peerID, "err", err)
		}
	case CE141_AssuranceDistribution:
		err := n.onAssuranceDistribution(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE141_AssuranceDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE142_PreimageAnnouncement:
		err := n.onPreimageAnnouncement(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE142_PreimageAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE143_PreimageRequest:
		err := n.onPreimageRequest(ctx, stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE143_PreimageRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE144_AuditAnnouncement:
		err := n.onAuditAnnouncement(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE144_AuditAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE145_JudgmentPublication:
		err := n.onJudgmentPublication(ctx, stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE145_JudgmentPublication", "n", n.id, "p", peerID, "err", err)
		}
	default:
		return errors.New("unknown message type")
	}
	return nil
}

func jamnp_test_vector(ce string, testVectorName string, b []byte, obj interface{}) {
	write_jamnp_test_vector(ce, "request", testVectorName, b, obj)
}
