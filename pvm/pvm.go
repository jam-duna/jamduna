package pvm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"

	"sort"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"

	//	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
	"golang.org/x/example/hello/reverse" // go get golang.org/x/example/hello/reverse
)

const (
	BackendInterpreter       = "interpreter"
	BackendRecompiler        = "recompiler"
	BackendRecompilerSandbox = "recompiler_sandbox"
)

const (
	regSize = 13

	W_X = 1024
	M   = 128
	V   = 1023
	Z_A = 2

	Z_P = (1 << 12)
	Z_Q = (1 << 16)
	Z_I = (1 << 24)
	Z_Z = (1 << 16)
)

var (
	PvmLogging = false
	PvmTrace   = false
	PvmTrace2  = false
	useRawRam  = true

	showDisassembly = false
	useEcalli500    = false
	debugRecompiler = false
	UseTally        = false
)

type VM struct {
	Backend        string
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
	Ram            RAMInterface
	Gas            int64
	hostenv        types.HostEnv

	VMs map[uint32]*VM

	// Work Package Inputs
	WorkItemIndex             uint32
	WorkPackage               types.WorkPackage
	Extrinsics                types.ExtrinsicsBlobs
	Authorization             []byte
	Imports                   [][][]byte
	AccumulateOperandElements []types.AccumulateOperandElements
	Transfers                 []types.DeferredTransfer
	N                         common.Hash

	// Invocation functions entry point
	EntryPoint uint32

	logging      string
	vmBasicBlock int

	// standard program initialization parameters
	o_size uint32
	w_size uint32
	z      uint32
	s      uint32
	o_byte []byte
	w_byte []byte

	// Refine argument
	RefineM_map        map[uint32]*RefineM
	Exports            [][]byte
	ExportSegmentIndex uint32

	// Accumulate argument
	PriorX   types.XContext
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

	BasicBlocks map[uint64]BasicBlock
	Logs        VMLogs

	//	snapshot *EmulatorSnapShot

	basicBlocks map[uint64]*BasicBlock // by PVM PC

	basicBlockExecutionCounter map[uint64]int // PVM PC to execution count

	OP_tally map[string]*X86InstTally `json:"tally"`

	initializationTime uint32 // time taken to initialize the VM
	standardInitTime   uint32
	compileTime        uint32
	executionTime      uint32
}

type Program struct {
	JSize uint64
	Z     uint8
	CSize uint64
	J     []uint32
	Code  []byte
	K     string
}

const (
	NONE = (1 << 64) - 1 // 2^32 - 1 15
	WHAT = (1 << 64) - 2 // 2^32 - 2 14
	OOB  = (1 << 64) - 3 // 2^32 - 3 13
	WHO  = (1 << 64) - 4 // 2^32 - 4 12
	FULL = (1 << 64) - 5 // 2^32 - 5 11
	CORE = (1 << 64) - 6 // 2^32 - 6 10
	CASH = (1 << 64) - 7 // 2^32 - 7 9
	LOW  = (1 << 64) - 8 // 2^32 - 8 8
	HUH  = (1 << 64) - 9 // 2^32 - 9 7
	OK   = 0             // 0
)

const (
	HALT  = 0 // regular halt ∎
	PANIC = 1 // panic ☇
	FAULT = 2 // page-fault F
	HOST  = 3 // host-call̵ h
	OOG   = 4 // out-of-gas ∞
)

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

func DecodeProgram(p []byte) (*Program, uint32, uint32, uint32, uint32, []byte, []byte) {
	pure := p
	// see A.37
	o_size := types.DecodeE_l(pure[:3])
	w_size := types.DecodeE_l(pure[3:6])
	z_val := types.DecodeE_l(pure[6:8])
	s_val := types.DecodeE_l(pure[8:11])

	var o_byte, w_byte []byte
	offset := uint64(11)
	if offset+o_size <= uint64(len(pure)) {
		o_byte = pure[offset : offset+o_size]
	} else {
		o_byte = make([]byte, o_size)
	}
	offset += o_size

	if offset+w_size <= uint64(len(pure)) {
		w_byte = pure[offset : offset+w_size]
	} else {
		w_byte = make([]byte, w_size)
	}
	offset += w_size

	if offset+4 <= uint64(len(pure)) {
		offset += 4 // skip standard_c_size_byte
	}
	//fmt.Printf("DecodeProgram o_size: %d, w_size: %d, z_val: %d, s_val: %d\n", o_size, w_size, z_val, s_val)
	return decodeCorePart(pure[offset:]), uint32(o_size), uint32(w_size), uint32(z_val), uint32(s_val), o_byte, w_byte
}

