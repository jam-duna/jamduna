package statedb

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/types" // go get golang.org/x/example/hello/reverse
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

// ExecutionContext tracks the state of a specific contract execution context
type ExecutionContext struct {
	ContractAddress uint64
	MemoryBase      uint64
	CallStackBase   uint64
	FrameBase       uint64
	CallDepth       uint64
	CodeBase        uint64
	CodeSize        uint64
	CallDataBase    uint64
	CallDataSize    uint64
	ReturnDataBase  uint64
	ReturnDataSize  uint64
}

const (
	RegSize = 13
	regSize = RegSize // for backward compatibility
)

var (
	PvmTrace  = false // Temporarily enabled to demonstrate context tracking
	PvmTrace2 = false

	UseTally = false
)

type VMGo struct {
	Backend        string
	IsChild        bool
	JSize          uint64
	Z              uint8
	J              []uint32
	code           []byte
	bitmask        []byte
	pc             uint64 // Program counter
	ResultCode     uint8
	HostResultCode uint64
	MachineState   uint8
	Fault_address  uint32
	terminated     bool
	hostCall       bool // ̵h in GP
	host_func_id   int  // h in GP
	hostVM         *VM  // Reference to host VM for host function calls
	Ram            *RawRam
	Gas            int64
	hostenv        types.HostEnv

	VMs map[uint32]*VMGo

	// Execution Context Tracking (for multi-contract debugging)
	ContextStack   []ExecutionContext // Stack of nested execution contexts
	CurrentContext *ExecutionContext  // Currently active execution context

	// Work Package Inputs
	WorkItemIndex uint32
	WorkPackage   *types.WorkPackage
	Extrinsics    [][][]byte
	Authorization []byte
	Imports       [][][]byte

	AccumulateOperandElements []types.AccumulateOperandElements
	AccumulateInputs          []types.AccumulateInput
	//	Transfers                 []types.DeferredTransfer
	N common.Hash

	// Invocation functions entry point
	EntryPoint uint32

	logging string

	// standard program initialization parameters
	o_size uint32
	w_size uint32
	z      uint32
	s      uint32
	o_byte []byte
	w_byte []byte

	// Refine argument
	Exports            [][]byte
	ExportSegmentIndex uint32

	// Accumulate argument
	X        *types.XContext
	Y        types.XContext
	Timeslot uint32

	// General argument
	ServiceAccount *types.ServiceAccount
	Service_index  uint32
	CoreIndex      uint16

	Delta map[uint32]*types.ServiceAccount

	// Output
	Outputs []byte

	// service metadata
	ServiceMetadata []byte
	Mode            string
	Identifier      string

	pushFrame       func([]byte)
	stopFrameServer func()

	Logs VMLogs

	basicBlockExecutionCounter map[uint64]int // PVM PC to execution count

	kv map[common.Hash]common.Hash // Legacy flat storage (deprecated)

	// Contract-scoped storage: map[contractAddress][storageSlot]value
	// Values are arbitrary-length byte slices scoped per contract
	// Contract address is the full 32-byte object_id for JAM object identification
	contractStorage map[common.Hash]map[common.Hash][]byte

	// Transient storage (EIP-1153): map[contractAddress][storageSlot]value
	// Like contractStorage but cleared at the end of each transaction
	// Used by TLOAD (0x5C) and TSTORE (0x5D) opcodes
	transientStorage map[common.Hash]map[common.Hash][]byte

	executionTime uint32

	returndata []byte
	jumpTable  map[int]uint64
	cookie     uint64
	//contracts  map[uint64]*ContractImage

	label_pc map[int]string

	prevStack []byte
	prevSP    uint64

	// instrumentation
	HostBigintCalls uint64

	// MoveVM interpreter integration state
	moveSnapshot *snapshotReaderState
	readCache    map[uint64]*cachedReadObject
	readSet      map[readSetKey]readSetEntry
	writeBuffer  []writeBufferEntry

	// MoveVM stack state for test comparison
	MoveStackVal     []uint64
	MoveStackTypeVal []uint64
}

// registerChildVM stores a reference to a child VM created via hostMachine/hostInvoke.
// The parent VM keeps this map so host functions (e.g. contract_storage) can
// access the child's execution state after it has run.
// func (vm *VMGo)registerChildVM(machineIdx uint32, child *VMGo) {
// 	if vm == nil || child == nil {
// 		return
// 	}
// 	if vm.VMs == nil {
// 		vm.VMs = make(map[uint32]*VMGo)
// 	}
// 	vm.VMs[machineIdx] = child
// }

// // unregisterChildVM removes the reference to a child VM once it has been expunged.
// func (vm *VMGo)unregisterChildVM(machineIdx uint32) {
// 	if vm == nil || vm.VMs == nil {
// 		return
// 	}
// 	delete(vm.VMs, machineIdx)
// }

// Phase 2 MoveVM integration types
type snapshotReaderState struct {
	enabled bool
	data    map[uint64][]byte
}

type cachedReadObject struct {
	value    []byte
	accessed bool
}

type readSetKey struct {
	address uint64
	offset  uint32
	size    uint32
}

type readSetEntry struct {
	touched bool
	value   []byte
}

type writeBufferEntry struct {
	address uint64
	offset  uint32
	data    []byte
}

func (vm *VMGo) DebugContractStorageKeys() {
	fmt.Printf("***HERE DebugContractStorageKeys:\n")
	if vm == nil {
		fmt.Println("DebugContractStorageKeys: vm is nil")
		return
	}

	if vm.contractStorage == nil || len(vm.contractStorage) == 0 {
		fmt.Println("DebugContractStorageKeys: Contract storage is empty")
		return
	}

	contractAddrs := make([]common.Hash, 0, len(vm.contractStorage))
	for addr, storage := range vm.contractStorage {
		if len(storage) == 0 {
			continue
		}
		contractAddrs = append(contractAddrs, addr)
	}

	if len(contractAddrs) == 0 {
		fmt.Println("No initialized storage keys found")
		return
	}

	sort.Slice(contractAddrs, func(i, j int) bool {
		return bytes.Compare(contractAddrs[i][:], contractAddrs[j][:]) < 0
	})

	for _, addr := range contractAddrs {
		storage := vm.contractStorage[addr]
		if len(storage) == 0 {
			continue
		}

		fmt.Printf("Contract 0x%x:\n", addr)

		keys := make([]string, 0, len(storage))
		for key := range storage {
			keys = append(keys, key.Hex())
		}

		sort.Strings(keys)

		for _, keyHex := range keys {
			fmt.Printf("  %s\n", keyHex)
		}
	}
}

func newSnapshotReaderState() *snapshotReaderState {
	return &snapshotReaderState{
		enabled: false,
		data:    make(map[uint64][]byte),
	}
}

func extractBytes(input []byte) ([]byte, []byte) {
	/*
		In GP_0.36 (272):
		If the input value of (272) is large, "l" will also increase and vice versa.
		"l" is than be used to encode first byte and the reaming "l" bytes.
		If the first byte is large, that means the number of the entire encoded bytes is large and vice versa.
		So the first byte can be used to determine the number of bytes to extract and the rule is as follows:
	*/

	if len(input) == 0 {
		return nil, input
	}

	firstByte := input[0]
	var numBytes int

	// Determine the number of bytes to extract based on the value of the 0th byte.
	switch {
	case firstByte < 128:
		numBytes = 1
	case firstByte >= 128 && firstByte < 192:
		numBytes = 2
	case firstByte >= 192 && firstByte < 224:
		numBytes = 3
	case firstByte >= 224 && firstByte < 240:
		numBytes = 4
	case firstByte >= 240 && firstByte < 248:
		numBytes = 5
	case firstByte >= 248 && firstByte < 252:
		numBytes = 6
	case firstByte >= 252 && firstByte < 254:
		numBytes = 7
	case firstByte >= 254:
		numBytes = 8
	default:
		numBytes = 1
	}

	// If the input length is insufficient to extract the specified number of bytes, return the original input.
	if len(input) < numBytes {
		return input, nil
	}

	// Extract the specified number of bytes and return the remaining bytes.
	extracted := input[:numBytes]
	remaining := input[numBytes:]

	return extracted, remaining
}

