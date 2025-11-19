package wal

// WalEntry represents a single entry in the Write-Ahead Log.
type WalEntry struct {
	Type        WalEntryType
	BucketIndex uint64  // For Clear entries
	PageId      []byte  // For Update entries (32 bytes)
	PageData    []byte  // For Update entries (16KB)
}

// WalEntryType indicates the type of WAL entry.
type WalEntryType uint8

const (
	WalEntryClear  WalEntryType = WalEntryType(WalEntryTagClear)
	WalEntryUpdate WalEntryType = WalEntryType(WalEntryTagUpdate)
)
