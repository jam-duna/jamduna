package witness

import (
	"math/big"
)

// Poseidon parameters (match services/orchard/src/crypto.rs simplified version)
const orchardTreeDepth = 32

var (
	// BN254 scalar field modulus
	bn254Modulus, _ = new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)
)

// orchardPoseidonHash hashes an arbitrary list of 32-byte inputs into a 32-byte output.
// This mirrors the simplified poseidon_hash in services/orchard/src/crypto.rs.
func orchardPoseidonHash(inputs [][]byte) [32]byte {
	// Convert inputs to field elements (big-endian to match Rust FFI expectations)
	state := make([]*big.Int, 3)
	for i := 0; i < 3; i++ {
		state[i] = big.NewInt(0)
	}

	fieldInputs := make([]*big.Int, 0, len(inputs))
	for _, in := range inputs {
		fieldInputs = append(fieldInputs, bytesToField(in))
	}

	inputIdx := 0
	for inputIdx < len(fieldInputs) {
		for i := 0; i < 2; i++ {
			if inputIdx < len(fieldInputs) {
				state[i].Add(state[i], fieldInputs[inputIdx])
				state[i].Mod(state[i], bn254Modulus)
				inputIdx++
			}
		}
		poseidonPermute(state)
	}

	return fieldToBytes(state[0])
}

// orchardPoseidonHash2 hashes two children.
func orchardPoseidonHash2(left, right [32]byte) [32]byte {
	return orchardPoseidonHash([][]byte{left[:], right[:]})
}

// OrchardTree is a helper Merkle tree (binary Poseidon, depth 32).
type OrchardTree struct {
	leaves [][]byte
}

// NewOrchardTree creates a tree with existing leaves (contiguous from 0).
func NewOrchardTree(leaves [][]byte) *OrchardTree {
	cp := make([][]byte, len(leaves))
	for i := range leaves {
		cp[i] = make([]byte, len(leaves[i]))
		copy(cp[i], leaves[i])
	}
	return &OrchardTree{leaves: cp}
}

// Size returns the current number of leaves.
func (t *OrchardTree) Size() uint64 {
	return uint64(len(t.leaves))
}

// Root computes the current root with zero padding to depth 32.
func (t *OrchardTree) Root() [32]byte {
	if len(t.leaves) == 0 {
		return zeroHashes()[orchardTreeDepth]
	}
	return merkleRootFromLeaves(t.leaves)
}

// Append appends new leaves in order and returns the new root.
func (t *OrchardTree) Append(newLeaves ...[]byte) [32]byte {
	for _, leaf := range newLeaves {
		cp := make([]byte, len(leaf))
		copy(cp, leaf)
		t.leaves = append(t.leaves, cp)
	}
	return t.Root()
}

// merkleRootFromLeaves computes the Poseidon root for a contiguous set of leaves.
func merkleRootFromLeaves(leaves [][]byte) [32]byte {
	zeros := zeroHashes()
	if len(leaves) == 0 {
		return zeros[orchardTreeDepth]
	}

	level := make([][32]byte, len(leaves))
	for i, leaf := range leaves {
		var h [32]byte
		copy(h[:], leaf)
		level[i] = h
	}
	depth := 0
	for depth < orchardTreeDepth {
		if len(level)%2 == 1 {
			level = append(level, zeros[depth])
		}
		next := make([][32]byte, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			next = append(next, orchardPoseidonHash2(level[i], level[i+1]))
		}
		level = next
		depth++
	}
	if len(level) == 0 {
		return zeros[orchardTreeDepth]
	}
	return level[0]
}

// zeroHashes precomputes zero hashes for each depth.
func zeroHashes() [orchardTreeDepth + 1][32]byte {
	var zeros [orchardTreeDepth + 1][32]byte
	for i := 1; i <= orchardTreeDepth; i++ {
		zeros[i] = orchardPoseidonHash2(zeros[i-1], zeros[i-1])
	}
	return zeros
}

// poseidonPermute applies the simplified permutation used in Rust.
func poseidonPermute(state []*big.Int) {
	for round := 0; round < 8; round++ {
		// add round constants (simplified)
		for i := 0; i < 3; i++ {
			rc := big.NewInt(int64(round*3 + i + 1))
			state[i].Add(state[i], rc)
			state[i].Mod(state[i], bn254Modulus)
		}

		// S-box x^5
		for i := 0; i < 3; i++ {
			x2 := new(big.Int).Mul(state[i], state[i])
			x2.Mod(x2, bn254Modulus)

			x4 := new(big.Int).Mul(x2, x2)
			x4.Mod(x4, bn254Modulus)

			state[i].Mul(x4, state[i])
			state[i].Mod(state[i], bn254Modulus)
		}

		// Linear layer (simplified MDS)
		t0 := new(big.Int).Add(state[0], state[1])
		t0.Add(t0, state[2])
		t0.Mod(t0, bn254Modulus)

		t1 := new(big.Int).Mul(state[1], big.NewInt(2))
		t1.Add(t1, state[0])
		t1.Add(t1, state[2])
		t1.Mod(t1, bn254Modulus)

		t2 := new(big.Int).Mul(state[2], big.NewInt(3))
		t2.Add(t2, state[0])
		t2.Add(t2, state[1])
		t2.Mod(t2, bn254Modulus)

		state[0], state[1], state[2] = t0, t1, t2
	}
}

func bytesToField(b []byte) *big.Int {
	// Rust expects big-endian field encodings
	n := new(big.Int).SetBytes(b)
	n.Mod(n, bn254Modulus)
	return n
}

func fieldToBytes(f *big.Int) [32]byte {
	var out [32]byte
	tmp := new(big.Int).Mod(f, bn254Modulus).Bytes()
	if len(tmp) > 32 {
		tmp = tmp[len(tmp)-32:]
	}
	copy(out[32-len(tmp):], tmp)
	return out
}