func DecodeProgram_pure_pvm_blob(p []byte) *Program {
	return decodeCorePart(p)
}

func decodeCorePart(p []byte) *Program {
	j_size_b, p := extractBytes(p)
	z_b, p := extractBytes(p)
	c_size_b, p := extractBytes(p)

	j_size, _ := types.DecodeE(j_size_b)
	z, _ := types.DecodeE(z_b)
	c_size, _ := types.DecodeE(c_size_b)

	j_len := j_size * z
	c_len := c_size

	j_byte := p[:min(len(p), int(j_len))]
	c_byte := p[min(len(p), int(j_len)):min(len(p), int(j_len+c_len))]
	k_bytes := p[min(len(p), int(j_len+c_len)):]

	var j_array []uint32
	for i := 0; i < len(j_byte); i += int(z) {
		end := min(i+int(z), len(j_byte))
		j_array = append(j_array, uint32(types.DecodeE_l(j_byte[i:end])))
	}

	var kCombined string
	for _, b := range k_bytes {
		kCombined += reverse.String(fmt.Sprintf("%08b", b))
	}
	if len(kCombined) > int(c_size) {
		kCombined = kCombined[:int(c_size)]
	}

	return &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: c_size,
		J:     j_array,
		Code:  c_byte,
		K:     kCombined,
	}
}

func CeilingDivide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

func P_func(x uint32) uint32 {
	return Z_P * CeilingDivide(x, Z_P)
}

func Z_func(x uint32) uint32 {
	return Z_Z * CeilingDivide(x, Z_Z)
}

