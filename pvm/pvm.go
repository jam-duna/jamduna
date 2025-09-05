package pvm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/sdbtiming"
	"github.com/colorfulnotion/jam/types"
)

// #cgo CFLAGS: -O3 -flto -ffast-math -finline-functions -fomit-frame-pointer
// #include "pvm.h"
// extern void goInvokeHostFunction(VM* vm, int hostFuncID);
// static inline void set_host_callback_wrapper(VM* vm) {
//     pvm_set_host_callback(vm, goInvokeHostFunction);
// }
import "C"

var benchRec = sdbtiming.New()

func BenchRows() []sdbtiming.Row { return benchRec.Snapshot() }

// Buffer pool to eliminate allocation overhead
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 20*1024*1024) // 20MB buffer
	},
}

const (
	BackendInterpreter = "interpreter" // Pure Go backend
	BackendCompiler    = "compiler"    // CGO C backend
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

const (
	ModeAccumulate   = "accumulate"
	ModeIsAuthorized = "is_authorized"
	ModeRefine       = "refine"
	ModeOnTransfer   = "on_transfer"
)

var (
	PvmLogging = false
	PvmTrace   = false
	PvmTrace2  = false
	useRawRam  = false

	UseTally = false

	// CGO VM mapping for callbacks
	vmMap = make(map[unsafe.Pointer]*VM)
)

type VM struct {
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

	Gas      int64
	hostenv  types.HostEnv
	register []uint64

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
	Identifier      string

	pushFrame       func([]byte)
	stopFrameServer func()

	BasicBlocks map[uint64]BasicBlock

	basicBlockExecutionCounter map[uint64]int // PVM PC to execution count

	initializationTime uint32 // time taken to initialize the VM
	standardInitTime   uint32
	compileTime        uint32
	executionTime      uint32

	// memory
	stack_address        uint32
	stack_address_end    uint32
	rw_data_address      uint32
	rw_data_address_end  uint32
	ro_data_address      uint32
	ro_data_address_end  uint32
	current_heap_pointer uint32
	output_address       uint32
	output_end           uint32

	stack   []byte
	rw_data []byte
	ro_data []byte
	output  []byte

	// CGO Integration
	cVM   unsafe.Pointer  // Pointer to C VM struct
	cRegs *[13]C.uint64_t // Direct pointer to C registers

	// Memory region pointers (shared with C)
	cRWDataPtr *C.uint8_t // C pointer to rw_data
	cRODataPtr *C.uint8_t // C pointer to ro_data
	cOutputPtr *C.uint8_t // C pointer to output
	cStackPtr  *C.uint8_t // C pointer to stack

	// Synchronization control
	syncRegisters bool // Flag to control register sync
}

type Program struct {
	JSize uint64
	Z     uint8
	CSize uint64
	J     []uint32
	Code  []byte
	K     []byte
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

	c_size := types.DecodeE_l(pure[offset : offset+4])
	offset += 4
	if len(pure[offset:]) != int(c_size) {
		// fmt.Printf("DecodeProgram o_size: %d, w_size: %d, z_val: %d, s_val: %d len(w_byte)=%d\n", o_size, w_size, z_val, s_val, len(w_byte))
		return nil, 0, 0, 0, 0, nil, nil
	}
	return decodeCorePart(pure[offset:]), uint32(o_size), uint32(w_size), uint32(z_val), uint32(s_val), o_byte, w_byte
}

func DecodeProgram_pure_pvm_blob(p []byte) *Program {
	return decodeCorePart(p)
}

// EncodeProgram encodes raw instruction code and bitmask into a PVM blob format
// that can be decoded by decodeCorePart
func EncodeProgram(code []byte, bitmask []byte) []byte {
	j_size := uint64(0) // TODO: add J support
	z := uint64(0)      // TODO: add z support correctly
	c_size := uint64(len(code))

	// Encode the header values using JAM's E() function
	j_size_encoded := types.E(j_size)
	z_encoded := types.E(z)
	c_size_encoded := types.E(c_size)

	// Compress bitmask: convert from bit array to byte array
	k_bytes := compressBits(bitmask)

	// Build the blob: header + j_bytes + c_bytes + k_bytes
	blob := make([]byte, 0)
	blob = append(blob, j_size_encoded...)
	blob = append(blob, z_encoded...)
	blob = append(blob, c_size_encoded...)
	// TODO: No j_bytes since j_size * z = 0 * 0 = 0
	blob = append(blob, code...)    // c_bytes
	blob = append(blob, k_bytes...) // k_bytes (compressed bitmask)

	return blob
}

// compressBits converts a bitmask bit array ([]byte with 0/1 values) to compressed bytes
func compressBits(bitmask []byte) []byte {
	if len(bitmask) == 0 {
		return []byte{}
	}

	// Pack bits into bytes
	numBytes := (len(bitmask) + 7) / 8
	compressed := make([]byte, numBytes)

	for i, bit := range bitmask {
		if bit != 0 {
			byteIndex := i / 8 // optimize?
			bitIndex := i % 8
			compressed[byteIndex] |= (1 << bitIndex)
		}
	}

	return compressed
}
func expandBits(k_bytes []byte, c_size uint32) []byte {
	totalBits := len(k_bytes) * 8
	if totalBits > int(c_size) {
		totalBits = int(c_size)
	}
	kCombined := make([]byte, totalBits)
	bitIndex := 0

	for _, b := range k_bytes {
		if bitIndex+8 <= totalBits {
			kCombined[bitIndex+0] = b & 1
			kCombined[bitIndex+1] = (b >> 1) & 1
			kCombined[bitIndex+2] = (b >> 2) & 1
			kCombined[bitIndex+3] = (b >> 3) & 1
			kCombined[bitIndex+4] = (b >> 4) & 1
			kCombined[bitIndex+5] = (b >> 5) & 1
			kCombined[bitIndex+6] = (b >> 6) & 1
			kCombined[bitIndex+7] = (b >> 7) & 1
			bitIndex += 8
		} else {
			// Handle final partial byte...
			for i := 0; bitIndex < totalBits; i++ {
				kCombined[bitIndex] = (b >> i) & 1
				bitIndex++
			}
			break
		}
	}
	return kCombined
}

func decodeCorePart(p []byte) *Program {

	j_size_b, p_remaining := extractBytes(p)
	z_b, p_remaining := extractBytes(p_remaining)
	c_size_b, p_remaining := extractBytes(p_remaining)

	j_size, _ := types.DecodeE(j_size_b)
	z, _ := types.DecodeE(z_b)
	c_size, _ := types.DecodeE(c_size_b)

	j_len := j_size * z
	c_len := c_size

	j_byte := p_remaining[:min(len(p_remaining), int(j_len))]
	c_byte := p_remaining[min(len(p_remaining), int(j_len)):min(len(p_remaining), int(j_len+c_len))]
	k_bytes := p_remaining[min(len(p_remaining), int(j_len+c_len)):]

	var j_array []uint32
	for i := 0; i < len(j_byte); i += int(z) {
		end := min(i+int(z), len(j_byte))
		j_array = append(j_array, uint32(types.DecodeE_l(j_byte[i:end])))
	}

	// build bitmask
	kCombined := expandBits(k_bytes, uint32(c_size))
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
	vm.WriteRAMBytes(Z_Z, vm.o_byte)

	// w_byte
	z_o := Z_func(vm.o_size)
	w_addr := 2*Z_Z + z_o
	vm.WriteRAMBytes(w_addr, vm.w_byte)

	// argument
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	vm.WriteRAMBytes(argAddr, argument_data_a)
	//fmt.Printf("Copied argument_data_a (len %d) to RAM at address %d\n", len(argument_data_a), argAddr)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return
	}

	vm.register[0] = uint64(0xFFFFFFFF - (1 << 16) + 1)
	vm.register[1] = uint64(0xFFFFFFFF - 2*Z_Z - Z_I + 1)
	vm.register[7] = uint64(argAddr)
	vm.register[8] = uint64(uint32(len(argument_data_a)))

	// fmt.Printf("Standard Program Initialization: %s=%x %s=%x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)))
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, pvmBackend string) *VM {
	t0 := time.Now()
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
	benchRec.Add("NewVM:DecodeProgram", time.Since(t0))

	t0 = time.Now()
	// o - read-only
	ro_data_address := uint32(Z_Z)
	ro_data_address_end := ro_data_address + P_func(o_size)
	//fmt.Printf("o_size: %d copied to %d up to %d\n", o_size, ro_data_address, ro_data_address_end)

	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	heap_end := rw_data_address + Z_func(current_heap_pointer)
	//fmt.Printf("w_size: %d copied to %d up to %d\n", w_size, rw_data_address, rw_data_address_end)

	//fmt.Printf("current_heap_pointer: %d (dec) %x (hex)\n", current_heap_pointer, current_heap_pointer)

	// s - stack
	p_s := P_func(s)
	stack_address := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I) - uint64(p_s))
	stack_address_end := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I))

	// 20MB stack buffer optimization
	// fmt.Printf("s_size: %d [hex %x] stack at %d [hex %x] up to %d [hex %x]\n", s, s, stack_address, stack_address, stack_address_end, stack_address_end)
	// a - argument outputs
	a_size := uint32(Z_Z + Z_I - 1)
	output_address := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	output_end := uint32(0xFFFFFFFF)

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
		register:                   make([]uint64, regSize),

		// o_bytes are be copied here
		ro_data_address:     ro_data_address,
		ro_data_address_end: ro_data_address_end,

		// w_bytes should be copied into here
		rw_data_address:     rw_data_address,
		rw_data_address_end: rw_data_address_end,

		stack_address:     stack_address,
		stack_address_end: stack_address_end,

		current_heap_pointer: current_heap_pointer,
		output_address:       output_address,
		output_end:           output_end,
	}
	benchRec.Add("NewVM:init", time.Since(t0))

	// Direct allocation without zeroing overhead
	t0 = time.Now()
	stackSize := p_s
	rwSize := heap_end - rw_data_address
	roSize := ro_data_address_end - ro_data_address
	outputSize := a_size

	// Get buffer from pool - no zeroing overhead
	totalSize := stackSize + rwSize + roSize + outputSize
	if totalSize <= 20*1024*1024 {
		buffer := bufferPool.Get().([]byte)

		// Slice the buffer for each component
		offset := uint32(0)
		vm.stack = buffer[offset : offset+stackSize]
		offset += stackSize
		vm.rw_data = buffer[offset : offset+rwSize]
		offset += rwSize
		vm.ro_data = buffer[offset : offset+roSize]
		offset += roSize
		vm.output = buffer[offset : offset+outputSize]
	} else {
		// Fallback for very large VMs
		vm.stack = make([]byte, stackSize)
		vm.rw_data = make([]byte, rwSize)
		vm.ro_data = make([]byte, roSize)
		vm.output = make([]byte, outputSize)
	}

	benchRec.Add("NewVM:alloc", time.Since(t0))

	t0 = time.Now()
	copy(vm.ro_data[:], o_byte[:])
	copy(vm.rw_data[:], w_byte[:])

	requiredMemory := uint64(uint64(5*Z_Z) + uint64(Z_func(o_size)) + uint64(Z_func(w_size+z*Z_P)) + uint64(Z_func(s)) + uint64(Z_I))
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		// TODO
	}

	for i := 0; i < len(initialRegs); i++ {
		vm.register[i] = initialRegs[i]
	}

	vm.VMs = nil
	return vm
}

