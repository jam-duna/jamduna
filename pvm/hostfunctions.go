package pvm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/scale"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/blake2b"
)

// Hash function using Blake2b
func bhash(data []byte) common.Hash {
	hash := blake2b.Sum256(data)
	return common.BytesToHash(hash[:])
}

func (vm *VM) IsAuthorized(p, c byte) (uint32, error) {
	// Implement the Is-Authorized logic
	// Return the amount of gas remaining or an error code
	if p == 0 {
		return LOW, nil // Example logic
	}
	return OK, nil
}

func (vm *VM) InvokeHostCall(opcode byte, operands []byte) (bool, error) {
	// Implement the logic for handling host calls
	// Return true if the call results in a halt condition, otherwise false
	switch opcode {
	case 0xFF: // Example opcode for a host call
		// Perform the host call logic
		return true, nil // Return true to indicate a halt condition
	// Add more cases for different host call opcodes here...
	default:
		return false, fmt.Errorf("unknown host call opcode: %d", opcode)
	}
}

type RefineInput struct {
	s uint32
}

type AccumulateInput struct {
	δ uint32
}

type OT struct {
	sb uint32
}

type OnTransferInput struct {
	sc uint32
	s  OT
	t  []uint32
}

func (vm *VM) Refine(input RefineInput) (uint32, []byte, error) {
	// Implement the Refine Invocation logic
	if input.s == BAD {
		return BAD, nil, errors.New("bad refine invocation")
	}
	if input.s > S {
		return BIG, nil, errors.New("refine invocation too big")
	}
	// Perform the actual refine logic and return the result
	output := []byte("refined_data")
	return OK, output, nil
}

func (vm *VM) Accumulate(input AccumulateInput) (uint32, error) {
	// Implement the Accumulate Invocation logic based on the provided specification
	// Handle the accumulation logic here
	if input.δ == BAD {
		return BAD, errors.New("accumulation failed")
	}
	// Perform the actual accumulation logic and return the result
	return OK, nil
}

