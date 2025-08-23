package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

type BlockHeader struct {
	// H_P
	ParentHeaderHash common.Hash `json:"parent"`
	// H_R
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// H_X
	ExtrinsicHash common.Hash `json:"extrinsic_hash"`
	// H_T
	Slot uint32 `json:"slot"`
	// H_E
	EpochMark *EpochMark `json:"epoch_mark,omitempty"`
	// H_W
	TicketsMark []*TicketBody `json:"tickets_mark,omitempty"`
	// H_O
	OffendersMark []Ed25519Key `json:"offenders_mark"`
	// H_I
	AuthorIndex uint16 `json:"author_index"`
	// H_V
	EntropySource BandersnatchVrfSignature `json:"entropy_source"`
	// H_S
	Seal BandersnatchVrfSignature `json:"seal"`
}

// BlockHeaderWithoutSig represents the BlockHeader without signature fields.
type BlockHeaderWithoutSig struct {
	ParentHeaderHash common.Hash              `json:"parent"`            // H_P
	PriorStateRoot   common.Hash              `json:"parent_state_root"` // H_R
	ExtrinsicHash    common.Hash              `json:"extrinsic_hash"`    // H_X
	TimeSlot         uint32                   `json:"slot"`              // H_T
	EpochMark        *EpochMark               `json:"epoch_mark"`        // H_E
	TicketsMark      *TicketsMark             `json:"tickets_mark"`      // H_W
	AuthorIndex      uint16                   `json:"author_index"`      // H_I
	EntropySource    BandersnatchVrfSignature `json:"entropy_source"`    // H_V
	OffendersMark    []Ed25519Key             `json:"offenders_mark"`    // H_O ???
}

func (b *BlockHeaderWithoutSig) String() string {
	jsonByte, _ := json.Marshal(b)
	return string(jsonByte)
}

// for codec
type CBlockHeader struct {
	ParentHeaderHash common.Hash              `json:"parent"`               // H_P
	ParentStateRoot  common.Hash              `json:"parent_state_root"`    // H_R
	ExtrinsicHash    common.Hash              `json:"extrinsic_hash"`       // H_X
	Slot             uint32                   `json:"slot"`                 // H_T
	EpochMark        *EpochMark               `json:"epoch_mark,omitempty"` // H_E
	TicketsMark      *TicketsMark             `json:"tickets_mark"`         // H_W
	AuthorIndex      uint16                   `json:"author_index"`         // H_I
	EntropySource    BandersnatchVrfSignature `json:"entropy_source"`       // H_V
	OffendersMark    []Ed25519Key             `json:"offenders_mark"`       // H_O
	Seal             BandersnatchVrfSignature `json:"seal"`                 // H_S
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
	return common.Blake2Hash(unsignedBytes)
}

func (b *BlockHeader) HeaderHash() common.Hash {
	return b.Hash()
}

// Hash returns the hash of the block in unsigned form.
func (b *BlockHeader) Hash() common.Hash {
	data := b.BytesWithSig()
	return common.Blake2Hash(data)
}

func (b *BlockHeader) BytesWithSig() []byte {
	cb, _ := b.toCBlockHeader()
	enc, err := Encode(cb)
	if err != nil {
		return nil
	}
	return enc
}

func (b BlockHeader) Encode() []byte {
	cb, _ := b.toCBlockHeader()
	enc, err := Encode(cb)
	if err != nil {
		return nil
	}
	return enc
}

func (b BlockHeader) Decode(data []byte) (interface{}, uint32) {
	decoded, dataLen, err := Decode(data, reflect.TypeOf(CBlockHeader{}))
	if err != nil {
		return nil, 0
	}
	cbh := decoded.(CBlockHeader)
	b.fromCBlockHeader(&cbh)
	return b, dataLen
}

// BytesWithoutSig returns the clean bytes of a block for sealing
func (b *BlockHeader) BytesWithoutSig() []byte {
	// Create an instance of the new struct without the signature fields.
	bwoSig := BlockHeaderWithoutSig{
		ParentHeaderHash: b.ParentHeaderHash,
		PriorStateRoot:   b.ParentStateRoot,
		ExtrinsicHash:    b.ExtrinsicHash,
		TimeSlot:         b.Slot,
		EpochMark:        b.EpochMark,
		OffendersMark:    b.OffendersMark,
		AuthorIndex:      b.AuthorIndex,
		EntropySource:    b.EntropySource,
	}
	ticketMark, ok, _ := b.ConvertTicketsMark()
	if ok && ticketMark != nil {
		bwoSig.TicketsMark = ticketMark
	}

	// Marshal the new struct to codec
	enc, err := Encode(bwoSig)
	if err != nil {
		return nil
	}
	return enc
}

func (b *BlockHeader) String() string {
	return ToJSON(b)
}

func (b *BlockHeader) ConvertTicketsMark() (*TicketsMark, bool, error) {

	var ticketsMark TicketsMark
	ticketCnt := 0

	// Handle TicketsMark conversion
	if len(b.TicketsMark) > 0 {
		if len(b.TicketsMark) > EpochLength {
			return nil, false, fmt.Errorf("TicketsMark length exceeds EpochLength")
		}

		if len(b.TicketsMark) != EpochLength {
			return nil, false, fmt.Errorf("TicketsMark length mismatch")
		}

		for i := 0; i < len(b.TicketsMark); i++ {
			//ticketsMark[i] = *b.TicketsMark[i]
			if b.TicketsMark[i] != nil {
				ticketCnt++
				ticketsMark[i] = *b.TicketsMark[i]
			} else {
				ticketsMark[i] = TicketBody{}
			}
		}
		if ticketCnt != EpochLength {
			return nil, false, fmt.Errorf("TicketsMark containing nil ticket")
		}
		return &ticketsMark, true, nil
	}
	return nil, true, nil
}

