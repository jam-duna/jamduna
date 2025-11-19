package leaf

const (
	// MaxLeafValueSize is the maximum size of a value that can be inlined in a leaf.
	// Values larger than this are stored as "overflow" values with a separate page chain.
	// For beatree, values > 1KB are considered overflow.
	MaxLeafValueSize = 1024 // 1KB

	// LeafNodeSize is the size of a leaf node page in bytes.
	LeafNodeSize = 16384 // 16KB for GP mode
)