func (vm *VM) panic(errCode uint64) {
	vm.ResultCode = types.WORKDIGEST_PANIC
	vm.MachineState = PANIC
	vm.terminated = true
	vm.Fault_address = uint32(errCode)
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, hostENV types.HostEnv, pvmBackend string) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)
	return NewVM(serviceIndex, c, []uint64{}, i, hostENV, true, []byte(metadata), pvmBackend)
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int, is_child bool) error {
	vm.terminated = false
	vm.IsChild = is_child
	// A.2 deblob
	if vm.code == nil {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute")
	}

	if len(vm.code) == 0 {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("no code to execute")
	}

	if len(vm.bitmask) == 0 {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return errors.New("failed to decode bitmask")
	}
	vm.pc = uint64(entryPoint)

	stepn := 1
	n := uint64(len(vm.bitmask))
	for !vm.terminated {
		// Check if pc is within bounds of the code
		if vm.pc >= uint64(len(vm.code)) {
			// PC is beyond the end of code - this should panic
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			vm.terminated = true
			return nil
		}
		opcode := vm.code[vm.pc]

		var len_operands uint64
		limit := vm.pc + 25
		if limit > n {
			limit = n
		}

		for i := vm.pc + 1; i < limit; i++ {
			if vm.bitmask[i] == 1 {
				len_operands = i - vm.pc - 1
				goto found
			}
		}

		len_operands = limit - vm.pc - 1
		if len_operands > 24 {
			len_operands = 24
		}

	found:
		operand_start := vm.pc + 1
		operand_end := operand_start + len_operands
		operands := vm.code[operand_start:operand_end]
		vm.Gas -= 1

		// optimize LOAD_IND_U64, ADD_IMM_64, STORE_IND_U64, LOAD_IMM
		dispatchTable[opcode](vm, operands)

		stepn++
		// avoid this: this is expensive
		if PvmLogging {
			registersJSON, _ := json.Marshal(vm.register)
			prettyJSON := strings.ReplaceAll(string(registersJSON), ",", ", ")
			fmt.Printf("Go: %s %d %d Gas: %d Registers:%s\n", opcode_str(opcode), stepn-1, vm.pc, vm.Gas, prettyJSON)
		}
		if vm.Gas < 0 {
			vm.ResultCode = types.WORKDIGEST_OOG
			vm.MachineState = OOG
			vm.terminated = true
			log.Warn(vm.logging, "Out of Gas", "service", string(vm.ServiceMetadata), "mode", vm.Mode, "pc", vm.pc, "gas", vm.Gas)
			return errors.New("out of gas")
		}

	}

	if !vm.terminated {
		vm.ResultCode = types.WORKDIGEST_OK
	}
	return nil
}

