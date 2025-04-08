package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 144: Audit announcement
Announcement of requirement to audit.

Auditors of a block (defined to be the posterior validator set) should, at the beginning of each tranche, broadcast an announcement to all other such auditors specifying which work-reports they intend to audit, unless they do not intend to audit any work-reports, in which case no announcement should be sent.

An announcement contains a list of work-reports as well as evidence backing up the announcer's decision to audit them. In combination with the block being audited and the prior state, the evidence should be sufficient for recipients of the announcement to verify the audit requirement claim. Note that although the announcements for any claimed no-shows must be provided, there is no way to prove/verify that the judgments were not received; this must simply be accepted.

Header Hash = [u8; 32]
Tranche = u8
Core Index = u16
Work Report Hash = [u8; 32]
Reports = len++[Core Index ++ Work Report Hash]
Ed25519 Signature = [u8; 64]
Bandersnatch Signature = [u8; 96]
Validator Index = u16
No Shows = len++[Validator Index ++ Reports ++ Ed25519 Signature] (Previous tranche announcements)

Auditor -> Auditor

--> Header Hash ++ Tranche ++ Reports ++ Ed25519 Signature
[Tranche 0] --> Bandersnatch Signature (s_0 in GP)
[Tranche not 0] --> [Bandersnatch Signature (s_n(w) in GP) ++ No Shows] (One entry per WR in the first message)
--> FIN
<-- FIN
*/
type JAMSNPAuditAnnouncementWithProof struct {
	Announcement        JAMSNPAuditAnnouncement              `json:"announcement"`
	Evidence_s0         types.BandersnatchVrfSignature       `json:"evidence_s0"`
	Evidence_sn         []types.BandersnatchVrfSignature     `json:"evidence_sn"`
	NoShowLength        map[common.Hash]int                  `json:"no_show_length"`
	Evidence_sn_no_show map[common.Hash][]types.Announcement `json:"evidence_sn_no_show"`
}

type JAMSNPAuditAnnouncement struct {
	HeaderHash common.Hash                     `json:"headerHash"`
	Tranche    uint8                           `json:"tranche"`
	Len        uint8                           `json:"len"`
	Reports    []JAMSNPAuditAnnouncementReport `json:"reports"`
	Signature  types.Ed25519Signature          `json:"signature"`
}

func (ann *JAMSNPAuditAnnouncement) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize HeaderHash (32 bytes)
	if _, err := buf.Write(ann.HeaderHash[:]); err != nil {
		return nil, err
	}

	// Serialize Tranche (1 byte)
	if err := binary.Write(buf, binary.LittleEndian, ann.Tranche); err != nil {
		return nil, err
	}

	// Serialize Len (1 byte)
	ann.Len = uint8(len(ann.Reports)) // Set the length of reports before serializing
	if err := binary.Write(buf, binary.LittleEndian, ann.Len); err != nil {
		return nil, err
	}

	// Serialize Reports (dynamically sized)
	for _, report := range ann.Reports {
		reportBytes, err := report.ToBytes()
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(reportBytes); err != nil {
			return nil, err
		}
	}

	// Serialize Signature (64 bytes)
	if _, err := buf.Write(ann.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ann *JAMSNPAuditAnnouncement) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize HeaderHash (32 bytes)
	if _, err := io.ReadFull(buf, ann.HeaderHash[:]); err != nil {
		return err
	}

	// Deserialize Tranche (1 byte)
	if err := binary.Read(buf, binary.LittleEndian, &ann.Tranche); err != nil {
		return err
	}

	// Deserialize Len (1 byte)
	if err := binary.Read(buf, binary.LittleEndian, &ann.Len); err != nil {
		return err
	}

	// Deserialize Reports based on the length in Len
	ann.Reports = make([]JAMSNPAuditAnnouncementReport, 0, ann.Len)
	for i := uint8(0); i < ann.Len; i++ {
		var report JAMSNPAuditAnnouncementReport
		reportBytes := make([]byte, 34) // hash + 2 bytes
		if _, err := io.ReadFull(buf, reportBytes); err != nil {
			return err
		}
		if err := report.FromBytes(reportBytes); err != nil {
			return err
		}
		ann.Reports = append(ann.Reports, report)
	}

	// Deserialize Signature (64 bytes)
	if _, err := io.ReadFull(buf, ann.Signature[:]); err != nil {
		return err
	}

	return nil
}