// type casting BlockHeader -> CBlockHeader
func (b *BlockHeader) toCBlockHeader() (*CBlockHeader, error) {
	cbh := &CBlockHeader{
		ParentHeaderHash: b.ParentHeaderHash, // H_P
		ParentStateRoot:  b.ParentStateRoot,  // H_R
		ExtrinsicHash:    b.ExtrinsicHash,    // H_X
		Slot:             b.Slot,             // H_T
		EpochMark:        b.EpochMark,        // H_E
		TicketsMark:      nil,                // H_W
		AuthorIndex:      b.AuthorIndex,      // H_I
		EntropySource:    b.EntropySource,    // H_V
		OffendersMark:    b.OffendersMark,    // H_O
		Seal:             b.Seal,             // H_S
	}

	// updating non-nil H_W
	ticketMark, ok, _ := b.ConvertTicketsMark()
	if ok && ticketMark != nil {
		cbh.TicketsMark = ticketMark //H_W
	}
	return cbh, nil
}

// type casting CBlockHeader -> BlockHeader
func (b *BlockHeader) fromCBlockHeader(cbh *CBlockHeader) {
	b.ParentHeaderHash = cbh.ParentHeaderHash
	b.ParentStateRoot = cbh.ParentStateRoot
	b.ExtrinsicHash = cbh.ExtrinsicHash
	b.Slot = cbh.Slot
	b.EpochMark = cbh.EpochMark
	b.OffendersMark = cbh.OffendersMark
	b.AuthorIndex = cbh.AuthorIndex
	b.EntropySource = cbh.EntropySource
	b.Seal = cbh.Seal

	// Handle TicketsMark conversion
	if cbh.TicketsMark != nil {
		ticketsMark := make([]*TicketBody, 0, EpochLength)
		for i := 0; i < len(cbh.TicketsMark); i++ {
			// Create a copy of the TicketBody
			ticket := cbh.TicketsMark[i]
			ticketsMark = append(ticketsMark, &ticket)
		}
		b.TicketsMark = ticketsMark
	} else {
		b.TicketsMark = nil
	}
}

func (b BlockHeader) MarshalJSON() ([]byte, error) {
	cbh, err := b.toCBlockHeader()
	if err != nil {
		return nil, err
	}
	return json.Marshal(cbh)
}

func (b *BlockHeader) UnmarshalJSON(data []byte) error {
	var cbh CBlockHeader
	if err := json.Unmarshal(data, &cbh); err != nil {
		return err
	}
	b.fromCBlockHeader(&cbh)
	return nil
}

func (a *CBlockHeader) UnmarshalJSON(data []byte) error {
	var s struct {
		ParentHeaderHash common.Hash  `json:"parent"`
		ParentStateRoot  common.Hash  `json:"parent_state_root"`
		ExtrinsicHash    common.Hash  `json:"extrinsic_hash"`
		Slot             uint32       `json:"slot"`
		EpochMark        *EpochMark   `json:"epoch_mark"`
		TicketsMark      *TicketsMark `json:"tickets_mark"`
		OffenderMarker   []string     `json:"offenders_mark"`
		AuthorIndex      uint16       `json:"author_index"`
		EntropySource    string       `json:"entropy_source"`
		Seal             string       `json:"seal"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	a.ParentHeaderHash = s.ParentHeaderHash
	a.ParentStateRoot = s.ParentStateRoot
	a.ExtrinsicHash = s.ExtrinsicHash
	a.Slot = s.Slot
	a.EpochMark = s.EpochMark
	a.TicketsMark = s.TicketsMark

	offendersMark := make([]Ed25519Key, len(s.OffenderMarker))
	for i, v := range s.OffenderMarker {
		offendersMark[i] = Ed25519Key(common.FromHex(v))
	}
	a.OffendersMark = offendersMark

	a.AuthorIndex = s.AuthorIndex

	entropySourceByte := common.FromHex(s.EntropySource)
	var entropySource BandersnatchVrfSignature
	copy(entropySource[:], entropySourceByte)
	a.EntropySource = entropySource

	sealByte := common.FromHex(s.Seal)
	var seal BandersnatchVrfSignature
	copy(seal[:], sealByte)
	a.Seal = seal

	return nil
}

func (a *CBlockHeader) MarshalJSON() ([]byte, error) {
	offendersMark := []string{}
	for _, v := range a.OffendersMark {
		offendersMark = append(offendersMark, common.HexString(v[:]))
	}

	return json.Marshal(&struct {
		ParentHeaderHash common.Hash  `json:"parent"`
		ParentStateRoot  common.Hash  `json:"parent_state_root"`
		ExtrinsicHash    common.Hash  `json:"extrinsic_hash"`
		Slot             uint32       `json:"slot"`
		EpochMark        *EpochMark   `json:"epoch_mark"`
		TicketsMark      *TicketsMark `json:"tickets_mark"`
		OffenderMarker   []string     `json:"offenders_mark"`
		AuthorIndex      uint16       `json:"author_index"`
		EntropySource    string       `json:"entropy_source"`
		Seal             string       `json:"seal"`
	}{
		ParentHeaderHash: a.ParentHeaderHash,
		ParentStateRoot:  a.ParentStateRoot,
		ExtrinsicHash:    a.ExtrinsicHash,
		Slot:             a.Slot,
		EpochMark:        a.EpochMark,
		TicketsMark:      a.TicketsMark,
		OffenderMarker:   offendersMark,
		AuthorIndex:      a.AuthorIndex,
		EntropySource:    common.HexString(a.EntropySource[:]),
		Seal:             common.HexString(a.Seal[:]),
	})
}
