package types

import (
	"encoding/binary"
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

func (p *Preimages) Hash() common.Hash {
	return common.Blake2Hash(p.Blob)
}

func (p *Preimages) AccountHash() common.Hash {
	return common.Blake2Hash(append(p.Blob, binary.LittleEndian.AppendUint32(nil, uint32(p.Requester))...))
}

func (p *Preimages) BlobLength() uint32 {
	return uint32(len(p.Blob))
}

func (p *Preimages) Service_Index() uint32 {
	return p.Requester
}

func (p *Preimages) String() string {
	return fmt.Sprintf("ServiceIndex=%v, (h,z)=(%v,%v)", p.Service_Index(), p.Hash(), p.BlobLength())
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