type JAMSNPNoShow struct {
	ValidatorIndex uint16                          `json:"validator_index"`
	Reports        []JAMSNPAuditAnnouncementReport `json:"reports"`
}

func (noShow *JAMSNPNoShow) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ValidatorIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, noShow.ValidatorIndex); err != nil {
		return nil, err
	}

	// Serialize Reports (dynamically sized)
	for _, report := range noShow.Reports {
		reportBytes, err := report.ToBytes()
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(reportBytes); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (noShow *JAMSNPNoShow) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ValidatorIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &noShow.ValidatorIndex); err != nil {
		return err
	}

	// Deserialize Reports
	for buf.Len() > 64 { // We leave 64 bytes for the signature at the end
		var report JAMSNPAuditAnnouncementReport
		if err := report.FromBytes(data); err != nil {
			return err
		}
		noShow.Reports = append(noShow.Reports, report)
	}

	return nil
}

type JAMSNPAuditAnnouncementNot0 struct {
	Len       uint8                          `json:"len"`
	Signature types.BandersnatchVrfSignature `json:"signature"`
	NoShows   []JAMSNPNoShow                 `json:"noshows"`
}

func (ann *JAMSNPAuditAnnouncementNot0) ToBytes() ([]byte, error) {
	//Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
	buf := new(bytes.Buffer)
	buf.Write(ann.Signature[:])
	buf.Write([]byte{ann.Len})
	for _, noShow := range ann.NoShows {
		noShowBytes, err := noShow.ToBytes()
		if err != nil {
			return nil, err
		}
		buf.Write(noShowBytes)
	}
	return buf.Bytes(), nil
}

func (ann *JAMSNPAuditAnnouncementNot0) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)
	if _, err := io.ReadFull(buf, ann.Signature[:]); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.LittleEndian, &ann.Len); err != nil {
		return err
	}
	for buf.Len() > 0 {
		var noShow JAMSNPNoShow
		if err := noShow.FromBytes(data); err != nil {
			return err
		}
		ann.NoShows = append(ann.NoShows, noShow)
	}
	return nil
}

type JAMSNPAuditAnnouncementReport struct {
	CoreIndex      uint16      `json:"core_index"`
	WorkReportHash common.Hash `json:"work_report_hash"`
}

func (report *JAMSNPAuditAnnouncementReport) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, report.CoreIndex); err != nil {
		return nil, err
	}

	// Serialize WorkReportHash (32 bytes)
	if _, err := buf.Write(report.WorkReportHash[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (report *JAMSNPAuditAnnouncementReport) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize CoreIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &report.CoreIndex); err != nil {
		return err
	}

	// Deserialize WorkReportHash (32 bytes)
	if _, err := io.ReadFull(buf, report.WorkReportHash[:]); err != nil {
		return err
	}

	return nil
}
func (p *Peer) SendAuditAnnouncement(ctx context.Context, a *JAMSNPAuditAnnouncementWithProof) error {
	// Start span if tracing is enabled
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendAuditAnnouncement", p.node.store.NodeID))
		defer span.End()
	}

	code := uint8(CE144_AuditAnnouncement)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE144_AuditAnnouncement]: %w", err)
	}
	defer stream.Close()

	// Build the basic audit announcement
	reports := make([]JAMSNPAuditAnnouncementReport, len(a.Announcement.Reports))
	for i, report := range a.Announcement.Reports {
		reports[i] = JAMSNPAuditAnnouncementReport{
			CoreIndex:      report.CoreIndex,
			WorkReportHash: report.WorkReportHash,
		}
	}

	req := &JAMSNPAuditAnnouncement{
		HeaderHash: a.Announcement.HeaderHash,
		Tranche:    uint8(a.Announcement.Tranche),
		Len:        uint8(len(reports)),
		Reports:    reports,
		Signature:  a.Announcement.Signature,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("ToBytes[AUDIT_ANNOUNCEMENT]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[AUDIT_ANNOUNCEMENT]: %w", err)
	}

	if a.Announcement.Tranche == 0 {
		// Send s_0 signature
		if err := sendQuicBytes(ctx, stream, a.Evidence_s0[:], p.PeerID, code); err != nil {
			return fmt.Errorf("sendQuicBytes[EVIDENCE_S0]: %w", err)
		}
	} else {
		// Build and send evidence_sn+no-show blocks
		for i, report := range req.Reports {
			noShow := a.Evidence_sn_no_show[report.WorkReportHash]
			jamNoShows := make([]JAMSNPNoShow, len(noShow))

			for j, ns := range noShow {
				reports := make([]JAMSNPAuditAnnouncementReport, len(ns.Selected_WorkReport))
				for k, wr := range ns.Selected_WorkReport {
					reports[k] = JAMSNPAuditAnnouncementReport{
						CoreIndex:      wr.Core,
						WorkReportHash: wr.WorkReportHash,
					}
				}
				jamNoShows[j] = JAMSNPNoShow{
					ValidatorIndex: uint16(ns.ValidatorIndex),
					Reports:        reports,
				}
			}

			ev := JAMSNPAuditAnnouncementNot0{
				Len:       uint8(len(noShow)),
				Signature: a.Evidence_sn[i],
				NoShows:   jamNoShows,
			}

			evBytes, err := ev.ToBytes()
			if err != nil {
				return fmt.Errorf("ToBytes[AUDIT_NOT0_EVIDENCE #%d]: %w", i, err)
			}

			if err := sendQuicBytes(ctx, stream, evBytes, p.PeerID, code); err != nil {
				return fmt.Errorf("sendQuicBytes[AUDIT_NOT0_EVIDENCE #%d]: %w", i, err)
			}
		}
	}

	return nil
}