func (vm *VMGo) Standard_Program_Initialization(argument_data_a []byte) {
	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}

	z_w := Z_func(vm.w_size + vm.z*Z_P)

	// o_byte
	vm.Ram.WriteRAMBytes(Z_Z, vm.o_byte)

	// w_byte
	z_o := Z_func(vm.o_size)
	w_addr := 2*Z_Z + z_o
	vm.Ram.WriteRAMBytes(w_addr, vm.w_byte)

	// argument
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	vm.Ram.WriteRAMBytes(argAddr, argument_data_a)
	//fmt.Printf("Copied argument_data_a (len %d) to RAM at address %x\n", len(argument_data_a), argAddr)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		panic("Standard Program Initialization Error")
	}

	//vm.Ram.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.Ram.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.Ram.WriteRegister(7, uint64(argAddr))
	vm.Ram.WriteRegister(8, uint64(uint32(len(argument_data_a))))

	// fmt.Printf("Standard Program Initialization: %s=%x %s=%x %x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)), argument_data_a)
}
func (vm *VMGo) SetJumpTable(a *program.Program) {
	//vm.jumpTable = a.jumpvec
	//vm.cookie = a.cookie
}

func (vm *VMGo) SetLabelPC(labels map[int]string) {
	vm.label_pc = labels
}

func (vm *VMGo) GetCurrentPC() uint64 {
	return vm.pc
}

func (vm *VMGo) GetArgumentOutputs() (types.Result, uint64) {
	return vm.getArgumentOutputs()
}

func (vm *VMGo) SetMemoryBounds(o_size uint32,
	w_size uint32,
	z uint32,
	s uint32,
	o_byte []byte,
	w_byte []byte) {
	// set memory bounds
	vm.o_size = o_size
	vm.w_size = w_size
	vm.z = z
	vm.s = s
	vm.o_byte = o_byte
	vm.w_byte = w_byte

}

// NewVMGo initializes a new VMGo with a given program
func NewVMGo(service_index uint32, p *Program, initialRegs []uint64, initialPC uint64, initialGas uint64, hostENV types.HostEnv) (vm *VMGo) {

	vm = &VMGo{
		Gas:           int64(initialGas),
		JSize:         p.JSize,
		Z:             p.Z,
		J:             p.J,
		code:          p.Code,
		bitmask:       p.K,
		pc:            initialPC,
		hostenv:       hostENV, //check if we need this
		Exports:       make([][]byte, 0),
		Service_index: service_index,
		CoreIndex:     2048,
		// evm host function
		kv:              make(map[common.Hash]common.Hash),
		contractStorage: make(map[common.Hash]map[common.Hash][]byte), // Contract-scoped storage
		//contracts:       make(map[uint64]*ContractImage),
		label_pc: make(map[int]string),
	}
	vm.Ram, _ = NewRawRam()
	vm.moveSnapshot = newSnapshotReaderState()
	vm.readCache = make(map[uint64]*cachedReadObject)
	vm.readSet = make(map[readSetKey]readSetEntry)
	vm.writeBuffer = make([]writeBufferEntry, 0)

	for i := 0; i < len(initialRegs); i++ {
		vm.Ram.WriteRegister(i, initialRegs[i])
	}

	if VMsCompare {
		vm.Logs = make(VMLogs, 0)
	} else {
		vm.VMs = nil
	}

	return vm
}

func (vm *VMGo) Destroy() {
	// TODO: implement if needed
}

func (vm *VMGo) Execute(host *VM, entryPoint uint32) error {
	vm.terminated = false
	vm.IsChild = false
	vm.hostVM = host           // Store host VM reference for host function calls
	vm.EntryPoint = entryPoint // Store entry point for gas accounting
	// A.2 deblob
	if vm.code == nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute1")
	}

	if len(vm.code) == 0 {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute2")
	}

	if len(vm.bitmask) == 0 {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("failed to decode bitmask")
	}
	vm.pc = uint64(entryPoint)
	stepn := 1
	for !vm.terminated {
		// charge gas for all the next steps until hitting a basic block instruction
		// _, _, step := vm.getBasicBlockGasCost(vm.pc)
		// og_gas := vm.Gas

		// fmt.Printf("charged gas %d, %d -> %d\n", gasBasicBlock, og_gas, vm.Gas)

		// now, run the block
		for i := 0; !vm.terminated; i++ {
			if err := vm.step(stepn); err != nil {
				if err == errChildHostCall {
					return errors.New("host call not allowed in child VM")
				}
				return err
			}
			stepn++
			if vm.Gas < 0 {
				vm.ResultCode = types.WORKRESULT_OOG
				vm.MachineState = OOG
				vm.hostVM.ResultCode = types.WORKRESULT_OOG
				vm.terminated = true
				return fmt.Errorf("out of gas , gas = %d", vm.Gas)
			}
		}
	}

	// if vm finished without error, set result code to OK
	if !vm.terminated {
		vm.ResultCode = types.WORKRESULT_OK
	} else if vm.ResultCode != types.WORKRESULT_OK {
		vm.hostVM.ResultCode = vm.ResultCode
		fmt.Printf("VM terminated with error code %d at PC %d (%v, %s, %s) Gas:%v\n", vm.ResultCode, vm.pc, vm.Service_index, vm.Mode, string(vm.ServiceMetadata), vm.Gas)
	}
	return nil
}

func (vm *VMGo) ReadRAMBytes(addr uint32, length uint32) ([]byte, uint64) {
	o, res := vm.Ram.ReadRAMBytes(addr, length)
	return o, res
}
func (vm *VMGo) WriteRAMBytes(addr uint32, data []byte) uint64 {
	return vm.Ram.WriteRAMBytes(addr, data)
}
func (vm *VMGo) ReadRegister(index int) uint64 {
	return vm.Ram.ReadRegister(index)
}
func (vm *VMGo) WriteRegister(index int, value uint64) {
	vm.Ram.WriteRegister(index, value)
}

func (vm *VMGo) ReadRegisters() [13]uint64 {
	return vm.Ram.ReadRegisters()
}

func (vm *VMGo) Panic(code uint64) {
	vm.ResultCode = types.WORKRESULT_PANIC
	vm.hostVM.ResultCode = types.WORKRESULT_PANIC
	vm.terminated = true
}

func (vm *VMGo) SetHeapPointer(pointer uint32) {
	// VMGo doesn't use a C-based heap pointer like Interpreter does
	// This is a stub to satisfy the ExecutionVM interface
	vm.Ram.SetCurrentHeapPointer(pointer)
}

func (vm *VMGo) SetHostResultCode(c uint64) {
	vm.HostResultCode = c
}

func (vm *VMGo) SetPage(uint32, uint32, uint8) {
	// Stub to satisfy ExecutionVM interface
}

func (vm *VMGo) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VMGo) GetServiceIndex() uint32 {
	return vm.Service_index
}

func (vm *VMGo) SetCore(coreIndex uint16) {
	vm.CoreIndex = coreIndex
}

