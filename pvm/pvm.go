package pvm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strings"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/example/hello/reverse" // go get golang.org/x/example/hello/reverse
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
)

type VM struct {
	JSize          uint64
	Z              uint8
	J              []uint32
	code           []byte
	bitmask        []byte
	pc             uint64 // Program counter
	ResultCode     uint8
	HostResultCode uint64
	Fault_address  uint32
	terminated     bool
	hostCall       bool // ̵h in GP
	host_func_id   int  // h in GP
	Ram            *RAM
	register       []uint64
	Gas            int64
	hostenv        types.HostEnv

	VMs map[uint32]*VM

	// Work Package Inputs
	WorkItemIndex uint32
	WorkPackage   types.WorkPackage
	Extrinsics    types.ExtrinsicsBlobs
	Authorization []byte
	Imports       [][][]byte

	// Invocation funtions entry point
	EntryPoint uint32

	// if logging = "author"
	logging string

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

	pushFrame       func([]byte)
	stopFrameServer func()
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
	case firstByte >= 0 && firstByte < 128:
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
	fmt.Printf("DecodeProgram o_size: %d, w_size: %d, z_val: %d, s_val: %d\n", o_size, w_size, z_val, s_val)
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

	//p_w := P_func(vm.w_size)
	//p_arg := P_func(argLen)

	// o_byte
	vm.Ram.WriteRAMBytes(Z_Z, vm.o_byte)
	fmt.Printf("Copied o_byte (%d bytes) to RAM at address %d\n", len(vm.o_byte), Z_Z)

	// w_byte
	z_o := Z_func(vm.o_size)
	w_addr := 2*Z_Z + z_o
	vm.Ram.WriteRAMBytes(w_addr, vm.w_byte)
	fmt.Printf("Copied w_byte (len %d) to RAM at address %d\n", len(vm.w_byte), w_addr)

	// stack
	//s_addr := uint32(0xFFFFFFFF) - 2*Z_Z - Z_I -  P_func(vm.s) + 1

	// argument
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	vm.Ram.WriteRAMBytes(argAddr, argument_data_a)
	fmt.Printf("Copied argument_data_a (len %d) to RAM at address %d\n", len(argument_data_a), argAddr)
	z_s := Z_func(vm.s)
	requiredMemory := uint32(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return
	}

	vm.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.WriteRegister(7, uint64(argAddr))
	vm.WriteRegister(8, uint64(uint32(len(argument_data_a))))

}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte) *VM {
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
		Gas:             0,
		JSize:           p.JSize,
		Z:               p.Z,
		J:               p.J,
		code:            p.Code,
		bitmask:         []byte(p.K),
		register:        make([]uint64, regSize),
		pc:              initialPC,
		hostenv:         hostENV, //check if we need this
		Exports:         make([][]byte, 0),
		Service_index:   service_index,
		o_size:          o_size,
		w_size:          w_size,
		z:               z,
		s:               s,
		o_byte:          o_byte,
		w_byte:          w_byte,
		ServiceMetadata: Metadata,
		CoreIndex:       2048,
	}
	copy(vm.register, initialRegs)
	vm.Ram = NewRAM(o_size, w_size, s)
	return vm
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, hostENV types.HostEnv) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)
	return NewVM(serviceIndex, c, []uint64{}, i, hostENV, true, []byte(metadata))
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int, is_child bool) error {
	vm.terminated = false

	// A.2 deblob
	if vm.code == nil {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("No code to execute")
	}

	if len(vm.code) == 0 {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("No code to execute")
	}

	if len(vm.bitmask) == 0 {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("Failed to decode bitmask")
	}

	vm.pc = uint64(entryPoint)
	stepn := 1
	for !vm.terminated {
		if err := vm.step(stepn); err != nil {
			fmt.Println("Error in step:", err)
			return err
		}
		if vm.hostCall && is_child {
			return nil
		}
		// host call invocation
		if vm.hostCall {
			vm.InvokeHostCall(vm.host_func_id)
			vm.hostCall = false
			vm.terminated = false
		}
		vm.Gas = vm.Gas - 1 // remove the else

		stepn++
	}
	fmt.Println("terminated")
	// vm.Mode = ...
	// vm.Gas = types.IsAuthorizedGasAllocation
	// log.Debug(vm.logging, "PVM Complete", "service", string(vm.ServiceMetadata), "pc", vm.pc)
	// if vm finished without error, set result code to OK
	if !vm.terminated {
		vm.ResultCode = types.RESULT_OK
	} else {
		fmt.Printf("VM terminated with ResultCode: %d\n", vm.ResultCode)
	}
	return nil
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

	switch {
	case opcode <= 1: // A.5.1 No arguments
		vm.HandleNoArgs(opcode)
	case opcode == ECALLI: // A.5.2 One immediate
		vm.HandleOneImm(opcode, operands)
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
		//fmt.Printf("OPCODE %d\n", opcode)
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
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		log.Warn(vm.logging, "terminated: unknown opcode", "service", string(vm.ServiceMetadata), "opcode", opcode)
		return nil
	}

	// avoid this: this is expensive
	if PvmLogging {
		registersJSON, _ := json.Marshal(vm.ReadRegisters())
		prettyJSON := strings.ReplaceAll(string(registersJSON), ",", ", ")
		//fmt.Printf("%-18s step:%6d pc:%6d g:%6d Registers:%s\n", opcode_str(opcode), stepn-1, this_step_pc, vm.Gas, prettyJSON)
		fmt.Printf("%s %d %d Registers:%s\n", opcode_str(opcode), stepn-1, vm.pc, prettyJSON)
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
		vm.ResultCode = types.PVM_HALT
	} else if a == 0 || a > uint64(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
	} else {
		vm.pc = uint64(vm.J[(a/Z_A)-1])
	}
}