func (vm *VM) Standard_Program_Initialization(argument_data_a []byte) {
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
	//fmt.Printf("Copied argument_data_a (len %d) to RAM at address %d\n", len(argument_data_a), argAddr)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return
	}

	vm.Ram.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.Ram.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.Ram.WriteRegister(7, uint64(argAddr))
	vm.Ram.WriteRegister(8, uint64(uint32(len(argument_data_a))))

	// fmt.Printf("Standard Program Initialization: %s=%x %s=%x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)))
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, pvmBackend string) *VM {
	if len(pvmBackend) == 0 {
		panic("pvmBackend cannot be empty")
	}
	if len(code) == 0 {
		return nil
	}
	var p *Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	if jam_ready_blob {
		p, o_size, w_size, z, s, o_byte, w_byte = DecodeProgram(code)
	} else {
		p = DecodeProgram_pure_pvm_blob(code)
		o_size = 0
		w_size = 0
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = []byte{}
	}

	vm := &VM{
		Gas:                        0,
		JSize:                      p.JSize,
		Z:                          p.Z,
		J:                          p.J,
		code:                       p.Code,
		bitmask:                    []byte(p.K),
		pc:                         initialPC,
		hostenv:                    hostENV, //check if we need this
		Exports:                    make([][]byte, 0),
		Service_index:              service_index,
		o_size:                     o_size,
		w_size:                     w_size,
		z:                          z,
		s:                          s,
		o_byte:                     o_byte,
		w_byte:                     w_byte,
		ServiceMetadata:            Metadata,
		CoreIndex:                  2048,
		Backend:                    pvmBackend,
		basicBlockExecutionCounter: make(map[uint64]int),
		OP_tally:                   make(map[string]*X86InstTally),
	}

	if useRawRam {
		vm.Ram = NewRawRAM() // for DOOM
	} else {
		vm.Ram = NewRAM(o_size, w_size, s) // for davxy 0.6.6 traces
	}

	for i := 0; i < len(initialRegs); i++ {
		vm.Ram.WriteRegister(i, initialRegs[i])
	}

	if PvmLogging {
		hiResGasRangeStart = 1
		hiResGasRangeEnd = int64(999999999999999)
	}

	if VMsCompare {
		vm.Logs = make(VMLogs, 0)
	} else {
		vm.VMs = nil
	}

	return vm
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, hostENV types.HostEnv, pvmBackend string) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)
	return NewVM(serviceIndex, c, []uint64{}, i, hostENV, true, []byte(metadata), pvmBackend)
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int, is_child bool) error {
	vm.terminated = false

	// A.2 deblob
	if vm.code == nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute")
	}

	if len(vm.code) == 0 {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute")
	}

	if len(vm.bitmask) == 0 {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("failed to decode bitmask")
	}
	vm.pc = uint64(entryPoint)
	/*	if snapshot != nil {
		// Load snapshot InitialRegs + InitialMemory
		for i, v := range snapshot.InitialRegs {
			vm.Ram.WriteRegister(i, v)
		}
		for page, mem := range snapshot.InitialMemory {
			vm.Ram.WriteRAMBytes(uint32(page)*PageSize, mem)
		}
		// for page, access := range snapshot.InitialPageMap {
		//	vm.Ram.SetPageMapping(uint32(page), uint32(frame))
		// }
		vm.vmBasicBlock = int(snapshot.BasicBlockNumber)

		vm.pc = uint64(snapshot.InitialPC)
		vm.Gas = int64(snapshot.InitialGas)
		fmt.Printf("Executing from snapshot: PC=%d, Gas=%d, Registers=%v\n", vm.pc, vm.Gas, vm.Ram.ReadRegisters())
		// a := make([]byte, 0)
		// vm.Standard_Program_Initialization(a)

	} */

	stepn := 1
	for !vm.terminated {
		// charge gas for all the next steps until hitting a basic block instruction
		gasBasicBlock, step := vm.getBasicBlockGasCost(vm.pc)
		vm.Gas -= int64(gasBasicBlock)
		// now, run the block
		for i := 0; i < step && !vm.terminated; i++ {
			if err := vm.step(stepn); err != nil {
				return err
			}

			if vm.hostCall && is_child {
				return nil
			}
			stepn++
		}
	}

	// vm.Mode = ...
	// vm.Gas = types.IsAuthorizedGasAllocation
	// if vm finished without error, set result code to OK
	if !vm.terminated {
		vm.ResultCode = types.WORKRESULT_OK
	} else if vm.ResultCode != types.WORKRESULT_OK {
		fmt.Printf("VM terminated with error code %d at PC %d (%v, %s, %s) Gas:%v\n", vm.ResultCode, vm.pc, vm.Service_index, vm.Mode, string(vm.ServiceMetadata), vm.Gas)
		//log.Warn(vm.logging, "PVM Result Code", "mode", vm.Mode, "service", string(vm.ServiceMetadata), "resultCode", vm.ResultCode)
	}
	return nil
}

func (vm *VM) SetIdentifier(id string) {
	vm.Identifier = id
}

func (vm *VM) GetIdentifier() string {
	return fmt.Sprintf("%d_%s_%s_%s", vm.Service_index, vm.Mode, vm.Backend, vm.Identifier)
}

func (vm *VM) getBasicBlockGasCost(pc uint64) (uint64, int) {
	gasCost := uint64(0)
	i := pc
	// charge gas for all the next steps until hitting a basic block instruction
	step := 0
	for i < uint64(len(vm.code)) {
		opcode := vm.code[pc]
		len_operands := vm.skip(pc)
		if opcode == ECALLI {
			operands := vm.code[pc+1 : pc+1+len_operands]
			lx := uint32(types.DecodeE_l(operands))
			host_fn := int(lx)
			gasCost += uint64(vm.chargeGas(host_fn))
		}
		pc += 1 + len_operands
		step++
		gasCost += 1
		if IsBasicBlockInstruction(opcode) {
			// step is the number of instructions executed in this basic block
			return gasCost, step
		}
	}
	return gasCost, step
}

