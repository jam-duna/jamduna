package node

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/colorfulnotion/jam/pvm"
)

type HostEnv struct {
	VMs map[uint32]*pvm.VM
}

const OK uint32 = 0


// Service Management
func (node *Node) NewService(c []byte, l, b uint32, g, m uint64) uint32 {
	return OK
}

func (node *Node) UpgradeService(c []byte, g, m uint64) uint32 {
	return OK
}

func (node *Node) AddTransfer(m []byte, a, g uint64, d uint32) uint32 {
	return OK
}

// Service Data Management: ReadServiceBytes, ReadServicePreimage, WriteServiceKey
func (node *Node) ReadServiceBytes(s uint32, k common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}

func (node *Node) ReadServicePreimage(s uint32, h common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}

func (node *Node) WriteServiceKey(s uint32, k common.Hash, v []byte) uint32 {
	return OK
}

// Preimage: GetPreimage, RequestPreimage(2), ForgetPreimage
func (node *Node) GetPreimage(k common.Hash, z uint32) (uint32, uint32) {
	return 0, OK
}

func (node *Node) RequestPreimage2(h common.Hash, z uint32, y uint32) uint32 {
	return OK
}

func (node *Node) RequestPreimage(h common.Hash, z uint32) uint32 {
	return OK
}

func (node *Node) ForgetPreimage(h common.Hash, z uint32) uint32 {
	return OK
}

// HistoricalLookup, GetImportItem, ExportSegment
func (node *Node) HistoricalLookup(s uint32, t uint32, h common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}

func (node *Node) GetImportItem(i uint32) ([]byte, uint32) {
	return []byte{}, OK
}

func (node *Node) ExportSegment(x []byte) uint32 {
	return OK
}

// StateDB: AddKey, SetKey, ContainsKey, DeleteKey
func (node *Node) AddKey(k common.Hash, v []byte) error {
	return nil
}

func (node *Node) SetKey(k common.Hash, v []byte, b0 uint32, numBytes int) error {
	return nil
}

func (node *Node) ContainsKey(h []byte, z []byte) bool {
	return false
}

func (node *Node) DeleteKey(k common.Hash) error {
	return nil
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (node *Node) CreateVM(code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n := range node.VMs {
		if n > maxN {
			maxN = n
		}
	}
	node.VMs[maxN+1] = pvm.NewVMFromCode(code, i)
	return maxN + 1
}

func (node *Node) GetVM(n uint32) (*pvm.VM, bool) {
	vm, ok := node.VMs[n]
	if !ok {
		return nil, false
	}
	return vm, true
}

func (node *Node) ExpungeVM(n uint32) bool {
	_, ok := node.VMs[n]
	if !ok {
		return false
	}
	node.VMs[n] = nil
	return true
}

// Privileged Services: Designate, Empower, Assign
func (node *Node) Designate(v []byte) uint32 {
	return OK
}

func (node *Node) Empower(m uint32, a uint32, v uint32) uint32 {
	return OK
}

func (node *Node) Assign(c []byte) uint32 {
	return OK
}