func (vm *VM) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= len(vm.register) {
		return 0, OOB
	}
	return vm.register[index], OK
}

func (vm *VM) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= len(vm.register) {
		return OOB
	}
	vm.register[index] = value
	return OK
}

func (vm *VM) ReadRegisters() []uint64 {
	return vm.register
}

func (vm *VM) branch(vx uint64, condition bool) {
	if condition {
		vm.pc = vx
	} else {
		vm.ResultCode = types.PVM_PANIC
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
		vm.ResultCode = types.RESULT_PANIC
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
		vm.ResultCode = types.PVM_HOST
		vm.HostResultCode = types.PVM_HOST
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
	vm.WriteRegister(registerIndexA, uint64(vx))
}

func (vm *VM) HandleTwoImms(opcode byte, operands []byte) {
	// handle no operand means 0
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
	case STORE_IMM_U16:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2)))
	case STORE_IMM_U32:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4)))
	case STORE_IMM_U64:
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy, 8)))
	}
}

func (vm *VM) HandleOneOffset(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)
	lx := min(4, len(originalOperands))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := z_encode(types.DecodeE_l(originalOperands[0:lx]), uint32(lx))
	vm.branch(uint64(int64(vm.pc)+vx), true)
}

func (vm *VM) HandleOneRegOneImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands))-1)
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	valueA, _ := vm.ReadRegister(registerIndexA)

	addr := uint32(vx)
	switch opcode {
	case JUMP_IND:
		vm.djump((valueA + vx) % (1 << 32))
	case LOAD_IMM:
		vm.WriteRegister(registerIndexA, vx)
	case LOAD_U8:
		value, errCode := vm.Ram.ReadRAMBytes((uint32(vx)), 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, uint64(value[0]))
	case LOAD_I8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(uint64(value[0]), 1))
	case LOAD_U16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	case LOAD_I16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(types.DecodeE_l(value), 2))
	case LOAD_U32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	case LOAD_I32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(types.DecodeE_l(value), 4))
	case LOAD_U64:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 8)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	case STORE_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{uint8(valueA)})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(valueA), 8))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	}
}

func (vm *VM) HandleOneRegTwoImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int(originalOperands[0])/16)%8)
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	valueA, _ := vm.ReadRegister(registerIndexA)

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly))

	addr := uint32(valueA) + uint32(vx)
	switch opcode {
	case STORE_IMM_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(vy))})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(vy), 8))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	}
}

func (vm *VM) HandleOneRegOneImmOneOffset(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int(originalOperands[0]) / 16 % 8))
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := uint64(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly)))

	valueA, _ := vm.ReadRegister(registerIndexA)

	switch opcode {
	case LOAD_IMM_JUMP:
		vm.WriteRegister(registerIndexA, vx)
		vm.branch(vy, true)
	case BRANCH_EQ_IMM:
		if valueA == vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_NE_IMM:
		if valueA != vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_U_IMM:
		if valueA < vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LE_U_IMM:
		if valueA <= vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_U_IMM:
		if valueA >= vx {
			//  --- BRANCH_GE_U_IMM value=13, vx=10
			//fmt.Printf(" --- BRANCH_GE_U_IMM value=%d, vx=%d\n", valueA, vx)
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GT_U_IMM:
		if valueA > vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_S_IMM:
		if int64(valueA) < int64(vx) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LE_S_IMM:
		if int64(valueA) <= int64(vx) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_S_IMM:
		if int64(valueA) >= int64(vx) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GT_S_IMM:
		if int64(valueA) > int64(vx) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	}
}

