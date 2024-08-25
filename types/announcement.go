package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	Core       uint16           `json:"core"`
	Tranche    uint32           `json:"tranche"`
	WorkReport WorkReport       `json:"work_report"`
	Signature  Ed25519Signature `json:"signature"`
}

// uint32ToBytes converts a uint32 value to a byte slice.
func uint32ToBytes(value uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, value)
	return bytes
}

// uint16ToBytes converts a uint16 value to a byte slice.
func uint16ToBytes(value uint16) []byte {
	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, value)
	return bytes
}

// computeAnnouncementBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Announcement) computeAnnouncementBytes() []byte {
	h := append(uint32ToBytes(a.Tranche), uint16ToBytes(a.Core)...)
	h0 := a.WorkReport.Hash()
	h = append(h, h0.Bytes()...)
	return append([]byte(X_I), h...)
}
func (a *Announcement) Sign(Ed25519Secret []byte, parentHash common.Hash) {
	announcementBytes := a.computeAnnouncementBytes()
	a.Signature = ed25519.Sign(Ed25519Secret, announcementBytes)
}

func (a *Announcement) ValidateSignature(publicKey []byte) error {
	announcementBytes := a.computeAnnouncementBytes()

	if !ed25519.Verify(publicKey, announcementBytes, a.Signature) {
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
