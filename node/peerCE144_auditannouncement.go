package node

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
Tranche = u8
Announcement = len++[Core Index ++ Work-Report Hash] ++ Ed25519 Signature

Bandersnatch Signature = [u8; 96]
First Tranche Evidence = Bandersnatch Signature (s_0 in GP)
No-Show = Validator Index ++ Announcement (From the previous tranche)
Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
Evidence = First Tranche Evidence (If tranche is 0) OR Subsequent Tranche Evidence (If tranche is not 0)

Auditor -> Auditor

--> Header Hash ++ Tranche ++ Announcement
--> Evidence
--> FIN
<-- FIN
*/
// First  Header Hash ++ Tranche ++ Announcement
type JAMSNPAuditAnnouncementReport struct {
	CoreIndex      uint16      `json:"core_index"`
	WorkReportHash common.Hash `json:"work_report_hash"`
}

type JAMSNPAuditAnnouncement struct {
	HeaderHash common.Hash                     `json:"headerHash"`
	Tranche    uint8                           `json:"tranche"`
	Reports    []JAMSNPAuditAnnouncementReport `json:"reports"`
	Signature  types.Ed25519Signature          `json:"signature"`
}

func (ann *JAMSNPAuditAnnouncement) ToBytes() ([]byte, error) {
	return types.Encode(ann)
}

func (ann *JAMSNPAuditAnnouncement) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(JAMSNPAuditAnnouncement{}))
	if err != nil {
		return fmt.Errorf("failed to decode JAMSNPAuditAnnouncement: %w", err)
	}
	decodedAnn := decoded.(JAMSNPAuditAnnouncement)
	*ann = decodedAnn
	return nil
}

type JAMSNPNoShow struct {
	ValidatorIndex uint16                          `json:"validator_index"`
	Reports        []JAMSNPAuditAnnouncementReport `json:"reports"`
	Signature      types.Ed25519Signature          `json:"signature"`
}
type SubsequentTrancheEvidence []TrancheEvidence
type TrancheEvidence struct {
	Signature types.BandersnatchVrfSignature `json:"signature"`
	NoShows   []JAMSNPNoShow                 `json:"no_shows"`
}

// TODO : Length!!!
func (s *SubsequentTrancheEvidence) ToBytes() ([]byte, error) {
	return types.Encode(s)
}

func (s *SubsequentTrancheEvidence) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(SubsequentTrancheEvidence{}))
	if err != nil {
		return fmt.Errorf("failed to decode SubsequentTrancheEvidence: %w", err)
	}
	decodedEvidence := decoded.(SubsequentTrancheEvidence)
	*s = decodedEvidence
	return nil
}

type Tranche0Evidence types.BandersnatchVrfSignature

func (t *Tranche0Evidence) ToBytes() ([]byte, error) {
	return types.Encode(t)
}
func (t *Tranche0Evidence) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(Tranche0Evidence{}))
	if err != nil {
		return fmt.Errorf("failed to decode Tranche0Evidence: %w", err)
	}
	decodedEvidence := decoded.(Tranche0Evidence)
	*t = decodedEvidence
	return nil
}

