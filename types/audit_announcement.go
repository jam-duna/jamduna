package types

import (
	"fmt"
	"sync"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/ed25519"
)

// AuditTrancheAnnouncement should probably just be [trancheIdx]AuditAnnounceBucket
// given [trancheIdx], get your currAnnouncementBucket and PrevAnnouncementBucket
type AuditTrancheAnnouncement struct {
	sync.Mutex
	AnnouncementBucket map[uint32]*AuditAnnounceBucket
}

func (ta *AuditTrancheAnnouncement) Clone() *AuditTrancheAnnouncement {
	ta.Lock()
	defer ta.Unlock()

	clone := &AuditTrancheAnnouncement{
		AnnouncementBucket: make(map[uint32]*AuditAnnounceBucket, len(ta.AnnouncementBucket)),
	}

	for k, v := range ta.AnnouncementBucket {
		if v != nil {
			clone.AnnouncementBucket[k] = v.Clone() // assumes AuditAnnounceBucket has a Clone method
		} else {
			clone.AnnouncementBucket[k] = nil
		}
	}

	return clone
}

func (T *AuditTrancheAnnouncement) PutAnnouncement(a AuditAnnouncement) error {
	T.Lock()
	defer T.Unlock()
	if T.AnnouncementBucket == nil {
		T.AnnouncementBucket = make(map[uint32]*AuditAnnounceBucket)
	}

	bucket, exists := T.AnnouncementBucket[a.Tranche]
	if !exists {
		T.AnnouncementBucket[a.Tranche] = &AuditAnnounceBucket{}
		bucket = T.AnnouncementBucket[a.Tranche]
	}
	bucket.PutAnnouncement(a)
	T.AnnouncementBucket[a.Tranche] = bucket
	return nil
}

func (T *AuditTrancheAnnouncement) HaveMadeAnnouncement(a AuditAnnouncement) bool {
	T.Lock()
	defer T.Unlock()
	if T.AnnouncementBucket == nil {
		return false
	}
	bucket, exists := T.AnnouncementBucket[a.Tranche]
	if !exists {
		T.AnnouncementBucket[a.Tranche] = &AuditAnnounceBucket{}
		bucket = T.AnnouncementBucket[a.Tranche]
	}
	return bucket.HaveMadeAnnouncement(a)
}

// AuditAnnouncement  Section 17.3 Equations (196)-(199) TBD
type AuditAnnouncement struct {
	HeaderHash          common.Hash                `json:"header_hash"`
	Tranche             uint32                     `json:"tranche"`
	Selected_WorkReport []AuditAnnouncementReport  `json:"selected_work_report"`
	Signature           Ed25519Signature           `json:"signature"`
	ValidatorIndex      uint32                     `json:"validator_index"`
}
type AuditAnnouncementReport struct {
	Core           uint16      `json:"core"`
	WorkReportHash common.Hash `json:"work_report_hash"`
}

func (a *AuditAnnouncement) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}

	return enc
}

func (a *AuditAnnouncement) Hash() common.Hash {
	data := a.Bytes()
	return common.Blake2Hash(data)
}

// computeAnnouncementBytes abstracts the process of generating the bytes to be signed or verified.
func (a *AuditAnnouncement) UnsignedBytes() []byte {
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

func (a *AuditAnnouncement) UnsignedBytesWithSalt() []byte {
	signtext := a.UnsignedBytes()
	return append([]byte(X_I), signtext...)
}
func (a *AuditAnnouncement) Sign(Ed25519Secret []byte) {
	signtext := a.UnsignedBytesWithSalt()
	a.Signature = Ed25519Signature(ed25519.Sign(Ed25519Secret, signtext))
}

func (a *AuditAnnouncement) Verify(key Ed25519Key) error {
	announcementBytes := a.UnsignedBytesWithSalt()

	if !ed25519.Verify(key.PublicKey(), announcementBytes, a.Signature.Bytes()) {
		return fmt.Errorf("invalid signature by signature %v", a.Signature)
	}
	return nil
}

func (a *AuditAnnouncement) GetWorkReportHashes() []common.Hash {
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
type AuditAnnounceBucket struct {
	sync.Mutex
	Tranche            uint32                              //?
	Announcements      map[common.Hash][]AuditAnnouncement `json:"reports"`
	KnownAnnouncements map[common.Hash]bool                // use identifier to filter duplicate A
}

func (ab *AuditAnnounceBucket) Clone() *AuditAnnounceBucket {
	ab.Lock()
	defer ab.Unlock()

	// Deep copy of Announcements map
	announcementsCopy := make(map[common.Hash][]AuditAnnouncement, len(ab.Announcements))
	for k, v := range ab.Announcements {
		// Copy the slice of announcements
		sliceCopy := make([]AuditAnnouncement, len(v))
		copy(sliceCopy, v)
		announcementsCopy[k] = sliceCopy
	}

	// Deep copy of KnownAnnouncements map
	knownCopy := make(map[common.Hash]bool, len(ab.KnownAnnouncements))
	for k, v := range ab.KnownAnnouncements {
		knownCopy[k] = v
	}

	return &AuditAnnounceBucket{
		Tranche:            ab.Tranche,
		Announcements:      announcementsCopy,
		KnownAnnouncements: knownCopy,
	}
}

func (W *AuditAnnounceBucket) GetLen(w common.Hash) int {
	W.Lock()
	defer W.Unlock()
	return len(W.Announcements[w])
}

func (W *AuditAnnounceBucket) PutAnnouncement(a AuditAnnouncement) {
	W.Lock()
	defer W.Unlock()
	if W.Announcements == nil {
		W.Announcements = make(map[common.Hash][]AuditAnnouncement)
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
}

func (W *AuditAnnounceBucket) HaveMadeAnnouncement(a AuditAnnouncement) bool {
	W.Lock()
	defer W.Unlock()
	if W.KnownAnnouncements == nil {
		return false
	}
	return W.KnownAnnouncements[a.Hash()]
}

// Deep copy of AuditAnnounceBucket

func (W *AuditAnnounceBucket) DeepCopy() (*AuditAnnounceBucket, error) {
	W.Lock()
	defer W.Unlock()

	type auditAnnounceBucketSerializable struct {
		Tranche            uint32
		Announcements      map[common.Hash][]AuditAnnouncement
		KnownAnnouncements map[common.Hash]bool
	}

	// Copy fields manually to a serializable version
	serializable := auditAnnounceBucketSerializable{
		Tranche:            W.Tranche,
		Announcements:      make(map[common.Hash][]AuditAnnouncement, len(W.Announcements)),
		KnownAnnouncements: make(map[common.Hash]bool, len(W.KnownAnnouncements)),
	}

	// Deep copy maps
	for k, v := range W.Announcements {
		copied := make([]AuditAnnouncement, len(v))
		copy(copied, v)
		serializable.Announcements[k] = copied
	}
	for k, v := range W.KnownAnnouncements {
		serializable.KnownAnnouncements[k] = v
	}

	// Construct the result
	return &AuditAnnounceBucket{
		Tranche:            serializable.Tranche,
		Announcements:      serializable.Announcements,
		KnownAnnouncements: serializable.KnownAnnouncements,
	}, nil
}
