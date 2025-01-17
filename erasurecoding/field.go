// field.go
package erasurecoding

type GFPoint uint16

const irreduciblePoly = 0x1002D

var cantorBasis = [16]GFPoint{
	0x0001,
	0xACCA,
	0x3C0E,
	0x163E,
	0xC582,
	0xED2E,
	0x914C,
	0x4012,
	0x6C98,
	0x10D8,
	0x6A72,
	0xB900,
	0xFDB8,
	0xFB34,
	0xFF38,
	0x991E,
}

var M, M_inv [16][16]bool
var cantorInited bool

// InitCantorMatrix initializes the Cantor basis matrix
func initCantorMatrix() {
	for col := 0; col < 16; col++ {
		v := cantorBasis[col]
		for row := 0; row < 16; row++ {
			if (v & (1 << row)) != 0 {
				M[row][col] = true
			} else {
				M[row][col] = false
			}
		}
	}
	M_inv = invertMatrix(M)
	cantorInited = true
}

// invertMatrix inverts a 16x16 matrix
func invertMatrix(A [16][16]bool) [16][16]bool {
	var aug [16][32]bool

	// Initialize the augmented matrix
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			aug[i][j] = A[i][j]
		}
		aug[i][16+i] = true
	}

	// Perform row reduction to get the identity matrix on the left
	for i := 0; i < 16; i++ {
		pivot := i
		for pivot < 16 && !aug[pivot][i] {
			pivot++
		}
		if pivot == 16 {
			panic("Matrix not invertible")
		}
		// Swap rows
		if pivot != i {
			for k := 0; k < 32; k++ {
				aug[i][k], aug[pivot][k] = aug[pivot][k], aug[i][k]
			}
		}
		for r := 0; r < 16; r++ {
			if r != i && aug[r][i] {
				for c := 0; c < 32; c++ {
					aug[r][c] = aug[r][c] != aug[i][c]
				}
			}
		}
	}

	var inv [16][16]bool
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			inv[i][j] = aug[i][16+j]
		}
	}
	return inv
}

// vectorMatrixMul multiplies a vector by a matrix
func vectorMatrixMul(vec GFPoint, mat [16][16]bool) GFPoint {
	var out GFPoint
	for row := 0; row < 16; row++ {
		sum := false
		for col := 0; col < 16; col++ {
			vecBit := ((vec >> col) & 1) != 0
			if vecBit && mat[row][col] {
				sum = !sum
			}
		}
		if sum {
			out |= (1 << row)
		}
	}
	return out
}

func ToCantorBasis(a GFPoint) GFPoint {
	if !cantorInited {
		initCantorMatrix()
	}
	return vectorMatrixMul(a, M_inv)
}

func FromCantorBasis(a GFPoint) GFPoint {
	if !cantorInited {
		initCantorMatrix()
	}
	return vectorMatrixMul(a, M)
}

func Add(a, b GFPoint) GFPoint {
	return a ^ b
}

// Mul multiplies two field elements
func Mul(a, b GFPoint) GFPoint {
	var result uint32
	aa := uint32(a)
	bb := uint32(b)

	// Multiply the two numbers together and reduce by the irreducible polynomial
	for i := 0; i < 16; i++ {
		if (bb & 1) != 0 {
			result ^= aa
		}
		bb >>= 1
		aa <<= 1
		if aa&0x10000 != 0 {
			aa ^= uint32(irreduciblePoly)
		}
	}
	return GFPoint(result & 0xFFFF)
}

// Pow raises a field element to a power
func Pow(base GFPoint, exp uint16) GFPoint {
	result := GFPoint(1)
	cur := base
	for exp > 0 {
		if (exp & 1) != 0 {
			result = Mul(result, cur)
		}
		cur = Mul(cur, cur)
		exp >>= 1
	}
	return result
}

func Inv(a GFPoint) GFPoint {
	if a == 0 {
		panic("inverse of zero")
	}
	return Pow(a, 0xFFFF-1)
}
