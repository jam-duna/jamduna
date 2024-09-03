package types

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

/*
Section 12.1. Preimage Integration. Prior to accumulation, we must first integrate all preimages provided in the lookup extrinsic. The lookup extrinsic is a sequence of pairs of service indices and data. These pairs must be ordered and without duplicates (equation 154 requires this). The data must have been solicited by a service but not yet be provided.  See equations 153-155.

`PreimageExtrinsic` ${\bf E}_P$:
*/
// LookupEntry represents a single entry in the lookup extrinsic.
/*
type PreimageLookup struct {
	ServiceIndex uint32 `json:"service_index"`
	Data         []byte `json:"data"`
}
*/
type Preimages struct {
	Requester uint32 `json:"requester"`
	Blob      []byte `json:"blob"`
}

type SPreimages struct {
	Requester uint32 `json:"requester"`
	Blob      string `json:"blob"`
}

func (p *Preimages) BlobHash() common.Hash {
	return common.BytesToHash(common.ComputeHash(p.Blob))
}

func (p *Preimages) BlobLength() uint32 {
	return uint32(len(p.Blob))
}

func (p *Preimages) Service_Index() uint32 {
	return p.Requester
}

func (p *Preimages) String() string {
	s := fmt.Sprintf("ServiceIndex=%v, (h,z)=(%v,%v), a_l=%v, a_p=%v\n", p.Service_Index(), p.BlobHash(), p.BlobLength(), p.AccountLookupHash(), p.AccountPreimageHash())
	return s
}

// A_l --- C(s, (E4(l) ⌢ (¬h4:))
func (p *Preimages) AccountLookupHash() common.Hash {
	s := p.Requester
	blob_hash := p.BlobHash().Bytes()
	blob_len := p.BlobLength()
	return ComputeAL(s, blob_hash, blob_len)
}

// A_p --- c(s, H(p))
func (p *Preimages) AccountPreimageHash() common.Hash {
	s := p.Requester
	blob_hash := p.BlobHash().Bytes()
	return ComputeC_SH(s, blob_hash)
}

// EQ 290 - state-key constructor functions C(s,h)
func ComputeC_SH(s uint32, h []byte) common.Hash {
	//s: service_index
	//h: hash_component (assumed to be exact 32bytes)
	//(s,h) ↦ [n0,h0,n1,h1,n2,h2,n3,h3,h4,h5,...,h27] where n = E4(s)
	stateKey := make([]byte, 32)
	nBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nBytes, s) // n = E4(s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = nBytes[i]
		if i < len(h) {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	return common.BytesToHash(stateKey)
}

func ComputeAL(s uint32, blob_hash []byte, blob_len uint32) common.Hash {
	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len) // E4(l)
	blob_hash = falseBytes(blob_hash[4:])           // (¬h4:)
	l_and_h := append(lBytes, blob_hash...)         // (E4(l) ⌢ (¬h4:)
	account_lookuphash := ComputeC_SH(s, l_and_h)   // C(s, (E4(l) ⌢ (¬h4:))
	return account_lookuphash
}

// Implement "¬"
func falseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = 0xFF - data[i]
		// result[i] = ^data[i]
	}
	return result
}

func (p Preimages) DeepCopy() (Preimages, error) {
	var copiedPreimageLookup Preimages

	// Serialize the original PreimageLookup to JSON
	data, err := json.Marshal(p)
	if err != nil {
		return copiedPreimageLookup, err
	}

	// Deserialize the JSON back into a new PreimageLookup instance
	err = json.Unmarshal(data, &copiedPreimageLookup)
	if err != nil {
		return copiedPreimageLookup, err
	}

	return copiedPreimageLookup, nil
}

// Bytes returns the bytes of the PreimageLookup.
func (p *Preimages) Bytes() []byte {
	enc, err := json.Marshal(p)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (p *Preimages) Hash() common.Hash {
	data := p.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (s *SPreimages) Deserialize() (Preimages, error) {
	blob := common.FromHex(s.Blob)
	return Preimages{
		Requester: s.Requester,
		Blob:      blob,
	}, nil
}