func AnnouncementToNoShow(a *types.Announcement) (JAMSNPNoShow, error) {
	var noShow JAMSNPNoShow
	noShow.ValidatorIndex = uint16(a.ValidatorIndex)
	noShow.Reports = make([]JAMSNPAuditAnnouncementReport, 0)
	for _, report := range a.Selected_WorkReport {
		var jam_ann_report JAMSNPAuditAnnouncementReport
		jam_ann_report.CoreIndex = report.Core
		jam_ann_report.WorkReportHash = report.WorkReportHash
		noShow.Reports = append(noShow.Reports, jam_ann_report)
	}
	return noShow, nil
}

func (n *Node) onAuditAnnouncement(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()
	code := uint8(CE144_AuditAnnouncement)

	var newReq JAMSNPAuditAnnouncement
	if err := newReq.FromBytes(msg); err != nil {
		return fmt.Errorf("onAuditAnnouncement: failed to deserialize announcement: %w", err)
	}

	// Convert reports to internal format
	selectedReports := make([]types.AnnouncementReport, len(newReq.Reports))
	for i, report := range newReq.Reports {
		selectedReports[i] = types.AnnouncementReport{
			Core:           report.CoreIndex,
			WorkReportHash: report.WorkReportHash,
		}
	}

	announcement := types.Announcement{
		HeaderHash:          newReq.HeaderHash,
		Tranche:             uint32(newReq.Tranche),
		Selected_WorkReport: selectedReports,
		ValidatorIndex:      uint32(peerID),
		Signature:           newReq.Signature,
	}
	n.announcementsCh <- announcement
	/*
		Bandersnatch Signature = [u8; 96]
		First Tranche Evidence = Bandersnatch Signature (s_0 in GP)
		No-Show = Validator Index ++ Announcement (From the previous tranche)
		Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
		Evidence = First Tranche Evidence (If tranche is 0) OR Subsequent Tranche Evidence (If tranche is not 0)
	*/
	if newReq.Tranche == 0 {
		var evidenceS0 types.BandersnatchVrfSignature
		evidenceBytes, err := receiveQuicBytes(ctx, stream, n.id, code)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: receive evidence_s0 failed: %w", err)
		}
		copy(evidenceS0[:], evidenceBytes)
		// TODO: verify evidenceS0
	} else {
		evidenceSN := make([]types.BandersnatchVrfSignature, 0, len(newReq.Reports))
		evidenceSNNoShow := make(map[common.Hash][]JAMSNPNoShow)

		for _, r := range newReq.Reports {
			evidenceBytes, err := receiveQuicBytes(ctx, stream, n.id, code)
			if err != nil {
				return fmt.Errorf("onAuditAnnouncement: receive evidence_sn for report %x failed: %w", r.WorkReportHash[:4], err)
			}

			var evNot0 JAMSNPAuditAnnouncementNot0
			if err := evNot0.FromBytes(evidenceBytes); err != nil {
				return fmt.Errorf("onAuditAnnouncement: decode evidence_sn not0: %w", err)
			}

			evidenceSN = append(evidenceSN, evNot0.Signature)
			evidenceSNNoShow[r.WorkReportHash] = append(evidenceSNNoShow[r.WorkReportHash], evNot0.NoShows...)
		}

		// TODO: verify evidenceSN and evidenceSNNoShow
	}

	return nil
}
