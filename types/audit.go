package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// eq 190
type WorkReportNeedAudit struct {
	Q [TotalCores]WorkReport `json:"available_work_report"`
}

type WorkReportSelection struct {
	WorkReport WorkReport `json:"work_report"`
	Core       uint16     `json:"core_index"`
}

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	Core       uint16           `json:"core"`
	Tranche    uint32           `json:"tranche"`
	WorkReport WorkReport       `json:"work_report"`
	Signature  Ed25519Signature `json:"signature"`
}

type Judgement struct {
	Core       uint16           `json:"core"`
	Judge      bool             `json:"judge"`
	Tranche    uint32           `json:"tranche"`
	WorkReport WorkReport       `json:"work_report"`
	Signature  Ed25519Signature `json:"signature"`
}

// Ed25519Signature to byte
func (e *Ed25519Signature) Bytes() []byte {
	return e[:]
}

// eq 198
type WhoAudit struct {
	Announcements map[common.Hash][]Announcement `json:"reports"`
}

func (W *WhoAudit) GetLen(w common.Hash) int {
	return len(W.Announcements[w])
}

type Announcement_N struct {
	An []WhoAudit //each tranche has a WhoAudit
}

type WhoJudge struct {
	Judgements map[common.Hash][]Judgement `json:"judgements"`
}

func (J *WhoJudge) GetTrueCount(W common.Hash) int {
	count := 0
	for _, j := range J.Judgements[W] {
		if j.Judge {
			count++
		}
	}
	return count
}

// Uint32ToBytes converts a uint32 value to a byte slice.
func Uint32ToBytes(value uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, value)
	return bytes
}

// Uint16ToBytes converts a uint16 value to a byte slice.
func Uint16ToBytes(value uint16) []byte {
	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, value)
	return bytes
}

// computeAnnouncementBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Announcement) computeAnnouncementBytes() []byte {
	h := append(Uint32ToBytes(a.Tranche), Uint16ToBytes(a.Core)...)
	h0 := a.WorkReport.Hash()
	h = append(h, h0.Bytes()...)
	return append([]byte(X_I), h...)
}
func (a *Announcement) Sign(Ed25519Secret []byte) {
	signtext := append([]byte(X_I), Uint32ToBytes(a.Tranche)...)
	signtext = append(signtext, Uint16ToBytes(a.Core)...)
	signtext = append(signtext, a.WorkReport.Hash().Bytes()...)
	a.Signature = Ed25519Signature(ed25519.Sign(Ed25519Secret, signtext))
}

func (a *Announcement) ValidateSignature(publicKey []byte) error {
	announcementBytes := a.computeAnnouncementBytes()

	if !ed25519.Verify(publicKey, announcementBytes, a.Signature.Bytes()) {
		return errors.New("invalid signature")
	}
	return nil
}

func (a *Announcement) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *Announcement) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (J *WhoJudge) GetFalseCount(W common.Hash) int {
	count := 0
	for _, j := range J.Judgements[W] {
		if !j.Judge {
			count++
		}
	}
	return count
}

func (J *WhoJudge) GetTrueJudgement(W common.Hash) []Judgement {
	judgements := make([]Judgement, 0)
	for _, j := range J.Judgements[W] {
		//drop duplicate
		for _, jj := range judgements {
			if (j.Core == jj.Core) && (j.Tranche == jj.Tranche) {
				continue
			}
		}
		if j.Judge {
			judgements = append(judgements, j)
		}
	}
	return judgements[:TotalValidators*2/3+1]
}

func (J *WhoJudge) GetFalseJudgement(W common.Hash) []Judgement {
	judgements := make([]Judgement, 0)
	for _, j := range J.Judgements[W] {
		//drop duplicate
		for _, jj := range judgements {
			if (j.Core == jj.Core) && (j.Tranche == jj.Tranche) {
				continue
			}
		}
		if !j.Judge {
			judgements = append(judgements, j)
		}
	}
	return judgements[:TotalValidators*2/3+1]
}

func (J *WhoJudge) GetWonkeyJudgement(W common.Hash) []Judgement {
	judgements := make([]Judgement, 0)
	trueCount := 0
	falseCount := 0
	for _, j := range J.Judgements[W] {
		//drop duplicate
		for _, jj := range judgements {
			if (j.Core == jj.Core) && (j.Tranche == jj.Tranche) {
				continue
			}
		}
		if !j.Judge {
			if falseCount >= TotalValidators*1/3 {
				continue
			}
			judgements = append(judgements, j)
			falseCount++
		} else {
			if trueCount >= TotalValidators*1/3+1 {
				continue
			}
			judgements = append(judgements, j)
			trueCount++
		}
	}

	return judgements[:TotalValidators*2/3+1]
}
