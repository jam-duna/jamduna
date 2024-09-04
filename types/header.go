package types

import (
	"encoding/json"

	"fmt"
	"github.com/colorfulnotion/jam/common"
)

type BlockHeader struct {
	// H_p
	Parent common.Hash `json:"parent"`
	// H_r
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// H_x
	ExtrinsicHash common.Hash `json:"extrinsic_hash"`
	// H_t
	Slot uint32 `json:"slot"`
	// H_e
	EpochMark *EpochMark `json:"epoch_mark,omitempty"`
	// H_w
	TicketsMark [12]*TicketBody `json:"tickets_mark,omitempty"`
	// H_o
	OffenderMarkers []Ed25519Key `json:"offenders_mark"`
	// H_i
	AuthorIndex uint16 `json:"author_index"`
	// H_v
	EntropySource [96]byte `json:"entropy_source"`
	// H_s
	Seal [96]byte `json:"seal"`
}

type SBlockHeader struct {
	Parent          common.Hash     `json:"parent"`
	ParentStateRoot common.Hash     `json:"parent_state_root"`
	ExtrinsicHash   common.Hash     `json:"extrinsic_hash"`
	Slot            uint32          `json:"slot"`
	EpochMark       *EpochMark      `json:"epoch_mark,omitempty"`
	TicketsMark     [12]*TicketBody `json:"tickets_mark,omitempty"`
	OffendersMark   []string        `json:"offenders_mark"`
	AuthorIndex     uint16          `json:"author_index"`
	EntropySource   string          `json:"entropy_source"`
	Seal            string          `json:"seal"`
}

// NewBlockHeader returns a fresh block header from scratch.
func NewBlockHeader() *BlockHeader {
	return &BlockHeader{}
}

// Encode returns Scale encoded version of the block.
func (b *BlockHeader) Bytes() ([]byte, error) {
	return b.BytesWithSig(), nil
}

// UnsignedHash returns the hash of the block in unsigned form.
func (b *BlockHeader) UnsignedHash() common.Hash {
	unsignedBytes := b.BytesWithoutSig()
	return common.BytesToHash(common.ComputeHash(unsignedBytes))
}

// Hash returns the hash of the block in unsigned form.
func (b *BlockHeader) Hash() common.Hash {
	data := b.BytesWithSig()
	return common.BytesToHash(common.ComputeHash(data))
}

func (b *BlockHeader) BytesWithSig() []byte {
	enc, err := json.Marshal(b)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

// BytesWithoutSig returns the bytes of a block without the signature.
func (b *BlockHeader) BytesWithoutSig() []byte {
	// Create an instance of the new struct without the signature fields.
	bwoSig := BlockHeaderWithoutSig{
		ParentHash:      b.Parent,
		PriorStateRoot:  b.ParentStateRoot,
		ExtrinsicHash:   b.ExtrinsicHash,
		TimeSlot:        b.Slot,
		EpochMark:       b.EpochMark,
		TicketsMark:     b.TicketsMark,
		//VerdictsMarkers: b.VerdictsMarkers,
		OffenderMarkers:    b.OffenderMarkers,
		AuthorIndex: b.AuthorIndex,
	}

	// Marshal the new struct to JSON.
	enc, err := json.Marshal(bwoSig)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

// BlockHeaderWithoutSig represents the BlockHeader without signature fields.
type BlockHeaderWithoutSig struct {
	ParentHash      common.Hash     `json:"parent_hash"`
	PriorStateRoot  common.Hash     `json:"prior_state_root"`
	ExtrinsicHash   common.Hash     `json:"extrinsic_hash"`
	TimeSlot        uint32          `json:"timeslot"`
	EpochMark       *EpochMark      `json:"epoch_mark"`
	TicketsMark     [12]*TicketBody `json:"tickets_mark"`
	VerdictsMarkers *VerdictMarker  `json:"verdict_markers"`
	OffenderMarkers []Ed25519Key	`json:"offender_markers"`
	AuthorIndex     uint16          `json:"block_author_key"`
}

func (s *SBlockHeader) Deserialize() (BlockHeader, error) {
	parent := s.Parent
	parent_state_root := s.ParentStateRoot
	extrinsic_hash := s.ExtrinsicHash
	slot := s.Slot
	epoch_mark := s.EpochMark
	tickets_mark := [12]*TicketBody{}
	copy(tickets_mark[:], s.TicketsMark[:])

	offenders_mark := make([]Ed25519Key, len(s.OffendersMark))
	for i, v := range s.OffendersMark {
		offenders_mark[i] = Ed25519Key(common.FromHex(v))
	}

	author_index := s.AuthorIndex
	entropy_source_byte := common.FromHex(s.EntropySource)
	var entropy_source [96]byte
	copy(entropy_source[:], entropy_source_byte)

	seal_byte := common.FromHex(s.Seal)
	var seal [96]byte
	copy(seal[:], seal_byte)

	return BlockHeader{
		Parent:          parent,
		ParentStateRoot: parent_state_root,
		ExtrinsicHash:   extrinsic_hash,
		Slot:            slot,
		EpochMark:       epoch_mark,
		TicketsMark:     tickets_mark,
		//OffendersMark:   offenders_mark,
		AuthorIndex:   author_index,
		EntropySource: entropy_source,
		Seal:          seal,
	}, nil
}
