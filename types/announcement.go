package types

import (
	"crypto/ed25519"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	Core           uint16           `json:"core"`
	Tranche        uint32           `json:"tranche"`
	WorkReport     WorkReport       `json:"work_report"`
	ValidatorIndex uint32           `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}

func (a *Announcement) Bytes() []byte {
	enc := Encode(a)

	return enc
}

func (a *Announcement) Hash() common.Hash {
	data := a.Bytes()
	return common.Blake2Hash(data)
}

// computeAnnouncementBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Announcement) UnsignedBytes() []byte {
	signtext := append(common.Uint32ToBytes(a.Tranche), common.Uint16ToBytes(a.Core)...)
	h0 := a.WorkReport.AvailabilitySpec.WorkPackageHash
	signtext = append(signtext, h0.Bytes()...)
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
	W.Announcements[a.WorkReport.AvailabilitySpec.WorkPackageHash] = append(W.Announcements[a.WorkReport.AvailabilitySpec.WorkPackageHash], a)
	W.KnownAnnouncements[a.Hash()] = true
}

// Deep copy of AnnounceBucket
func (W *AnnounceBucket) DeepCopy() (AnnounceBucket, error) {
	var copiedAnnounceBucket AnnounceBucket

	// Serialize the original AnnounceBucket to JSON
	data := Encode(W)

	// Deserialize the JSON back into a new AnnounceBucket instance
	decoded, _ := Decode(data, reflect.TypeOf(AnnounceBucket{}))
	copiedAnnounceBucket = decoded.(AnnounceBucket)

	return copiedAnnounceBucket, nil
}
