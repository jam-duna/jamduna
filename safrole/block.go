package safrole

import (
	"fmt"
	// "github.com/colorfulnotion/jam/scale"
	"encoding/binary"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/blake2b"
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

// 6.2 Header Digest log
// DigestItem represents an individual item in the Digest log
type DigestItem struct {
	ID   [4]byte // Fixed 4-byte ASCII string
	Data []byte  // SCALE-encoded protocol-specific structure
}

// Digest represents the ordered sequence of DigestItems
type Digest struct {
	Items []DigestItem
}

// Sassafras-related entry types
const (
	SassafrasID        = "SASS"
	EpochChangeSignal  = "EpochChangeSignal"  // information about the next epoch; mandatory for the first block of a new epoch.
	EpochTicketsSignal = "EpochTicketsSignal" // sequence of tickets for claiming slots in the next epoch; mandatory for the first block in the epoch's tail
	SlotClaimInfo      = "SlotClaimInfo"      // Additional data required for block verification; mandatory and must be the second-to-last entry in the log.
	Seal               = "Seal"               // Block signature added by the block author; mandatory and must be the last entry in the log.
)

// NewDigestItem creates a new DigestItem with given id and data
func NewDigestItem(id string, data []byte) (DigestItem, error) {
	if len(id) != 4 {
		return DigestItem{}, fmt.Errorf("id must be 4 characters")
	}
	var idArray [4]byte
	copy(idArray[:], id)
	return DigestItem{
		ID:   idArray,
		Data: data,
	}, nil
}

type WinningTicket struct {
	// Define fields for WinningTicket
}

type JudgementMarker struct {
	// Define fields for JudgementMarker
}

type ValidatorKey struct {
	BandersnatchPublicKey [32]byte
}

type GenesisAuthorities struct {
	Authorities [NumValidators]ValidatorKey
}

// NewBlockHeader returns a fresh block header from scratch.
func NewBlockHeader() *BlockHeader {
	return &BlockHeader{}
}

// computeHash computes the BLAKE2b hash of the given data
func computeHash(data []byte) []byte {
	hash := blake2b.Sum256(data)
	return hash[:]
}

func uint32ToBytes(val uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, val)
	return bytes
}

func blake2AsHex(data []byte) common.Hash {
	//hash := blake2b.Sum256([]byte(data))
	//return hex.EncodeToString(hash[:])
	return common.BytesToHash(computeHash(data))
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
