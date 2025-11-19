package wal

// WAL entry tags
const (
	WalEntryTagStart  uint8 = 1
	WalEntryTagEnd    uint8 = 2
	WalEntryTagClear  uint8 = 3
	WalEntryTagUpdate uint8 = 4
)
