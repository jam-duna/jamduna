package pvm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"sort"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/sdbtiming"
	"github.com/colorfulnotion/jam/types"
)

var benchRec = sdbtiming.New()

// used in ApplyStateTransitionFromBlock
func BenchRows() []sdbtiming.Row { return benchRec.Snapshot() }

type OpcodeHandler func(opcode byte, operands []byte)

const (
	BackendInterpreter = "interpreter"
	BackendCompiler    = "compiler"
	BackendSandbox     = "sandbox"
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

	showDisassembly = false
	useEcalli500    = false
	debugCompiler   = false
	UseTally        = false
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
	Ram            RAMInterface
	Gas            int64
	hostenv        types.HostEnv

	VMs           map[uint32]*VM
	dispatchTable [256]OpcodeHandler

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
func expandBits(k_bytes []byte, c_size uint32) []byte {
	totalBits := len(k_bytes) * 8
	if totalBits > int(c_size) {
		totalBits = int(c_size)
	}
	kCombined := make([]byte, totalBits)
	bitIndex := 0

	// Unroll inner loop for efficiency
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
			// Handle final partial byte
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
		vm.Ram = NewRAM(o_size, w_size, z, o_byte, w_byte, s)
		requiredMemory := uint64(uint64(5*Z_Z) + uint64(Z_func(o_size)) + uint64(Z_func(w_size+z*Z_P)) + uint64(Z_func(s)) + uint64(Z_I))
		if requiredMemory > math.MaxUint32 {
			log.Error(vm.logging, "Standard Program Initialization Error")
			// TODO
		}
	}

	for i := 0; i < len(initialRegs); i++ {
		vm.Ram.WriteRegister(i, initialRegs[i])
	}

	if PvmLogging {
		hiResGasRangeStart = 1
		hiResGasRangeEnd = int64(999999999999999)
	}

	vm.VMs = nil
	vm.buildDispatchTable()
	return vm
}
func (vm *VM) buildDispatchTable() {
	// Initialize all opcodes to default handler
	for i := 0; i < 256; i++ {
		vm.dispatchTable[i] = func(opcode byte, operands []byte) {
			log.Warn(vm.logging, "terminated: unknown opcode", "service", string(vm.ServiceMetadata), "opcode", opcode)
			vm.HandleNoArgs(0, []byte{}) //TRAP
		}
	}

	// A.5.1 No arguments
	vm.dispatchTable[TRAP] = vm.handleTRAP
	vm.dispatchTable[FALLTHROUGH] = vm.handleFALLTHROUGH

	// A.5.2 One immediate
	vm.dispatchTable[ECALLI] = vm.handleECALLI

	// A.5.3 One Register and One Extended Width Immediate
	vm.dispatchTable[LOAD_IMM_64] = vm.handleLOAD_IMM_64

	// A.5.4 Two Immediates
	vm.dispatchTable[STORE_IMM_U8] = vm.handleSTORE_IMM_U8
	vm.dispatchTable[STORE_IMM_U16] = vm.handleSTORE_IMM_U16
	vm.dispatchTable[STORE_IMM_U32] = vm.handleSTORE_IMM_U32
	vm.dispatchTable[STORE_IMM_U64] = vm.handleSTORE_IMM_U64

	// A.5.5 One offset
	vm.dispatchTable[JUMP] = vm.handleJUMP

	// A.5.6 One Register and One Immediate
	vm.dispatchTable[JUMP_IND] = vm.handleJUMP_IND
	vm.dispatchTable[LOAD_IMM] = vm.handleLOAD_IMM
	vm.dispatchTable[LOAD_U8] = vm.handleLOAD_U8
	vm.dispatchTable[LOAD_I8] = vm.handleLOAD_I8
	vm.dispatchTable[LOAD_U16] = vm.handleLOAD_U16
	vm.dispatchTable[LOAD_I16] = vm.handleLOAD_I16
	vm.dispatchTable[LOAD_U32] = vm.handleLOAD_U32
	vm.dispatchTable[LOAD_I32] = vm.handleLOAD_I32
	vm.dispatchTable[LOAD_U64] = vm.handleLOAD_U64
	vm.dispatchTable[STORE_U8] = vm.handleSTORE_U8
	vm.dispatchTable[STORE_U16] = vm.handleSTORE_U16
	vm.dispatchTable[STORE_U32] = vm.handleSTORE_U32
	vm.dispatchTable[STORE_U64] = vm.handleSTORE_U64

	// A.5.7 One Register and Two Immediates
	vm.dispatchTable[STORE_IMM_IND_U8] = vm.handleSTORE_IMM_IND_U8
	vm.dispatchTable[STORE_IMM_IND_U16] = vm.handleSTORE_IMM_IND_U16
	vm.dispatchTable[STORE_IMM_IND_U32] = vm.handleSTORE_IMM_IND_U32
	vm.dispatchTable[STORE_IMM_IND_U64] = vm.handleSTORE_IMM_IND_U64

	// A.5.8 One Register, One Immediate and One Offset
	vm.dispatchTable[LOAD_IMM_JUMP] = vm.handleLOAD_IMM_JUMP
	vm.dispatchTable[BRANCH_EQ_IMM] = vm.handleBRANCH_EQ_IMM
	vm.dispatchTable[BRANCH_NE_IMM] = vm.handleBRANCH_NE_IMM
	vm.dispatchTable[BRANCH_LT_U_IMM] = vm.handleBRANCH_LT_U_IMM
	vm.dispatchTable[BRANCH_LE_U_IMM] = vm.handleBRANCH_LE_U_IMM
	vm.dispatchTable[BRANCH_GE_U_IMM] = vm.handleBRANCH_GE_U_IMM
	vm.dispatchTable[BRANCH_GT_U_IMM] = vm.handleBRANCH_GT_U_IMM
	vm.dispatchTable[BRANCH_LT_S_IMM] = vm.handleBRANCH_LT_S_IMM
	vm.dispatchTable[BRANCH_LE_S_IMM] = vm.handleBRANCH_LE_S_IMM
	vm.dispatchTable[BRANCH_GE_S_IMM] = vm.handleBRANCH_GE_S_IMM
	vm.dispatchTable[BRANCH_GT_S_IMM] = vm.handleBRANCH_GT_S_IMM

	// A.5.9 Two Registers
	vm.dispatchTable[MOVE_REG] = vm.handleMOVE_REG
	vm.dispatchTable[SBRK] = vm.handleSBRK
	vm.dispatchTable[COUNT_SET_BITS_64] = vm.handleCOUNT_SET_BITS_64
	vm.dispatchTable[COUNT_SET_BITS_32] = vm.handleCOUNT_SET_BITS_32
	vm.dispatchTable[LEADING_ZERO_BITS_64] = vm.handleLEADING_ZERO_BITS_64
	vm.dispatchTable[LEADING_ZERO_BITS_32] = vm.handleLEADING_ZERO_BITS_32
	vm.dispatchTable[TRAILING_ZERO_BITS_64] = vm.handleTRAILING_ZERO_BITS_64
	vm.dispatchTable[TRAILING_ZERO_BITS_32] = vm.handleTRAILING_ZERO_BITS_32
	vm.dispatchTable[SIGN_EXTEND_8] = vm.handleSIGN_EXTEND_8
	vm.dispatchTable[SIGN_EXTEND_16] = vm.handleSIGN_EXTEND_16
	vm.dispatchTable[ZERO_EXTEND_16] = vm.handleZERO_EXTEND_16
	vm.dispatchTable[REVERSE_BYTES] = vm.handleREVERSE_BYTES

	// A.5.10 Two Registers and One Immediate
	vm.dispatchTable[STORE_IND_U8] = vm.handleSTORE_IND_U8
	vm.dispatchTable[STORE_IND_U16] = vm.handleSTORE_IND_U16
	vm.dispatchTable[STORE_IND_U32] = vm.handleSTORE_IND_U32
	vm.dispatchTable[STORE_IND_U64] = vm.handleSTORE_IND_U64
	vm.dispatchTable[LOAD_IND_U8] = vm.handleLOAD_IND_U8
	vm.dispatchTable[LOAD_IND_I8] = vm.handleLOAD_IND_I8
	vm.dispatchTable[LOAD_IND_U16] = vm.handleLOAD_IND_U16
	vm.dispatchTable[LOAD_IND_I16] = vm.handleLOAD_IND_I16
	vm.dispatchTable[LOAD_IND_U32] = vm.handleLOAD_IND_U32
	vm.dispatchTable[LOAD_IND_I32] = vm.handleLOAD_IND_I32
	vm.dispatchTable[LOAD_IND_U64] = vm.handleLOAD_IND_U64
	vm.dispatchTable[ADD_IMM_32] = vm.handleADD_IMM_32
	vm.dispatchTable[AND_IMM] = vm.handleAND_IMM
	vm.dispatchTable[XOR_IMM] = vm.handleXOR_IMM
	vm.dispatchTable[OR_IMM] = vm.handleOR_IMM
	vm.dispatchTable[MUL_IMM_32] = vm.handleMUL_IMM_32
	vm.dispatchTable[SET_LT_U_IMM] = vm.handleSET_LT_U_IMM
	vm.dispatchTable[SET_LT_S_IMM] = vm.handleSET_LT_S_IMM
	vm.dispatchTable[SHLO_L_IMM_32] = vm.handleSHLO_L_IMM_32
	vm.dispatchTable[SHLO_R_IMM_32] = vm.handleSHLO_R_IMM_32
	vm.dispatchTable[SHAR_R_IMM_32] = vm.handleSHAR_R_IMM_32
	vm.dispatchTable[NEG_ADD_IMM_32] = vm.handleNEG_ADD_IMM_32
	vm.dispatchTable[SET_GT_U_IMM] = vm.handleSET_GT_U_IMM
	vm.dispatchTable[SET_GT_S_IMM] = vm.handleSET_GT_S_IMM
	vm.dispatchTable[SHLO_L_IMM_ALT_32] = vm.handleSHLO_L_IMM_ALT_32
	vm.dispatchTable[SHLO_R_IMM_ALT_32] = vm.handleSHLO_R_IMM_ALT_32
	vm.dispatchTable[SHAR_R_IMM_ALT_32] = vm.handleSHAR_R_IMM_ALT_32
	vm.dispatchTable[CMOV_IZ_IMM] = vm.handleCMOV_IZ_IMM
	vm.dispatchTable[CMOV_NZ_IMM] = vm.handleCMOV_NZ_IMM
	vm.dispatchTable[ADD_IMM_64] = vm.handleADD_IMM_64
	vm.dispatchTable[MUL_IMM_64] = vm.handleMUL_IMM_64
	vm.dispatchTable[SHLO_L_IMM_64] = vm.handleSHLO_L_IMM_64
	vm.dispatchTable[SHLO_R_IMM_64] = vm.handleSHLO_R_IMM_64
	vm.dispatchTable[SHAR_R_IMM_64] = vm.handleSHAR_R_IMM_64
	vm.dispatchTable[NEG_ADD_IMM_64] = vm.handleNEG_ADD_IMM_64
	vm.dispatchTable[SHLO_L_IMM_ALT_64] = vm.handleSHLO_L_IMM_ALT_64
	vm.dispatchTable[SHLO_R_IMM_ALT_64] = vm.handleSHLO_R_IMM_ALT_64
	vm.dispatchTable[SHAR_R_IMM_ALT_64] = vm.handleSHAR_R_IMM_ALT_64
	vm.dispatchTable[ROT_R_64_IMM] = vm.handleROT_R_64_IMM
	vm.dispatchTable[ROT_R_64_IMM_ALT] = vm.handleROT_R_64_IMM_ALT
	vm.dispatchTable[ROT_R_32_IMM] = vm.handleROT_R_32_IMM
	vm.dispatchTable[ROT_R_32_IMM_ALT] = vm.handleROT_R_32_IMM_ALT

	// A.5.11 Two Registers and One Offset
	vm.dispatchTable[BRANCH_EQ] = vm.handleBRANCH_EQ
	vm.dispatchTable[BRANCH_NE] = vm.handleBRANCH_NE
	vm.dispatchTable[BRANCH_LT_U] = vm.handleBRANCH_LT_U
	vm.dispatchTable[BRANCH_LT_S] = vm.handleBRANCH_LT_S
	vm.dispatchTable[BRANCH_GE_U] = vm.handleBRANCH_GE_U
	vm.dispatchTable[BRANCH_GE_S] = vm.handleBRANCH_GE_S

	// A.5.12 Two Registers and Two Immediates
	vm.dispatchTable[LOAD_IMM_JUMP_IND] = vm.handleLOAD_IMM_JUMP_IND

	// A.5.13 Three Registers
	vm.dispatchTable[ADD_32] = vm.handleADD_32
	vm.dispatchTable[SUB_32] = vm.handleSUB_32
	vm.dispatchTable[MUL_32] = vm.handleMUL_32
	vm.dispatchTable[DIV_U_32] = vm.handleDIV_U_32
	vm.dispatchTable[DIV_S_32] = vm.handleDIV_S_32
	vm.dispatchTable[REM_U_32] = vm.handleREM_U_32
	vm.dispatchTable[REM_S_32] = vm.handleREM_S_32
	vm.dispatchTable[SHLO_L_32] = vm.handleSHLO_L_32
	vm.dispatchTable[SHLO_R_32] = vm.handleSHLO_R_32
	vm.dispatchTable[SHAR_R_32] = vm.handleSHAR_R_32
	vm.dispatchTable[ADD_64] = vm.handleADD_64
	vm.dispatchTable[SUB_64] = vm.handleSUB_64
	vm.dispatchTable[MUL_64] = vm.handleMUL_64
	vm.dispatchTable[DIV_U_64] = vm.handleDIV_U_64
	vm.dispatchTable[DIV_S_64] = vm.handleDIV_S_64
	vm.dispatchTable[REM_U_64] = vm.handleREM_U_64
	vm.dispatchTable[REM_S_64] = vm.handleREM_S_64
	vm.dispatchTable[SHLO_L_64] = vm.handleSHLO_L_64
	vm.dispatchTable[SHLO_R_64] = vm.handleSHLO_R_64
	vm.dispatchTable[SHAR_R_64] = vm.handleSHAR_R_64
	vm.dispatchTable[AND] = vm.handleAND
	vm.dispatchTable[XOR] = vm.handleXOR
	vm.dispatchTable[OR] = vm.handleOR
	vm.dispatchTable[MUL_UPPER_S_S] = vm.handleMUL_UPPER_S_S
	vm.dispatchTable[MUL_UPPER_U_U] = vm.handleMUL_UPPER_U_U
	vm.dispatchTable[MUL_UPPER_S_U] = vm.handleMUL_UPPER_S_U
	vm.dispatchTable[SET_LT_U] = vm.handleSET_LT_U
	vm.dispatchTable[SET_LT_S] = vm.handleSET_LT_S
	vm.dispatchTable[CMOV_IZ] = vm.handleCMOV_IZ
	vm.dispatchTable[CMOV_NZ] = vm.handleCMOV_NZ
	vm.dispatchTable[ROT_L_64] = vm.handleROT_L_64
	vm.dispatchTable[ROT_L_32] = vm.handleROT_L_32
	vm.dispatchTable[ROT_R_64] = vm.handleROT_R_64
	vm.dispatchTable[ROT_R_32] = vm.handleROT_R_32
	vm.dispatchTable[AND_INV] = vm.handleAND_INV
	vm.dispatchTable[OR_INV] = vm.handleOR_INV
	vm.dispatchTable[XNOR] = vm.handleXNOR
	vm.dispatchTable[MAX] = vm.handleMAX
	vm.dispatchTable[MAX_U] = vm.handleMAX_U
	vm.dispatchTable[MIN] = vm.handleMIN
	vm.dispatchTable[MIN_U] = vm.handleMIN_U
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
	for !vm.terminated {
		opcode := vm.code[vm.pc]
		// Inlined optimized skip function logic
		var len_operands uint64
		n := uint64(len(vm.bitmask))
		end := vm.pc + 25
		if end > n {
			end = n
		}
		found := false
		for i := vm.pc + 1; i < end; i++ {
			if vm.bitmask[i] == 1 {
				len_operands = i - vm.pc - 1
				found = true
				break
			}
		}
		if !found {
			if end < vm.pc+25 {
				len_operands = end - vm.pc - 1
			} else {
				len_operands = 24
			}
		}
		operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
		vm.Gas -= 1
		if PvmTrace2 {
			fmt.Printf("%s %s\n", prefixTrace, DisassembleSingleInstruction(opcode, operands))
		}

		//		t0 := time.Now()
		vm.dispatchTable[opcode](opcode, operands)
		// if opcode == 130 || opcode == 149 || opcode == 123 || opcode == 100 {
		// 	benchRec.Add(fmt.Sprintf("%d]", opcode), time.Since(t0))
		// }
		if opcode == ECALLI {
			if vm.hostCall && vm.IsChild {
				return nil
			}
			if vm.hostCall {
				vm.Gas -= int64(vm.chargeGas(vm.host_func_id))
				vm.InvokeHostCall(vm.host_func_id)
				vm.hostCall = false
			}
		}

		// avoid this: this is expensive
		if PvmLogging {
			registersJSON, _ := json.Marshal(vm.Ram.ReadRegisters())
			prettyJSON := strings.ReplaceAll(string(registersJSON), ",", ", ")
			fmt.Printf("%d-%s: %s %d %d Gas: %d Registers:%s\n", vm.Service_index, vm.Mode, opcode_str(opcode), stepn-1, vm.pc, vm.Gas, prettyJSON)
		}

		stepn++
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

func (vm *VM) SetIdentifier(id string) {
	vm.Identifier = id
}

func (vm *VM) GetIdentifier() string {
	return fmt.Sprintf("%d_%s_%s_%s", vm.Service_index, vm.Mode, vm.Backend, vm.Identifier)
}

func (vm *VM) getBasicBlockGasCost(pc uint64) (uint64, uint64, int) {
	gasCost := uint64(0)
	i := pc
	// charge gas for all the next steps until hitting a basic block instruction
	step := 0
	hostGasCost := uint64(0)
	for i < uint64(len(vm.code)) {
		opcode := vm.code[pc]
		len_operands := vm.skip(pc)
		if opcode == ECALLI {
			operands := vm.code[pc+1 : pc+1+len_operands]
			lx := uint32(types.DecodeE_l(operands))
			host_fn := int(lx)
			hostGasCost += uint64(vm.chargeGas(host_fn))
		}
		pc += 1 + len_operands
		step++
		gasCost += 1
		if IsBasicBlockInstruction(opcode) {
			// step is the number of instructions executed in this basic block
			return gasCost, hostGasCost, step
		}
	}
	return gasCost, hostGasCost, step
}

var childHostCall = errors.New("host call not allowed in child VM")

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
		if vm.bitmask[i] == 1 {
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
	shift := uint(64 - 8*n)
	return uint64(int64(x<<shift) >> shift)
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

func (vm *VM) HandleNoArgs(opcode byte, operands []byte) {
	switch opcode {
	case TRAP:
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		//log.Warn(vm.logging, "TRAP encountered", "service", string(vm.ServiceMetadata), "mode", vm.Mode, "pc", vm.pc)
		//fmt.Printf("TRAP encountered at pc %d in mode %s\n", vm.pc, vm.Mode)
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
		vm.Fault_address = uint32(vm.Ram.WriteRAMBytes64(addr, vy))
		dumpStoreGeneric("STORE_IMM_U64", uint64(addr), "imm", vy, 64)
	}
}

func (vm *VM) HandleOneOffset(opcode byte, operands []byte) {
	vx := extractOneOffset(operands)
	dumpJumpOffset("JUMP", vx, vm.pc)
	vm.branch(uint64(int64(vm.pc)+vx), true)
}

// Individual opcode handlers

func (vm *VM) handleTRAP(opcode byte, operands []byte) {
	vm.ResultCode = types.WORKDIGEST_PANIC
	vm.MachineState = PANIC
	vm.terminated = true
	vm.pc += 1
}

func (vm *VM) handleFALLTHROUGH(opcode byte, operands []byte) {
	vm.pc += 1
}

func (vm *VM) handleECALLI(opcode byte, operands []byte) {
	lx := uint32(types.DecodeE_l(operands))
	vm.hostCall = true
	vm.host_func_id = int(lx)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_IMM_64(opcode byte, operands []byte) {
	registerIndexA := min(12, int(operands[0])%16)
	lx := 8
	vx := types.DecodeE_l(operands[1 : 1+lx])
	dumpLoadImm("LOAD_IMM_64", registerIndexA, uint64(vx), vx, 64, false)
	vm.Ram.WriteRegister(registerIndexA, uint64(vx))
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_U8(opcode byte, operands []byte) {
	vx, vy := extractTwoImm(operands)
	addr := uint32(vx)
	vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, []byte{uint8(vy)}))
	dumpStoreGeneric("STORE_IMM_U8", uint64(addr), "imm", vy, 8)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_U16(opcode byte, operands []byte) {
	vx, vy := extractTwoImm(operands)
	addr := uint32(vx)
	vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<16), 2)))
	dumpStoreGeneric("STORE_IMM_U16", uint64(addr), "imm", vy%(1<<16), 16)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_U32(opcode byte, operands []byte) {
	vx, vy := extractTwoImm(operands)
	addr := uint32(vx)
	vm.Fault_address = uint32(vm.Ram.WriteRAMBytes(addr, types.E_l(vy%(1<<32), 4)))
	dumpStoreGeneric("STORE_IMM_U32", uint64(addr), "imm", vy%(1<<32), 32)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_IMM_U64(opcode byte, operands []byte) {
	vx, vy := extractTwoImm(operands)
	addr := uint32(vx)
	vm.Fault_address = uint32(vm.Ram.WriteRAMBytes64(addr, vy))
	dumpStoreGeneric("STORE_IMM_U64", uint64(addr), "imm", vy, 64)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleJUMP(opcode byte, operands []byte) {
	vx := extractOneOffset(operands)
	dumpJumpOffset("JUMP", vx, vm.pc)
	vm.branch(uint64(int64(vm.pc)+vx), true)
}

func (vm *VM) handleJUMP_IND(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	dumpBranchImm("JUMP_IND", registerIndexA, valueA, vx, valueA+vx, false, true)
	vm.djump((valueA + vx) % (1 << 32))
}

func (vm *VM) handleLOAD_IMM(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	vm.Ram.WriteRegister(registerIndexA, vx)
	dumpLoadImm("LOAD_IMM", registerIndexA, uint64(vx), vx, 64, false)
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_U8(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 1)
	if errCode == OK {
		vm.Ram.WriteRegister(registerIndexA, uint64(value[0]))
		dumpLoadGeneric("LOAD_U8", registerIndexA, vx, uint64(value[0]), 8, false)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_I8(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 1)
	if errCode == OK {
		res := x_encode(uint64(value[0]), 1)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_I8", registerIndexA, uint64(addr), res, 8, true)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_U16(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
	if errCode == OK {
		res := types.DecodeE_l(value)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_U16", registerIndexA, uint64(addr), res, 16, false)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_I16(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 2)
	if errCode == OK {
		res := x_encode(types.DecodeE_l(value), 2)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_I16", registerIndexA, uint64(addr), res, 16, true)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_U32(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
	if errCode == OK {
		res := types.DecodeE_l(value)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_U32", registerIndexA, uint64(addr), res, 32, false)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_I32(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 4)
	if errCode == OK {
		res := x_encode(types.DecodeE_l(value), 4)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_I32", registerIndexA, uint64(addr), res, 32, true)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleLOAD_U64(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	addr := uint32(vx)
	value, errCode := vm.Ram.ReadRAMBytes(addr, 8)
	if errCode == OK {
		res := types.DecodeE_l(value)
		vm.Ram.WriteRegister(registerIndexA, res)
		dumpLoadGeneric("LOAD_U64", registerIndexA, uint64(addr), res, 64, false)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_U8(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, []byte{uint8(valueA)})
	if errCode == OK {
		dumpStoreGeneric("STORE_U8", uint64(addr), reg(registerIndexA), valueA, 8)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_U16(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<16), 2))
	if errCode == OK {
		dumpStoreGeneric("STORE_U16", uint64(addr), reg(registerIndexA), valueA%(1<<16), 16)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_U32(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(vx)
	errCode := vm.Ram.WriteRAMBytes(addr, types.E_l(valueA%(1<<32), 4))
	if errCode == OK {
		dumpStoreGeneric("STORE_U32", uint64(addr), reg(registerIndexA), valueA%(1<<32), 32)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
}

func (vm *VM) handleSTORE_U64(opcode byte, operands []byte) {
	registerIndexA, vx := extractOneRegOneImm(operands)
	valueA, _ := vm.Ram.ReadRegister(registerIndexA)
	addr := uint32(vx)
	errCode := vm.Ram.WriteRAMBytes64(addr, valueA)
	if errCode == OK {
		dumpStoreGeneric("STORE_U64", uint64(addr), reg(registerIndexA), valueA, 64)
	} else {
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		vm.Fault_address = uint32(errCode)
		return
	}
	vm.pc += 1 + uint64(len(operands))
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

var hiResGasRangeStart = int64(0)
var hiResGasRangeEnd = int64(math.MaxInt64)
var BBSampleRate = 20_000_000
var RecordLogSampleRate = 1

type VMLogs []VMLog

func (vm *VM) GetMemory() (map[int][]byte, map[int]int) {
	panic(2)
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
