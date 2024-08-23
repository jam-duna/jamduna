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

func (nh *NodeHostEnv) ReadServiceBytes(s uint32) []byte {
	tree := nh.GetTrie()
	value, err := tree.GetService(255, s)
	if err != nil {
		return nil
	}
	return value
}

func (nh *NodeHostEnv) WriteServiceBytes(s uint32, v []byte) {
	tree := nh.GetTrie()
	tree.SetService(255, s, v)
}

func (nh *NodeHostEnv) ReadServiceStorage(s uint32, storage_hash common.Hash) []byte {
	tree := nh.GetTrie()
	storage, err := tree.GetServiceStorage(s, storage_hash.Bytes())
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", storage, err)
		return storage
	}
}

func (nh *NodeHostEnv) WriteServiceStorage(s uint32, storage_hash common.Hash, storage []byte) {
	tree := nh.GetTrie()
	tree.SetServiceStorage(s, storage_hash.Bytes(), storage)
}

func (nh *NodeHostEnv) ReadServicePreimageBlob(s uint32, blob_hash common.Hash) []byte {
	tree := nh.GetTrie()
	blob, err := tree.GetPreImageBlob(s, blob_hash.Bytes())
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", blob, err)
		return blob
	}
}

func (nh *NodeHostEnv) WriteServicePreimageBlob(s uint32, blob []byte) {
	tree := nh.GetTrie()
	tree.SetPreImageBlob(s, blob)
}

func (nh *NodeHostEnv) ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) []uint32 {
	tree := nh.GetTrie()
	time_slots, err := tree.GetPreImageLookup(s, blob_hash.Bytes(), blob_length)
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", time_slots, err)
		return time_slots
	}
}

func (nh *NodeHostEnv) WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	tree := nh.GetTrie()
	tree.SetPreImageLookup(s, blob_hash.Bytes(), blob_length, time_slots)

}

/* Does this make sense?
func (nh *NodeHostEnv) WriteServicePreimageBlob(s uint32, blob []byte) bool {
	t := nh.GetTrie()
	t.SetPreImageBlob(s, blob)
	return true
}
*/

// HistoricalLookup, GetImportItem, ExportSegment
func (nh *NodeHostEnv) HistoricalLookup(s uint32, t uint32, blob_hash common.Hash) []byte {
	tree := nh.GetTrie()
	rootHash := tree.GetRoot()
	fmt.Printf("Root Hash=%v\n", rootHash)
	blob, err_v := tree.GetPreImageBlob(s, blob_hash.Bytes())
	if err_v != nil {
		return nil
	}

	blob_length := uint32(len(blob))

	//MK: william to fix & verify
	//hbytes := falseBytes(h.Bytes()[4:])
	//lbytes := uint32ToBytes(blob_length)
	//key := append(lbytes, hbytes...)
	//timeslots, err_t := tree.GetPreImageLookup(s, key)
	timeslots, err_t := tree.GetPreImageLookup(s, blob_hash.Bytes(), blob_length)
	if err_t != nil {
		return nil
	}

	if timeslots[0] == 0 {
		return nil
	} else if len(timeslots) == (12 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		z := timeslots[2]
		if (x <= t && t < y) || (z <= t) {
			return blob
		} else {
			return nil
		}
	} else if len(timeslots) == (8 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		if x <= t && t < y {
			return blob
		} else {
			return nil
		}
	} else {
		x := timeslots[0]
		if x <= t {
			return blob
		} else {
			return nil
		}
	}
}

func (nh *NodeHostEnv) DeleteServiceStorageKey(s uint32, storage_hash common.Hash) error {
	tree := nh.GetTrie()
	err := tree.DeleteServiceStorage(s, storage_hash.Bytes())
	if err != nil {
		fmt.Printf("Failed to delete storage_hash: %x, error: %v", storage_hash.Bytes(), err)
		return err
	}
	return nil
}

func (nh *NodeHostEnv) DeleteServicePreimageKey(s uint32, blob_hash common.Hash) error {
	tree := nh.GetTrie()
	err := tree.DeletePreImageBlob(s, blob_hash.Bytes())
	if err != nil {
		fmt.Printf("Failed to delete blob_hash: %x, error: %v", blob_hash.Bytes(), err)
		return err
	}
	return nil
}

func (nh *NodeHostEnv) DeleteServicePreimageLookupKey(s uint32, blob_hash common.Hash, blob_length uint32) error {
	tree := nh.GetTrie()

	err := tree.DeletePreImageLookup(s, blob_hash.Bytes(), blob_length)
	if err != nil {
		fmt.Printf("Failed to delete blob_hash: %x, blob_lookup_len: %d, error: %v", blob_hash.Bytes(), blob_length, err)
		return err
	}
	return nil
}

// Not used:

func (nh *NodeHostEnv) NewService(c []byte, l, b uint32, g, m uint64) uint32 {
	return OK
}

func (nh *NodeHostEnv) UpgradeService(c []byte, g, m uint64) uint32 {
	return OK
}

func (nh *NodeHostEnv) AddTransfer(m []byte, a, g uint64, d uint32) uint32 {
	return OK
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
