package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {
	// Service Management
	NewService(c []byte, l, b uint32, g, m uint64) uint32
	UpgradeService(c []byte, g, m uint64) uint32
	AddTransfer(m []byte, a, g uint64, d uint32) uint32

	// Service Data Managemrnt
	ReadServiceBytes(s uint32) ([]byte, uint32, bool)                   // GP(229)
	WriteServiceBytes(s uint32, v []byte) bool                          // GP(229)
	ReadServicePreimage(s uint32, h common.Hash) ([]byte, uint32, bool) // GP(229)
	WriteServicePreimage(s uint32, k common.Hash, v []byte) bool        // GP(229)

	// Preimage/DA
	GetPreimage(k common.Hash, z uint32) (uint32, uint32)
	RequestPreimage2(h common.Hash, z uint32, y uint32) uint32
	RequestPreimage(h common.Hash, z uint32) uint32
	ForgetPreimage(h common.Hash, z uint32) uint32
	HistoricalLookup(s uint32, t uint32, h common.Hash) ([]byte, uint32, bool)
	GetImportItem(i uint32) ([]byte, uint32)
	ExportSegment(x []byte) uint32

	// StateDB
	DeleteKey(k common.Hash) error
	ContainsKey(h []byte, z []byte) bool
	SetKey(k common.Hash, v []byte, b0 uint32, numBytes int) error
	AddKey(k common.Hash, v []byte) error

	// VM Management
	CreateVM(code []byte, i uint32) uint32
	//GetVM(n uint32) (*VM, bool)
	ExpungeVM(n uint32) bool

	// Privileged Services
	Designate(v []byte) uint32
	Empower(m uint32, a uint32, v uint32) uint32
	Assign(c []byte) uint32
}