func (vm *VM) djump(a uint64) {
	if a == uint64((1<<32)-(1<<16)) {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_OK
	} else if a == 0 || a > uint64(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
	} else {
		vm.pc = uint64(vm.J[(a/Z_A)-1])
	}
}

func (vm *VM) branch(vx uint64, condition bool) {
	if condition {
		vm.pc = vx
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
	}
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

// ===============================
// CGO Integration Functions
// ===============================

//export goInvokeHostFunction
func goInvokeHostFunction(cvm *C.VM, hostFuncID C.int) {
	// Look up the Go VM from our mapping
	vm, ok := vmMap[unsafe.Pointer(cvm)]
	if !ok {
		// VM not found, this shouldn't happen - log error and return
		if PvmTrace {
			fmt.Printf("Error: goInvokeHostFunction called with unknown C VM pointer\n")
		}
		return
	}

	// Sync C VM state to Go VM before host call
	if vm.syncRegisters {
		vm.SyncVMState("C_to_Go")
	}

	// Set host call state
	vm.hostCall = true
	vm.host_func_id = int(hostFuncID)

	// Invoke Go host function using the existing method with error handling
	vm.InvokeHostCall(int(hostFuncID))

	// Sync Go VM state back to C VM after host call
	if vm.syncRegisters {
		vm.SyncVMState("Go_to_C")
	}

	// Reset host call state
	vm.hostCall = false
	vm.host_func_id = 0
}

func (vm *VM) initCGOIntegration() error {
	if len(vm.code) == 0 {
		return fmt.Errorf("code must be set before CGO integration (code length: %d, bitmask length: %d)", len(vm.code), len(vm.bitmask))
	}
	if len(vm.register) == 0 {
		vm.register = make([]uint64, 13)
	}

	// Create C VM
	cvm := C.pvm_create(
		C.uint32_t(vm.Service_index),
		(*C.uint8_t)(unsafe.Pointer(&vm.code[0])),
		C.size_t(len(vm.code)),
		(*C.uint64_t)(unsafe.Pointer(&vm.register[0])),
		C.size_t(len(vm.register)),
		C.uint64_t(vm.pc))
	vm.cVM = unsafe.Pointer(cvm)

	if vm.cVM == nil {
		return errors.New("failed to create C VM")
	}

	// WORKAROUND: Set bitmask and jump table directly via unsafe pointer manipulation
	// This bypasses the CGO function call issues
	cvmPtr := (*C.VM)(vm.cVM)
	if len(vm.bitmask) > 0 {
		cvmPtr.bitmask = (*C.uint8_t)(unsafe.Pointer(&vm.bitmask[0]))
		cvmPtr.bitmask_len = C.uint32_t(len(vm.bitmask))
	}
	if len(vm.J) > 0 {
		cvmPtr.j = (*C.uint32_t)(unsafe.Pointer(&vm.J[0]))
		cvmPtr.j_size = C.uint32_t(len(vm.J))
	}

	// Setup memory region pointers
	if len(vm.rw_data) > 0 {
		vm.cRWDataPtr = (*C.uint8_t)(unsafe.Pointer(&vm.rw_data[0]))
	}
	if len(vm.ro_data) > 0 {
		vm.cRODataPtr = (*C.uint8_t)(unsafe.Pointer(&vm.ro_data[0]))
	}
	if len(vm.output) > 0 {
		vm.cOutputPtr = (*C.uint8_t)(unsafe.Pointer(&vm.output[0]))
	}
	if len(vm.stack) > 0 {
		vm.cStackPtr = (*C.uint8_t)(unsafe.Pointer(&vm.stack[0]))
	}

	C.pvm_set_memory_regions(
		(*C.VM)(vm.cVM),
		vm.cRWDataPtr, C.uint32_t(len(vm.rw_data)),
		vm.cRODataPtr, C.uint32_t(len(vm.ro_data)),
		vm.cOutputPtr, C.uint32_t(len(vm.output)),
		vm.cStackPtr, C.uint32_t(len(vm.stack)))

	// Set memory bounds
	C.pvm_set_memory_bounds(
		(*C.VM)(vm.cVM),
		C.uint32_t(vm.rw_data_address), C.uint32_t(vm.rw_data_address_end),
		C.uint32_t(vm.ro_data_address), C.uint32_t(vm.ro_data_address_end),
		C.uint32_t(vm.output_address), C.uint32_t(vm.output_end),
		C.uint32_t(vm.stack_address), C.uint32_t(vm.stack_address_end))

	// Set current heap pointer
	C.vm_set_current_heap_pointer((*C.VM)(vm.cVM), C.uint32_t(vm.current_heap_pointer))

	// Get direct pointer to C registers
	vm.cRegs = (*[13]C.uint64_t)(unsafe.Pointer(C.pvm_get_registers_ptr((*C.VM)(vm.cVM))))

	// Set host function callback
	C.set_host_callback_wrapper((*C.VM)(vm.cVM))

	// Add to VM mapping for callbacks
	vmMap[vm.cVM] = vm

	// Enable register synchronization by default
	vm.syncRegisters = true

	// Sync initial gas from Go to C
	cvmPtr.gas = C.uint64_t(vm.Gas)

	return nil
}

func (vm *VM) destroyCGOIntegration() {
	if vm.cVM != nil {
		// Remove from VM mapping
		delete(vmMap, vm.cVM)
		C.pvm_destroy((*C.VM)(vm.cVM))
		vm.cVM = nil
	}
	vm.cRegs = nil
	vm.cRWDataPtr = nil
	vm.cRODataPtr = nil
	vm.cOutputPtr = nil
	vm.cStackPtr = nil
}

// ExecuteWithCGO runs VM execution using the C interpreter
func (vm *VM) ExecuteWithCGO() error {
	if vm.cVM == nil {
		if err := vm.initCGOIntegration(); err != nil {
			return fmt.Errorf("CGO integration failed: %v", err)
		}
		defer vm.destroyCGOIntegration()
	}

	result := C.pvm_execute((*C.VM)(vm.cVM), C.int(vm.EntryPoint), C.int(0))

	if vm.IsChild {
		result = C.pvm_execute((*C.VM)(vm.cVM), C.int(vm.EntryPoint), C.int(1))
	}

	if result != 0 {
		return fmt.Errorf("C VM execution failed with code %d", result)
	}

	// Sync VM state from C back to Go
	cvmPtr := (*C.VM)(vm.cVM)
	vm.ResultCode = uint8(cvmPtr.result_code)
	vm.MachineState = uint8(cvmPtr.machine_state)
	vm.terminated = cvmPtr.terminated != 0
	vm.Fault_address = uint32(cvmPtr.fault_address)
	vm.pc = uint64(cvmPtr.pc)
	vm.Gas = int64(cvmPtr.gas)
	// Sync registers if available
	for i := 0; i < 13 && i < len(vm.register); i++ {
		vm.register[i] = uint64(cvmPtr.registers[i])
	}
	// State sync completed successfully

	return nil
}

// Register Sharing Utility Functions

// SyncRegistersFromC copies register values from C VM to Go VM
func (vm *VM) SyncRegistersFromC() {
	if vm.cRegs == nil || vm.cVM == nil {
		return
	}
	for i := 0; i < 13 && i < len(vm.register); i++ {
		vm.register[i] = uint64(vm.cRegs[i])
	}
	vm.current_heap_pointer = uint32(C.vm_get_current_heap_pointer((*C.VM)(vm.cVM)))
	if PvmTrace2 {
		fmt.Printf("Synced registers from C to Go: %v\n", vm.register)
	}
}

// SyncRegistersToC copies register values from Go VM to C VM
func (vm *VM) SyncRegistersToC() {
	if vm.cRegs == nil || vm.cVM == nil {
		return
	}
	for i := 0; i < 13 && i < len(vm.register); i++ {
		vm.cRegs[i] = C.uint64_t(vm.register[i])
	}
	C.vm_set_current_heap_pointer((*C.VM)(vm.cVM), C.uint32_t(vm.current_heap_pointer))
	if PvmTrace2 {
		fmt.Printf("Synced registers from Go to C: %v\n", vm.register)
	}
}

// SetRegisterC sets a register value in the C VM
func (vm *VM) SetRegisterC(idx int, value uint64) {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return
	}
	C.pvm_set_register((*C.VM)(vm.cVM), C.int(idx), C.uint64_t(value))
	// Also update the Go register array for consistency
	if idx < len(vm.register) {
		vm.register[idx] = value
	}
}