func (vm *VMGo) Init(argument_data_a []byte) error {
	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	//1)
	// o_byte
	o_len := len(vm.o_byte)
	var err error
	if err = vm.Ram.SetMemAccess(Z_Z, uint32(o_len), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess1 failed o_len=%d (o_byte): %w", o_len, err)
	}
	if err = vm.Ram.WriteMemory(Z_Z, vm.o_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (o_byte): %w", err)
	}
	if err = vm.Ram.SetMemAccess(Z_Z, uint32(o_len), PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess2 failed (o_byte): %w", err)
	}
	//2)
	//p|o|
	p_o_len := P_func(uint32(o_len))
	if err = vm.Ram.SetMemAccess(Z_Z+uint32(o_len), p_o_len, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (p_o_byte): %w", err)
	}

	z_o := Z_func(vm.o_size)
	z_w := Z_func(vm.w_size + vm.z*Z_P)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		return fmt.Errorf("Standard Program Initialization Error: requiredMemory too large")
	}
	// 3)
	// w_byte
	w_addr := 2*Z_Z + z_o
	w_len := uint32(len(vm.w_byte))
	if err = vm.Ram.SetMemAccess(w_addr, w_len, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (w_byte): %w", err)
	}
	if err = vm.Ram.WriteMemory(w_addr, vm.w_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (w_byte): %w", err)
	}
	// 4)
	addr4 := 2*Z_Z + z_o + w_len
	little_z := vm.z
	len4 := P_func(w_len) + little_z*Z_P - w_len
	if err = vm.Ram.SetMemAccess(addr4, len4, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr4): %w", err)
	}
	// 5)
	addr5 := 0xFFFFFFFF + 1 - 2*Z_Z - Z_I - P_func(vm.s)
	len5 := P_func(vm.s)
	if err = vm.Ram.SetMemAccess(addr5, len5, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr5): %w", err)
	}
	// 6)
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	if err = vm.Ram.SetMemAccess(argAddr, uint32(len(argument_data_a)), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr): %w", err)
	}
	if err = vm.Ram.WriteMemory(argAddr, argument_data_a); err != nil {
		return fmt.Errorf("WriteMemory failed (argAddr): %w", err)
	}
	// set it back to immutable
	if err = vm.Ram.SetMemAccess(argAddr+uint32(len(argument_data_a)), Z_I, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr+len): %w", err)
	}
	// 7)
	addr7 := argAddr + uint32(len(argument_data_a))
	len7 := argAddr + P_func(uint32(len(argument_data_a))) - addr7
	if err = vm.Ram.SetMemAccess(addr7, len7, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr7): %w", err)
	}

	vm.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.WriteRegister(7, uint64(argAddr))
	vm.WriteRegister(8, uint64(uint32(len(argument_data_a))))

	return nil
}

func (vm *VMGo) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ResultCode == types.WORKRESULT_OOG {
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	if vm.ResultCode != types.WORKRESULT_OK {
		return r, 0
	}
	o := vm.Ram.ReadRegister(7)
	l := vm.Ram.ReadRegister(8)
	output, res := vm.Ram.ReadRAMBytes(uint32(o), uint32(l))
	if vm.ResultCode == types.WORKRESULT_OK && res == 0 {
		r.Ok = output
		return r, res
	}
	if vm.ResultCode == types.WORKRESULT_OK && res != 0 {
		r.Ok = output
		return r, res
	}

	return r, 0
}

func (vm *VMGo) SetIdentifier(id string) {
	vm.Identifier = id
}

func (vm *VMGo) GetIdentifier() string {
	return fmt.Sprintf("%d_%s_%s_%s", vm.Service_index, vm.Mode, vm.Backend, vm.Identifier)
}

var errChildHostCall = errors.New("host call not allowed in child VM")

// step performs a single step in the PVM
func (vm *VMGo) step(stepn int) error {
	if vm.pc >= uint64(len(vm.code)) {
		return errors.New("program counter out of bounds")
	}
	prevpc := vm.pc
	opcode := vm.code[vm.pc]
	len_operands := vm.skip(vm.pc)
	operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
	vm.Gas -= 1

	switch {
	case opcode <= 1: // A.5.1 No arguments
		vm.HandleNoArgs(opcode)
	case opcode == ECALLI: // A.5.2 One immediate
		vm.HandleOneImm(opcode, operands)
		// host call invocation
		// if vm.hostCall && vm.IsChild {
		// 	return childHostCall
		// }
		if vm.hostCall {
			if vm.host_func_id == TRANSFER {
				vm.Gas -= 10
			} else {
				vm.Gas -= 10
			}
			// Invoke host function with panic recovery
			if vm.Gas < 0 {
				fmt.Printf("Out of gas during host function %d call\n", vm.host_func_id)
				vm.ResultCode = types.WORKRESULT_OOG
				vm.MachineState = OOG
				vm.terminated = true
				return nil
			}

			// Call the host function
			if vm.hostVM != nil {
				_, err := vm.hostVM.InvokeHostCall(vm.host_func_id)
				if err != nil {
					fmt.Printf("HostCall %s (%d) ERROR: %v\n", HostFnToName(vm.host_func_id), vm.host_func_id, err)
					vm.terminated = true
					return nil
				}
				if vm.host_func_id == TRANSFER && vm.ReadRegister(7) == OK {
					vm.Gas -= int64(vm.ReadRegister(9))
				}
				// Check if host function caused panic
				if vm.MachineState == PANIC {
					vm.Panic(PANIC)
					//fmt.Printf("HostCall %s (%d) PANIC!\n", HostFnToName(vm.host_func_id), vm.host_func_id)
					vm.terminated = true
					return nil
				}
			}

			vm.hostCall = false
		}
	case opcode == LOAD_IMM_64: // A.5.3 One Register and One Extended Width Immediate
		vm.HandleOneRegOneEWImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 30 <= opcode && opcode <= 33: // A.5.4 Two Immediates
		vm.HandleTwoImms(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case opcode == JUMP: // A.5.5 One offset
		vm.HandleOneOffset(opcode, operands)
	case 50 <= opcode && opcode <= 62: // A.5.6 One Register and One Immediate
		vm.HandleOneRegOneImm(opcode, operands)
		if opcode != JUMP_IND {
			if !vm.terminated {
				vm.pc += 1 + len_operands
			}
		}
	case 70 <= opcode && opcode <= 73: // A.5.7 One Register and Two Immediate
		vm.HandleOneRegTwoImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 80 <= opcode && opcode <= 90: // A.5.8 One Register, One Immediate and One Offset
		vm.HandleOneRegOneImmOneOffset(opcode, operands)
	case 100 <= opcode && opcode <= 111: // A.5.9 Two Registers
		vm.HandleTwoRegs(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 120 <= opcode && opcode <= 161: // A.5.10 Two Registers and One Immediate
		vm.HandleTwoRegsOneImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 170 <= opcode && opcode <= 175: // A.5.11 Two Registers and One Offset
		vm.HandleTwoRegsOneOffset(opcode, operands)
	case opcode == LOAD_IMM_JUMP_IND: // A.5.12 Two Register and Two Immediate
		vm.HandleTwoRegsTwoImms(opcode, operands)
	case 190 <= opcode && opcode <= 230: // A.5.13 Three Registers
		vm.HandleThreeRegs(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}

	default:
		fmt.Printf(vm.logging, "terminated: unknown opcode", "service", string(vm.ServiceMetadata), "opcode", opcode)
		vm.HandleNoArgs(0) //TRAP
	}

	// avoid this: this is expensive
	// Show every 1000 gas, plus every instruction in high-res window
	showThisGas := (vm.Gas%1000 == 0) || (vm.Gas > 972100000 && vm.Gas <= 972300000)
	if PvmLogging && showThisGas {
		registers := vm.Ram.ReadRegisters()
		prettyHex := "["
		for i := 0; i < 13; i++ {
			if i > 0 {
				prettyHex = prettyHex + ", "
			}
			prettyHex = prettyHex + fmt.Sprintf("%d", registers[i])
		}
		prettyHex = prettyHex + "]"

		// Read 16 bytes at 0x2d01e0 to track the memory region containing 0x2d01e8
		debugMemAddr := uint32(0x2d01e0)
		debugMemBytes, _ := vm.Ram.ReadRAMBytes(debugMemAddr, 16)
		debugMemHex := fmt.Sprintf("%032x", debugMemBytes)

		fmt.Printf("%s %d %d Gas: %d Registers:%s Mem[0x%x..0x%x]:%s%s\n",
			opcode_str(opcode), stepn, prevpc, vm.Gas, prettyHex, debugMemAddr, debugMemAddr+15, debugMemHex, lastMemOp)
		lastMemOp = "" // Clear for next instruction
	}
	if label, ok := vm.label_pc[int(vm.pc)]; ok {
		fmt.Printf("[%s%s%s FINISH]\n", ColorBlue, lastLabel, ColorReset)
		fmt.Printf("[%s%s%s START]\n", ColorBlue, label, ColorReset)
		if lastLabel != label {
			lastLabel = label
		}
	}
	return nil
}

var lastLabel string

type StepSample struct {
	Op   string   `json:"op"`
	Mode string   `json:"mode"`
	Step int      `json:"step"`
	PC   uint64   `json:"pc"`
	Gas  int64    `json:"gas"`
	Reg  []uint64 `json:"reg"`
}

// skip function calculates the distance to the next instruction
func (vm *VMGo) skip(pc uint64) uint64 {
	n := uint64(len(vm.bitmask))
	end := pc + 25
	if end > n {
		end = n
	}
	for i := pc + 1; i < end; i++ {
		if vm.bitmask[i] != 0 {
			return i - pc - 1
		}
	}
	if end < pc+25 {
		return end - pc - 1
	}
	return 24
}

func (vm *VMGo) djump(a uint64) {
	if a == uint64((1<<32)-(1<<16)) {
		vm.terminated = true
		vm.ResultCode = types.WORKRESULT_OK
	} else if vm.cookie > 0 {
		// a is an evmpc so we need to map to pvm pc
		pvmpc, ok := vm.jumpTable[int(a)]
		if !ok {
			// very hacky solution...
			for _, v := range vm.jumpTable {
				if v == a {
					vm.pc = uint64(v)
					ok = true
					break
				}
			}
			if !ok {
				fmt.Printf("djump: unknown evmpc %d %v\n", a, vm.jumpTable)
				vm.terminated = true
				vm.ResultCode = types.WORKRESULT_PANIC
				vm.MachineState = PANIC
				return
			}
		} else {
			vm.pc = pvmpc
			fmt.Printf("djump to PC=%d\n", vm.pc)
		}
	} else if a == 0 {
		// Handle the exit case where a=0 from the SDIV result
		vm.terminated = true
		vm.ResultCode = types.WORKRESULT_OK
	} else if a > uint64(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
	} else {
		target_pc := uint64(vm.J[(a/Z_A)-1])
		// Validate target PC is within bounds
		if target_pc >= uint64(len(vm.code)) {
			vm.terminated = true
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
		} else {
			vm.pc = target_pc
		}
	}
}

func (vm *VMGo) branch(vx uint64, taken int, operand_len int) {
	if taken == 1 && vx < uint64(len(vm.code)) && vx < uint64(len(vm.bitmask)) && (vm.bitmask[vx]&2) != 0 {
		vm.pc = vx
	} else if taken == 1 {
		// Branch marked taken but invalid target
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	} else {
		// Not taken; increment pc for fall-through
		vm.pc += uint64(1 + operand_len)
	}
}

func z_encode(a uint64, n uint32) int64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 64 - 8*n
	return int64(a<<shift) >> shift
}

func x_encode(x uint64, n uint32) uint64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 8*n - 1
	q := x >> shift
	if n == 8 {
		return x
	}
	mask := (uint64(1) << (8 * n)) - 1
	factor := ^mask
	return x + q*factor
}

