package types

import (
	"encoding/hex"
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

type SBlockHeader struct {
	// H_p
	Parent common.Hash `json:"parent"`
	// H_r
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// H_x
	ExtrinsicHash common.Hash `json:"extrinsic_hash"`
	// H_t
	Slot uint32 `json:"slot"`
	// H_e
	EpochMark *EpochMark `json:"epoch_mark"`
	// H_w
	WinningTicketsMark []*TicketBody `json:"winning_tickets_mark"`
	// H_j
	VerdictsMarkers *VerdictMarker `json:"verdict_markers"` // renamed from judgement
	// H_o
	OffenderMarkers *SOffenderMarker `json:"offender_markers"`
	// H_i
	BlockAuthorKey uint16 `json:"block_author_key"`
	// H_v
	VRFSignature string `json:"vrf_signature"`
	// H_s
	BlockSeal string `json:"block_seal"`
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
		ParentHash:         b.Parent,
		PriorStateRoot:     b.ParentStateRoot,
		ExtrinsicHash:      b.ExtrinsicHash,
		TimeSlot:           b.Slot,
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

func (s *SBlockHeader) Deserialize() (BlockHeader, error) {
	vrfSignature, err := hex.DecodeString(s.VRFSignature)
	if err != nil {
		return BlockHeader{}, fmt.Errorf("failed to decode VRF signature: %v", err)
	}

	blockSeal, err := hex.DecodeString(s.BlockSeal)
	if err != nil {
		return BlockHeader{}, fmt.Errorf("failed to decode Block Seal: %v", err)
	}
	var offenderMarkers *OffenderMarker
	if s.OffenderMarkers != nil {
		offenderMarkers, err = s.OffenderMarkers.Deserialize()
		if err != nil {
			return BlockHeader{}, err
		}
	} else {
	}

	return BlockHeader{
		Parent:             s.Parent,
		ParentStateRoot:    s.ParentStateRoot,
		ExtrinsicHash:      s.ExtrinsicHash,
		Slot:               s.Slot,
		EpochMark:          s.EpochMark,
		WinningTicketsMark: s.WinningTicketsMark,
		VerdictsMarkers:    s.VerdictsMarkers,
		OffenderMarkers:    offenderMarkers,
		BlockAuthorKey:     s.BlockAuthorKey,
		VRFSignature:       vrfSignature,
		BlockSeal:          blockSeal,
	}, nil
}