// GetRegisterC gets a register value from the C VM
func (vm *VM) GetRegisterC(idx int) uint64 {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return 0
	}
	return uint64(C.pvm_get_register((*C.VM)(vm.cVM), C.int(idx)))
}

// SyncVMState synchronizes VM state between C and Go during host calls
func (vm *VM) SyncVMState(direction string) {
	if vm.cVM == nil {
		return
	}

	cvm := (*C.VM)(vm.cVM)

	switch direction {
	case "C_to_Go":
		// Sync C VM state to Go VM
		vm.pc = uint64(cvm.pc)
		vm.Gas = int64(cvm.gas)
		vm.terminated = cvm.terminated != 0
		vm.MachineState = uint8(cvm.machine_state)
		vm.ResultCode = uint8(cvm.result_code)
		vm.SyncRegistersFromC()

		if PvmTrace2 {
			fmt.Printf("Synced VM state C->Go: PC=%d, Gas=%d, State=%d\n",
				vm.pc, vm.Gas, vm.MachineState)
		}

	case "Go_to_C":
		// Sync Go VM state to C VM
		cvm.pc = C.uint64_t(vm.pc)
		cvm.gas = C.uint64_t(vm.Gas)
		cvm.terminated = 0
		if vm.terminated {
			cvm.terminated = 1
		}
		cvm.machine_state = C.int(vm.MachineState)
		cvm.result_code = C.int(vm.ResultCode)
		vm.SyncRegistersToC()

		if PvmTrace2 {
			fmt.Printf("Synced VM state Go->C: PC=%d, Gas=%d, State=%d\n",
				vm.pc, vm.Gas, vm.MachineState)
		}
	}
}

