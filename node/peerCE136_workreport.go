package node

import (
	"context"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 136: Work-report request
Request for the work-report with the given hash.

This should be used to request missing work-reports on receipt of a new block; blocks directly contain only the hashes of included work-reports.

A node announcing a new block may be assumed to possess the referenced work-reports. Such nodes should thus be queried first for missing reports.

Work Report Hash = [u8; 32]
Work Report = As in GP

Node -> Node

--> Work Report Hash
--> FIN
<-- Work Report
<-- FIN
*/
func (p *Peer) SendWorkReportRequest(ctx context.Context, workReportHash common.Hash) (types.WorkReport, error) {
	code := uint8(CE136_WorkReportRequest)

	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendWorkReportRequest", p.node.store.NodeID))
		defer span.End()
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		return types.WorkReport{}, fmt.Errorf("openStream[CE136_WorkReportRequest]: %w", err)
	}
	defer stream.Close()

	if err := sendQuicBytes(ctx, stream, workReportHash.Bytes(), p.PeerID, code); err != nil {
		return types.WorkReport{}, fmt.Errorf("sendQuicBytes[CE136_WorkReportRequest]: %w", err)
	}

	workReportBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		return types.WorkReport{}, fmt.Errorf("receiveQuicBytes[CE136_WorkReportRequest]: %w", err)
	}

	decoded, _, err := types.Decode(workReportBytes, reflect.TypeOf(types.WorkReport{}))
	if err != nil {
		return types.WorkReport{}, fmt.Errorf("decode[WorkReport]: %w", err)
	}

	return decoded.(types.WorkReport), nil
}

func (n *NodeContent) onWorkReportRequest(ctx context.Context, stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	// --> Hash
	h := common.BytesToHash(msg)
	workReport, ok, err := n.WorkReportLookup(h)
	if err != nil {
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onWorkReportRequest: WorkReportLookup failed: %w", err)
	}
	if !ok {
		// work report not found; gracefully ignore
		return nil
	}

	// <-- WorkReport
	code := uint8(CE136_WorkReportRequest)
	respBytes := workReport.Bytes()
	err = sendQuicBytes(ctx, stream, respBytes, n.id, CE136_WorkReportRequest)
	if err != nil {
		return fmt.Errorf("onWorkReportRequest: sendQuicBytes failed: %w, code=%d", err, code)
	}

	// <-- FIN
	return nil
}
