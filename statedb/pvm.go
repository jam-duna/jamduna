package statedb

import (
	"fmt"
	"math"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/types"
)

const (
	BackendInterpreter = "interpreter" // C interpreter
	BackendCompiler    = "compiler"    // X86 recompiler
)

const (
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
)

type ExecutionVM interface {
	ReadRegister(int) uint64
	ReadRegisters() [13]uint64
	WriteRegister(int, uint64)
	ReadRAMBytes(uint32, uint32) ([]byte, uint64)
	WriteRAMBytes(uint32, []byte) uint64
	SetHeapPointer(uint32)

	GetGas() int64

	Init(argument_data_a []byte) (err error)
	Execute(vm *VM, EntryPoint uint32) error
	Destroy()
}

type VM struct {
	ExecutionVM
	VMs map[uint32]*ExecutionVM

	Backend        string
	IsChild        bool
	ResultCode     uint8
	HostResultCode uint64
	MachineState   uint8
	Fault_address  uint32
	terminated     bool

	Mode    string
	hostenv types.HostEnv
	logging string

	// service metadata
	ServiceAccount  *types.ServiceAccount
	Service_index   uint32
	ServiceMetadata []byte

	// Refine Inputs and Outputs
	WorkItemIndex             uint32
	WorkPackage               types.WorkPackage
	Extrinsics                types.ExtrinsicsBlobs
	Authorization             []byte
	Imports                   [][][]byte
	AccumulateOperandElements []types.AccumulateOperandElements
	Transfers                 []types.DeferredTransfer
	N                         common.Hash
	Delta                     map[uint32]*types.ServiceAccount

	RefineM_map        map[uint32]*RefineM
	Exports            [][]byte
	ExportSegmentIndex uint32

	// Accumulate (used in host functions)
	X        *types.XContext
	Y        types.XContext
	Timeslot uint32
}

type Program program.Program

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
)

const (
	HALT  = 0 // regular halt ∎
	PANIC = 1 // panic ☇
	FAULT = 2 // page-fault F
	HOST  = 3 // host-call̵ h
	OOG   = 4 // out-of-gas ∞
)

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
	return (*Program)(decodeCorePart(pure[offset:])), uint32(o_size), uint32(w_size), uint32(z_val), uint32(s_val), o_byte, w_byte
}

func DecodeProgram_pure_pvm_blob(p []byte) *Program {
	return (*Program)(decodeCorePart(p))
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

	fmt.Printf("EncodeProgram: j_size=%d, z=%d, c_size=%d, code_len=%d, bitmask_len=%d, k_bytes_len=%d, total_blob_len=%d BLOB: %v\n",
		j_size, z, c_size, len(code), len(bitmask), len(k_bytes), len(blob), blob)
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

var decodeCorePart = program.DecodeCorePart

func CeilingDivide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

func P_func(x uint32) uint32 {
	return Z_P * CeilingDivide(x, Z_P)
}

func Z_func(x uint32) uint32 {
	return Z_Z * CeilingDivide(x, Z_Z)
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, pvmBackend string, initialGas uint64) *VM {

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
		w_size = uint32(initialHeap)
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = make([]byte, w_size)
	}

	vm := &VM{
		hostenv:         hostENV, //check if we need this
		Exports:         make([][]byte, 0),
		Service_index:   service_index,
		ServiceMetadata: Metadata,
		Backend:         pvmBackend,
	}

	requiredMemory := uint64(uint64(5*Z_Z) + uint64(Z_func(o_size)) + uint64(Z_func(w_size+z*Z_P)) + uint64(Z_func(s)) + uint64(Z_I))
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		// TODO
	}

	// Create C VM immediately with initial registers
	if len(initialRegs) == 0 {
		initialRegs = make([]uint64, 13)
	} else if len(initialRegs) < 13 {
		// Extend to 13 registers if needed
		extended := make([]uint64, 13)
		copy(extended, initialRegs)
		initialRegs = extended
	}
	if len(p.Code) == 0 {
		panic("No code provided to NewVM")
	}
	if vm.Backend == BackendInterpreter {
		// Create VM using FFI API
		machine := NewInterpreter(service_index, p, initialRegs, initialPC, initialGas, vm)
		vm.ExecutionVM = machine

		// o - read-only
		ro_data_address := uint32(Z_Z)
		ro_data_address_end := ro_data_address + P_func(o_size)

		machine.SetHostCallBack()
		machine.SetLogging()
		// s - stack
		p_s := P_func(s)
		stack_address := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I) - uint64(p_s))
		stack_address_end := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I))

		// a - argument outputs
		output_address := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
		output_end := uint32(0xFFFFFFFF)

		// w - read-write
		rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
		rw_data_address_end := rw_data_address + P_func(w_size)
		current_heap_pointer := rw_data_address_end

		z_o := Z_func(o_size)
		z_w := Z_func(w_size + z*Z_P)
		z_s := Z_func(s)
		requiredMemory = uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
		if requiredMemory > math.MaxUint32 {
			log.Error(vm.logging, "Standard Program Initialization Error")
		}
		machine.SetHeapPointer(current_heap_pointer)
		machine.SetMemoryBounds(rw_data_address, rw_data_address_end, ro_data_address, ro_data_address_end, output_address, output_end, stack_address, stack_address_end)
		if len(o_byte) > 0 {
			result := vm.WriteRAMBytes(Z_Z, o_byte)
			if result != OK {
				fmt.Printf("Warning: Failed to initialize o_byte data: error %d\n", result)
				panic("Failed to initialize o_byte data")
			}
		}
		if len(w_byte) > 0 {
			result := vm.WriteRAMBytes(rw_data_address, w_byte)
			if result != OK {
				fmt.Printf("Warning: Failed to initialize w_byte data: error %d\n", result)
				panic("Failed to initialize w_byte data")
			}
		}
	} else if vm.Backend == BackendCompiler {
		rvm := NewRecompilerVM(service_index, code, initialRegs, initialPC, initialHeap, hostENV, jam_ready_blob, Metadata, initialGas, pvmBackend)
		if rvm == nil {
			return nil
		}
		vm.ExecutionVM = rvm
	}

	vm.VMs = nil
	return vm
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, initialHeap uint64, hostENV types.HostEnv, pvmBackend string, initialGas uint64) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)
	return NewVM(serviceIndex, c, []uint64{}, i, initialHeap, hostENV, true, []byte(metadata), pvmBackend, initialGas)
}

