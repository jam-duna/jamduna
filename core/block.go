package safron

import (
       //	"github.com/colorfulnotion/jam/scale"
	//"github.com/ethereum/go-ethereum/common"
)

// Block is the core data structure of the wolk blockchain
type BlockHeader struct {
	ParentHash common.Hash `json:"parentHash"` // H_p

	// ParentStateRoot - root of a Merkle trie composed by the mapping of the prior state’s Merkle root,which by definition is also the parent block’s posterior state.
	ParentStateRoot common.Hash `json:"priorStateRoot"` // H_r

	ExtrinsicHash common.Hash `json:"extrinsicHash"` // H_x
	BlockNumber   uint64      `json:"blockNumber"`   // H_t time slot index

	// H_e - specifies key and entropy relevant to the following epoch in case the ticket contest does not complete adequately
	Epoch uint64 `json:"epoch"`

	// Similarly, the winning-tickets marker, if not ∅, provides the series of 600 slot sealing “tickets” for the next epoch
	WinningTickets []WinningTicket `json:"winningTickets"` // H_w

	JudgementMarkers []JudgementMarker `json:"judgementMarkers"` // H_j

	// H_k - block author key - index into current validator set
	BlockAuthorKey uint64 `json:"blockAuthorKey"`
	VRFSignature   []byte `json:"vrfSignature"` // H_v - entropy yielding VRF Signature

	// sealing key is in fact a pseudonym for some validator which was agreed the privilege of authoring a block in the corresponding timeslot.
	Seal common.Hash `json:"seal"` // H_s - block seal
}

type WinningTicket struct {
}

type JudgementMarker struct {
}

const (
// L = 24
// We only require implementations to store headers of ancestors which were authored in the previous L = 24 hours of any block B they wish to validate.
)

// NewBlock returns a fresh block from scratch
func NewBlockHeader() *BlockHeader {
	b := &BlockHeader{}
	// ...
	return b
}

func Computehash([]byte) []byte {
	return []byte{}
}

// Hash returns the hash of the block in unsigned form
func (b *BlockHeader) Hash() (h common.Hash) {
	data, _ := scale.EncodeToBytes(b)
	return common.BytesToHash(Computehash(data))
}

// BytesWithoutSig returns the bytes of a block without signature
func (b *BlockHeader) BytesWithoutSig() []byte {
	enc, _ := scale.EncodeToBytes(&b)
	return enc
}

// UnsignedHash returns the has of the block in unsigned form
func (b *BlockHeader) UnsignedHash() common.Hash {
	unsignedBytes := b.BytesWithoutSig()
	return common.BytesToHash(Computehash(unsignedBytes))
}

// Number returns the block's blockNumber
func (b *BlockHeader) Number() (n uint64) {
	return b.BlockNumber
}

// Encode returns Scale encoded version of the block
func (b *BlockHeader) Encode() ([]byte, error) {
	return scale.EncodeToBytes(b)
}
