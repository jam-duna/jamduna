package node

import (
	"bytes"
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

func (p *Peer) SendAuditAnnouncement(a *JAMSNPAuditAnnouncementWithProof) (err error) {
	reports := make([]JAMSNPAuditAnnouncementReport, 0)
	for _, report := range a.Announcement.Reports {
		var jam_ann_report JAMSNPAuditAnnouncementReport
		jam_ann_report.CoreIndex = report.CoreIndex
		jam_ann_report.WorkReportHash = report.WorkReportHash
		reports = append(reports, jam_ann_report)
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
		return err
	}
	code := uint8(CE144_AuditAnnouncement)
	stream, err := p.openStream(code)
	if err != nil {
		return err
	}
	defer stream.Close()

	err = sendQuicBytes(stream, reqBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	/*
		Bandersnatch Signature = [u8; 96]
		First Tranche Evidence = Bandersnatch Signature (s_0 in GP)
		No-Show = Validator Index ++ Announcement (From the previous tranche)
		Subsequent Tranche Evidence = [Bandersnatch Signature (s_n(w) in GP) ++ len++[No-Show]] (One entry per announced work-report)
		Evidence = First Tranche Evidence (If tranche is 0) OR Subsequent Tranche Evidence (If tranche is not 0)
	*/
	if a.Announcement.Tranche == 0 {
		err = sendQuicBytes(stream, a.Evidence_s0[:], p.PeerID, code)
		if err != nil {
			return err
		}
	} else {
		ev_not0 := make([]JAMSNPAuditAnnouncementNot0, 0)
		for i, report := range req.Reports {
			noShow := a.Evidence_sn_no_show[report.WorkReportHash]
			var jam_noshow []JAMSNPNoShow
			for _, a := range noShow {
				jam_noshow = append(jam_noshow, JAMSNPNoShow{
					ValidatorIndex: uint16(a.ValidatorIndex),
					Reports:        make([]JAMSNPAuditAnnouncementReport, 0),
				})
				for _, report := range a.Selected_WorkReport {
					var jam_ann_report JAMSNPAuditAnnouncementReport
					jam_ann_report.CoreIndex = report.Core
					jam_ann_report.WorkReportHash = report.WorkReportHash
					jam_noshow[len(jam_noshow)-1].Reports = append(jam_noshow[len(jam_noshow)-1].Reports, jam_ann_report)
				}
			}
			if err != nil {
				return err
			}
			ev_not0 = append(ev_not0, JAMSNPAuditAnnouncementNot0{
				Len:       uint8(len(noShow)),
				Signature: a.Evidence_sn[i],
				NoShows:   jam_noshow,
			})
		}
		for _, ev := range ev_not0 {
			evBytes, err := ev.ToBytes()
			if err != nil {
				return err
			}
			err = sendQuicBytes(stream, evBytes, p.PeerID, code)
			if err != nil {
				return err
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

// TODO: Shawn CHECK
func (n *Node) onAuditAnnouncement(stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()
	code := uint8(CE144_AuditAnnouncement)
	var newReq JAMSNPAuditAnnouncement
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	selected_work_report := make([]types.AnnouncementReport, 0)
	for _, report := range newReq.Reports {
		selected_work_report = append(selected_work_report, types.AnnouncementReport{
			Core:           report.CoreIndex,
			WorkReportHash: report.WorkReportHash,
		})
	}
	// <-- FIN
	announcement := types.Announcement{
		HeaderHash:          newReq.HeaderHash,
		Tranche:             uint32(newReq.Tranche),
		Selected_WorkReport: selected_work_report,
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
		var evidence_s0 types.BandersnatchVrfSignature
		req, err := receiveQuicBytes(stream, n.id, code)
		if err != nil {
			return err
		}
		copy(evidence_s0[:], req)
		// TODO verify evidence_s0
	} else {
		evidence_sn := make([]types.BandersnatchVrfSignature, 0)
		evidence_sn_no_show := make(map[common.Hash][]JAMSNPNoShow)
		for _, r := range newReq.Reports {
			var ev_not0 JAMSNPAuditAnnouncementNot0
			req, err := receiveQuicBytes(stream, n.id, code)
			if err != nil {
				return err
			}
			err = ev_not0.FromBytes(req)
			if err != nil {
				return err
			}
			evidence_sn = append(evidence_sn, ev_not0.Signature)
			for _, noShow := range ev_not0.NoShows {
				if err != nil {
					return err
				}
				evidence_sn_no_show[r.WorkReportHash] = append(evidence_sn_no_show[r.WorkReportHash], noShow)
			}
		}
		// TODO verify evidence_sn
	}

	return
}