func (vm *VM) OnTransfer(input OnTransferInput) (byte, error) {
	// Implement the On-Transfer Invocation logic based on the provided specification
	// If sc is empty or t is empty, return s
	/*if input.sc == 0 || len(input.t) == 0 {
		return input.s.sb, nil
	}

	// Update the service account state as specified in the function definition
	for _, ret := range input.t {
		input.s.sb += ret
	}
	*/
	// Perform further processing as per the spec
	return OK, nil
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

type Service struct {
	tc uint32
	tb uint32
	tt uint32
	tg uint32
	tm uint32
	tl uint32
	ti uint32
}

func (vm *VM) getService(service uint32) (*Service, error) {
	return &Service{}, nil
}

// Information-on-Service
func (vm *VM) hostInfo() uint32 {
	service, _ := vm.readRegister(0)
	bo, _ := vm.readRegister(1)
	s, err := vm.getService(service)
	if err != nil {
		return NONE
	}
	buffer := bytes.NewBuffer(nil)
	encoder := scale.NewEncoder(buffer)
	t := []interface{}{s.tc, s.tb, s.tt, s.tg, s.tm, s.tl, s.ti}
	err = encoder.Encode(t)
	m := buffer.Bytes()
	vm.writeRAMBytes(bo, m[:])

	return OK
}

// Empower updates
func (vm *VM) hostEmpower() uint32 {
	m, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	v, _ := vm.readRegister(0)
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.hostenv.Empower(m, a, v)
	return OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() uint32 {
	core, _ := vm.readRegister(0)
	if core >= numCores {
		return CORE
	}
	o, _ := vm.readRegister(1)
	Q := 1
	c, _ := vm.readRAMBytes(o, 32*Q)
	vm.hostenv.Assign(c)
	return OK
}

// Designate validators
func (vm *VM) hostDesignate() uint32 {
	o, _ := vm.readRegister(0)
	v, errCode := vm.readRAMBytes(o, 176*V)
	if errCode == OOB {
		return OOB
	}
	return vm.hostenv.Designate(v)
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() uint32 {
	ξ := vm.ξ
	vm.writeRegister(0, uint32(ξ%(1<<32)))
	vm.writeRegister(1, uint32(ξ/(1<<32)))
	return OK
}

// New service
func (vm *VM) hostNew() uint32 {
	// put 'g' and 'm' together
	o, _ := vm.readRegister(0)
	c, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	l, _ := vm.readRegister(1)
	gl, _ := vm.readRegister(2)
	gh, _ := vm.readRegister(3)
	ml, _ := vm.readRegister(4)
	mh, _ := vm.readRegister(5)
	g := uint64(gh)<<32 + uint64(gl)
	m := uint64(mh)<<32 + uint64(ml)
	b := uint32(0) // TODO
	// TODO: CASH if balance insufficient
	return vm.hostenv.NewService(c, l, b, g, m)
}

// Upgrade service
func (vm *VM) hostUpgrade() uint32 {
	o, _ := vm.readRegister(0)
	gl, _ := vm.readRegister(1)
	gh, _ := vm.readRegister(2)
	ml, _ := vm.readRegister(3)
	mh, _ := vm.readRegister(4)
	c, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	g := uint64(gh)<<32 + uint64(gl)
	m := uint64(mh)<<32 + uint64(ml)
	return vm.hostenv.UpgradeService(c, g, m)
}

// Transfer host call
func (vm *VM) hostTransfer() uint32 {
	d, _ := vm.readRegister(0)
	al, _ := vm.readRegister(1)
	ah, _ := vm.readRegister(2)
	gl, _ := vm.readRegister(3)
	gh, _ := vm.readRegister(4)
	o, _ := vm.readRegister(5)

	a := uint64(ah)<<32 + uint64(al)
	g := uint64(gh)<<32 + uint64(gl)
	// TODO check d -- WHO; check g -- LOW, HIGH
	m, errCode := vm.readRAMBytes(o, M)
	if errCode == OOB {
		return OOB
	}
	return vm.hostenv.AddTransfer(m, a, g, d)
}

// Gas Service
func (vm *VM) hostGas() uint32 {
	// a := (xs[len(xs)-1] - xs[0]) + o
	// g := vm.ξ
	// return WHO
	// return LOW
	return OK
}

// Quit Service
func (vm *VM) hostQuit() uint32 {
	// a := (xs[len(xs)-1] - xs[0]) + o
	// g := vm.ξ
	// return WHO
	// return LOW
	return OK
}
func (vm *VM) setGasRegister(gasBytes, registerBytes []byte) {
	// TODO
}

// Invoke
func (vm *VM) hostInvoke() uint32 {
	n, _ := vm.readRegister(0)
	o, _ := vm.readRegister(1)
	gasBytes, _ := vm.readRAMBytes(o, 8)
	registerBytes, _ := vm.readRAMBytes(o, 8+13*4)
	m, ok := vm.hostenv.GetVM(n)
	if !ok {
		return WHO
	}
	m.setGasRegister(gasBytes, registerBytes)
	m.Execute()
	// TODO: HOST, FAULT, PANIC
	return HALT
}

// Lookup preimage
func (vm *VM) hostLookup() uint32 {
	s, _ := vm.readRegister(0)
	ho, _ := vm.readRegister(1)
	bo, _ := vm.readRegister(2)
	bz, _ := vm.readRegister(3)

	// TODO: OOB for ho+..32
	h0, _ := vm.readRAMBytes(ho, 32)
	h := bhash(h0)
	v, vLength, ok := vm.hostenv.ReadServicePreimage(s, h)
	if !ok {
		return NONE
	}
	l := vLength
	if l > bz {
		l = bz
	}
	// copy v to bo ... vLength
	vm.writeRAMBytes(bo, v[:l])
	return vLength
}

func uint32ToBytes(s uint32) []byte {
	sbytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sbytes, s)
	return sbytes
}

// Read Storage
func (vm *VM) hostRead() uint32 {
	s, _ := vm.readRegister(0)
	ko, _ := vm.readRegister(1)
	kz, _ := vm.readRegister(2)
	bo, _ := vm.readRegister(3)
	bz, _ := vm.readRegister(4)
	// TODO: return OOB
	sbytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sbytes, s)
	vBytes, _ := vm.readRAMBytes(ko, int(kz))
	k := bhash(append(sbytes, vBytes...))

	v, vLength, ok := vm.hostenv.ReadServiceBytes(s, k)
	if !ok {
		return NONE
	}
	l := vLength
	if bz < l {
		l = bz
	}
	vm.writeRAMBytes(bo, v[:l])
	return uint32(vLength)
}

// Write Storage
func (vm *VM) hostWrite() uint32 {
	s, _ := vm.readRegister(0)
	ko, _ := vm.readRegister(1)
	kz, _ := vm.readRegister(2)
	vo, _ := vm.readRegister(3)
	vz, _ := vm.readRegister(4)
	//  TODO: return OOB
	h, _ := vm.readRAMBytes(ko, int(kz))
	sbytes := uint32ToBytes(s)
	k := bhash(append(sbytes, h...))
	if vz == 0 {
		vm.hostenv.DeleteKey(k)
	} else {
		v, _ := vm.readRAMBytes(vo, int(vz))
		l := vm.hostenv.WriteServiceKey(s, k, v)
		return l
	}
	return OK
}

// Solicit preimage
func (vm *VM) hostSolicit() uint32 {
	o, _ := vm.readRegister(1)
	z, _ := vm.readRegister(2)
	hBytes, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	h := common.BytesToHash(hBytes)
	y, errCode := vm.hostenv.GetPreimage(h, z)
	if errCode == OK {
		return vm.hostenv.RequestPreimage2(h, z, y)
	}
	return vm.hostenv.RequestPreimage(h, z)

}

// Forget preimage
func (vm *VM) hostForget() uint32 {
	o, _ := vm.readRegister(0)
	z, _ := vm.readRegister(1)
	hBytes, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	h := common.BytesToHash(hBytes)
	errCode = vm.hostenv.ForgetPreimage(h, z)
	return errCode
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup(t uint32) uint32 {
	s, _ := vm.readRegister(0)
	ho, _ := vm.readRegister(1)
	bo, _ := vm.readRegister(2)
	bz, _ := vm.readRegister(3)

	hBytes, errCode := vm.readRAMBytes(ho, 32)
	if errCode == OOB {
		return errCode
	}
	h := bhash(hBytes)
	v, vLength, ok := vm.hostenv.HistoricalLookup(s, t, h)
	if !ok {
		return NONE
	}
	l := vLength
	if bz < l {
		l = bz
	}
	vm.writeRAMBytes(bo, v[:l])
	return vLength
}

// Import Segment
func (vm *VM) hostImport() uint32 {
	i, _ := vm.readRegister(0)
	o, _ := vm.readRegister(1)
	v, errCode := vm.hostenv.GetImportItem(i)
	if errCode == NONE {
		return errCode
	}

	errCode = vm.writeRAMBytes(o, v[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

// Export segment host-call
func (vm *VM) hostExport(pi uint32) (uint32, []byte) {
	p, _ := vm.readRegister(0)
	z, _ := vm.readRegister(1)

	x, errCode := vm.readRAMBytes(p, int(z*W_S))
	if errCode == OOB {
		return OOB, []byte{}
	}
	// if len(e) >= WX return FULL
	// where ... const W_X = 1024
	errCode = vm.hostenv.ExportSegment(x)

	return errCode, x
}

// Not sure but this is running a machine at instruction i? using bytes from po to pz
func (vm *VM) hostMachine() uint32 {
	po, _ := vm.readRegister(0)
	pz, _ := vm.readRegister(1)
	i, _ := vm.readRegister(2)

	p, errCode := vm.readRAMBytes(po, int(pz))
	if errCode != OK {
		return errCode
	}
	n := vm.hostenv.CreateVM(p, i)
	return n
}

func (vm *VM) hostPeek() uint32 {
	n, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	b, _ := vm.readRegister(2)
	l, _ := vm.readRegister(3)
	m, ok := vm.hostenv.GetVM(n)
	if !ok {
		return WHO
	}
	// read l bytes from m
	s, errCode := m.readRAMBytes(b, int(l))
	if errCode == OOB {
		return errCode
	}
	// write l bytes to vm
	errCode = vm.writeRAMBytes(a, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostPoke() uint32 {
	n, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	b, _ := vm.readRegister(2)
	l, _ := vm.readRegister(3)
	m, ok := vm.hostenv.GetVM(n)
	if !ok {
		return WHO
	}
	s, errCode := m.readRAMBytes(a, int(l))
	if errCode == OOB {
		return errCode
	}
	errCode = m.writeRAMBytes(b, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostExpunge() uint32 {
	n, _ := vm.readRegister(0)
	if vm.hostenv.ExpungeVM(n) {
		return OK
	}
	return WHO
}
