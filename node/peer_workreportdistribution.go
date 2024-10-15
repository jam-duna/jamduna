package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
	"io"
	"reflect"
)

/*
CE 135: Work-report distribution
Distribution of a fully guaranteed work-report ready for inclusion in a block.

After sharing a work-package received via CE 133 and getting back a signature from at least one other guarantor, a guaranteed work-report may be constructed and distributed.

Guaranteed work-reports should be distributed to all current validators, and during the last core rotation of an epoch, additionally to all validators for the next epoch. Note that these validator sets are likely to overlap.

Once in possession of two signatures for a work-report, the third guarantor should be given a reasonable amount of time (e.g. two seconds) to produce an additional signature before the guaranteed work-report is distrubuted.

Work Report = As in GP
Slot = u32
Validator Index = u16
Ed25519 Signature = [u8; 64]
Guaranteed Work Report = Work Report ++ Slot ++ len++[Validator Index ++ Ed25519 Signature]

Guarantor -> Validator

--> Guaranteed Work Report
--> FIN
<-- FIN
*/

type JAMSNPWorkReport struct {
	Slot        uint32                      `json:"slot"`
	Len         uint8                       `json:"len"`
	Credentials []types.GuaranteeCredential `json:"guarantee"`
	WorkReport  types.WorkReport            `json:"work_report"`
}

func (wr *JAMSNPWorkReport) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize Slot (4 bytes)
	if err := binary.Write(buf, binary.BigEndian, wr.Slot); err != nil {
		return nil, err
	}

	// Serialize Len (1 byte)
	if err := buf.WriteByte(wr.Len); err != nil {
		return nil, err
	}

	// Serialize Credentials (dynamically sized)
	for _, cred := range wr.Credentials {
		credBytes, err := cred.ToBytes()
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(credBytes); err != nil {
			return nil, err
		}
	}

	// Serialize WorkReport
	workReportBytes := wr.WorkReport.Bytes()
	if _, err := buf.Write(workReportBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
func (wr *JAMSNPWorkReport) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize Slot (4 bytes)
	if err := binary.Read(buf, binary.BigEndian, &wr.Slot); err != nil {
		return err
	}

	// Deserialize Len (1 byte)
	lenByte, err := buf.ReadByte()
	if err != nil {
		return err
	}
	wr.Len = lenByte

	// Deserialize Credentials (dynamically sized)
	for i := 0; i < int(wr.Len); i++ {
		var cred types.GuaranteeCredential
		credData := make([]byte, 66) // Assuming 66 bytes for GuaranteeCredential
		if _, err := io.ReadFull(buf, credData); err != nil {
			return err
		}
		if err := cred.FromBytes(credData); err != nil {
			return err
		}
		wr.Credentials = append(wr.Credentials, cred)
	}

	// Deserialize WorkReport (assuming it knows its own length)
	workReportData := make([]byte, buf.Len()) // Read remaining bytes for WorkReport
	if _, err := io.ReadFull(buf, workReportData); err != nil {
		return err
	}
	r, _, err := types.Decode(workReportData, reflect.TypeOf(types.WorkReport{}))
	if err != nil {
		return err
	}
	wr.WorkReport = r.(types.WorkReport)

	return nil
}

func (p *Peer) SendWorkReportDistribution(wr types.WorkReport, slot uint32, credentials []types.GuaranteeCredential) (err error) {
	stream, err := p.openStream(CE135_WorkReportDistribution)
	newReq := JAMSNPWorkReport{
		Slot:        slot,
		Len:         uint8(len(credentials)),
		Credentials: credentials,
		WorkReport:  wr,
	}
	reqBytes, err := newReq.ToBytes()
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) onWorkReportDistribution(stream quic.Stream, msg []byte) (err error) {
	var newReq JAMSNPWorkReport
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	workReport := newReq.WorkReport
	guarantee := types.Guarantee{
		Report:     workReport,
		Slot:       newReq.Slot,
		Signatures: newReq.Credentials,
		// ValidatorIndex: n.lookupPeer(stream)
	}
	n.guaranteesCh <- guarantee
	n.workReportsCh <- workReport
	return nil
}
