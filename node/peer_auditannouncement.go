package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
	"io"
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
	if err := binary.Write(buf, binary.BigEndian, ann.Tranche); err != nil {
		return nil, err
	}

	// Serialize Len (1 byte)
	ann.Len = uint8(len(ann.Reports)) // Set the length of reports before serializing
	if err := binary.Write(buf, binary.BigEndian, ann.Len); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &ann.Tranche); err != nil {
		return err
	}

	// Deserialize Len (1 byte)
	if err := binary.Read(buf, binary.BigEndian, &ann.Len); err != nil {
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
	Signature      types.Ed25519Signature          `json:"signature"`
}

func (noShow *JAMSNPNoShow) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ValidatorIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, noShow.ValidatorIndex); err != nil {
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

	// Serialize Signature (64 bytes)
	if _, err := buf.Write(noShow.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (noShow *JAMSNPNoShow) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ValidatorIndex (2 bytes)
	if err := binary.Read(buf, binary.BigEndian, &noShow.ValidatorIndex); err != nil {
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

	// Deserialize Signature (64 bytes)
	if _, err := io.ReadFull(buf, noShow.Signature[:]); err != nil {
		return err
	}

	return nil
}

type JAMSNPAuditAnnouncementNot0 struct {
	Len       uint8                           `json:"len"`
	Signature types.BandersnatchRingSignature `json:"signature"`
	NoShows   []JAMSNPNoShow                  `json:"noshows"`
}
type JAMSNPAuditAnnouncementReport struct {
	CoreIndex      uint16      `json:"core_index"`
	WorkReportHash common.Hash `json:"work_report_hash"`
}

func (report *JAMSNPAuditAnnouncementReport) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, report.CoreIndex); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &report.CoreIndex); err != nil {
		return err
	}

	// Deserialize WorkReportHash (32 bytes)
	if _, err := io.ReadFull(buf, report.WorkReportHash[:]); err != nil {
		return err
	}

	return nil
}

func (p *Peer) SendAuditAnnouncement(workReportHash common.Hash, headerHash common.Hash, coreIndex uint16, a *types.Announcement) (err error) {
	reports := make([]JAMSNPAuditAnnouncementReport, 0)
	report := JAMSNPAuditAnnouncementReport{
		CoreIndex:      coreIndex,
		WorkReportHash: workReportHash,
	}
	reports = append(reports, report)
	req := &JAMSNPAuditAnnouncement{
		HeaderHash: headerHash,
		Tranche:    uint8(a.Tranche), // check
		Reports:    reports,
		Signature:  a.Signature,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, err := p.openStream(CE144_AuditAnnouncement)
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	/*
		  for _, r := range reports {
		    if a.Tranche == 0 {
					// [Tranche 0] --> Bandersnatch Signature (s_0 in GP)
		      // TODO: Shawn: need BandersnatchSignature
		    } else {
				//	[Tranche not 0] --> [Bandersnatch Signature (s_n(w) in GP) ++ No Shows] (One entry per WR in the first message)
		      // TODO: Shawn: need BandersnatchSignature
		    }
		  }
	*/

	return nil
}

// TODO: Shawn CHECK
func (n *Node) onAuditAnnouncement(stream quic.Stream, msg []byte) (err error) {
	defer 	stream.Close()
	var newReq JAMSNPAuditAnnouncement
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	coreIndexes := make([]uint16, 0)
	workReportHashes := make([]common.Hash, 0)
	for _, r := range newReq.Reports {
		coreIndexes = append(coreIndexes, r.CoreIndex)
		workReportHashes = append(workReportHashes, r.WorkReportHash)
	}

	// <-- FIN
	announcement := types.Announcement{
		// Core:  0,
		Tranche: uint32(newReq.Tranche),
		//WorkReport:     WorkReport
		//ValidatorIndex: uint32(validatorIndex),
		Signature: newReq.Signature,
	}
	n.announcementsCh <- announcement

	return
}
