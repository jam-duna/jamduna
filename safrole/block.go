package safrole

import (
	// "fmt"

	// "github.com/colorfulnotion/jam/scale"
	"github.com/ethereum/go-ethereum/common"
)

// BlockHeader
type BlockHeader struct {
	ParentHash       common.Hash       `json:"parentHash"`       // H_p
	ParentStateRoot  common.Hash       `json:"priorStateRoot"`   // H_r
	ExtrinsicHash    common.Hash       `json:"extrinsicHash"`    // H_x
	BlockNumber      uint64            `json:"blockNumber"`      // H_t time slot index
	Epoch            uint64            `json:"epoch"`            // H_e
	WinningTickets   []WinningTicket   `json:"winningTickets"`   // H_w
	JudgementMarkers []JudgementMarker `json:"judgementMarkers"` // H_j
	BlockAuthorKey   uint64            `json:"blockAuthorKey"`   // H_k
	VRFSignature     []byte            `json:"vrfSignature"`     // H_v
	Seal             common.Hash       `json:"seal"`             // H_s
}

type WinningTicket struct {
	// Define fields for WinningTicket
}

type JudgementMarker struct {
	// Define fields for JudgementMarker
}

type TicketEnvelope struct {
	Attempt       byte
	Extra         []byte
	RingSignature []byte
	TicketID      common.Hash
}

type ValidatorKey struct {
	BandersnatchPublicKey [32]byte
}

type GenesisAuthorities struct {
	Authorities [NumValidators]ValidatorKey
}

const (
	RedundancyFactor = 1
	EpochNumSlots    = 600
	NumValidators    = 8
	NumAttempts      = 100
)

// NewBlockHeader returns a fresh block header from scratch.
func NewBlockHeader() *BlockHeader {
	return &BlockHeader{}
}

func computeHash(data []byte) []byte {
	// Implement hash computation
	return []byte{}
}

// Hash returns the hash of the block in unsigned form.
func (b *BlockHeader) Hash() common.Hash {
	data := b.BytesWithoutSig()
	return common.BytesToHash(computeHash(data))
}

// BytesWithoutSig returns the bytes of a block without signature.
func (b *BlockHeader) BytesWithoutSig() []byte {
	enc := []byte{} // TODO
	return enc
}

// UnsignedHash returns the hash of the block in unsigned form.
func (b *BlockHeader) UnsignedHash() common.Hash {
	unsignedBytes := b.BytesWithoutSig()
	return common.BytesToHash(computeHash(unsignedBytes))
}

// Number returns the block's block number.
func (b *BlockHeader) Number() uint64 {
	return b.BlockNumber
}

// Encode returns Scale encoded version of the block.
func (b *BlockHeader) Encode() ([]byte, error) {
	return b.BytesWithoutSig(), nil
}
