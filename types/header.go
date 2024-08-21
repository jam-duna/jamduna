package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

type BlockHeader struct {
	// H_p
	ParentHash common.Hash `json:"parent_hash"`
	// H_r
	PriorStateRoot common.Hash `json:"prior_state_root"`
	// H_x
	ExtrinsicHash common.Hash `json:"extrinsic_hash"`
	// H_t
	TimeSlot uint32 `json:"timeslot"`
	// H_e
	EpochMark *EpochMark `json:"epoch_mark"`
	// H_w
	WinningTicketsMark []*TicketBody `json:"winning_tickets_mark"`
	// H_j
	VerdictsMarkers *VerdictMarker `json:"verdict_markers"` // renamed from judgement
	// H_o
	OffenderMarkers *OffenderMarker `json:"offender_markers"`
	// H_i
	BlockAuthorKey uint16 `json:"block_author_key"`
	// H_v
	VRFSignature []byte `json:"vrf_signature"`
	// H_s
	BlockSeal []byte `json:"block_seal"`
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
		ParentHash:         b.ParentHash,
		PriorStateRoot:     b.PriorStateRoot,
		ExtrinsicHash:      b.ExtrinsicHash,
		TimeSlot:           b.TimeSlot,
		EpochMark:          b.EpochMark,
		WinningTicketsMark: b.WinningTicketsMark,
		VerdictsMarkers:    b.VerdictsMarkers,
		OffenderMarkers:    b.OffenderMarkers,
		BlockAuthorKey:     b.BlockAuthorKey,
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
	ParentHash         common.Hash     `json:"parent_hash"`
	PriorStateRoot     common.Hash     `json:"prior_state_root"`
	ExtrinsicHash      common.Hash     `json:"extrinsic_hash"`
	TimeSlot           uint32          `json:"timeslot"`
	EpochMark          *EpochMark      `json:"epoch_mark"`
	WinningTicketsMark []*TicketBody   `json:"winning_tickets_mark"`
	VerdictsMarkers    *VerdictMarker  `json:"verdict_markers"`
	OffenderMarkers    *OffenderMarker `json:"offender_markers"`
	BlockAuthorKey     uint16          `json:"block_author_key"`
}
