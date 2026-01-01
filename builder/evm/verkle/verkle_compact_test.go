package verkle

import (
	"reflect"
	"testing"

	verkle "github.com/ethereum/go-verkle"
)

func TestCompactProofRoundTrip(t *testing.T) {
	stem1 := [31]byte{1}
	stem2 := [31]byte{2}
	cl1 := [32]byte{7}
	cl2 := [32]byte{8}
	cr1 := [32]byte{9}
	cr2 := [32]byte{10}

	original := &verkle.VerkleProof{
		OtherStems:            [][31]byte{stem1, stem2},
		DepthExtensionPresent: []byte{0x0a, 0x1b},
		CommitmentsByPath:     [][32]byte{{3}, {4}, {5}},
		D:                     [32]byte{6},
		IPAProof: &verkle.IPAProof{
			CL:              [8][32]byte{cl1, cl2},
			CR:              [8][32]byte{cr1, cr2},
			FinalEvaluation: [32]byte{11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 12},
		},
	}

	serialized, err := marshalCompactProof(original)
	if err != nil {
		t.Fatalf("marshalCompactProof failed: %v", err)
	}

	roundTrip, err := unmarshalCompactProof(serialized)
	if err != nil {
		t.Fatalf("unmarshalCompactProof failed: %v", err)
	}

	if !reflect.DeepEqual(original, roundTrip) {
		t.Fatalf("round-trip mismatch:\noriginal=%+v\nroundTrip=%+v", original, roundTrip)
	}
}