func smod(a, b int64) int64 {
	if b == 0 {
		return a
	}

	absA := a
	if absA < 0 {
		absA = -absA
	}
	absB := b
	if absB < 0 {
		absB = -absB
	}

	modVal := absA % absB

	if a < 0 {
		return -modVal
	}
	return modVal
}

func (vm *VMGo) HandleNoArgs(opcode byte) {
	switch opcode {
	case TRAP:
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		fmt.Printf("TRAP encountered at pc %d in mode %s\n", vm.pc, vm.Mode)
		vm.terminated = true
	case FALLTHROUGH:
		vm.pc += 1
	}
}

func (vm *VMGo) HandleOneImm(opcode byte, operands []byte) {
	switch opcode {
	case ECALLI:
		lx := uint32(types.DecodeE_l(operands))
		vm.hostCall = true
		vm.host_func_id = int(lx)
		// vm.ResultCode = types.
		// vm.HostResultCode = types.
		vm.pc += 1 + uint64(len(operands))
	}
}

func (vm *VMGo) HandleOneRegOneEWImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := 8
	vx := types.DecodeE_l(originalOperands[1 : 1+lx])
	dumpLoadImm("LOAD_IMM_64", registerIndexA, uint64(vx), vx, 64, false)
	vm.Ram.WriteRegister(registerIndexA, uint64(vx))
}

func (vm *VMGo) HandleTwoImms(opcode byte, operands []byte) {
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	lx := min(4, int(originalOperands[0])%8)
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly))

	addr := uint32(vx)
	switch opcode {
	case STORE_IMM_U8:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, []byte{uint8(vy)}))
		dumpStoreGeneric("STORE_IMM_U8", uint64(addr), "imm", vy, 8)
	case STORE_IMM_U16:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2)))
		dumpStoreGeneric("STORE_IMM_U16", uint64(addr), "imm", vy%(1<<16), 16)
	case STORE_IMM_U32:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4)))
		dumpStoreGeneric("STORE_IMM_U32", uint64(addr), "imm", vy%(1<<32), 32)
	case STORE_IMM_U64:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy, 8)))
		dumpStoreGeneric("STORE_IMM_U64", uint64(addr), "imm", vy, 64)
	}
}

func (vm *VMGo) HandleOneOffset(opcode byte, operands []byte) {
	vx := extractOneOffset(operands)
	dumpJumpOffset("JUMP", vx, vm.pc, vm.label_pc)
	vm.branch(uint64(int64(vm.pc)+vx), 1, len(operands))
}

// A.5.6. Instructions with Arguments of One Register & One Immediate.
func (vm *VMGo) HandleOneRegOneImm(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA := vm.Ram.ReadRegister(registerIndexA)

	addr := uint32(vx)
	switch opcode {
	case JUMP_IND:
		dumpBranchImm("JUMP_IND", registerIndexA, valueA, vx, valueA+vx, false, true)
		vm.djump((valueA + vx) % (1 << 32))
	case LOAD_IMM:
		vm.Ram.WriteRegister(registerIndexA, vx)
		dumpLoadImm("LOAD_IMM", registerIndexA, uint64(addr), vx, 64, false)
	case LOAD_U8:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 1)
		if errCode == OK {
			vm.Ram.WriteRegister(registerIndexA, uint64(value[0]))
			dumpLoadGeneric("LOAD_U8", registerIndexA, uint64(addr), uint64(value[0]), 8, false)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_I8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		if errCode == OK {
			res := x_encode(uint64(value[0]), 1)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_I8", registerIndexA, uint64(addr), res, 8, true)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_U16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode == OK {
			res := types.DecodeE_l(value)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_U16", registerIndexA, uint64(addr), res, 16, false)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_I16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode == OK {
			res := x_encode(types.DecodeE_l(value), 2)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_I16", registerIndexA, uint64(addr), res, 16, true)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_U32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode == OK {
			res := types.DecodeE_l(value)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_U32", registerIndexA, uint64(addr), res, 32, false)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_I32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode == OK {
			res := x_encode(types.DecodeE_l(value), 4)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_I32", registerIndexA, uint64(addr), res, 32, true)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case LOAD_U64:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 8)
		if errCode == OK {
			res := types.DecodeE_l(value)
			vm.Ram.WriteRegister(registerIndexA, res)
			dumpLoadGeneric("LOAD_U64", registerIndexA, uint64(addr), res, 64, false)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
	case STORE_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{uint8(valueA)})
		if errCode == OK {
			dumpStoreGeneric("STORE_U8", uint64(addr), reg(registerIndexA), valueA, 8)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
		if errCode == OK {
			dumpStoreGeneric("STORE_U16", uint64(addr), reg(registerIndexA), valueA%(1<<16), 16)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
		if errCode == OK {
			dumpStoreGeneric("STORE_U32", uint64(addr), reg(registerIndexA), valueA%(1<<32), 32)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(valueA), 8))
		if errCode == OK {
			dumpStoreGeneric("STORE_U64", uint64(addr), reg(registerIndexA), valueA, 64)
		} else {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	}
}

// A.5.7. Instructions with Arguments of One Register & Two Immediates.
func (vm *VMGo) HandleOneRegTwoImm(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(valueA) + uint32(vx)
	switch opcode {
	case STORE_IMM_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(vy))})
		dumpStoreGeneric("STORE_IMM_IND_U8", uint64(addr), fmt.Sprintf("0x%x", vy), vy&0xff, 8)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2))
		dumpStoreGeneric("STORE_IMM_IND_U16", uint64(addr), fmt.Sprintf("0x%x", vy), vy%(1<<16), 16)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4))
		dumpStoreGeneric("STORE_IMM_IND_U32", uint64(addr), fmt.Sprintf("0x%x", vy), vy%(1<<32), 32)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(vy), 8))
		dumpStoreGeneric("STORE_IMM_IND_U64", uint64(addr), fmt.Sprintf("0x%x", vy), vy, 64)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	}
}