func (vm *VM) HandleTwoRegs(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexD := min(12, int(originalOperands[0])%16)
	registerIndexA := min(12, int(originalOperands[0])/16)

	valueA, _ := vm.ReadRegister(registerIndexA)

	var result uint64
	switch opcode {
	case MOVE_REG:
		result, _ = vm.ReadRegister(registerIndexA)
	case SBRK:
		if valueA == 0 {
			// The guest is querying the current heap pointer
			vm.WriteRegister(registerIndexD, uint64(vm.Ram.current_heap_pointer))
			return
		}

		// Record current heap pointer to return
		result = uint64(vm.Ram.current_heap_pointer)

		next_page_boundary := P_func(vm.Ram.current_heap_pointer)
		new_heap_pointer := uint64(vm.Ram.current_heap_pointer) + valueA

		if new_heap_pointer > uint64(next_page_boundary) {
			final_boundary := P_func(uint32(new_heap_pointer))
			idx_start := next_page_boundary / Z_P
			idx_end := final_boundary / Z_P
			page_count := idx_end - idx_start

			vm.Ram.allocatePages(idx_start, page_count)
		}

		// Advance the heap
		vm.Ram.current_heap_pointer = uint32(new_heap_pointer)
		break

	case COUNT_SET_BITS_64:
		result = uint64(bits.OnesCount64(valueA))
	case COUNT_SET_BITS_32:
		result = uint64(bits.OnesCount32(uint32(valueA)))
	case LEADING_ZERO_BITS_64:
		result = uint64(bits.LeadingZeros64(valueA))
	case LEADING_ZERO_BITS_32:
		result = uint64(bits.LeadingZeros32(uint32(valueA)))
	case TRAILING_ZERO_BITS_64:
		result = uint64(bits.TrailingZeros64(valueA))
	case TRAILING_ZERO_BITS_32:
		result = uint64(bits.TrailingZeros32(uint32(valueA)))
	case SIGN_EXTEND_8:
		result = uint64(int8(valueA & 0xFF))
	case SIGN_EXTEND_16:
		result = uint64(int16(valueA & 0xFFFF))
	case ZERO_EXTEND_16:
		result = valueA & 0xFFFF
	case REVERSE_BYTES:
		result = bits.ReverseBytes64(valueA)
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexD, result)
}

func (vm *VM) HandleTwoRegsOneImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	addr := uint32(valueB) + uint32(vx)
	var result uint64

	switch opcode {
	case STORE_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(addr, []byte{byte(uint8(valueA))})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		//fmt.Printf("**** opcode STORE_IND_U8 addr=%x value=%d\n", addr, uint8(valueA))
		return
	case STORE_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		//fmt.Printf("**** opcode STORE_IND_U16 addr=%x value=%d\n", addr, types.E_l(valueA%(1<<16), 2))
		return
	case STORE_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		//fmt.Printf("**** opcode STORE_IND_U32 addr=%x value=%d\n", addr, types.E_l(valueA%(1<<32), 4))
		return
	case STORE_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(uint64(valueA), 8))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
		//fmt.Printf("**** opcode STORE_IND_U64 addr=%x value=%d\n", addr, types.E_l(uint64(valueA), 8))
		return
	case LOAD_IND_U8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		// Ours 13
		// LOAD_IND_U8 3883 75818 Registers:[2266, 4278056448, 32, 32, 200963, 201009, 1, 4278056520, 201009, 1, 4278056520, 5, 40]
		// **** opcode LOAD_IND_U8 registerIndexA=11 value=5 addr=31131
		// JavaJAM:
		// LOAD_IND_U8 3883 75818 Registers:[2266, 4278056448, 32, 32, 200963, 201009, 1, 4278056520, 201009, 1, 4278056520, 194, 40]

		result = uint64(value[0])
		if addr < 10000000 {
			//	fmt.Printf("**** opcode LOAD_IND_U8 registerIndexA=%d value=%d addr=%x\n", registerIndexA, result, addr)
		}
	case LOAD_IND_I8:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(int8(value[0]))
	case LOAD_IND_U16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
	case LOAD_IND_I16:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(int16(types.DecodeE_l(value)))
	case LOAD_IND_U32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
	case LOAD_IND_I32:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = uint64(int32(types.DecodeE_l(value)))
	case LOAD_IND_U64:
		value, errCode := vm.Ram.ReadRAMBytes(addr, 8)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result = types.DecodeE_l(value)
	case ADD_IMM_32:
		result = x_encode(uint64(uint32(valueB)+uint32(vx)), 4)
	case AND_IMM:
		result = valueB & vx
	case XOR_IMM:
		result = valueB ^ vx
	case OR_IMM:
		result = valueB | vx
	case MUL_IMM_32:
		result = x_encode(uint64(uint32(valueB*vx)), 4)
	case SET_LT_U_IMM:
		if valueB < vx {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		if int64(valueB) < int64(vx) {
			result = 1
		} else {
			result = 0
		}
	case SHLO_L_IMM_32:
		result = x_encode(uint64(uint32(valueB)<<(vx&31)), 4)
	case SHLO_R_IMM_32:
		result = x_encode(uint64(uint32(valueB)>>(vx&31)), 4)
	case SHAR_R_IMM_32:
		result = uint64(int64(int32(valueB) >> (vx & 31)))
	case NEG_ADD_IMM_32:
		result = x_encode(uint64(uint32(vx)-uint32(valueB)), 4)
	case SET_GT_U_IMM:
		if valueB > vx {
			result = 1
		} else {
			result = 0
		}
	case SET_GT_S_IMM:
		if int64(valueB) > int64(vx) {
			result = 1
		} else {
			result = 0
		}
	case SHLO_L_IMM_ALT_32:
		result = x_encode(uint64(uint32(vx)<<(valueB&31)), 4)
	case SHLO_R_IMM_ALT_32:
		result = x_encode(uint64(uint32(vx)>>(valueB&31)), 4)
	case SHAR_R_IMM_ALT_32:
		result = uint64(int64(int32(vx) >> (valueB & 31)))
	case CMOV_IZ_IMM:
		if valueB == 0 {
			result = vx
		} else {
			result = valueA
		}
	case CMOV_NZ_IMM:
		if valueB != 0 {
			result = vx
		} else {
			result = valueA
		}
	case ADD_IMM_64:
		result = valueB + vx
	case MUL_IMM_64:
		result = (valueB * vx)
	case SHLO_L_IMM_64:
		result = valueB << (vx & 63)
	case SHLO_R_IMM_64:
		result = valueB >> (vx & 63)
	case SHAR_R_IMM_64:
		result = uint64(int64(valueB) >> (vx & 63))
	case NEG_ADD_IMM_64:
		result = vx - valueB
	case SHLO_L_IMM_ALT_64:
		result = vx << (valueB & 63)
	case SHLO_R_IMM_ALT_64:
		result = vx >> (valueB & 63)
	case SHAR_R_IMM_ALT_64:
		result = uint64(int64(vx) >> (valueB & 63))
	case ROT_R_64_IMM:
		result = bits.RotateLeft64(valueB, -int(vx&63))
	case ROT_R_64_IMM_ALT:
		result = bits.RotateLeft64(vx, -int(valueB&63))
	case ROT_R_32_IMM:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueB), -int(vx&31))), 4)
	case ROT_R_32_IMM_ALT:
		result = x_encode(uint64(bits.RotateLeft32(uint32(vx), -int(valueB&31))), 4)
	}
	vm.WriteRegister(registerIndexA, result)
}

