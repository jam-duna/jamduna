package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"

	//"bytes"
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
	CE201_DA_Announcement              = 201
	CE202_DA_Request                   = 202
	CE203_DA_Reconstruction            = 203
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

	if p.conn == nil {
		//fmt.Printf("openStream: connecting %s\n", p.PeerAddr)
		p.conn, err = quic.DialAddr(context.Background(), p.PeerAddr, p.node.tlsConfig, GenerateQuicConfig())
		//TODO defer p.connectionMu.Unlock()
		if err != nil {
			fmt.Printf("-- openStream ERR %v peerAddr=%s\n", err, p.PeerAddr)
			return nil, err
		}
	}
	stream, err = p.conn.OpenStreamSync(context.Background())
	if err != nil {
		// Error opening stream, remove the connection from the cache
		//defer n.connectionMu.Unlock()
		//p.connectionMu.Lock()
		fmt.Printf("OpenStreamSync ERR %v\n", err)
		p.conn = nil
		return nil, err
	}

	//fmt.Printf("OpenStream wrote CODE=%d\n", code)
	_, err = stream.Write([]byte{code})
	if err != nil {
		fmt.Printf("Write -- ERR %v\n", err)
	}

	// Additional check for potential EOF issue
	if err == io.EOF {
		fmt.Println("EOF encountered during write.")
		return
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
		log.Println("Error writing message length:", err)
		return err
	}

	// Then, write the actual message to the stream
	_, err = stream.Write(msg)
	if err != nil {
		log.Println("Error writing message:", err)
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
	_, err := stream.Read(msgTypeBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream1 ERR %v\n", err)
		return err
	}
	msgType = msgTypeBytes[0]
	// fmt.Printf("%s DispatchIncomingQUICStream from %d bytesRead=%d CODE=%d\n", n.String(), peerID, nRead, msgType)

	_, err = stream.Read(msgLenBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream2 ERR %v\n", err)
		return err
	}
	msgLen := binary.BigEndian.Uint32(msgLenBytes)
	msg := make([]byte, msgLen)
	_, err = stream.Read(msg)
	//fmt.Printf("DispatchIncomingQUICStream3 nRead=%d msgLen=%d\n", nRead, msgLen)
	// Dispatch based on msgType
	switch msgType {
	case UP0_BlockAnnouncement:
		n.onBlockAnnouncement(stream, msg, peerID)
	case CE128_BlockRequest:
		n.onBlockRequest(stream, msg)
	case CE129_StateRequest:
		n.onStateRequest(stream, msg)
	case CE131_TicketDistribution, CE132_TicketDistribution:
		n.onTicketDistribution(stream, msg)
	case CE133_WorkPackageSubmission:
		n.onWorkPackageSubmission(stream, msg)
	case CE134_WorkPackageShare:
		n.onWorkPackageShare(stream, msg)
	case CE135_WorkReportDistribution:
		n.onWorkReportDistribution(stream, msg)
	case CE136_WorkReportRequest:
		n.onWorkReportRequest(stream, msg)
	case CE137_FullShardRequest:
		n.onFullShardRequest(stream, msg)
	case CE138_BundleShardRequest:
		n.onBundleShardRequest(stream, msg)
	case CE139_SegmentShardRequest:
		n.onSegmentShardRequest(stream, msg, false)
	case CE140_SegmentShardRequestP:
		n.onSegmentShardRequest(stream, msg, true)
	case CE141_AssuranceDistribution:
		n.onAssuranceDistribution(stream, msg, peerID)
	case CE142_PreimageAnnouncement:
		n.onPreimageAnnouncement(stream, msg, peerID)
	case CE143_PreimageRequest:
		n.onPreimageRequest(stream, msg)
	case CE144_AuditAnnouncement:
		n.onAuditAnnouncement(stream, msg, peerID)
	case CE145_JudgmentPublication:
		n.onJudgmentPublication(stream, msg, peerID)
	case CE201_DA_Announcement:
		n.onDA_Announcement(stream, msg)
	case CE202_DA_Request:
		n.onDA_Request(stream, msg)
	case CE203_DA_Reconstruction:
		n.onDA_Reconstruction(stream, msg)
	default:
		return errors.New("unknown message type")
	}
	return nil
}
