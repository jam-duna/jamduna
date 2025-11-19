package merkle

// KeyReadWrite represents the type of operation on a key.
type KeyReadWrite int

const (
	// KeyRead indicates a read operation
	KeyRead KeyReadWrite = iota
	// KeyWrite indicates a write operation
	KeyWrite
	// KeyDelete indicates a delete operation
	KeyDelete
)

// String returns a string representation of the operation type.
func (krw KeyReadWrite) String() string {
	switch krw {
	case KeyRead:
		return "Read"
	case KeyWrite:
		return "Write"
	case KeyDelete:
		return "Delete"
	default:
		return "Unknown"
	}
}

// IsRead returns true if this is a read operation.
func (krw KeyReadWrite) IsRead() bool {
	return krw == KeyRead
}

// IsWrite returns true if this is a write operation.
func (krw KeyReadWrite) IsWrite() bool {
	return krw == KeyWrite
}

// IsDelete returns true if this is a delete operation.
func (krw KeyReadWrite) IsDelete() bool {
	return krw == KeyDelete
}

// IsModifying returns true if this operation modifies data.
func (krw KeyReadWrite) IsModifying() bool {
	return krw == KeyWrite || krw == KeyDelete
}