// step performs a single step in the PVM
func (vm *VM) step(stepn int) error {
	if vm.pc >= uint64(len(vm.code)) {
		return errors.New("program counter out of bounds")
	}
	//this_step_pc := vm.pc
	opcode := vm.code[vm.pc]

	len_operands := vm.skip(vm.pc)
	operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
	og_pc := vm.pc
	if PvmTrace2 {
		fmt.Printf("%s %s\n", prefixTrace, DisassembleSingleInstruction(opcode, operands))
	}
	switch {
	case opcode <= 1: // A.5.1 No arguments
		vm.HandleNoArgs(opcode)
	case opcode == ECALLI: // A.5.2 One immediate
		vm.HandleOneImm(opcode, operands)
		// host call invocation
		if vm.hostCall {
			vm.InvokeHostCall(vm.host_func_id)
			vm.hostCall = false
			vm.terminated = false
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
		log.Warn(vm.logging, "terminated: unknown opcode", "service", string(vm.ServiceMetadata), "opcode", opcode)
		vm.HandleNoArgs(0) //TRAP
	}

	// avoid this: this is expensive
	if PvmLogging { //  || opcode == ECALLI || opcode == SBRK {
		registersJSON, _ := json.Marshal(vm.Ram.ReadRegisters())
		prettyJSON := strings.ReplaceAll(string(registersJSON), ",", " ")
		fmt.Printf("%s: %-18s step:%6d pc:%6d gas:%d Registers:%s\n", vm.Mode, opcode_str(opcode), stepn-1, vm.pc, vm.Gas, prettyJSON)

		//fmt.Printf("instruction=%d pc=%d g=%d Registers=%s\n", opcode, vm.pc, vm.Gas-1, prettyJSON)
		//fmt.Printf("%s %d %d Registers:%s\n", opcode_str(opcode), stepn-1, vm.pc, prettyJSON)
	}
	if VMsCompare {
		// preexecution logging
		vm.LogCurrentState(opcode, operands, og_pc, vm.Gas)
	}
	return nil
}

type StepSample struct {
	Op   string   `json:"op"`
	Mode string   `json:"mode"`
	Step int      `json:"step"`
	PC   uint64   `json:"pc"`
	Gas  int64    `json:"gas"`
	Reg  []uint64 `json:"reg"`
}

func (vm *VM) Compile() {
	vm.BasicBlocks = make(map[uint64]BasicBlock)
	for pc := uint64(0); pc < uint64(len(vm.code)); {
		block, nextPC := vm.compileBasicBlock(pc)
		if len(block.Instructions) == 0 {
			break
		}
		vm.BasicBlocks[pc] = *block
		pc = nextPC
	}
}

func (vm *VM) compileBasicBlock(pc uint64) (*BasicBlock, uint64) {
	block := NewBasicBlock(0)
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]
		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		block.AddInstruction(op, operands, int(pc), pc)
		pc += uint64(olen) + 1
		block.GasUsage += 1
		if IsBasicBlockInstruction(op) {
			break
		}
	}
	return block, pc
}

// skip function calculates the distance to the next instruction
func (vm *VM) skip(pc uint64) uint64 {
	n := uint64(len(vm.bitmask))
	end := pc + 25
	if end > n {
		end = n
	}
	for i := pc + 1; i < end; i++ {
		if vm.bitmask[i] == '1' {
			return i - pc - 1
		}
	}
	if end < pc+25 {
		return end - pc - 1
	}
	return 24
}

func (vm *VM) djump(a uint64) {
	if a == uint64((1<<32)-(1<<16)) {
		vm.terminated = true
		vm.ResultCode = types.WORKRESULT_OK
	} else if a == 0 || a > uint64(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
	} else {
		vm.pc = uint64(vm.J[(a/Z_A)-1])
	}
}

func (vm *VM) branch(vx uint64, condition bool) {
	if condition {
		vm.pc = vx
	} else {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	}
}

