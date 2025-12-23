package node

import (
	"context"
	"fmt"
	"reflect"

	bandersnatch "github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
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

// AuditAnnouncementObj wraps an audit announcement with the sender's keys and evidence.
// This allows immediate enqueuing without waiting for statedb, deferring ValidatorIndex
// derivation and evidence verification to processAuditAnnouncement (similar to AssuranceObject pattern).
type AuditAnnouncementObj struct {
	HeaderHash          common.Hash                     `json:"header_hash"`
	Tranche             uint32                          `json:"tranche"`
	Selected_WorkReport []types.AuditAnnouncementReport `json:"selected_workreport"`
	Signature           types.Ed25519Signature          `json:"signature"`
	Ed25519Key          types.Ed25519Key                `json:"ed25519_key"`      // Sender's key for ValidatorIndex derivation
	BandersnatchKey     types.BandersnatchKey           `json:"bandersnatch_key"` // Sender's Bandersnatch key for evidence verification
	EvidenceS0          []byte                          `json:"evidence_s0"`      // Tranche 0 evidence (nil if tranche > 0)
	EvidenceSN          SubsequentTrancheEvidence       `json:"evidence_sn"`      // Tranche N evidence (empty if tranche == 0)
}

func (p *Peer) SendAuditAnnouncement(ctx context.Context, announcement JAMSNPAuditAnnouncement, evidence interface{}) error {
	log.Debug(log.Node, "SendAuditAnnouncement", "peerKey", p.SanKey(), "headerHash", announcement.HeaderHash.String_short(), "tranche", announcement.Tranche)
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

	if err := sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code); err != nil {
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
		if err := sendQuicBytes(ctx, stream, evidenceBytes, p.SanKey(), code); err != nil {
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
		if err := sendQuicBytes(ctx, stream, evidenceBytes, p.SanKey(), code); err != nil {
			return fmt.Errorf("sendQuicBytes[SubsequentTrancheEvidence]: %w", err)
		}
	}

	return nil
}

func AuditAnnouncementToNoShow(a *types.AuditAnnouncement) (JAMSNPNoShow, error) {
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

// verifyBandersnatchS0 tries to verify s0 evidence against multiple validator sets.
// After epoch rotation, the Bandersnatch key at the claimed index may be different.
// We try the claimed index first (fast path), then search all validators in all sets.
func verifyBandersnatchS0(sdb *statedb.StateDB, bandersnatchPub types.BandersnatchKey, evidenceBytes []byte) (bool, error) {
	ok, err := sdb.Verify_s0(bandersnatch.BanderSnatchKey(bandersnatchPub[:]), evidenceBytes)
	if err == nil && ok {
		return true, nil
	}
	return false, fmt.Errorf("s0 evidence does not match any known validator")
}

// verifyBandersnatchSN verifies sN evidence using the peer's Bandersnatch key directly.
func verifyBandersnatchSN(sdb *statedb.StateDB, bandersnatchPub types.BandersnatchKey, workreportHash common.Hash, signature []byte, tranche uint32) (bool, error) {
	ok, err := sdb.Verify_sn(bandersnatch.BanderSnatchKey(bandersnatchPub[:]), workreportHash, signature, tranche)
	if err == nil && ok {
		return true, nil
	}
	return false, fmt.Errorf("sN evidence does not match any known validator")
}

func (n *Node) onAuditAnnouncement(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()
	code := uint8(CE144_AuditAnnouncement)
	var newReq JAMSNPAuditAnnouncement
	if err := newReq.FromBytes(msg); err != nil {
		log.Warn(log.Node, "onAuditAnnouncement: failed to deserialize announcement", "peerKey", peerKey, "error", err)
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onAuditAnnouncement: failed to deserialize announcement: %w", err)
	}

	// Convert reports to internal format
	selectedReports := make([]types.AuditAnnouncementReport, len(newReq.Reports))
	for i, report := range newReq.Reports {
		selectedReports[i] = types.AuditAnnouncementReport{
			Core:           report.CoreIndex,
			WorkReportHash: report.WorkReportHash,
		}
	}
	// Get peer to access its validator info
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onAuditAnnouncement: could not find peer for key %s", peerKey)
	}

	/*
		Bandersnatch Signature = [u8; 96]
		First Tranche Evidence = Bandersnatch Signature (s_0 in GP)
		No-Show = Validator Index ++ Announcement (From the previous tranche)
		Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
		Evidence = First Tranche Evidence (If tranche is 0) OR Subsequent Tranche Evidence (If tranche is not 0)
	*/

	// Read evidence from network FIRST to avoid back-pressure/timeout on sender
	// while we wait for statedb to become available
	var evidenceS0Bytes []byte
	var evidenceSN SubsequentTrancheEvidence

	if newReq.Tranche == 0 {
		var err error
		evidenceS0Bytes, err = receiveQuicBytes(ctx, stream, n.GetEd25519Key().SAN(), code)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: receive evidence_s0 failed: %w", err)
		}
	} else {
		evidenceBytes, err := receiveQuicBytes(ctx, stream, n.GetEd25519Key().SAN(), code)
		if err != nil {
			return fmt.Errorf("onAuditAnnouncement: receive evidence_sN failed: %w", err)
		}
		if err := evidenceSN.FromBytes(evidenceBytes); err != nil {
			return fmt.Errorf("onAuditAnnouncement: failed to deserialize evidenceSN: %w", err)
		}
	}

	// Create AuditAnnouncementObj with all data needed for deferred processing.
	// ValidatorIndex derivation and evidence verification are deferred to processAuditAnnouncement
	// to decouple network I/O from state availability (similar to AssuranceObject pattern).
	auditAnnouncementObj := AuditAnnouncementObj{
		HeaderHash:          newReq.HeaderHash,
		Tranche:             uint32(newReq.Tranche),
		Selected_WorkReport: selectedReports,
		Signature:           newReq.Signature,
		Ed25519Key:          peer.Validator.Ed25519,
		BandersnatchKey:     peer.Validator.Bandersnatch,
		EvidenceS0:          evidenceS0Bytes,
		EvidenceSN:          evidenceSN,
	}

	// Non-blocking send to auditAnnouncementsCh - verification deferred to processAuditAnnouncement
	select {
	case n.auditAnnouncementsCh <- auditAnnouncementObj:
		// Sent successfully
	default:
		log.Warn(log.Node, "onAuditAnnouncement: auditAnnouncementsCh full, dropping announcement",
			"peerKey", peerKey,
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
