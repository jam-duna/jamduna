package node

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/quic-go/quic-go"
)

const (
	CE128_BlockRequest           = 128
	CE129_StateRequest           = 129
	CE131_TicketDistribution     = 131
	CE132_TicketDistribution     = 132
	CE133_WorkPackageSubmission  = 133
	CE134_WorkPackageShare       = 134
	CE135_WorkReportDistribution = 135
	CE136_WorkReportRequest      = 136
	CE137_ShardRequest           = 137
	CE138_ShardRequest           = 138
	CE139_SegmentShardRequest    = 139
	CE140_SegmentShardRequest    = 140
	CE141_AssuranceDistribution  = 141
	CE142_PreimageAnnouncement   = 142
	CE143_PreimageRequest        = 143
	CE144_AuditAnnouncement      = 144
	CE145_JudgmentPublication    = 145
)

type Peer struct {
	node           *Node
	peerIdentifer  string
	validatorIndex uint16
}

func (p *Peer) sendCode(code byte) (err error) {
	return nil
}

func (p *Peer) sendQuicBytes(msg []byte) (err error) {
	return nil
}

func (p *Peer) receiveQuicBytes() (resp []byte, err error) {
	return []byte{}, nil
}

func (p *Peer) sendFIN() (err error) {
	return nil
}

func (p *Peer) receiveFIN() (resp []byte, err error) {
	return []byte{}, nil
}

// jamsnp_dispatch reads from QUIC and dispatches based on message type
func (p *Peer) jamsnp_dispatch(stream quic.Stream) error {
	var msgType byte
	var msgLen uint32

	// Read msgType (1 byte)
	if err := binary.Read(stream, binary.BigEndian, &msgType); err != nil {
		return err
	}

	// Read message length (4 bytes)
	if err := binary.Read(stream, binary.BigEndian, &msgLen); err != nil {
		return err
	}

	// Read message bytes
	msg := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, msg); err != nil {
		return err
	}

	// Dispatch based on msgType
	switch msgType {
	case 0:
		p.processBlockAnnouncement(msg)
	case 128:
		p.processBlockRequest(msg)
	case 129:
		p.processStateRequest(msg)
	case 0x83, 0x84:
		p.processTicketDistribution(msg)
	case 0x85:
		p.processWorkPackageSubmission(msg)
	case 0x87:
		p.processWorkReportDistribution(msg)
	case 0x88:
		p.processWorkReportRequest(msg)
	case 0x89:
		p.processShardRequest(msg, false)
	case 0x8A:
		p.processAuditShardRequest(msg, true)
	case 0x8B, 0x8C:
		p.processSegmentShardRequest(msg, false)
	case 0x8D:
		p.processAssuranceDistribution(msg)
	case 0x8E:
		p.processPreimageAnnouncement(msg)
	case 0x8F:
		p.processPreimageRequest(msg)
	case 0x90:
		p.processAuditAnnouncement(msg)
	case 0x91:
		p.processJudgmentPublication(msg)
	default:
		return errors.New("unknown message type")
	}

	return nil
}