func z_encode(a uint64, n uint32) int64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 64 - 8*n
	return int64(a<<shift) >> shift
}

// func z_decode(a int64, n uint32) uint64 {
// 	if n == 0 || n > 8 {
// 		return 0
// 	}
// 	var mask uint64
// 	if n == 8 {
// 		mask = ^uint64(0)
// 	} else {
// 		mask = (uint64(1) << (8 * n)) - 1
// 	}
// 	return uint64(a) & mask
// }

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

func (vm *VM) HandleNoArgs(opcode byte) {
	switch opcode {
	case TRAP:
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		//log.Warn(vm.logging, "TRAP encountered", "service", string(vm.ServiceMetadata), "mode", vm.Mode, "pc", vm.pc)
		fmt.Printf("TRAP encountered at pc %d in mode %s\n", vm.pc, vm.Mode)
		vm.terminated = true
	case FALLTHROUGH:
		vm.pc += 1
	}
}

func (vm *VM) HandleOneImm(opcode byte, operands []byte) {
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

func (vm *VM) HandleOneRegOneEWImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := 8
	vx := types.DecodeE_l(originalOperands[1 : 1+lx])
	dumpLoadImm("LOAD_IMM_64", registerIndexA, uint64(vx), vx, 64, false)
	vm.Ram.WriteRegister(registerIndexA, uint64(vx))
}

func (vm *VM) HandleTwoImms(opcode byte, operands []byte) {
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
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, []byte{uint8(vx)}))
		dumpStoreGeneric("STORE_IMM_U8", uint64(addr), "imm", vx, 8)
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

func (vm *VM) HandleOneOffset(opcode byte, operands []byte) {
	vx := extractOneOffset(operands)
	dumpJumpOffset("JUMP", vx, vm.pc)
	vm.branch(uint64(int64(vm.pc)+vx), true)
}

