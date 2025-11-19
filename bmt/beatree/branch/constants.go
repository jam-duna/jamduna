package branch

const (
	// BranchNodeSize is the size of a branch node in bytes.
	// A branch node stores separators and child pointers.
	// For a 4KB page: ~100 separators + pointers
	// For GP mode (16KB): ~400 separators + pointers
	BranchNodeSize = 16384 // 16KB for GP mode

	// MaxSeparators is the maximum number of separators in a branch node.
	// Each separator is 32 bytes (key) + 8 bytes (page number) = 40 bytes
	// MaxSeparators = (16384 - overhead) / 40 ≈ 400
	MaxSeparators = 400
)
