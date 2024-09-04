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

type SBlock struct {
	Header    SBlockHeader   `json:"header"`
	Extrinsic SExtrinsicData `json:"extrinsic"`
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
	c.Header.Parent = b.Header.Parent
	c.Header.ParentStateRoot = b.Header.ParentStateRoot
	c.Header.ExtrinsicHash = b.Header.ExtrinsicHash
	c.Header.Slot = b.Header.Slot

	if b.Header.EpochMark != nil {
		epochMarkCopy := *b.Header.EpochMark
		c.Header.EpochMark = &epochMarkCopy
	}

	var winningTicketsCopy [12]*TicketBody
	for i, ticket := range b.Header.TicketsMark {
		if ticket != nil {
			ticketCopy := *ticket
			winningTicketsCopy[i] = &ticketCopy
		}
	}
	c.Header.TicketsMark = winningTicketsCopy

/*
	if b.Header.VerdictsMarkers != nil {
		judgementsMarkersCopy := *b.Header.VerdictsMarkers
		c.Header.VerdictsMarkers = &judgementsMarkersCopy
	}
*/
	if b.Header.OffenderMarkers != nil {
		c.Header.OffenderMarkers = b.Header.OffenderMarkers
	}

	c.Header.AuthorIndex = b.Header.AuthorIndex

	copy(c.Header.EntropySource[:], b.Header.EntropySource[:])

	copy(c.Header.Seal[:], b.Header.Seal[:])

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
	return b.GetHeader().Slot
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
	return b.Header.Parent
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

func (b *Block) PreimageLookups() []Preimages {
	extrinsicData := b.Extrinsic
	return extrinsicData.Preimages
}

func (b *Block) Guarantees() []Guarantee {
	extrinsicData := b.Extrinsic
	return extrinsicData.Guarantees
}

func (b *Block) Assurances() []Assurance {
	extrinsicData := b.Extrinsic
	return extrinsicData.Assurances
}

func (b *Block) Disputes() Dispute {
	extrinsicData := b.Extrinsic
	return extrinsicData.Disputes
}

func (s *SBlock) Deserialize() (Block, error) {
	header, err := s.Header.Deserialize()
	if err != nil {
		return Block{}, err
	}
	extrinsic, err := s.Extrinsic.Deserialize()
	if err != nil {
		return Block{}, err
	}

	return Block{
		Header:    header,
		Extrinsic: extrinsic,
	}, nil
}