// A.5.6. Instructions with Arguments of One Register & One Immediate.
func (vm *VM) HandleOneRegOneImm(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)

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
func (vm *VM) HandleOneRegTwoImm(opcode byte, operands []byte) {
	registerIndexA, vx, vy := extractOneReg2Imm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
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
func (vm *VM) HandleOneRegOneImmOneOffset(opcode byte, operands []byte) {
	registerIndexA, vx, vy0 := extractOneRegOneImmOneOffset(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	vy := uint64(int64(vm.pc) + vy0)
	switch opcode {
	case LOAD_IMM_JUMP:
		vm.Ram.WriteRegister(registerIndexA, vx)
		dumpLoadImmJump("LOAD_IMM_JUMP", registerIndexA, vx)
		vm.branch(vy, true)
	case BRANCH_EQ_IMM:
		taken := valueA == vx
		dumpBranchImm("BRANCH_EQ_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_NE_IMM:
		taken := valueA != vx
		dumpBranchImm("BRANCH_NE_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_LT_U_IMM:
		taken := valueA < vx
		dumpBranchImm("BRANCH_LT_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_LE_U_IMM:
		taken := valueA <= vx
		dumpBranchImm("BRANCH_LE_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_GE_U_IMM:
		taken := valueA >= vx
		dumpBranchImm("BRANCH_GE_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_GT_U_IMM:
		taken := valueA > vx
		dumpBranchImm("BRANCH_GT_U_IMM", registerIndexA, valueA, vx, vy, false, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_LT_S_IMM:
		taken := int64(valueA) < int64(vx)
		dumpBranchImm("BRANCH_LT_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_LE_S_IMM:
		taken := int64(valueA) <= int64(vx)
		dumpBranchImm("BRANCH_LE_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_GE_S_IMM:
		taken := int64(valueA) >= int64(vx)
		dumpBranchImm("BRANCH_GE_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}

	case BRANCH_GT_S_IMM:
		taken := int64(valueA) > int64(vx)
		dumpBranchImm("BRANCH_GT_S_IMM", registerIndexA, valueA, vx, vy, true, taken)
		if taken {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	}
}

// A.5.9. Instructions with Arguments of Two Registers.
func (vm *VM) HandleTwoRegs(opcode byte, operands []byte) {
	registerIndexD, registerIndexA := extractTwoRegisters(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)

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
func (vm *VM) HandleTwoRegsOneImm(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)
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
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		return
	case LOAD_IND_U8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
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
		result = uint64(int32(types.DecodeE_l(value)))
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
func (vm *VM) HandleTwoRegsOneOffset(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx0 := extractTwoRegsOneOffset(operands)
	vx := uint64(int64(vm.pc) + int64(vx0))
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	switch opcode {
	case BRANCH_EQ:
		taken := valueA == valueB
		dumpBranch("BRANCH_EQ", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_NE:
		taken := valueA != valueB
		dumpBranch("BRANCH_NE", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_U:
		taken := valueA < valueB
		dumpBranch("BRANCH_LT_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_S:
		taken := int64(valueA) < int64(valueB)
		dumpBranch("BRANCH_LT_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_U:
		taken := valueA >= valueB
		dumpBranch("BRANCH_GE_U", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_S:
		taken := int64(valueA) >= int64(valueB)
		dumpBranch("BRANCH_GE_S", registerIndexA, registerIndexB, valueA, valueB, vx, taken)
		if taken {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	default:
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	}
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
func (vm *VM) HandleTwoRegsTwoImms(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, vx, vy := extractTwoRegsAndTwoImmediates(operands)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

	vm.Ram.WriteRegister(registerIndexA, vx)

	vm.djump((valueB + vy) % (1 << 32))
}

// A.5.13. Instructions with Arguments of Three Registers.
func (vm *VM) HandleThreeRegs(opcode byte, operands []byte) {
	registerIndexA, registerIndexB, registerIndexD := extractThreeRegs(operands)

	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	valueB, _ := vm.Ram.ReadRegister(registerIndexB)

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
		return "ra"
	}
	if index == 1 {
		return "sp"
	}
	if index == 2 {
		return "t0"
	}
	if index == 3 {
		return "t1"
	}
	if index == 4 {
		return "t2"
	}
	if index == 5 {
		return "s0"
	}
	if index == 6 {
		return "s1"
	}
	if index == 7 {
		return "a0"
	}
	if index == 8 {
		return "a1"
	}
	if index == 9 {
		return "a2"
	}
	if index == 10 {
		return "a3"
	}
	if index == 11 {
		return "a4"
	}
	if index == 12 {
		return "a5"
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

func (vm *VM) GetMemory() (map[int][]byte, map[int]int) {
	memory := make(map[int][]byte)
	pageMap := make(map[int]int)
	for i := 0; i < TotalPages; i++ {
		pageMap[i] = PageMutable
	}
	// 1) collect all the dirty, accessible pages
	pages := vm.Ram.GetDirtyPages()
	if len(pages) == 0 {
		fmt.Println("No writable memory found in the snapshot.")
		return memory, pageMap
	}

	// 2) sort so we can find contiguous runs
	sort.Ints(pages)

	// 3) walk the sorted list and group into runs
	const pageSize = PageSize
	bytesSaved := 0
	for i := 0; i < len(pages); {
		runStart := pages[i]
		runEnd := runStart

		// extend run while next page is exactly +1
		j := i + 1
		for j < len(pages) && pages[j] == runEnd+1 {
			runEnd = pages[j]
			j++
		}

		// 4) one ReadRAMBytes for the entire run
		byteOffset := uint32(runStart * pageSize)
		byteLen := uint32((runEnd - runStart + 1) * pageSize)
		chunk, errCode := vm.Ram.ReadRAMBytes(byteOffset, byteLen)
		if errCode == OOB {
			fmt.Printf("Error reading memory at pages %d–%d: %v\n", runStart, runEnd, errCode)
		} else if len(chunk) != int(byteLen) {
			fmt.Printf("ReadRAMBytes returned %d bytes for pages %d–%d, expected %d\n",
				len(chunk), runStart, runEnd, byteLen)
		} else {
			// 5) slice that big chunk back into per-page entries
			for p := runStart; p <= runEnd; p++ {
				off := (p - runStart) * pageSize
				memory[p] = chunk[off : off+pageSize]
				pageMap[p] = PageMutable
			}
			bytesSaved += (runEnd - runStart + 1) * pageSize
			//fmt.Printf(" GetMemory pages %d to %d (%d bytes) %s\n", runStart, runEnd, (runEnd-runStart+1)*pageSize, common.Blake2Hash(chunk))
		}

		// move to the next run
		i = j
	}
	//	fmt.Printf(" GetMemory %d\n", bytesSaved)
	return memory, pageMap
}

/*
	func (vm *VM) TakeSnapShot(name string, pc uint32, registers []uint64, gas uint64, failAddress uint64, BaseRegValue uint64, basicBlockNumber uint64) *EmulatorSnapShot {
		memory, pagemap := vm.GetMemory()
		snapshot := &EmulatorSnapShot{
			Name:             name,
			InitialRegs:      registers,
			InitialPC:        pc,
			FailAddress:      failAddress,
			InitialPageMap:   pagemap,
			InitialMemory:    memory,
			InitialGas:       gas,
			Code:             make([]byte, 0), // regenerated anyway
			BaseRegValue:     BaseRegValue,
			BasicBlockNumber: basicBlockNumber,
		}
		return snapshot
	}

	func (vm *VM) SaveSnapShot(snapshot *EmulatorSnapShot) error {
		filePath := fmt.Sprintf("%s/BB%d.json", vm.Backend, snapshot.BasicBlockNumber)
		data, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot: %w", err)
		}

		err = os.WriteFile(filePath, data, 0644)
		if err != nil {
			return fmt.Errorf("failed to write snapshot to file %s: %w", filePath, err)
		}
		fmt.Printf("Snapshot %s saved [PC: %d, BasicBlock: %d, Gas: %d Registers: %v]\n", filePath, snapshot.InitialPC, snapshot.BasicBlockNumber, snapshot.InitialGas, snapshot.InitialRegs)
		return nil
	}
*/
func (vm *VM) LogCurrentState(opcode byte, operands []byte, currentPC uint64, gas int64) {
	if opcode == ECALLI {
		return
	}
	recordLog := false
	if gas >= hiResGasRangeStart && gas <= hiResGasRangeEnd {
		recordLog = true
	}
	/*	if vm.snapshot != nil {
		vm.snapshot.InitialPC = uint32(currentPC)
		vm.snapshot.BasicBlockNumber = uint64(vm.vmBasicBlock)
		vm.SaveSnapShot(vm.snapshot)
		vm.snapshot = nil
	} */

	if IsBasicBlockInstruction(opcode) {
		vm.vmBasicBlock++
		if vm.vmBasicBlock%RecordLogSampleRate == 0 { // every ___ basic blocks, record a log
			if vm.vmBasicBlock%100000 == 0 {
				//fmt.Printf("vmBasicBlock: %d Gas: %d PC: %d Opcode: %s Registers: %v\n", vm.vmBasicBlock, gas, currentPC, opcode_str(opcode), vm.Ram.ReadRegisters())
			}

		}
	} else {
	}
	if recordLog {
		log := VMLog{
			Opcode:   opcode,
			OpStr:    opcode_str(opcode),
			Operands: operands,
			PvmPc:    currentPC,
			Gas:      gas,
		}
		if vm.vmBasicBlock%10000 == 0 {
			//			fmt.Printf("ivmBasicBlock: %d Gas: %d PC: %d Opcode: %s Registers: %v\n", vm.vmBasicBlock, gas, currentPC, opcode_str(opcode), vm.Ram.ReadRegisters())
		}
		log.Registers = make([]uint64, len(vm.Ram.ReadRegisters()))
		for i := 0; i < regSize; i++ {
			log.Registers[i], _ = vm.Ram.ReadRegister(i)
		}
		vm.Logs = append(vm.Logs, log)
		// if (len(vm.Logs) > 10 && (gas < hiResGasRangeStart || gas > hiResGasRangeEnd)) || len(vm.Logs) > 1000 {
		// 	vm.saveLogs()
		// }
	}
}
