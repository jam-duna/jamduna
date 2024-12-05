package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

type BlockHeader struct {
	// H_p
	ParentHeaderHash common.Hash `json:"parent"`
	// H_r
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// H_x
	ExtrinsicHash common.Hash `json:"extrinsic_hash"`
	// H_t
	Slot uint32 `json:"slot"`
	// H_e
	EpochMark *EpochMark `json:"epoch_mark,omitempty"`
	// H_w
	TicketsMark []*TicketBody `json:"tickets_mark,omitempty"`
	// H_o
	OffendersMark []Ed25519Key `json:"offenders_mark"`
	// H_i
	AuthorIndex uint16 `json:"author_index"`
	// H_v
	EntropySource BandersnatchVrfSignature `json:"entropy_source"`
	// H_s
	Seal BandersnatchVrfSignature `json:"seal"`
	// H_j
	// VerdictsMarkers *VerdictMarker `json:"verdict_markers"` // renamed from judgement
}

// BlockHeaderWithoutSig represents the BlockHeader without signature fields.
type BlockHeaderWithoutSig struct {
	ParentHeaderHash common.Hash  `json:"parent_hash"`
	PriorStateRoot   common.Hash  `json:"prior_state_root"`
	ExtrinsicHash    common.Hash  `json:"extrinsic_hash"`
	TimeSlot         uint32       `json:"timeslot"`
	EpochMark        *EpochMark   `json:"epoch_mark"`
	TicketsMark      *TicketsMark `json:"tickets_mark"`
	OffendersMark    []Ed25519Key `json:"offenders_mark"`
	AuthorIndex      uint16       `json:"block_author_key"`
}

func (b *BlockHeaderWithoutSig) String() string {
	jsonByte, _ := json.Marshal(b)
	return string(jsonByte)
}

// for codec
type CBlockHeader struct {
	ParentHeaderHash common.Hash              `json:"parent"`
	ParentStateRoot  common.Hash              `json:"parent_state_root"`
	ExtrinsicHash    common.Hash              `json:"extrinsic_hash"`
	Slot             uint32                   `json:"slot"`
	EpochMark        *EpochMark               `json:"epoch_mark,omitempty"`
	TicketsMark      *TicketsMark             `json:"tickets_mark"`
	OffendersMark    []Ed25519Key             `json:"offenders_mark"`
	AuthorIndex      uint16                   `json:"author_index"`
	EntropySource    BandersnatchVrfSignature `json:"entropy_source"`
	Seal             BandersnatchVrfSignature `json:"seal"`
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

// TODO FOR SEAN: why is ticet mark empty
// BytesWithoutSig returns the bytes of a block without the signature.
func (b *BlockHeader) BytesWithoutSig() []byte {
	// Create an instance of the new struct without the signature fields.
	bwoSig := BlockHeaderWithoutSig{
		ParentHeaderHash: b.ParentHeaderHash,
		PriorStateRoot:   b.ParentStateRoot,
		ExtrinsicHash:    b.ExtrinsicHash,
		TimeSlot:         b.Slot,
		EpochMark:        b.EpochMark,
		// TicketsMark:    b.TicketsMark,
		OffendersMark: b.OffendersMark,
		AuthorIndex:   b.AuthorIndex,
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
		ParentHeaderHash: b.ParentHeaderHash,
		ParentStateRoot:  b.ParentStateRoot,
		ExtrinsicHash:    b.ExtrinsicHash,
		Slot:             b.Slot,
		EpochMark:        b.EpochMark,
		//TicketsMark:     nil,
		OffendersMark: b.OffendersMark,
		AuthorIndex:   b.AuthorIndex,
		EntropySource: b.EntropySource,
		Seal:          b.Seal,
	}

	ticketMark, ok, _ := b.ConvertTicketsMark()
	if ok && ticketMark != nil {
		cbh.TicketsMark = ticketMark
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
