package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

type Block struct {
	Header    BlockHeader   `json:"header"`
	Extrinsic ExtrinsicData `json:"extrinsic"`
}

type CBlock struct {
	Header    CBlockHeader  `json:"header"`
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
	c.Header.Parent = b.Header.Parent
	c.Header.ParentStateRoot = b.Header.ParentStateRoot
	c.Header.ExtrinsicHash = b.Header.ExtrinsicHash
	c.Header.Slot = b.Header.Slot

	if b.Header.EpochMark != nil {
		epochMarkCopy := *b.Header.EpochMark
		c.Header.EpochMark = &epochMarkCopy
	}

	c.Header.TicketsMark = b.Header.TicketsMark

	// if b.Header.VerdictsMarkers != nil {
	// 	judgementsMarkersCopy := *b.Header.VerdictsMarkers
	// 	c.Header.VerdictsMarkers = &judgementsMarkersCopy
	// }

	if b.Header.OffendersMark != nil {
		c.Header.OffendersMark = b.Header.OffendersMark
	}

	c.Header.AuthorIndex = b.Header.AuthorIndex

	copy(c.Header.EntropySource[:], b.Header.EntropySource[:])

	copy(c.Header.Seal[:], b.Header.Seal[:])

	// Copy ExtrinsicData fields
	c.Extrinsic.Tickets = make([]Ticket, len(b.Extrinsic.Tickets))
	copy(c.Extrinsic.Tickets, b.Extrinsic.Tickets)

	c.Extrinsic.Preimages = make([]Preimages, len(b.Extrinsic.Preimages))
	copy(c.Extrinsic.Preimages, b.Extrinsic.Preimages)

	c.Extrinsic.Guarantees = make([]Guarantee, len(b.Extrinsic.Guarantees))
	copy(c.Extrinsic.Guarantees, b.Extrinsic.Guarantees)

	c.Extrinsic.Assurances = make([]Assurance, len(b.Extrinsic.Assurances))
	copy(c.Extrinsic.Assurances, b.Extrinsic.Assurances)

	c.Extrinsic.Disputes.Verdict = make([]Verdict, len(b.Extrinsic.Disputes.Verdict))
	copy(c.Extrinsic.Disputes.Verdict, b.Extrinsic.Disputes.Verdict)
	c.Extrinsic.Disputes.Culprit = make([]Culprit, len(b.Extrinsic.Disputes.Culprit))
	copy(c.Extrinsic.Disputes.Culprit, b.Extrinsic.Disputes.Culprit)
	c.Extrinsic.Disputes.Fault = make([]Fault, len(b.Extrinsic.Disputes.Fault))
	copy(c.Extrinsic.Disputes.Fault, b.Extrinsic.Disputes.Fault)

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
	decoded, _, err := Decode(data, reflect.TypeOf(Block{}))
	if err != nil {
		return nil, err
	}
	b = decoded.(Block)
	return &b, nil
}

// Bytes returns the bytes of the block.
func (b *Block) Bytes() []byte {
	cb, err := b.toCBlock()
	if err != nil {
		return nil
	}
	enc, err := Encode(cb)
	if err != nil {
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
	return common.Blake2Hash(blockBytes)
}

// Hash returns the hash of the block.
func (b *Block) ParentHash() common.Hash {
	return b.Header.Parent
}

func (b *Block) Str() string {
	out := fmt.Sprintf("[N%d] ", b.Header.AuthorIndex)
	out += fmt.Sprintf("H_t=%d ", b.Header.Slot)
	out += fmt.Sprintf("H_r=%s ", common.Str(b.Header.ParentStateRoot))
	if b.Header.EpochMark != nil {
		out += b.Header.EpochMark.String()
	}
	if len(b.Header.TicketsMark) > 0 {
		out += fmt.Sprintf(" \033[32m WinningTickets\033[0m(%d)", len(b.Header.TicketsMark))
	}
	if len(b.Extrinsic.Tickets) > 0 {
		out += fmt.Sprintf(" \033[34m |E_T|=%d\033[0m", len(b.Extrinsic.Tickets))
	}
	if len(b.Extrinsic.Guarantees) > 0 {
		out += fmt.Sprintf(" \033[31m |E_G|=%d\033[0m", len(b.Extrinsic.Guarantees))
	}
	/*if len(b.Extrinsic.Disputes) > 0 {
		out += fmt.Sprintf(" \032[32m |E_D|=%d\033[0m %d", len(b.Extrinsic.Disputes))
	} */
	if len(b.Extrinsic.Preimages) > 0 {
		out += fmt.Sprintf(" \033[31m |E_P|=%d\033[0m", len(b.Extrinsic.Preimages))
	}
	if len(b.Extrinsic.Assurances) > 0 {
		out += fmt.Sprintf(" \033[31m |E_A|=%d\033[0m", len(b.Extrinsic.Assurances))
	}
	return out
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

// type casting CBlock -> Block
func (b *Block) fromCBlock(cb *CBlock) {
	if cb == nil {
		return
	}

	// Convert CBlockHeader to BlockHeader
	b.Header.fromCBlockHeader(&cb.Header)

	// Copy the Extrinsic field directly
	b.Extrinsic = cb.Extrinsic
}

// type casting Block -> CBlock
func (b *Block) toCBlock() (*CBlock, error) {
	// Convert BlockHeader to CBlockHeader
	cbh, err := b.Header.toCBlockHeader()
	if err != nil {
		return nil, err
	}

	// Create the CBlock
	cb := &CBlock{
		Header:    *cbh,
		Extrinsic: b.Extrinsic,
	}

	return cb, nil
}

func (b Block) MarshalJSON() ([]byte, error) {
	// Convert Block to CBlock
	cb, err := b.toCBlock()
	if err != nil {
		return nil, err
	}
	// Marshal CBlock to JSON
	return json.Marshal(cb)
}

func (b *Block) UnmarshalJSON(data []byte) error {
	// Unmarshal data into a CBlock
	var cb CBlock
	if err := json.Unmarshal(data, &cb); err != nil {
		return err
	}
	// Convert CBlock to Block
	b.fromCBlock(&cb)
	return nil
}
