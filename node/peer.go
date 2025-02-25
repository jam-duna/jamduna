package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

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
	CE201_DA_Announcement              = 201
	CE202_DA_Request                   = 202
	CE203_DA_Reconstruction            = 203
	CE204_DA_Announcemented            = 204
)

type Peer struct {
	// these are initialized in NewPeer
	node      *Node
	PeerID    uint16          `json:"peer_id"`
	PeerAddr  string          `json:"peer_addr"`
	Validator types.Validator `json:"validator"`

	// these are established early on but may change
	connectionMu sync.Mutex
	conn         quic.Connection

	// TODO: UP0 will keep this
	//stream quic.Stream
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
		node:      n,
		PeerAddr:  peerAddr,
		Validator: validator,
		PeerID:    validatorIndex,
	}
	return p
}
func (p *Peer) String() string {
	return fmt.Sprintf("[N%d => %d]", p.node.id, p.PeerID)
}

func (p *Peer) openStream(code uint8) (stream quic.Stream, err error) {
	p.node.openedStreamsMu.Lock()
	defer p.node.openedStreamsMu.Unlock()

	if p.conn == nil {
		p.conn, err = quic.DialAddr(context.Background(), p.PeerAddr, p.node.tlsConfig, GenerateQuicConfig())
		//TODO defer p.connectionMu.Unlock()
		if err != nil {
			fmt.Printf("-- openStream ERR %v peerAddr=%s\n", err, p.PeerAddr)
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	stream, err = p.conn.OpenStreamSync(ctx)
	if err != nil {
		// Error opening stream, remove the connection from the cache
		//defer n.connectionMu.Unlock()
		//p.connectionMu.Lock()
		fmt.Printf("[%s] OpenStreamSync ERR %v\n", p.PeerAddr, err)
		if p.conn != nil {
			if closeErr := p.conn.CloseWithError(0, "stream open failed"); closeErr != nil {
				fmt.Printf("Failed to close connection: %v\n", closeErr)
			}
			p.conn = nil
		}
		return nil, err
	}

	defer func() {
		if err != nil || ctx.Err() != nil {
			if closeErr := stream.Close(); closeErr != nil {
				fmt.Printf("Failed to close stream: %v\n", closeErr)
			}
			if p.conn != nil {
				if closeErr := p.conn.CloseWithError(0, "error occurred"); closeErr != nil {
					fmt.Printf("Failed to close connection: %v\n", closeErr)
				}
				p.conn = nil
			}
		}
	}()

	_, err = stream.Write([]byte{code})
	if err != nil {
		if err == io.EOF {
			fmt.Println("EOF encountered during write.")
		}

		_ = stream.Close()
		if p.conn != nil {
			_ = p.conn.CloseWithError(0, "error occurred")
			p.conn = nil
		}
		return nil, err
	}
	return stream, nil
}

func sendQuicBytes(stream quic.Stream, msg []byte) (err error) {
	// Create a buffer to hold the length of the message (big-endian uint32)
	msgLen := uint32(len(msg))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, msgLen)
	// First, write the message length to the stream
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Error(debugStream, "sendQuicBytes", "err", err)
		return err
	}

	// Then, write the actual message to the stream
	_, err = stream.Write(msg)
	if err != nil {
		log.Error(debugStream, "sendQuicBytes", "err", err)
		return err
	}

	return nil
}

func receiveQuicBytes(stream quic.Stream) (resp []byte, err error) {

	var lengthPrefix [4]byte
	_, err = io.ReadFull(stream, lengthPrefix[:])
	if err != nil {
		return
	}
	messageLength := binary.BigEndian.Uint32(lengthPrefix[:])
	buf := make([]byte, messageLength)
	_, err = io.ReadFull(stream, buf)
	if err != nil {
		return
	}
	return buf, nil
}