// A.5.8 One Register, One Immediate and One Offset
func (vm *VMGo) HandleOneRegOneImmOneOffset(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	operand_len := len(operands)
	switch opcode {
	case LOAD_IMM_JUMP:
		vm.Ram.WriteRegister(registerIndexA, vx)
		dumpLoadImmJump("LOAD_IMM_JUMP", registerIndexA, vx)
		vm.branch(vy, 1, operand_len)
	case BRANCH_EQ_IMM:
		taken := 0
		if valueA == vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_EQ_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_NE_IMM:
		taken := 0
		if valueA != vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_NE_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_LT_U_IMM:
		taken := 0
		if valueA < vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_LT_U_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_LE_U_IMM:
		taken := 0
		if valueA <= vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_LE_U_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_GE_U_IMM:
		taken := 0
		if valueA >= vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_GE_U_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_GT_U_IMM:
		taken := 0
		if valueA > vx {
			taken = 1
		}
		dumpBranchImm("BRANCH_GT_U_IMM", registerIndexA, valueA, vx, vy, false, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_LT_S_IMM:
		taken := 0
		if int64(valueA) < int64(vx) {
			taken = 1
		}
		dumpBranchImm("BRANCH_LT_S_IMM", registerIndexA, valueA, vx, vy, true, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_LE_S_IMM:
		taken := 0
		if int64(valueA) <= int64(vx) {
			taken = 1
		}
		dumpBranchImm("BRANCH_LE_S_IMM", registerIndexA, valueA, vx, vy, true, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_GE_S_IMM:
		taken := 0
		if int64(valueA) >= int64(vx) {
			taken = 1
		}
		dumpBranchImm("BRANCH_GE_S_IMM", registerIndexA, valueA, vx, vy, true, taken == 1)
		vm.branch(vy, taken, operand_len)
	case BRANCH_GT_S_IMM:
		taken := 0
		if int64(valueA) > int64(vx) {
			taken = 1
		}
		dumpBranchImm("BRANCH_GT_S_IMM", registerIndexA, valueA, vx, vy, true, taken == 1)
		vm.branch(vy, taken, operand_len)
	}
}

// A.5.9. Instructions with Arguments of Two Registers.
func (vm *VMGo) HandleTwoRegs(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA := vm.Ram.ReadRegister(registerIndexA)

	var result uint64
	switch opcode {
	case MOVE_REG:
		result = valueA
		dumpMov(registerIndexD, registerIndexA, result)
	case SBRK:
		if valueA == 0 {
			vm.Ram.WriteRegister(registerIndexD, uint64(vm.Ram.GetCurrentHeapPointer()))
			return
		}
		result = uint64(vm.Ram.GetCurrentHeapPointer())
		next_page_boundary := P_func(vm.Ram.GetCurrentHeapPointer())
		new_heap_pointer := uint64(vm.Ram.GetCurrentHeapPointer()) + valueA

		if new_heap_pointer > uint64(next_page_boundary) {
			final_boundary := P_func(uint32(new_heap_pointer))
			idx_start := next_page_boundary / Z_P
			idx_end := final_boundary / Z_P
			page_count := idx_end - idx_start

			vm.Ram.allocatePages(idx_start, page_count)
		}
		vm.Ram.SetCurrentHeapPointer(uint32(new_heap_pointer))
		dumpTwoRegs("SBRK", registerIndexD, registerIndexA, valueA, result)
	case COUNT_SET_BITS_64:
		result = uint64(bits.OnesCount64(valueA))
		dumpTwoRegs("COUNT_SET_BITS_64", registerIndexD, registerIndexA, valueA, result)
	case COUNT_SET_BITS_32:
		result = uint64(bits.OnesCount32(uint32(valueA)))
		dumpTwoRegs("COUNT_SET_BITS_32", registerIndexD, registerIndexA, valueA, result)
	case LEADING_ZERO_BITS_64:
		result = uint64(bits.LeadingZeros64(valueA))
		dumpTwoRegs("LEADING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	case LEADING_ZERO_BITS_32:
		result = uint64(bits.LeadingZeros32(uint32(valueA)))
		dumpTwoRegs("LEADING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	case TRAILING_ZERO_BITS_64:
		result = uint64(bits.TrailingZeros64(valueA))
		dumpTwoRegs("TRAILING_ZERO_BITS_64", registerIndexD, registerIndexA, valueA, result)
	case TRAILING_ZERO_BITS_32:
		result = uint64(bits.TrailingZeros32(uint32(valueA)))
		dumpTwoRegs("TRAILING_ZERO_BITS_32", registerIndexD, registerIndexA, valueA, result)
	case SIGN_EXTEND_8:
		result = uint64(int8(valueA & 0xFF))
		dumpTwoRegs("SIGN_EXTEND_8", registerIndexD, registerIndexA, valueA, result)
	case SIGN_EXTEND_16:
		result = uint64(int16(valueA & 0xFFFF))
		dumpTwoRegs("SIGN_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	case ZERO_EXTEND_16:
		result = valueA & 0xFFFF
		dumpTwoRegs("ZERO_EXTEND_16", registerIndexD, registerIndexA, valueA, result)
	case REVERSE_BYTES:
		result = bits.ReverseBytes64(valueA)
		dumpTwoRegs("REVERSE_BYTES", registerIndexD, registerIndexA, valueA, result)
	default:
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	}
	vm.Ram.WriteRegister(registerIndexD, result)
}

// A.5.10 Two Registers and One Immediate
func (vm *VMGo) HandleTwoRegsOneImm(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA := vm.Ram.ReadRegister(registerIndexA)
	valueB := vm.Ram.ReadRegister(registerIndexB)
	addr := uint32((uint64(valueB) + vx) % (1 << 32))
	var result uint64

	switch opcode {
	case STORE_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(valueA))})
		dumpStoreGeneric("STORE_IND_U8", uint64(addr), reg(registerIndexA), valueA&0xff, 8)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		return
	case STORE_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
		dumpStoreGeneric("STORE_IND_U16", uint64(addr), reg(registerIndexA), valueA&0xffff, 16)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		return
	case STORE_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
		dumpStoreGeneric("STORE_IND_U32", uint64(addr), reg(registerIndexA), valueA&0xffffffff, 32)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		return
	case STORE_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(valueA), 8))
		dumpStoreGeneric("STORE_IND_U64", uint64(addr), reg(registerIndexA), valueA, 64)
		if errCode != OK {
			fmt.Printf("Failed to write 64-bit value to RAM at address 0x%x\n", addr)
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		return
	case LOAD_IND_U8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		//fmt.Printf("LOAD_IND_U8 from address %x == %d (addr=%x + %x)\n", addr, value, valueB, vx)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(uint8(value[0]))

		dumpLoadGeneric("LOAD_IND_U8", registerIndexA, uint64(addr), result, 8, false)

	case LOAD_IND_I8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(int8(value[0]))
		dumpLoadGeneric("LOAD_IND_I8", registerIndexA, uint64(addr), result, 8, true)
	case LOAD_IND_U16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
		dumpLoadGeneric("LOAD_IND_U16", registerIndexA, uint64(addr), result, 16, false)
	case LOAD_IND_I16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(int16(types.DecodeE_l(value)))
		dumpLoadGeneric("LOAD_IND_I16", registerIndexA, uint64(addr), result, 16, true)
	case LOAD_IND_U32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
		dumpLoadGeneric("LOAD_IND_U32", registerIndexA, uint64(addr), result, 32, false)
	case LOAD_IND_I32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		rawDecoded := types.DecodeE_l(value)
		result = uint64(int32(rawDecoded))
		dumpLoadGeneric("LOAD_IND_I32", registerIndexA, uint64(addr), result, 32, true)
	case LOAD_IND_U64:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 8)
		if errCode != OK {
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
		dumpLoadGeneric("LOAD_IND_U64", registerIndexA, uint64(addr), result, 64, false)
	case ADD_IMM_32:
		result = x_encode((valueB+vx)%(1<<32), 4)
		dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	case ADD_IMM_64:
		result = valueB + vx
		dumpBinOp("+", registerIndexA, registerIndexB, vx, result)
	case AND_IMM:
		result = valueB & vx
		dumpBinOp("&", registerIndexA, registerIndexB, vx, result)
	case XOR_IMM:
		result = valueB ^ vx
		dumpBinOp("^", registerIndexA, registerIndexB, vx, result)
	case OR_IMM:
		result = valueB | vx
		dumpBinOp("|", registerIndexA, registerIndexB, vx, result)
	case MUL_IMM_32:
		result = x_encode((valueB*vx)%(1<<32), 4)
		dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	case MUL_IMM_64:
		result = valueB * vx
		dumpBinOp("*", registerIndexA, registerIndexB, vx, result)
	case SET_LT_U_IMM:
		result = boolToUint(valueB < vx)
		dumpCmpOp("<u", registerIndexA, registerIndexB, vx, result)
	case SET_LT_S_IMM:
		result = boolToUint(int64(valueB) < int64(vx))
		dumpCmpOp("<s", registerIndexA, registerIndexB, vx, result)
	case SET_GT_U_IMM:
		result = boolToUint(valueB > vx)
		dumpCmpOp("u>", registerIndexA, registerIndexB, vx, result)
	case SET_GT_S_IMM:
		result = boolToUint(int64(valueB) > int64(vx))
		dumpCmpOp("s>", registerIndexA, registerIndexB, vx, result)
	case NEG_ADD_IMM_32:
		result = x_encode((vx-valueB)%(1<<32), 4)
		dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	case NEG_ADD_IMM_64:
		result = vx - valueB
		dumpBinOp("-+", registerIndexA, registerIndexB, vx, result)
	case SHLO_L_IMM_32:
		result = x_encode(valueB<<(vx&63)%(1<<32), 4)
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	case SHLO_L_IMM_64:
		result = valueB << (vx & 63)
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	case SHLO_R_IMM_32:
		result = x_encode(uint64(uint32(valueB)>>(vx&31)), 4)
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHLO_R_IMM_64:
		result = valueB >> (vx & 63)
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHAR_R_IMM_32:
		result = uint64(int64(int32(valueB) >> (vx & 31)))
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHAR_R_IMM_64:
		result = uint64(int64(valueB) >> (vx & 63))
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHLO_L_IMM_ALT_32:
		result = x_encode(vx<<(valueB&63)%(1<<32), 4)
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	case SHLO_L_IMM_ALT_64:
		result = vx << (valueB & 63)
		dumpShiftOp("<<", registerIndexA, registerIndexB, vx, result)
	case SHLO_R_IMM_ALT_32:
		result = x_encode(vx>>(valueB&63)%(1<<32), 4)
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHLO_R_IMM_ALT_64:
		result = vx >> (valueB & 63)
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case SHAR_R_IMM_ALT_64:
		result = uint64(int64(vx) >> (valueB & 63))
		dumpShiftOp(">>", registerIndexA, registerIndexB, vx, result)
	case ROT_R_64_IMM:
		result = bits.RotateLeft64(valueB, -int(vx&63))
		dumpRotOp("ROT_R_64_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	case ROT_R_64_IMM_ALT:
		result = bits.RotateLeft64(vx, -int(valueB&63))
		dumpRotOp("ROT_R_64_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	case ROT_R_32_IMM:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueB), -int(vx&31))), 4)
		dumpRotOp("ROT_R_32_IMM", reg(registerIndexA), reg(registerIndexB), vx, result)
	case ROT_R_32_IMM_ALT:
		result = x_encode(uint64(bits.RotateLeft32(uint32(vx), -int(valueB&31))), 4)
		dumpRotOp("ROT_R_32_IMM_ALT", reg(registerIndexA), reg(registerIndexB), vx, result)
	case CMOV_IZ_IMM:
		result = vx
		if valueB != 0 {
			result = valueA
		}
		dumpCmovOp("== 0", registerIndexA, registerIndexB, vx, valueA, result, true)
	case CMOV_NZ_IMM:
		if valueB != 0 {
			result = vx
		} else {
			result = valueA
		}
		dumpCmovOp("!= 0", registerIndexA, registerIndexB, vx, valueA, result, false)
	}
	vm.Ram.WriteRegister(registerIndexA, result)
}

// A.5.11 Two Registers and One Offset
func (vm *VMGo) HandleTwoRegsOneOffset(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA := vm.Ram.ReadRegister(registerIndexA)
	valueB := vm.Ram.ReadRegister(registerIndexB)
	operand_len := len(operands)

	switch opcode {
	case BRANCH_EQ:
		taken := 0
		if valueA == valueB {
			taken = 1
		}
		dumpBranch("BRANCH_EQ", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	case BRANCH_NE:
		taken := 0
		if valueA != valueB {
			taken = 1
		}
		dumpBranch("BRANCH_NE", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	case BRANCH_LT_U:
		taken := 0
		if valueA < valueB {
			taken = 1
		}
		dumpBranch("BRANCH_LT_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	case BRANCH_LT_S:
		taken := 0
		if int64(valueA) < int64(valueB) {
			taken = 1
		}
		dumpBranch("BRANCH_LT_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	case BRANCH_GE_U:
		taken := 0
		if valueA >= valueB {
			taken = 1
		}
		dumpBranch("BRANCH_GE_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	case BRANCH_GE_S:
		taken := 0
		if int64(valueA) >= int64(valueB) {
			taken = 1
		}
		dumpBranch("BRANCH_GE_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken == 1)
		vm.branch(vx, taken, operand_len)
	default:
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	}
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
func (vm *VMGo) HandleTwoRegsTwoImms(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx, vy := extractTwoRegsAndTwoImmediates(operands)
	valueB := vm.Ram.ReadRegister(registerIndexB)

	vm.Ram.WriteRegister(registerIndexA, vx)

	vm.djump((valueB + vy) % (1 << 32))
}

// A.5.13. Instructions with Arguments of Three Registers.
func (vm *VMGo) HandleThreeRegs(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)

	valueA := vm.Ram.ReadRegister(registerIndexA)
	valueB := vm.Ram.ReadRegister(registerIndexB)

	var result uint64
	switch opcode {
	case ADD_32:
		result = x_encode(uint64(uint32(valueA)+uint32(valueB)), 4)
		dumpThreeRegOp("ADD_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case SUB_32:
		result = x_encode(uint64(uint32(valueA)-uint32(valueB)), 4)
		dumpThreeRegOp("SUB_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MUL_32:
		result = x_encode(uint64(uint32(valueA)*uint32(valueB)), 4)
		dumpThreeRegOp("MUL_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case DIV_U_32:
		if valueB&0xFFFF_FFFF == 0 {
			result = maxUint64
		} else {
			result = x_encode(uint64(uint32(valueA)/uint32(valueB)), 4)
		}
		dumpThreeRegOp("DIV_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case DIV_S_32:
		a, b := int32(valueA), int32(valueB)
		switch {
		case b == 0:
			result = maxUint64
		case a == math.MinInt32 && b == -1:
			result = uint64(a)
		default:
			result = uint64(int64(a / b))
		}
		dumpThreeRegOp("DIV_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case REM_U_32:
		if valueB&0xFFFF_FFFF == 0 {
			result = x_encode(uint64(uint32(valueA)), 4)
		} else {
			r := uint32(valueA) % uint32(valueB)
			result = x_encode(uint64(r), 4)
		}
		dumpThreeRegOp("REM_U_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case REM_S_32:
		a, b := int32(valueA), int32(valueB)
		switch {
		case b == 0:
			result = uint64(a)
		case a == math.MinInt32 && b == -1:
			result = 0
		default:
			result = uint64(int64(a % b))
		}
		dumpThreeRegOp("REM_S_32", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case SHLO_L_32:
		result = x_encode(uint64(uint32(valueA)<<(valueB&31)), 4)
		dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&31, result)
	case SHLO_R_32:
		result = x_encode(uint64(uint32(valueA)>>(valueB&31)), 4)
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	case SHAR_R_32:
		result = uint64(int32(valueA) >> (valueB & 31))
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&31, result)
	case ADD_64:
		result = valueA + valueB
		dumpThreeRegOp("+", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case SUB_64:
		result = valueA - valueB
		dumpThreeRegOp("-", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MUL_64:
		result = valueA * valueB
		dumpThreeRegOp("*", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case DIV_U_64:
		if valueB == 0 {
			result = maxUint64
		} else {
			result = valueA / valueB
		}
		dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case DIV_S_64:
		if valueB == 0 {
			result = maxUint64
		} else if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
			result = valueA
		} else {
			result = uint64(int64(valueA) / int64(valueB))
		}
		dumpThreeRegOp("/", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case REM_U_64:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
		dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case REM_S_64:
		if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
			result = 0
		} else {
			result = uint64(smod(int64(valueA), int64(valueB)))
		}
		dumpThreeRegOp("%", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case SHLO_L_64:
		result = valueA << (valueB & 63)
		dumpShiftOp("<<", registerIndexD, registerIndexA, valueB&63, result)
	case SHLO_R_64:
		result = valueA >> (valueB & 63)
		dumpShiftOp(">>", registerIndexD, registerIndexA, valueB&63, result)
	case SHAR_R_64:
		result = uint64(int64(valueA) >> (valueB & 63))
		dumpShiftOp("SHAR_R_64", registerIndexD, registerIndexA, valueB&63, result)
	case AND:
		result = valueA & valueB
		dumpThreeRegOp("&", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case XOR:
		result = valueA ^ valueB
		dumpThreeRegOp("^", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case OR:
		result = valueA | valueB
		dumpThreeRegOp("|", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MUL_UPPER_S_S:
		hi, _ := bits.Mul64(valueA, valueB)
		if valueA>>63 == 1 {
			hi -= valueB
		}
		if valueB>>63 == 1 {
			hi -= valueA
		}
		result = hi
		dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MUL_UPPER_U_U:
		result, _ = bits.Mul64(valueA, valueB)
		dumpThreeRegOp("*u", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MUL_UPPER_S_U:
		hi, _ := bits.Mul64(valueA, valueB)
		if valueA>>63 == 1 {
			hi -= valueB
		}
		result = hi
		dumpThreeRegOp("*s", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case SET_LT_U:
		if valueA < valueB {
			result = 1
		} else {
			result = 0
		}
		dumpCmpOp("<u", registerIndexD, registerIndexA, valueB, result)
	case SET_LT_S:
		if int64(valueA) < int64(valueB) {
			result = 1
		} else {
			result = 0
		}
		dumpCmpOp("<s", registerIndexD, registerIndexA, valueB, result)
	case CMOV_IZ:
		if valueB == 0 {
			result = valueA
			dumpCmovOp("CMOV_IZ", registerIndexD, registerIndexB, valueA, valueA, result, true)
		} else {
			return
		}
	case CMOV_NZ:
		if valueB != 0 {
			result = valueA
			dumpCmovOp("CMOV_NZ", registerIndexD, registerIndexB, valueA, valueA, result, false)
		} else {
			return
		}
	case ROT_L_64:
		result = bits.RotateLeft64(valueA, int(valueB&63))
		dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	case ROT_L_32:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueA), int(valueB&31))), 4)
		dumpRotOp("<<", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	case ROT_R_64:
		result = bits.RotateLeft64(valueA, -int(valueB&63))
		dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&63, result)
	case ROT_R_32:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueA), -int(valueB&31))), 4)
		dumpRotOp(">>", reg(registerIndexD), reg(registerIndexA), valueB&31, result)
	case AND_INV:
		result = valueA & (^valueB)
		dumpThreeRegOp("&!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case OR_INV:
		result = valueA | (^valueB)
		dumpThreeRegOp("|!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case XNOR:
		result = ^(valueA ^ valueB)
		dumpThreeRegOp("^!", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MAX:
		result = uint64(max(int64(valueA), int64(valueB)))
		dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MAX_U:
		result = max(valueA, valueB)
		dumpThreeRegOp("max", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MIN:
		result = uint64(min(int64(valueA), int64(valueB)))
		dumpThreeRegOp("min", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	case MIN_U:
		result = min(valueA, valueB)
		dumpThreeRegOp("minu", registerIndexD, registerIndexA, registerIndexB, valueA, valueB, result)
	}

	vm.Ram.WriteRegister(registerIndexD, result)
}

func reg(index int) string {
	if index == 0 {
		return "sp"
	}
	if index == 1 {
		return "r1"
	}
	if index == 2 {
		return "r2"
	}
	if index == 3 {
		return "r3"
	}
	if index == 4 {
		return "r4"
	}
	if index == 5 {
		return "r5"
	}
	if index == 6 {
		return "r6"
	}
	if index == 7 {
		return "r7"
	}
	if index == 8 {
		return "r8"
	}
	if index == 9 {
		return "r9"
	}
	if index == 10 {
		return "r10"
	}
	if index == 11 {
		return "r11"
	}
	if index == 12 {
		return "r12"
	}
	if index < 0 || index > 15 {
		return fmt.Sprintf("R%d", index)
	}
	return fmt.Sprintf("R%d", index%16)
}

type VMLog struct {
	Opcode    byte
	OpStr     string
	Operands  []byte
	PvmPc     uint64
	Registers []uint64
	Gas       int64
}

var VMsCompare = false

var hiResGasRangeStart = int64(0)
var hiResGasRangeEnd = int64(math.MaxInt64)
var BBSampleRate = 20_000_000
var RecordLogSampleRate = 1

type VMLogs []VMLog

func (vm *VMGo) SetPVMContext(l string) {
	vm.logging = l
}

func (vm *VMGo) GetGas() int64 {
	return vm.Gas
}

// A.5.4. Instructions with Arguments of Two Immediates.
func extractTwoImm(oargs []byte) (vx uint64, vy uint64) {
	args := slices.Clone(oargs)
	lx := min(4, int(args[0])%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value

	return
}

// A.5.5. Instructions with Arguments of One Offset. (JUMP)
func extractOneOffset(oargs []byte) (vx int64) {
	args := slices.Clone(oargs)
	lx := min(4, len(args))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = z_encode(types.DecodeE_l(args[0:lx]), uint32(lx))
	return vx
}

// A.5.6. Instructions with Arguments of One Register & One Immediate. (JUMP_IND)
func extractOneRegOneImm(oargs []byte) (reg1 int, vx uint64) {
	args := slices.Clone(oargs)
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, max(0, len(args))-1)
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return int(registerIndexA), vx
}

// A.5.7. Instructions with Arguments of One Register and Two Immediates.
func extractOneReg2Imm(oargs []byte) (reg1 int, vx uint64, vy uint64) {
	args := slices.Clone(oargs)

	reg1 = min(12, int(args[0])%16)
	lx := min(4, (int(args[0])/16)%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return int(reg1), vx, vy
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset. (LOAD_IMM_JUMP, BRANCH_{EQ/NE/...}_IMM)
func extractOneRegOneImmOneOffset(oargs []byte) (registerIndexA int, vx uint64, vy int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	lx := min(4, (int(args[0]) / 16 % 8))
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return registerIndexA, vx, vy
}

// A.5.9. Instructions with Arguments of Two Registers.
func extractTwoRegisters(args []byte) (regD, regA int) {
	regD = min(12, int(args[0]&0x0F))
	regA = min(12, int(args[0]>>4))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func extractTwoRegsOneImm(args []byte) (reg1, reg2 int, imm uint64) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.11. Instructions with Arguments of Two Registers and One Offset. (BRANCH_{EQ/NE/...})
func extractTwoRegsOneOffset(oargs []byte) (registerIndexA, registerIndexB int, vx int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = int64(z_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	return registerIndexA, registerIndexB, vx
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
// This instruction is used to load an immediate value into a register and then jump to an address.
func extractTwoRegsAndTwoImmediates(oargs []byte) (registerIndexA, registerIndexB int, vx, vy uint64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, (int(args[1]) % 8))
	ly := min(4, max(0, len(args)-lx-2))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[2:2+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[2+lx:2+lx+ly]), uint32(ly))
	return registerIndexA, registerIndexB, vx, vy
}

// A.5.13. Instructions with Arguments of Three Registers.
func extractThreeRegs(args []byte) (reg1, reg2, dst int) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	dst = min(12, int(args[1]))
	return
}

const (
	AddressSpace = 1 << 32
	TotalPages   = AddressSpace / PageSize
	PageSize     = 4096
)

const (
	PageInaccessible = unix.PROT_NONE
	PageMutable      = unix.PROT_READ | unix.PROT_WRITE
	PageImmutable    = unix.PROT_READ
)

type RawRam struct {
	reg                  [13]uint64
	current_heap_pointer uint32
	memory               []byte
	mem_access           map[int]int // Changed from array to map for lazy allocation
}

const memSize = 4 * 1024 * 1024 * 1024 // 4GB + 1MB

func NewRawRam() (*RawRam, error) {
	return &RawRam{
		memory:     make([]byte, memSize),
		mem_access: make(map[int]int), // Initialize the map
		reg:        [13]uint64{},
	}, nil
}
func (ram *RawRam) Close() error {
	// deallocate resources if needed
	ram.memory = nil

	return nil
}

// GetMemAccess checks access rights for a memory range using mprotect probe.
// DANGER: This function uses unsafe operations and can cause SIGSEGV if the address is invalid or inaccessible.
func (ram *RawRam) GetMemAccess(address uint32, length uint32) (int, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return PageInaccessible, fmt.Errorf("invalid address")
	}
	// Map lookup with default value of PageInaccessible (0)
	access, ok := ram.mem_access[pageIndex]
	if !ok {
		access = PageInaccessible
	}
	if pageIndex == 0 {
		fmt.Printf("Page 0 Access = %d\n", access)
	}
	return access, nil
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (ram *RawRam) ReadMemory(address uint32, length uint32) (data []byte, err error) {
	if length == 0 {
		return []byte{}, nil
	}
	endAddr := address + length
	if endAddr < address {
		return nil, fmt.Errorf("range overflow: addr=%x len=%d", address, length)
	}
	if int(endAddr) > len(ram.memory) {
		return nil, fmt.Errorf("out of bounds: end=%x memlen=%d", endAddr, len(ram.memory))
	}
	access, err := ram.GetMemAccess(address, length)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory access: %w", err)
	}
	if access == PageInaccessible {
		return nil, fmt.Errorf("memory at address %x is not readable", address)
	}

	return ram.memory[int(address):int(endAddr)], nil
}

// WriteMemory writes data to a specific address in the memory if it's writable.
func (ram *RawRam) WriteMemory(address uint32, data []byte) error {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}

	access, err := ram.GetMemAccess(address, uint32(len(data)))
	if err != nil {
		return fmt.Errorf("failed to get memory access: %w", err)
	}
	if access != PageMutable {
		return fmt.Errorf("memory at address %x is not writable", address)
	}

	start := pageIndex * PageSize
	offset := int(address % PageSize)
	copy(ram.memory[start+offset:start+offset+len(data)], data)
	return nil
}

// SetPageAccess sets the memory protection of a single page using BaseReg as memory base.
func (ram *RawRam) SetPageAccess(pageIndex int, access int) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}
	ram.mem_access[pageIndex] = access
	return nil
}
func (ram *RawRam) SetPagesAccessRange(startPage, pageCount int, access int) error {
	if startPage < 0 || pageCount <= 0 || startPage+pageCount > TotalPages {
		return fmt.Errorf("invalid page range")
	}
	endPage := startPage + pageCount
	for i := startPage; i < endPage; i++ {
		ram.mem_access[i] = access
	}
	return nil
}

func (ram *RawRam) SetMemAccess(address uint32, length uint32, access byte) error {
	if length == 0 {
		return nil
	}

	// Check for overflow in address + length - 1
	if address > ^uint32(0)-(length-1) {
		return fmt.Errorf("invalid address/length: address=0x%x, length=0x%x", address, length)
	}

	// Calculate start and end page indices
	startPage := int(address / PageSize)
	endAddress := address + length - 1
	endPage := int(endAddress / PageSize)

	// Validate page indices
	if startPage < 0 || startPage >= TotalPages || endPage < 0 || endPage >= TotalPages {
		return fmt.Errorf(
			"invalid address range: 0x%x(len=0x%x) spans pages %d~%d, total pages=%d",
			address, length, startPage, endPage, TotalPages,
		)
	}

	// Set access for each page in the range
	return ram.SetPagesAccessRange(startPage, endPage-startPage+1, int(access))
}

func (ram *RawRam) WriteRAMBytes(address uint32, data []byte) (resultCode uint64) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("WriteRamBytes panic: %v\n", r)
			resultCode = OOB // Out of bounds
			return
		}
	}()
	if len(data) == 0 {
		return OK
	}
	err := ram.WriteMemory(address, data)
	if err != nil {
		fmt.Printf("WriteRamBytes error: %v\n", err)
		return OOB
	}
	return OK
}
func (ram *RawRam) ReadRAMBytes(address uint32, length uint32) (data []byte, resultCode uint64) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ReadRamBytes panic: %v\n", r)
			data = nil
			resultCode = OOB // Out of bounds
		}
	}()
	if length == 0 {
		return []byte{}, OK
	}
	data, err := ram.ReadMemory(address, length)
	if err != nil {
		fmt.Printf("ReadRamBytes error: %v\n", err)
		return nil, OOB
	}
	return data, OK
}

func (ram *RawRam) allocatePages(startPage uint32, count uint32) {
	ram.SetPagesAccessRange(int(startPage), int(count), PageMutable)
}

// GetCurrentHeapPointer
func (ram *RawRam) GetCurrentHeapPointer() uint32 {
	return ram.current_heap_pointer
}

func (ram *RawRam) SetCurrentHeapPointer(pointer uint32) {
	ram.current_heap_pointer = pointer
}

// 	GetCurrentHeapPointer() uint32
// 	SetCurrentHeapPointer(pointer uint32)
// 	ReadRegister(index int) (uint64, uint64)
// 	WriteRegister(index int, value uint64) uint64
// 	ReadRegisters() []uint64

func (ram *RawRam) ReadRegister(index int) uint64 {
	return ram.reg[index]
}

func (ram *RawRam) WriteRegister(index int, value uint64) {
	ram.reg[index] = value
}

// ReadRegisters returns a copy of the current register values.
func (ram *RawRam) ReadRegisters() [regSize]uint64 {
	return ram.reg
}
