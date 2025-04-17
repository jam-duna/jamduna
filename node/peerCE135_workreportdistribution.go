package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"reflect"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
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
	if err := binary.Write(buf, binary.LittleEndian, wr.Slot); err != nil {
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
	WR := wr.WorkReport
	workReportBytes := WR.Bytes()
	if _, err := buf.Write(workReportBytes); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (wr *JAMSNPWorkReport) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)
	// Deserialize Slot (4 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &wr.Slot); err != nil {
		return fmt.Errorf("Error deserializing Slot: %v", err)
	}
	// Deserialize Len (1 byte)
	lenByte, err := buf.ReadByte()
	if err != nil {
		return fmt.Errorf("Error deserializing Len: %v", err)
	}
	wr.Len = lenByte

	// Deserialize Credentials (dynamically sized)
	wr.Credentials = make([]types.GuaranteeCredential, wr.Len)
	for i := 0; i < int(wr.Len); i++ {
		var cred types.GuaranteeCredential
		credData := make([]byte, 66) // VERIFIED 66 bytes for GuaranteeCredential
		if _, err := io.ReadFull(buf, credData); err != nil {
			return fmt.Errorf("Error reading GuaranteeCredential: %v", err)
		}
		if err := cred.FromBytes(credData); err != nil {
			return fmt.Errorf("Error deserializing GuaranteeCredential: %v", err)
		}
		wr.Credentials[i] = cred
	}

	// Deserialize WorkReport (assuming it knows its own length)
	workReportBytes := make([]byte, buf.Len()) // Read remaining bytes for WorkReport
	if _, err := io.ReadFull(buf, workReportBytes); err != nil {
		return fmt.Errorf("Error reading WorkReport: %v", err)
	}

	workReportRaw, _, err := types.Decode(workReportBytes, reflect.TypeOf(types.WorkReport{}))
	if err != nil {
		return fmt.Errorf("WorkReport Decode Error: %v", err)
	}
	wr.WorkReport = workReportRaw.(types.WorkReport)
	return nil
}

func (p *Peer) SendWorkReportDistribution(
	ctx context.Context,
	wr types.WorkReport,
	slot uint32,
	credentials []types.GuaranteeCredential,
) error {
	code := uint8(CE135_WorkReportDistribution)

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendWorkReportDistribution", p.node.store.NodeID))
		defer span.End()
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE135_WorkReportDistribution]: %w", err)
	}
	defer stream.Close()

	newReq := JAMSNPWorkReport{
		Slot:        slot,
		Len:         uint8(len(credentials)),
		Credentials: credentials,
		WorkReport:  wr,
	}
	reqBytes, err := newReq.ToBytes()
	if err != nil {
		return fmt.Errorf("ToBytes[CE135_WorkReportDistribution]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[CE135_WorkReportDistribution]: %w", err)
	}

	return nil
}

func (n *Node) onWorkReportDistribution(ctx context.Context, stream quic.Stream, msg []byte) error {
	defer stream.Close()

	var newReq JAMSNPWorkReport
	if err := newReq.FromBytes(msg); err != nil {
		log.Error(debugG, "onWorkReportDistribution", "err", err)
		return fmt.Errorf("onWorkReportDistribution: failed to decode message: %w", err)
	}

	workReport := newReq.WorkReport
	guarantee := types.Guarantee{
		Report:     workReport,
		Slot:       newReq.Slot,
		Signatures: newReq.Credentials,
	}

	select {
	case n.guaranteesCh <- guarantee:
	case <-ctx.Done():
		log.Warn(debugG, "onWorkReportDistribution", "ctx", "canceled before sending guarantee")
		return ctx.Err()
	default:
		log.Warn(debugG, "onWorkReportDistribution", "msg", "guaranteesCh full, dropping guarantee")
	}

	log.Trace(debugG, "onWorkReportDistribution incoming Guarantee from Core on slot",
		"n", n.String(),
		"workReport", workReport.GetWorkPackageHash().String_short(),
		"guarantee.Slot", guarantee.Slot,
	)

	select {
	case n.workReportsCh <- workReport:
	case <-ctx.Done():
		log.Warn(debugG, "onWorkReportDistribution", "ctx", "canceled before sending work report")
		return ctx.Err()
	default:
		log.Warn(debugG, "onWorkReportDistribution", "msg", "workReportsCh full, dropping work report")
	}

	return nil
}
