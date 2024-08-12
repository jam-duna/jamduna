package statedb

import (
	"fmt"
	// "github.com/colorfulnotion/jam/scale"
	"encoding/json"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/safrole"
)

// NewBlockHeader returns a fresh block header from scratch.
func NewBlockHeader() *BlockHeader {
	return &BlockHeader{}
}

func NewExtrinsic() ExtrinsicData {
	return ExtrinsicData{}
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
	ParentHash         common.Hash              `json:"parent_hash"`
	PriorStateRoot     common.Hash              `json:"prior_state_root"`
	ExtrinsicHash      common.Hash              `json:"extrinsic_hash"`
	TimeSlot           uint32                   `json:"timeslot"`
	EpochMark          *safrole.EpochMark       `json:"epoch_mark"`
	WinningTicketsMark []*safrole.TicketBody    `json:"winning_tickets_mark"`
	VerdictsMarkers    *safrole.VerdictMarker   `json:"verdict_markers"`
	OffenderMarkers    *safrole.OffendertMarker `json:"offender_markers"`
	BlockAuthorKey     uint16                   `json:"block_author_key"`
}

func NewBlock() *Block {
	//var b Block
	b := &Block{}
	return b
}

// StateCopy returns a state copy that is necessary for generating the next block
func (b *Block) Copy() *Block {
	if b == nil {
		return nil
	}
	c := NewBlock()

	// Copy Header fields
	c.Header.ParentHash = b.Header.ParentHash
	c.Header.PriorStateRoot = b.Header.PriorStateRoot
	c.Header.ExtrinsicHash = b.Header.ExtrinsicHash
	c.Header.TimeSlot = b.Header.TimeSlot

	if b.Header.EpochMark != nil {
		epochMarkCopy := *b.Header.EpochMark
		c.Header.EpochMark = &epochMarkCopy
	}

	if b.Header.WinningTicketsMark != nil {
		winningTicketsCopy := make([]*safrole.TicketBody, len(b.Header.WinningTicketsMark))
		for i, ticket := range b.Header.WinningTicketsMark {
			if ticket != nil {
				ticketCopy := *ticket
				winningTicketsCopy[i] = &ticketCopy
			}
		}
		c.Header.WinningTicketsMark = winningTicketsCopy
	}

	if b.Header.VerdictsMarkers != nil {
		judgementsMarkersCopy := *b.Header.VerdictsMarkers
		c.Header.VerdictsMarkers = &judgementsMarkersCopy
	}

	if b.Header.OffenderMarkers != nil {
		OffenderMarkersCopy := *b.Header.OffenderMarkers
		c.Header.OffenderMarkers = &OffenderMarkersCopy
	}

	c.Header.BlockAuthorKey = b.Header.BlockAuthorKey

	c.Header.VRFSignature = make([]byte, len(b.Header.VRFSignature))
	copy(c.Header.VRFSignature, b.Header.VRFSignature)

	c.Header.BlockSeal = make([]byte, len(b.Header.BlockSeal))
	copy(c.Header.BlockSeal, b.Header.BlockSeal)

	// Copy ExtrinsicData fields
	c.Extrinsic.Tickets = make([]safrole.Ticket, len(b.Extrinsic.Tickets))
	copy(c.Extrinsic.Tickets, b.Extrinsic.Tickets)

	return c
}

// T.D.P.A.G
type ExtrinsicData struct {
	Tickets []safrole.Ticket `json:"tickets"`
	//Judgetments []Judgement  `json:"judgements"`
	//PreimageExtrinsics []PreimageExtrinsic  `json:"preimages"`
	//AvailabilitySpecifications []availability  `json:"availabilities"`
	//Reports []Report  `json:"reports"`
}

func (e *ExtrinsicData) Bytes() []byte {
	enc, err := json.Marshal(e)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (e *ExtrinsicData) Hash() common.Hash {
	data := e.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

type Block struct {
	Header    BlockHeader   `json:"header"`
	Extrinsic ExtrinsicData `json:"extrinsic"`
}

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
	EpochMark *safrole.EpochMark `json:"epoch_mark"`
	// H_w
	WinningTicketsMark []*safrole.TicketBody `json:"winning_tickets_mark"`
	// H_j
	VerdictsMarkers *safrole.VerdictMarker `json:"verdict_markers"` // renamed from judgement
	// H_o
	OffenderMarkers *safrole.OffendertMarker `json:"offender_markers"`
	// H_i
	BlockAuthorKey uint16 `json:"block_author_key"`
	// H_v
	VRFSignature []byte `json:"vrf_signature"`
	// H_s
	BlockSeal []byte `json:"block_seal"`
}

func (b *Block) ConvertToSafroleHeader() safrole.SafroleHeader {
	return b.Header.ConvertToSafroleHeader()
}

func (b *Block) GetHeader() BlockHeader {
	return b.Header
}

func (b *Block) TimeSlot() uint32 {
	return b.GetHeader().TimeSlot
}

// ConvertToSafroleHeader converts a statedb.BlockHeader to a safrole.SafroleHeader
func (header *BlockHeader) ConvertToSafroleHeader() safrole.SafroleHeader {
	return safrole.SafroleHeader{
		ParentHash:         header.ParentHash,
		PriorStateRoot:     header.PriorStateRoot,
		ExtrinsicHash:      header.ExtrinsicHash,
		TimeSlot:           header.TimeSlot,
		EpochMark:          header.EpochMark,
		WinningTicketsMark: header.WinningTicketsMark,
		VerdictsMarkers:    header.VerdictsMarkers,
		OffenderMarkers:    header.OffenderMarkers,
		BlockAuthorKey:     header.BlockAuthorKey,
		VRFSignature:       header.VRFSignature,
		BlockSeal:          header.BlockSeal,
	}
}

func BlockFromBytes(data []byte) (*Block, error) {
	var b Block
	err := json.Unmarshal(data, &b)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON: %v", err)
	}
	return &b, nil
}

// Bytes returns the bytes of the block.
func (b *Block) Bytes() []byte {
	enc, err := json.Marshal(b)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

// Hash returns the hash of the block.
func (b *Block) Hash() common.Hash {
	blockBytes := b.Bytes()
	if blockBytes == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(blockBytes)
}

// Hash returns the hash of the block.
func (b *Block) ParentHash() common.Hash {
	return b.Header.ParentHash
}

func (b *Block) String() string {
	enc, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

func (b *Block) Tickets() []safrole.Ticket {
	extrinsicData := b.Extrinsic
	return extrinsicData.Tickets
}
