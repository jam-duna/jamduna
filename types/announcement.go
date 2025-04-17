package types

import (
	"crypto/ed25519"
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
)

// TrancheAnnouncement should probably just be [trancheIdx]AnnounceBucket
// given [trancheIdx], get your currAnnouncementBucket and PrevAnnouncementBucket
type TrancheAnnouncement struct {
	sync.Mutex
	AnnouncementBucket map[uint32]*AnnounceBucket
}

func (T *TrancheAnnouncement) PutAnnouncement(a Announcement) error {
	T.Lock()
	defer T.Unlock()
	if T.AnnouncementBucket == nil {
		T.AnnouncementBucket = make(map[uint32]*AnnounceBucket)
	}

	bucket, exists := T.AnnouncementBucket[a.Tranche]
	if !exists {
		T.AnnouncementBucket[a.Tranche] = &AnnounceBucket{}
		bucket = T.AnnouncementBucket[a.Tranche]
	}
	bucket.PutAnnouncement(a)
	T.AnnouncementBucket[a.Tranche] = bucket
	return nil
}

func (T *TrancheAnnouncement) HaveMadeAnnouncement(a Announcement) bool {
	T.Lock()
	defer T.Unlock()
	if T.AnnouncementBucket == nil {
		return false
	}
	bucket, exists := T.AnnouncementBucket[a.Tranche]
	if !exists {
		T.AnnouncementBucket[a.Tranche] = &AnnounceBucket{}
		bucket = T.AnnouncementBucket[a.Tranche]
	}
	return bucket.HaveMadeAnnouncement(a)
}

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	HeaderHash          common.Hash          `json:"header_hash"`
	Tranche             uint32               `json:"tranche"`
	Selected_WorkReport []AnnouncementReport `json:"selected_work_report"`
	Signature           Ed25519Signature     `json:"signature"`
	ValidatorIndex      uint32               `json:"validator_index"`
}
type AnnouncementReport struct {
	Core           uint16      `json:"core"`
	WorkReportHash common.Hash `json:"work_report_hash"`
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
	var signtext_n []byte
	for _, w := range a.Selected_WorkReport {
		c, _ := Encode(w.Core)
		signtext_n = append(signtext_n, c...)
		signtext_n = append(signtext_n, w.WorkReportHash.Bytes()...)
	}
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

func (a *Announcement) GetWorkReportHashes() []common.Hash {
	report_hashes := make([]common.Hash, len(a.Selected_WorkReport))
	for idx, report := range a.Selected_WorkReport {
		report_hashes[idx] = report.WorkReportHash
	}
	return report_hashes
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
	sync.Mutex
	Tranche            uint32                         //?
	Announcements      map[common.Hash][]Announcement `json:"reports"`
	KnownAnnouncements map[common.Hash]bool           // use identifier to filter duplicate A
}

func (W *AnnounceBucket) GetLen(w common.Hash) int {
	W.Lock()
	defer W.Unlock()
	return len(W.Announcements[w])
}

func (W *AnnounceBucket) PutAnnouncement(a Announcement) {
	W.Lock()
	defer W.Unlock()
	if W.Announcements == nil {
		W.Announcements = make(map[common.Hash][]Announcement)
	}
	if W.KnownAnnouncements == nil {
		W.KnownAnnouncements = make(map[common.Hash]bool)
	}
	if W.KnownAnnouncements[a.Hash()] {
		return
	}
	// W.Announcements[a.WorkReportHash] = append(W.Announcements[a.WorkReportHash], a)
	for _, w := range a.Selected_WorkReport {
		W.Announcements[w.WorkReportHash] = append(W.Announcements[w.WorkReportHash], a)
	}
	W.KnownAnnouncements[a.Hash()] = true
	return
}

func (W *AnnounceBucket) HaveMadeAnnouncement(a Announcement) bool {
	W.Lock()
	defer W.Unlock()
	if W.KnownAnnouncements == nil {
		return false
	}
	return W.KnownAnnouncements[a.Hash()]
}

// Deep copy of AnnounceBucket

func (W *AnnounceBucket) DeepCopy() (*AnnounceBucket, error) {
	W.Lock()
	defer W.Unlock()

	type announceBucketSerializable struct {
		Tranche            uint32
		Announcements      map[common.Hash][]Announcement
		KnownAnnouncements map[common.Hash]bool
	}

	// Copy fields manually to a serializable version
	serializable := announceBucketSerializable{
		Tranche:            W.Tranche,
		Announcements:      make(map[common.Hash][]Announcement, len(W.Announcements)),
		KnownAnnouncements: make(map[common.Hash]bool, len(W.KnownAnnouncements)),
	}

	// Deep copy maps
	for k, v := range W.Announcements {
		copied := make([]Announcement, len(v))
		copy(copied, v)
		serializable.Announcements[k] = copied
	}
	for k, v := range W.KnownAnnouncements {
		serializable.KnownAnnouncements[k] = v
	}

	// Construct the result
	return &AnnounceBucket{
		Tranche:            serializable.Tranche,
		Announcements:      serializable.Announcements,
		KnownAnnouncements: serializable.KnownAnnouncements,
	}, nil
}
