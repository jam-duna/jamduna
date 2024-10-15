package types

import (
	"crypto/ed25519"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	HeaderHash      common.Hash      `json:"header_hash"`
	Core            uint16           `json:"core"`
	Tranche         uint32           `json:"tranche"`
	WorkReportHash  common.Hash      `json:"work_report_hash"`
	Signature       Ed25519Signature `json:"signature"`
	ValidatorIndex  uint32           `json:"validator_index"`
	TrancheEvidence common.Hash      `json:"tranche_evidence"`
}

func (a *Announcement) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}

	return enc
}

func (a *Announcement) Hash() common.Hash {
	data := a.Bytes()
	return common.Blake2Hash(data)
}

// computeAnnouncementBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Announcement) UnsignedBytes() []byte {
	// eq 214
	signtext_n, _ := Encode(a.Core)
	signtext_n = append(signtext_n, a.WorkReportHash.Bytes()...)
	signtext, _ := Encode(a.Tranche)
	signtext = append(signtext, signtext_n...)
	signtext = append(signtext, a.HeaderHash.Bytes()...)
	return signtext
}

func (a *Announcement) UnsignedBytesWithSalt() []byte {
	signtext := a.UnsignedBytes()
	return append([]byte(X_I), signtext...)
}
func (a *Announcement) Sign(Ed25519Secret []byte) {
	signtext := a.UnsignedBytesWithSalt()
	a.Signature = Ed25519Signature(ed25519.Sign(Ed25519Secret, signtext))
}

func (a *Announcement) Verify(key Ed25519Key) error {
	announcementBytes := a.UnsignedBytesWithSalt()

	if !ed25519.Verify(key.PublicKey(), announcementBytes, a.Signature.Bytes()) {
		return fmt.Errorf("invalid signature by signature %v", a.Signature)
	}
	return nil
}

// func (a *Announcement) Bytes() []byte {
// 	enc := Encode(a)
// 	return enc
// }

// func (a *Announcement) Hash() common.Hash {
// 	data := a.Bytes()
// 	if data == nil {
// 		return common.Hash{}
// 	}
// 	return common.Blake2Hash(data)
// }

// eq 198
type AnnounceBucket struct {
	Tranche            uint32                         //?
	Announcements      map[common.Hash][]Announcement `json:"reports"`
	KnownAnnouncements map[common.Hash]bool           // use identifier to filter duplicate A
}

func (W *AnnounceBucket) GetLen(w common.Hash) int {
	return len(W.Announcements[w])
}

func (W *AnnounceBucket) PutAnnouncement(a Announcement) {
	if W.Announcements == nil {
		W.Announcements = make(map[common.Hash][]Announcement)
	}
	if W.KnownAnnouncements == nil {
		W.KnownAnnouncements = make(map[common.Hash]bool)
	}
	if W.KnownAnnouncements[a.Hash()] {
		return
	}
	W.Announcements[a.WorkReportHash] = append(W.Announcements[a.WorkReportHash], a)
	W.KnownAnnouncements[a.Hash()] = true
}

// Deep copy of AnnounceBucket
func (W *AnnounceBucket) DeepCopy() (AnnounceBucket, error) {
	var copiedAnnounceBucket AnnounceBucket

	// Serialize the original AnnounceBucket to JSON
	data, err := Encode(W)
	if err != nil {
		return copiedAnnounceBucket, err
	}

	// Deserialize the JSON back into a new AnnounceBucket instance
	decoded, _, err := Decode(data, reflect.TypeOf(AnnounceBucket{}))
	if err != nil {
		return copiedAnnounceBucket, err
	}
	copiedAnnounceBucket = decoded.(AnnounceBucket)

	return copiedAnnounceBucket, nil
}
