package node

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
	"io"
	"log"
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
	CE137_ShardRequest                 = 137
	CE138_ShardRequest                 = 138
	CE139_SegmentShardRequest          = 139
	CE140_SegmentShardRequest          = 140
	CE141_AssuranceDistribution        = 141
	CE142_PreimageAnnouncement         = 142
	CE143_PreimageRequest              = 143
	CE144_AuditAnnouncement            = 144
	CE145_JudgmentPublication          = 145
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

	_, err = stream.Write([]byte{byte(p.node.id)})
	if err != nil {
		fmt.Printf("Write -- ERR %v\n", err)
	}

	//fmt.Printf("Write successful, bytes written: %d\n", numBytes)

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
func (n *Node) DispatchIncomingQUICStream(stream quic.Stream) error {
	var msgType byte

	msgTypeBytes := make([]byte, 2) // code  + validatorIndex
	msgLenBytes := make([]byte, 4)
	_, err := stream.Read(msgTypeBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream1 ERR %v\n", err)
		return err
	}
	msgType = msgTypeBytes[0]
	peerID := uint16(msgTypeBytes[1])
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
	case CE137_ShardRequest:
		n.onShardRequest(stream, msg, false)
	case CE138_ShardRequest:
		n.onAuditShardRequest(stream, msg, true)
	case CE139_SegmentShardRequest, CE140_SegmentShardRequest:
		n.onSegmentShardRequest(stream, msg, false)
	case CE141_AssuranceDistribution:
		n.onAssuranceDistribution(stream, msg, peerID)
	case CE142_PreimageAnnouncement:
		n.onPreimageAnnouncement(stream, msg, peerID)
	case CE143_PreimageRequest:
		n.onPreimageRequest(stream, msg)
	case CE144_AuditAnnouncement:
		n.onAuditAnnouncement(stream, msg)
	case CE145_JudgmentPublication:
		n.onJudgmentPublication(stream, msg, peerID)
	default:
		return errors.New("unknown message type")
	}
	return nil
}