func (p *Peer) SendAuditAnnouncement(ctx context.Context, announcement JAMSNPAuditAnnouncement, evidence interface{}) error {
	// Start span if tracing is enabled
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(ctx, fmt.Sprintf("[N%d] SendAuditAnnouncement", p.node.store.NodeID))
		defer span.End()
	}
	log.Debug(module, "SendAuditAnnouncement", "peerID", p.PeerID, "headerHash", announcement.HeaderHash.String_short(), "tranche", announcement.Tranche)
	code := uint8(CE144_AuditAnnouncement)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return fmt.Errorf("openStream[CE144_AuditAnnouncement]: %w", err)
	}
	defer stream.Close()

	// Build the basic audit announcement
	reqBytes, err := announcement.ToBytes()
	if err != nil {
		return fmt.Errorf("ToBytes[AUDIT_ANNOUNCEMENT]: %w", err)
	}

	if err := sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		return fmt.Errorf("sendQuicBytes[AUDIT_ANNOUNCEMENT]: %w", err)
	}

	if announcement.Tranche == 0 {
		// Send evidence for tranche 0
		evidence, ok := evidence.(Tranche0Evidence)
		if !ok {
			return fmt.Errorf("SendAuditAnnouncement: evidence is not of type Tranche0Evidence")
		}
		evidenceBytes, err := evidence.ToBytes()
		if err != nil {
			return fmt.Errorf("ToBytes[Tranche0Evidence]: %w", err)
		}
		if err := sendQuicBytes(ctx, stream, evidenceBytes, p.PeerID, code); err != nil {
			return fmt.Errorf("sendQuicBytes[Tranche0Evidence]: %w", err)
		}
	} else {
		// Send evidence for subsequent tranches
		evidence, ok := evidence.(SubsequentTrancheEvidence)
		if !ok {
			return fmt.Errorf("SendAuditAnnouncement: evidence is not of type SubsequentTrancheEvidence")
		}
		evidenceBytes, err := evidence.ToBytes()
		if err != nil {
			return fmt.Errorf("ToBytes[SubsequentTrancheEvidence]: %w", err)
		}
		if err := sendQuicBytes(ctx, stream, evidenceBytes, p.PeerID, code); err != nil {
			return fmt.Errorf("sendQuicBytes[SubsequentTrancheEvidence]: %w", err)
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
		log.Warn(module, "onAuditAnnouncement: failed to deserialize announcement", "peerID", peerID, "error", err)
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

	/*
		Bandersnatch Signature = [u8; 96]
		First Tranche Evidence = Bandersnatch Signature (s_0 in GP)
		No-Show = Validator Index ++ Announcement (From the previous tranche)
		Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
		Evidence = First Tranche Evidence (If tranche is 0) OR Subsequent Tranche Evidence (If tranche is not 0)
	*/
	// step 1. get the data

	if newReq.Tranche == 0 {
		var evidenceS0 types.BandersnatchVrfSignature
		evidenceBytes, err := receiveQuicBytes(ctx, stream, n.id, code)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: receive evidence_s0 failed: %w", err)
		}
		copy(evidenceS0[:], evidenceBytes)
		// TODO: verify evidenceS0
		var audit_statedb *statedb.StateDB
		var ok bool
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				audit_statedb, ok = n.getStateDBByHeaderHash(newReq.HeaderHash)
				if ok {
					break
				}
			case <-ctx.Done():
				return fmt.Errorf("onAuditAnnouncement: audit statedb not found for header hash %s after timeout", newReq.HeaderHash.String_short())
			}
			if ok {
				break
			}
		}

		validator := audit_statedb.JamState.SafroleState.GetCurrValidator(int(peerID))
		bandersnatchPub := validator.Bandersnatch

		ok, err = audit_statedb.Verify_s0(bandersnatch.BanderSnatchKey(bandersnatchPub[:]), evidenceBytes)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: failed to verify evidenceS0: %w", err)
		}
		if !ok {
			return fmt.Errorf("onAuditAnnouncement: evidenceS0 verification failed")
		}
	} else {
		var evidenceSN SubsequentTrancheEvidence
		evidenceBytes, err := receiveQuicBytes(ctx, stream, n.id, code)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: receive evidence_sN failed: %w", err)
		}
		if err := evidenceSN.FromBytes(evidenceBytes); err != nil {
			return fmt.Errorf("onAuditAnnouncement: failed to deserialize evidenceSN: %w", err)
		}
		// TODO: verify evidenceSN and evidenceSNNoShow
		var audit_statedb *statedb.StateDB
		var ok bool
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				audit_statedb, ok = n.getStateDBByHeaderHash(newReq.HeaderHash)
				if ok {
					break
				}
			case <-ctx.Done():
				return fmt.Errorf("onAuditAnnouncement: audit statedb not found for header hash %s after timeout", newReq.HeaderHash.String_short())
			}
			if ok {
				break
			}
		}

		validator := audit_statedb.JamState.SafroleState.GetCurrValidator(int(peerID))
		bandersnatchPub := validator.Bandersnatch
		for _, evidence := range evidenceSN {
			signature := evidence.Signature
			if len(evidence.NoShows) == 0 {
				return fmt.Errorf("onAuditAnnouncement: no shows are empty")
			}
			if len(evidence.NoShows[0].Reports) == 0 {
				return fmt.Errorf("onAuditAnnouncement: no shows reports are empty")
			}
			workreportHash := evidence.NoShows[0].Reports[0].WorkReportHash
			ok, err := audit_statedb.Verify_sn(bandersnatch.BanderSnatchKey(bandersnatchPub[:]), workreportHash, signature[:], uint32(newReq.Tranche))
			if err != nil {
				return fmt.Errorf("onAuditAnnouncement: failed to verify evidenceSN: %w", err)
			}
			if !ok {
				return fmt.Errorf("onAuditAnnouncement: evidenceSN verification failed")
			}
		}
	}
	// Non-blocking send to announcementsCh and warns when the channel is full
	select {
	case n.announcementsCh <- announcement:
		// Sent successfully
	default:
		log.Warn(module, "onAuditAnnouncement: announcementsCh full, dropping announcement",
			"peerID", peerID,
			"headerHash", newReq.HeaderHash.String_short())
	}

	return nil
}

// for broadcasting

type JAMSNPAuditAnnouncementWithProof struct {
	Announcement     JAMSNPAuditAnnouncement   `json:"announcement"`
	EvidenceTranche0 Tranche0Evidence          `json:"evidence_tranche_0"`
	EvidenceTrancheN SubsequentTrancheEvidence `json:"evidence_tranche_n"`
}
