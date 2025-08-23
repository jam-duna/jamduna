package types

import (
	"bytes"
	"encoding/binary"

	"github.com/colorfulnotion/jam/common"
)

/* Availability Specifier
WorkPackageHash(h)	: the hash of the workpackage
BundleLength(l)		: the len of the packagebundle
ErasureRoot(u)		: MB([x∣x∈T[b♣,s♣]]) - root of a binary Merkle tree which (WBT) functions as a commitment to all data required for the auditing of the report and for use by later workpackages should they need to retrieve any data yielded.
					  The root of a transport (packagebundle Hashed and segment) encoding which is built by CDT
SegmentRoot(e)		: M(s) - root of a constant-depth, left-biased and zero-hash-padded binary Merkle tree (CDT) committing to the hashes of each of the exported segments of each work-item.
*/

// EQ(186):Availability Specifier C.25 11.5
type AvailabilitySpecifier struct {
	WorkPackageHash       common.Hash `json:"hash"`          // p
	BundleLength          uint32      `json:"length"`        // l
	ErasureRoot           common.Hash `json:"erasure_root"`  // u
	ExportedSegmentRoot   common.Hash `json:"exports_root"`  // e
	ExportedSegmentLength uint16      `json:"exports_count"` // n
}

func (as *AvailabilitySpecifier) String() string {
	return ToJSON(as)
}

// ToBytes serializes the GuaranteeCredential struct into a byte array
func (as *AvailabilitySpecifier) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, as.WorkPackageHash); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, as.BundleLength); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, as.ErasureRoot); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, as.ExportedSegmentRoot); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, as.ExportedSegmentLength); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a GuaranteeCredential struct
func (as *AvailabilitySpecifier) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ValidatorIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &as.WorkPackageHash); err != nil {
		return err
	}

	if err := binary.Read(buf, binary.LittleEndian, &as.BundleLength); err != nil {
		return err
	}

	if err := binary.Read(buf, binary.LittleEndian, &as.ErasureRoot); err != nil {
		return err
	}

	if err := binary.Read(buf, binary.LittleEndian, &as.ExportedSegmentRoot); err != nil {
		return err
	}

	if err := binary.Read(buf, binary.LittleEndian, &as.ExportedSegmentLength); err != nil {
		return err
	}

	return nil
}

// sharing (justified) DA chunks:  Vec<Hash> ++ Blob ++ Vec<Hash> ++ Vec<SegmentChunk> ++ Vec<Hash>.
// The Vec<Hash> will just be complementary Merkle-node-hashes from leaf to root.
// The first will contain hashes for the blob-subtree, the second for the segments subtree and the third for the super-tree.

// type AvailabilityJustification struct {}
