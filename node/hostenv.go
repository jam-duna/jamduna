package node

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/trie"
)

const OK uint32 = 0

type HostEnv struct {
	VMs map[uint32]*pvm.VM
}

type NodeHostEnv struct {
	trie *trie.MerkleTree
	node *Node
}

func (n *Node) NewNodeHostEnv() *NodeHostEnv {
	trie_backend := n.getTrie()
	return &NodeHostEnv{trie: trie_backend, node: n}
}

func (nh *NodeHostEnv) GetNode() *Node {
	return nh.node
}

func (nh *NodeHostEnv) GetTrie() *trie.MerkleTree {
	return nh.trie
}

// Service Management
func (nh *NodeHostEnv) NewService(c []byte, l, b uint32, g, m uint64) uint32 {
	return OK
}

func (nh *NodeHostEnv) UpgradeService(c []byte, g, m uint64) uint32 {
	return OK
}

func (nh *NodeHostEnv) AddTransfer(m []byte, a, g uint64, d uint32) uint32 {
	return OK
}

// Service Data Management: ReadServiceBytes, ReadServicePreimage, WriteServiceKey
func (nh *NodeHostEnv) ReadServiceBytes(s uint32) ([]byte, uint32, bool) {
	t := nh.GetTrie()
	value, err := t.GetService(255, s)
	if err != nil {
		return nil, 0, false
	} else {
		fmt.Printf("get Service Account value=%x, err=%v\n", value, err)
	}
	return value, uint32(len(value)), true
}

func (nh *NodeHostEnv) WriteServiceBytes(s uint32, v []byte) bool {
	//
	return true
}

func (nh *NodeHostEnv) ReadServicePreimage(s uint32, h common.Hash) ([]byte, uint32, bool) {
	t := nh.GetTrie()
	value, err := t.GetPreImage(s, h.Bytes())
	if err != nil {
		return nil, 0, false
	} else {
		fmt.Printf("get value=%x, err=%v\n", value, err)
		return value, uint32(len(value)), true
	}
}

func (nh *NodeHostEnv) WriteServiceKey(s uint32, k common.Hash, v []byte) uint32 {
	t := nh.GetTrie()
	t.SetService(255, s, v)
	return OK
}

func (nh *NodeHostEnv) WriteServicePreimage(s uint32, k common.Hash, v []byte) bool {
	t := nh.GetTrie()
	t.SetPreImage(s, k.Bytes(), v)
	return true
}

// Preimage: GetPreimage, RequestPreimage(2), ForgetPreimage
func (nh *NodeHostEnv) GetPreimage(k common.Hash, z uint32) (uint32, uint32) {
	return 0, OK
}

func (nh *NodeHostEnv) RequestPreimage2(h common.Hash, z uint32, y uint32) uint32 {
	return OK
}

func (nh *NodeHostEnv) RequestPreimage(h common.Hash, z uint32) uint32 {
	return OK
}

func (nh *NodeHostEnv) ForgetPreimage(h common.Hash, z uint32) uint32 {
	return OK
}

// HistoricalLookup, GetImportItem, ExportSegment
func (nh *NodeHostEnv) HistoricalLookup(s uint32, ts uint32, h common.Hash) ([]byte, uint32, bool) {
	t := nh.GetTrie()
	rootHash := t.GetRoot()
	fmt.Printf("Root Hash=%v\n", rootHash)
	value, err_v := t.GetPreImage(s, h.Bytes())
	if err_v != nil {
		return nil, 0, false
	}

	hbytes := falseBytes(h.Bytes()[4:])
	lbytes := uint32ToBytes(uint32(len(value)))
	key := append(lbytes, hbytes...)

	timeslots, err_t := t.GetPreImage(s, key)
	if err_t != nil {
		return nil, 0, false
	}

	if timeslots[0] == 0 {
		return nil, 0, false
	} else if len(timeslots) == (12 + 1) {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		y := binary.LittleEndian.Uint32(timeslots[5:9])
		z := binary.LittleEndian.Uint32(timeslots[9:13])
		if (x <= ts && ts < y) || (z <= ts) {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	} else if len(timeslots) == (8 + 1) {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		y := binary.LittleEndian.Uint32(timeslots[5:9])
		if x <= ts && ts < y {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	} else {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		if x <= ts {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	}
}

func (nh *NodeHostEnv) GetImportItem(i uint32) ([]byte, uint32) {
	return []byte{}, OK
}

func (nh *NodeHostEnv) ExportSegment(x []byte) uint32 {
	return OK
}

// StateDB: AddKey, SetKey, ContainsKey, DeleteKey
func (nh *NodeHostEnv) AddKey(k common.Hash, v []byte) error {
	return nil
}

func (nh *NodeHostEnv) SetKey(k common.Hash, v []byte, b0 uint32, numBytes int) error {
	return nil
}

func (nh *NodeHostEnv) ContainsKey(h []byte, z []byte) bool {
	return false
}

func (nh *NodeHostEnv) DeleteKey(k common.Hash) error {
	t := nh.GetTrie()
	err := t.Delete(k.Bytes())
	if err != nil {
		fmt.Printf("Failed to delete key: %v, error: %v", k.Bytes(), err)
		return err
	}
	return nil
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (nh *NodeHostEnv) CreateVM(code []byte, i uint32) uint32 {
	return nh.GetNode().CreateVM(code, i)
}

func (nh *NodeHostEnv) GetVM(n uint32) (*pvm.VM, bool) {
	return nh.GetNode().GetVM(n)
}

func (nh *NodeHostEnv) ExpungeVM(n uint32) bool {
	return nh.GetNode().ExpungeVM(n)
}

func (node *Node) CreateVM(code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n := range node.VMs {
		if n > maxN {
			maxN = n
		}
	}
	nh := &NodeHostEnv{trie: node.getTrie(), node: node}
	node.VMs[maxN+1] = pvm.NewVMFromCode(code, i, nh)
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
func (nh *NodeHostEnv) Designate(v []byte) uint32 {
	return OK
}

func (nh *NodeHostEnv) Empower(m uint32, a uint32, v uint32) uint32 {
	return OK
}

func (nh *NodeHostEnv) Assign(c []byte) uint32 {
	return OK
}

func falseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = 0xFF - data[i]
		// result[i] = ^data[i]
	}
	return result
}

func uint32ToBytes(s uint32) []byte {
	sbytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sbytes, s)
	return sbytes
}