var RecordTime = true

func (vm *VM) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VM) GetServiceIndex() uint32 {
	return vm.Service_index
}

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = ModeRefine

	workitem := workPackage.WorkItems[workitemIndex]

	a := types.E(uint64(workitemIndex))
	serviceBytes := types.E(uint64(workitem.Service))
	a = append(a, serviceBytes...)
	//fmt.Printf("ExecuteRefine  s %d bytes - %x\n", len(serviceBytes), serviceBytes)
	encoded_workitem_payload, _ := types.Encode(workitem.Payload)
	a = append(a, encoded_workitem_payload...) // variable number of bytes
	//fmt.Printf("ExecuteRefine  payload %d bytes - %x\n", len(encoded_workitem_payload), encoded_workitem_payload)
	a = append(a, workPackage.Hash().Bytes()...) // 32

	//fmt.Printf("ExecuteRefine TOTAL len(a)=%d %x\n", len(a), a)
	vm.WorkItemIndex = workitemIndex
	vm.WorkPackage = workPackage

	vm.N = n
	vm.Authorization = authorization.Ok
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments
	vm.executeWithBackend(a, types.EntryPointRefine)
	r, res = vm.getArgumentOutputs()

	log.Trace(vm.logging, string(vm.ServiceMetadata), "Result", r.String(), "fault_address", vm.Fault_address, "resultCode", vm.ResultCode)
	exportedSegments = vm.Exports

	return r, res, exportedSegments
}

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, elements []types.AccumulateOperandElements, X *types.XContext, n common.Hash) (r types.Result, res uint64, xs *types.ServiceAccount) {
	vm.Mode = ModeAccumulate
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()
	input_bytes := make([]byte, 0)
	t_bytes := types.E(uint64(t))
	s_bytes := types.E(uint64(s))
	o_bytes := types.E(uint64(len(elements))) // TODO: check
	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, o_bytes...)
	vm.AccumulateOperandElements = elements
	vm.N = n

	x_s, found := X.U.ServiceAccounts[s]
	if !found {
		log.Error(vm.logging, "ExecuteAccumulate - ServiceAccount not found in X.U.ServiceAccounts", "s", s, "X.U.ServiceAccounts", X.U.ServiceAccounts)
		return
	}
	x_s.Mutable = true
	vm.X.U.ServiceAccounts[s] = x_s
	vm.ServiceAccount = x_s

	vm.executeWithBackend(input_bytes, types.EntryPointAccumulate)
	r, res = vm.getArgumentOutputs()

	return r, res, x_s
}

func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	vm.Mode = ModeOnTransfer
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account
	vm.executeWithBackend(arguments, types.EntryPointOnTransfer)
	// return vm.getArgumentOutputs()
	r.Err = vm.ResultCode
	r.Ok = []byte{}
	return r, 0
}

func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16) (r types.Result) {
	vm.Mode = ModeIsAuthorized
	// NOT 0.7.0 COMPLIANT
	a, _ := types.Encode(uint8(c))

	// fmt.Printf("ExecuteAuthorization - c=%d len(p_bytes)=%d len(c_bytes)=%d len(a)=%d a=%x WP=%s\n", c, len(p_bytes), len(c_bytes), len(a), a, p.String())
	vm.executeWithBackend(a, types.EntryPointAuthorization)
	r, _ = vm.getArgumentOutputs()
	return r
}

func (vm *VM) executeWithBackend(argumentData []byte, entryPoint uint32) {
	// fmt.Printf("Standard Program Initialization: %s=%x %s=%x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)))
	vm.Init(argumentData)
	vm.IsChild = false
	err := vm.ExecutionVM.Execute(vm, entryPoint)
	if err != nil {
		log.Error(vm.logging, "C VM execution failed", "error", err)
	}
}

func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ResultCode == types.WORKDIGEST_OOG {
		r.Err = types.WORKDIGEST_OOG
		log.Error(vm.logging, "getArgumentOutputs - OOG", "service", string(vm.ServiceMetadata))
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	if vm.ResultCode != types.WORKDIGEST_OK {
		r.Err = vm.ResultCode
		log.Trace(vm.logging, "getArgumentOutputs - Error", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
		return r, 0
	}
	o := vm.ReadRegister(7)
	l := vm.ReadRegister(8)
	output, res := vm.ReadRAMBytes(uint32(o), uint32(l))
	//log.Info(vm.logging, "getArgumentOutputs - OK", "output", fmt.Sprintf("%x", output), "l", l)
	if vm.ResultCode == types.WORKDIGEST_OK && res == 0 {
		r.Ok = output
		return r, res
	}
	if vm.ResultCode == types.WORKDIGEST_OK && res != 0 {
		r.Ok = []byte{}
		return r, res
	}
	r.Err = types.WORKDIGEST_PANIC
	//log.Error(vm.logging, "getArgumentOutputs - PANIC", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
	return r, 0
}
