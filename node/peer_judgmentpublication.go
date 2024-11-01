package node

import (
	"bytes"
	"encoding/binary"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"

	"io"
)

/*
CE 145: Judgment publication
Announcement of a judgment, ready for inclusion in a block and as a signal for potential further auditing.

An announcement declaring intention to audit a particular work-report must be followed by a judgment, declaring the work-report to either be valid or invalid, as soon as this has been determined.

Any judgments produced should also be broadcast to the validator set(s) for the epoch(s) following the block(s) in which the audited work-report was declared available. This is to ensure the judgments are available for block authors to include in the disputes extrinsic. For positive judgments, this broadcasting may optionally be deferred until a negative judgment for the work-report is observed (which may never happen).

On receipt of a new negative judgment for a work-report that the node is (potentially) responsible for auditing, the judgment should be forwarded to all other known auditors that are linked via the grid structure. The intent of this is to increase the likelihood that negative judgments are seen by all auditors.

Epoch Index = u32
Validator Index = u16
Validity = { 0 (Invalid), 1 (Valid) } (Single byte)
Work Report Hash = [u8; 32]
Ed25519 Signature = [u8; 64]

Auditor -> Validator

--> Epoch Index ++ Validator Index ++ Validity ++ Work Report Hash ++ Ed25519 Signature
--> FIN
<-- FIN
*/
type JAMSNPJudgmentPublication struct {
	Epoch          uint32                 `json:"epoch"`
	ValidatorIndex uint16                 `json:"validator_index"`
	Validity       uint8                  `json:"validity"`
	WorkReportHash common.Hash            `json:"work_report_hash"`
	Signature      types.Ed25519Signature `json:"signature"`
}

// ToBytes serializes the JAMSNPJudgmentPublication struct into a byte array
func (pub *JAMSNPJudgmentPublication) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize Epoch (4 bytes)
	if err := binary.Write(buf, binary.BigEndian, pub.Epoch); err != nil {
		return nil, err
	}

	// Serialize ValidatorIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, pub.ValidatorIndex); err != nil {
		return nil, err
	}

	// Serialize Validity (1 byte)
	if err := binary.Write(buf, binary.BigEndian, pub.Validity); err != nil {
		return nil, err
	}

	// Serialize WorkReportHash (32 bytes for common.Hash)
	if _, err := buf.Write(pub.WorkReportHash[:]); err != nil {
		return nil, err
	}

	// Serialize Signature (64 bytes for Ed25519Signature)
	if _, err := buf.Write(pub.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPJudgmentPublication struct
func (pub *JAMSNPJudgmentPublication) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize Epoch (4 bytes)
	if err := binary.Read(buf, binary.BigEndian, &pub.Epoch); err != nil {
		return err
	}

	// Deserialize ValidatorIndex (2 bytes)
	if err := binary.Read(buf, binary.BigEndian, &pub.ValidatorIndex); err != nil {
		return err
	}

	// Deserialize Validity (1 byte)
	if err := binary.Read(buf, binary.BigEndian, &pub.Validity); err != nil {
		return err
	}

	// Deserialize WorkReportHash (32 bytes)
	if _, err := io.ReadFull(buf, pub.WorkReportHash[:]); err != nil {
		return err
	}

	// Deserialize Signature (64 bytes)
	if _, err := io.ReadFull(buf, pub.Signature[:]); err != nil {
		return err
	}
	return nil
}

func (p *Peer) SendJudgmentPublication(epoch uint32, j types.Judgement) (err error) {
	//--> Epoch Index ++ Validator Index ++ Validity ++ Work Report Hash ++ Ed25519 Signature
	validity := uint8(0)
	if j.Judge {
		validity = 1
	}
	req := &JAMSNPJudgmentPublication{
		Epoch:          epoch,
		ValidatorIndex: j.Validator,
		Validity:       validity,
		WorkReportHash: j.WorkReportHash,
	}
	copy(req.Signature[:], j.Signature[:])

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, err := p.openStream(CE145_JudgmentPublication)
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}

	return nil
}

// Node has received a JudgementPublication message to act on
func (n *Node) onJudgmentPublication(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()
	var jp JAMSNPJudgmentPublication
	err = jp.FromBytes(msg)
	if err != nil {
		return err
	}
	// <-- FIN

	judge := true
	if jp.Validity == 0 {
		judge = false
	}
	judgement := types.Judgement{
		//Epoch: jp.Epoch,
		Judge: judge,
		// TODO: Shawn CHECK
		WorkReportHash: jp.WorkReportHash,
		Validator:      jp.ValidatorIndex,
		Signature:      jp.Signature,
	}
	copy(judgement.Signature[:], jp.Signature[:])
	n.judgementsCh <- judgement

	return nil
}