// Error handling and state management for host function calls

// HandleHostCallError processes errors that occur during host function execution
func (vm *VM) HandleHostCallError(hostFuncID int, err error) {
	if PvmTrace {
		fmt.Printf("Host function %d error: %v\n", hostFuncID, err)
	}

	// Set VM to error state
	vm.MachineState = PANIC
	vm.ResultCode = 1
	vm.terminated = true

	// Update C VM state if available
	if vm.cVM != nil {
		cvm := (*C.VM)(vm.cVM)
		cvm.machine_state = C.int(PANIC)
		cvm.result_code = C.int(1)
		cvm.terminated = 1
	}
}

// ValidateHostCall checks if a host function call is valid before execution
func (vm *VM) ValidateHostCall(hostFuncID int) error {
	// Check if VM is in a valid state for host calls
	if vm.terminated {
		return fmt.Errorf("cannot invoke host function %d: VM is terminated", hostFuncID)
	}

	if vm.MachineState == PANIC {
		return fmt.Errorf("cannot invoke host function %d: VM is in PANIC state", hostFuncID)
	}

	// Check gas availability (basic check)
	if vm.Gas < 0 {
		return fmt.Errorf("cannot invoke host function %d: out of gas", hostFuncID)
	}

	return nil
}

// RecoverFromHostCallError attempts to recover from host call errors when possible
func (vm *VM) RecoverFromHostCallError(originalState VMState) {
	// Only attempt recovery for non-critical errors
	if vm.MachineState == PANIC {
		// Cannot recover from PANIC state
		return
	}

	// Restore previous state for recoverable errors
	vm.pc = originalState.PC
	vm.Gas = originalState.Gas
	vm.MachineState = originalState.MachineState
	vm.ResultCode = originalState.ResultCode

	if PvmTrace {
		fmt.Printf("Recovered VM state after host call error\n")
	}
}

// VMState represents a snapshot of VM state for recovery purposes
type VMState struct {
	PC           uint64
	Gas          int64
	MachineState uint8
	ResultCode   uint8
	Registers    [13]uint64
}

// SaveVMState creates a snapshot of current VM state
func (vm *VM) SaveVMState() VMState {
	state := VMState{
		PC:           vm.pc,
		Gas:          vm.Gas,
		MachineState: vm.MachineState,
		ResultCode:   vm.ResultCode,
	}

	// Copy registers
	for i := 0; i < 13 && i < len(vm.register); i++ {
		state.Registers[i] = vm.register[i]
	}

	return state
}

// RestoreVMState restores VM state from a snapshot
func (vm *VM) RestoreVMState(state VMState) {
	vm.pc = state.PC
	vm.Gas = state.Gas
	vm.MachineState = state.MachineState
	vm.ResultCode = state.ResultCode

	// Restore registers
	for i := 0; i < 13 && i < len(vm.register); i++ {
		vm.register[i] = state.Registers[i]
	}

	// Sync to C VM if available
	if vm.cVM != nil {
		vm.SyncVMState("Go_to_C")
	}
}
