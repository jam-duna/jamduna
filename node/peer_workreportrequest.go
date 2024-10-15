package node

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
	"reflect"
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

func (p *Peer) SendWorkReportRequest(workReportHash common.Hash) (workReport types.WorkReport, err error) {
	stream, err := p.openStream(CE136_WorkReportRequest)
	err = sendQuicBytes(stream, workReportHash.Bytes())
	if err != nil {
		return workReport, err
	}
	workReportBytes, err := receiveQuicBytes(stream)
	if err != nil {
		return workReport, err
	}

	wr, _, err := types.Decode(workReportBytes, reflect.TypeOf(types.WorkReport{}))
	if err != nil {
		return workReport, err
	}
	workReport = wr.(types.WorkReport)

	return workReport, nil
}

func (n *Node) onWorkReportRequest(stream quic.Stream, msg []byte) (err error) {
	h := common.BytesToHash(msg)
	workReport, ok, err := n.WorkReportLookup(h)
	if err != nil {
		return err
	}
	if !ok {
		stream.Close()
		return nil
	}

	err = sendQuicBytes(stream, workReport.Bytes())
	if err != nil {
		return err
	}

	return nil
}
