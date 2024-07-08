package pvm

import (
	"github.com/ethereum/go-ethereum/common"
)

type HostEnv struct {
	VMs map[uint32]*VM
}

func (hostenv *HostEnv) addTransfer(m []byte, a, g uint64, d uint32) uint32 {
	return OK
}
func (hostenv *HostEnv) NewService(c []byte, l, b uint32, g, m uint64) uint32 {
	return OK

}
func (hostenv *HostEnv) UpgradeService(c []byte, g, m uint64) uint32 {

	return OK
}
func (hostenv *HostEnv) createVM(code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n, _ := range hostenv.VMs {
		if n > maxN {
			maxN = n
		}
	}
	hostenv.VMs[maxN+1] = NewVMFromCode(code, i)

	return maxN + 1
}

func (hostenv *HostEnv) getVM(n uint32) (*VM, bool) {
	vm, ok := hostenv.VMs[n]
	if !ok {
		return nil, false
	}
	return vm, true
}

func (hostenv *HostEnv) designate(v []byte) uint32 {
	return OK
}

func (hostenv *HostEnv) expungeVM(n uint32) bool {
	_, ok := hostenv.VMs[n]
	if !ok {
		return false
	}
	hostenv.VMs[n] = nil
	return true
}

func (hostenv *HostEnv) exportSegment(x []byte) uint32 {
	return OK
}

func (hostenv *HostEnv) empower(m uint32, a uint32, v uint32) uint32 {
	return OK
}

func (hostenv *HostEnv) assign(c []byte) uint32 {
	return OK
}

func (hostenv *HostEnv) readServiceBytes(s uint32, k common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}

func (hostenv *HostEnv) readServicePreimage(s uint32, h common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}

func (hostenv *HostEnv) writeServiceKey(s uint32, k common.Hash, v []byte) uint32 {
	return OK
}
func (hostenv *HostEnv) getPreimage(k common.Hash, z uint32) (uint32, uint32) {
	return 0, OK
}

func (hostenv *HostEnv) requestPreimage2(h common.Hash, z uint32, y uint32) uint32 {
	return OK
}
func (hostenv *HostEnv) requestPreimage(h common.Hash, z uint32) uint32 {
	return OK
}

func (hostenv *HostEnv) forgetPreimage(h common.Hash, z uint32) uint32 {
	return OK
}
func (hostenv *HostEnv) historicalLookup(s uint32, t uint32, h common.Hash) ([]byte, uint32, bool) {
	return []byte{}, 0, true
}
func (hostenv *HostEnv) getImportItem(i uint32) ([]byte, uint32) {
	return []byte{}, OK
}

func (hostenv *HostEnv) deleteKey(k common.Hash) error {
	return nil
}

func (hostenv *HostEnv) containsKey(h []byte, z []byte) bool {
	return false
}

func (hostenv *HostEnv) setKey(k common.Hash, v []byte, b0 uint32, numBytes int) error {
	return nil
}

func (hostenv *HostEnv) addKey(k common.Hash, v []byte) error {
	return nil
}
