package types

import (
	//"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

/*
Section 12.1. Preimage Integration. Prior to accumulation, we must first integrate all preimages provided in the lookup extrinsic. The lookup extrinsic is a sequence of pairs of service indices and data. These pairs must be ordered and without duplicates (equation 154 requires this). The data must have been solicited by a service but not yet be provided.  See equations 153-155.

`PreimageExtrinsic` ${\bf E}_P$:
*/
// LookupEntry represents a single entry in the lookup extrinsic.
type Preimages struct {
	Requester uint32 `json:"requester"`
	Blob      []byte `json:"blob"`
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
	return ComputeAP(s, blob_hash)
}

func ComputeAP(s uint32, blob_hash []byte) common.Hash {
	blobHash := common.Blake2Hash(blob_hash)
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	return account_preimage_hash
}

func ComputeAL(s uint32, blob_hash []byte, blob_len uint32) common.Hash {
	blobHash := common.Blake2Hash(blob_hash)
	al_internal_key := common.Compute_preimageLookup_internal(blobHash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	return account_lookuphash
}

func (p Preimages) DeepCopy() (Preimages, error) {
	var copiedPreimageLookup Preimages

	// Serialize the original PreimageLookup to JSON
	data, err := Encode(p)
	if err != nil {
		return copiedPreimageLookup, err
	}

	// Deserialize the JSON back into a new PreimageLookup instance
	decoded, _, err := Decode(data, reflect.TypeOf(Preimages{}))
	if err != nil {
		return copiedPreimageLookup, err
	}
	copiedPreimageLookup = decoded.(Preimages)

	return copiedPreimageLookup, nil
}

// Bytes returns the bytes of the PreimageLookup.
func (p *Preimages) Bytes() []byte {
	enc, err := Encode(p)
	if err != nil {
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
	return common.Blake2Hash(data)
}

func (a *Preimages) UnmarshalJSON(data []byte) error {
	type Alias Preimages
	aux := &struct {
		*Alias
		Blob string `json:"blob"`
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	blobBytes := common.FromHex(aux.Blob)
	a.Blob = blobBytes
	return nil
}

func (a Preimages) MarshalJSON() ([]byte, error) {
	type Alias Preimages
	blob := common.HexString(a.Blob)
	return json.Marshal(&struct {
		*Alias
		Blob string `json:"blob"`
	}{
		Alias: (*Alias)(&a),
		Blob:  blob,
	})
}
