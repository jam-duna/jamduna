package types

import (
	"fmt"
	// "github.com/colorfulnotion/jam/scale"
	"encoding/json"
	"github.com/colorfulnotion/jam/common"
)

type Block struct {
	Header    BlockHeader   `json:"header"`
	Extrinsic ExtrinsicData `json:"extrinsic"`
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
		winningTicketsCopy := make([]*TicketBody, len(b.Header.WinningTicketsMark))
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
	c.Extrinsic.Tickets = make([]Ticket, len(b.Extrinsic.Tickets))
	copy(c.Extrinsic.Tickets, b.Extrinsic.Tickets)

	return c
}

/*func (b *Block) ConvertToSafroleHeader() SafroleHeader {
	return b.Header.ConvertToSafroleHeader()
} */

func (b *Block) GetHeader() BlockHeader {
	return b.Header
}

func (b *Block) TimeSlot() uint32 {
	return b.GetHeader().TimeSlot
}

func (b *Block) EpochMark() *EpochMark {
	return b.GetHeader().EpochMark
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
	return common.BytesToHash(common.ComputeHash(blockBytes))
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

func (b *Block) Tickets() []Ticket {
	extrinsicData := b.Extrinsic
	return extrinsicData.Tickets
}

func (b *Block) PreimageLookups() []PreimageLookup {
	extrinsicData := b.Extrinsic
	return extrinsicData.PreimageLookups
}

func (b *Block) Guarantees() []Guarantee {
	extrinsicData := b.Extrinsic
	return extrinsicData.Guarantees
}

func (b *Block) Assurances() []Assurance {
	extrinsicData := b.Extrinsic
	return extrinsicData.Assurances
}

func (b *Block) Disputes() []Dispute {
	extrinsicData := b.Extrinsic
	return extrinsicData.Disputes
}
