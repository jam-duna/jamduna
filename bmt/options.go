package bmt

// Options configures a Nomt database instance.
type Options struct {
	// Path to the database directory
	Path string

	// ValueHasher computes hashes of values for the Merkle tree
	// If nil, uses default Blake3 hasher
	ValueHasher ValueHasher

	// Maximum in-memory deltas for rollback (0 = default)
	RollbackInMemoryCapacity int

	// Rollback seglog directory (empty = Path/rollback)
	RollbackSegLogDir string

	// Maximum rollback seglog segment size (0 = default)
	RollbackSegLogMaxSize uint64
}

// DefaultOptions returns recommended default options.
func DefaultOptions(path string) Options {
	return Options{
		Path:                     path,
		ValueHasher:              nil, // Use default hasher
		RollbackInMemoryCapacity: 100,
		RollbackSegLogDir:        "", // Will use Path/rollback
		RollbackSegLogMaxSize:    0,  // Use seglog default
	}
}

// ValueHasher computes hashes of values for the Merkle tree.
type ValueHasher interface {
	// Hash computes the hash of a value.
	// Returns a 32-byte hash.
	Hash(value []byte) [32]byte
}