// jamsnp_dispatch reads from QUIC and dispatches based on message type
func (n *Node) DispatchIncomingQUICStream(stream quic.Stream, peerID uint16) error {
	var msgType byte

	msgTypeBytes := make([]byte, 1) // code

	msgLenBytes := make([]byte, 4)
	_, err := io.ReadFull(stream, msgTypeBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream1 ERR %v\n", err)
		return err
	}
	msgType = msgTypeBytes[0]

	_, err = io.ReadFull(stream, msgLenBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream2 ERR %v\n", err)
		return err
	}
	msgLen := binary.BigEndian.Uint32(msgLenBytes)
	msg := make([]byte, msgLen)
	_, err = io.ReadFull(stream, msg)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream3 ERR %v\n", err)
		return err
	}
	// Dispatch based on msgType
	switch msgType {
	case UP0_BlockAnnouncement:
		err := n.onBlockAnnouncement(stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "UP0_BlockAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE101_VoteMessage:
		err := n.onVoteMessage(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE101_VoteMessage", "n", n.id, "p", peerID, "err", err)
		}
	case CE102_CommitMessage:
		err := n.onCommitMessage(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE102_CommitMessage", "n", n.id, "p", peerID, "err", err)
		}
	case CE128_BlockRequest:
		err := n.onBlockRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE128_BlockRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE129_StateRequest:
		err := n.onStateRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE129_StateRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE131_TicketDistribution, CE132_TicketDistribution:
		err := n.onTicketDistribution(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE131_TicketDistribution, CE132_TicketDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE133_WorkPackageSubmission:
		err := n.onWorkPackageSubmission(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE133_WorkPackageSubmission", "n", n.id, "p", peerID, "err", err)
		}
	case CE134_WorkPackageShare:
		err := n.onWorkPackageShare(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE134_WorkPackageShare", "n", n.id, "p", peerID, "err", err)
		}
	case CE135_WorkReportDistribution:
		err := n.onWorkReportDistribution(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE135_WorkReportDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE136_WorkReportRequest:
		err := n.onWorkReportRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE136_WorkReportRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE137_FullShardRequest:
		err := n.onFullShardRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE137_FullShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE138_BundleShardRequest:
		err := n.onBundleShardRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE138_BundleShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE139_SegmentShardRequest:
		err := n.onSegmentShardRequest(stream, msg, false)
		if err != nil {
			log.Warn(debugStream, "CE139_SegmentShardRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE140_SegmentShardRequestP:
		err := n.onSegmentShardRequest(stream, msg, true)
		if err != nil {
			log.Warn(debugStream, "CE140_SegmentShardRequestP", "n", n.id, "p", peerID, "err", err)
		}
	case CE141_AssuranceDistribution:
		err := n.onAssuranceDistribution(stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE141_AssuranceDistribution", "n", n.id, "p", peerID, "err", err)
		}
	case CE142_PreimageAnnouncement:
		err := n.onPreimageAnnouncement(stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE142_PreimageAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE143_PreimageRequest:
		err := n.onPreimageRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE143_PreimageRequest", "n", n.id, "p", peerID, "err", err)
		}
	case CE144_AuditAnnouncement:
		err := n.onAuditAnnouncement(stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE144_AuditAnnouncement", "n", n.id, "p", peerID, "err", err)
		}
	case CE145_JudgmentPublication:
		err := n.onJudgmentPublication(stream, msg, peerID)
		if err != nil {
			log.Warn(debugStream, "CE145_JudgmentPublication", "n", n.id, "p", peerID, "err", err)
		}
	case CE201_DA_Announcement:
		err := n.onDA_Announcement(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE201_DA_Announcement", "n", n.id, "p", peerID, "err", err)
		}
	case CE202_DA_Request:
		err := n.onDA_Request(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE202_DA_Request", "n", n.id, "p", peerID, "err", err)
		}
	case CE203_DA_Reconstruction:
		err := n.onDA_Reconstruction(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE203_DA_Reconstruction", "n", n.id, "p", peerID, "err", err)
		}
	case CE204_DA_Announcemented:
		err := n.onDA_Announced(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE204_DA_Announcemented", "n", n.id, "p", peerID, "err", err)
		}
	default:
		return errors.New("unknown message type")
	}
	return nil
}