func (vm *VM) HandleTwoRegsOneOffset(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := uint64(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx)))

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)

	switch opcode {
	case BRANCH_EQ:
		if valueA == valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_NE:
		if valueA != valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_U:
		if valueA < valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_S:
		if int64(valueA) < int64(valueB) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_U:
		if valueA >= valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_S:
		if int64(valueA) >= int64(valueB) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

func (vm *VM) HandleTwoRegsTwoImms(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, (int(originalOperands[1]) % 8))
	ly := min(4, max(0, len(originalOperands)-lx-2))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx := x_encode(types.DecodeE_l(originalOperands[2:2+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[2+lx:2+lx+ly]), uint32(ly))

	valueB, _ := vm.ReadRegister(registerIndexB)

	vm.WriteRegister(registerIndexA, vx)
	vm.djump((valueB + vy) % (1 << 32))
}

func (vm *VM) HandleThreeRegs(opcode byte, operands []byte) {
	registerIndexA := min(12, int(operands[0]&0x0F))
	registerIndexB := min(12, int(operands[0]>>4))
	registerIndexD := min(12, int(operands[1]))

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)
	// valueD, _ := vm.ReadRegister(registerIndexD)

	var result uint64
	switch opcode {
	case ADD_32:
		result = x_encode(uint64(uint32(valueA)+uint32(valueB)), 4)
	case SUB_32:
		result = x_encode(uint64(uint32(valueA)-uint32(valueB)), 4)
	case MUL_32:
		result = uint64(uint32(valueA) * uint32(valueB))
	case DIV_U_32:
		if valueB&0xFFFF_FFFF == 0 {
			result = maxUint64
		} else {
			result = x_encode(uint64(uint32(valueA)/uint32(valueB)), 4)
		}
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
	case REM_U_32:
		if valueB&0xFFFF_FFFF == 0 {
			result = x_encode(uint64(uint32(valueA)), 4)
		} else {
			r := uint32(valueA) % uint32(valueB)
			result = x_encode(uint64(r), 4)
		}
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
	case SHLO_L_32:
		result = x_encode(uint64(uint32(valueA)<<(valueB&31)), 4)
	case SHLO_R_32:
		result = x_encode(uint64(uint32(valueA)>>(valueB&31)), 4)
	case SHAR_R_32:
		result = uint64(int32(valueA) >> (valueB & 31))
	case ADD_64:
		result = valueA + valueB
	case SUB_64:
		result = valueA - valueB
	case MUL_64:
		result = valueA * valueB
	case DIV_U_64:
		if valueB == 0 {
			result = maxUint64
		} else {
			result = valueA / valueB
		}
	case DIV_S_64:
		if valueB == 0 {
			result = maxUint64
		} else if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
			result = valueA
		} else {
			result = uint64(int64(valueA) / int64(valueB))
		}
	case REM_U_64:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
	case REM_S_64:
		if int64(valueA) == -(1<<63) && int64(valueB) == -1 {
			result = 0
		} else {
			result = uint64(smod(int64(valueA), int64(valueB)))
		}
	case SHLO_L_64:
		result = valueA << (valueB & 63)
	case SHLO_R_64:
		result = valueA >> (valueB & 63)
	case SHAR_R_64:
		result = uint64(int64(valueA) >> (valueB & 63))
	case AND:
		result = valueA & valueB
	case XOR:
		result = valueA ^ valueB
	case OR:
		result = valueA | valueB
	case MUL_UPPER_S_S:
		hi, _ := bits.Mul64(valueA, valueB)
		if valueA>>63 == 1 {
			hi -= valueB
		}
		if valueB>>63 == 1 {
			hi -= valueA
		}
		result = hi
	case MUL_UPPER_U_U:
		result, _ = bits.Mul64(valueA, valueB)
	case MUL_UPPER_S_U:
		hi, _ := bits.Mul64(valueA, valueB)
		if valueA>>63 == 1 {
			hi -= valueB
		}
		result = hi
	case SET_LT_U:
		if valueA < valueB {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S:
		if int64(valueA) < int64(valueB) {
			result = 1
		} else {
			result = 0
		}
	case CMOV_IZ:
		if valueB == 0 {
			result = valueA
		} else {
			result, _ = vm.ReadRegister(registerIndexD)
		}
	case CMOV_NZ:
		if valueB != 0 {
			result = valueA
		} else {
			result, _ = vm.ReadRegister(registerIndexD)
		}
	case ROT_L_64:
		result = bits.RotateLeft64(valueA, int(valueB&63))
	case ROT_L_32:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueA), int(valueB&31))), 4)
	case ROT_R_64:
		result = bits.RotateLeft64(valueA, -int(valueB&63))
	case ROT_R_32:
		result = x_encode(uint64(bits.RotateLeft32(uint32(valueA), -int(valueB&31))), 4)
	case AND_INV:
		result = valueA & (^valueB)
	case OR_INV:
		result = valueA | (^valueB)
	case XNOR:
		result = ^(valueA ^ valueB)
	case MAX:
		result = uint64(max(int64(valueA), int64(valueB)))
	case MAX_U:
		result = max(valueA, valueB)
	case MIN:
		result = uint64(min(int64(valueA), int64(valueB)))
	case MIN_U:
		result = min(valueA, valueB)
	}
	vm.WriteRegister(registerIndexD, result)
}
