package statedb

import (
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// #cgo CFLAGS: -I${SRCDIR}/../pvm/include
// #cgo LDFLAGS: ${SRCDIR}/../pvm/lib/libpvm.a
/*
#include "pvm.h"
extern pvm_host_result_t goInvokeHostFunction(pvm_vm_t* vm, int hostFuncID);
*/
import "C"

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
	PvmLogging = true

	// VM mapping for callbacks
	vmMap = make(map[*C.pvm_vm_t]*VM)
)

type VM struct {
	Backend        string
	IsChild        bool
	ResultCode     uint8
	HostResultCode uint64
	MachineState   uint8
	Fault_address  uint32
	terminated     bool
	hostCall       bool // ̵h in GP
	host_func_id   int  // h in GP

	hostenv types.HostEnv

	cVM *C.pvm_vm_t // FFI VM handle
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

	// service metadata
	ServiceMetadata []byte
	Mode            string
	Identifier      string
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

// NewVM initializes a new VM with a given program
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
		CoreIndex:       2048,
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
	// Create VM using FFI API with integrated setup
	var bitmaskPtr *C.uint8_t
	var jumpTablePtr *C.uint32_t

	if len(p.K) > 0 {
		bitmaskPtr = (*C.uint8_t)(unsafe.Pointer(&p.K[0]))
	}
	if len(p.J) > 0 {
		jumpTablePtr = (*C.uint32_t)(unsafe.Pointer(&p.J[0]))
	}

	vm.cVM = C.pvm_create(
		C.uint32_t(vm.Service_index),
		(*C.uint8_t)(unsafe.Pointer(&p.Code[0])),
		C.size_t(len(p.Code)),
		(*C.uint64_t)(unsafe.Pointer(&initialRegs[0])),
		bitmaskPtr,
		C.size_t(len(p.K)),
		jumpTablePtr,
		C.size_t(len(p.J)),
		C.uint64_t(initialGas))

	// o - read-only
	ro_data_address := uint32(Z_Z)
	ro_data_address_end := ro_data_address + P_func(o_size)

	// TODO: correct error handling
	if vm.cVM == nil {
		return nil
	}

	// Set host function callback
	C.pvm_set_host_callback(vm.cVM, C.pvm_host_callback_t(C.goInvokeHostFunction))

	// Set logging and tracing based on PvmLogging
	if PvmLogging {
		C.pvm_set_logging(vm.cVM, C.int(1))
	} else {
		C.pvm_set_logging(vm.cVM, C.int(1))
	}
	C.pvm_set_tracing(vm.cVM, C.int(0))

	// s - stack
	p_s := P_func(s)
	stack_address := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I) - uint64(p_s))
	stack_address_end := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I))

	// a - argument outputs
	output_address := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	output_end := uint32(0xFFFFFFFF)

	vmMap[vm.cVM] = vm

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

	C.pvm_set_heap_pointer(vm.cVM, C.uint32_t(current_heap_pointer))
	vm.SetMemoryBounds(rw_data_address, rw_data_address_end, ro_data_address, ro_data_address_end, output_address, output_end, stack_address, stack_address_end)
	if len(o_byte) > 0 {
		result := vm.WriteRAMBytes(Z_Z, o_byte)
		if result != OK {
			fmt.Printf("Warning: Failed to initialize o_byte data: error %d\n", result)
			panic(3)
		}
	}
	if len(w_byte) > 0 {
		result := vm.WriteRAMBytes(rw_data_address, w_byte)
		if result != OK {
			fmt.Printf("Warning: Failed to initialize w_byte data: error %d\n", result)
		}
	}

	vm.VMs = nil
	return vm
}
func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, initialHeap uint64, hostENV types.HostEnv, pvmBackend string, initialGas uint64) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)
	return NewVM(serviceIndex, c, []uint64{}, i, initialHeap, hostENV, true, []byte(metadata), pvmBackend, initialGas)
}

//export goInvokeHostFunction
func goInvokeHostFunction(cvm *C.pvm_vm_t, hostFuncID C.int) C.pvm_host_result_t {
	// Look up the Go VM from our mapping
	vm, ok := vmMap[cvm]
	if !ok {
		// VM not found, this shouldn't happen - log error and return
		fmt.Printf("Error: goInvokeHostFunction called with unknown C VM pointer\n")
		return C.PVM_HOST_ERROR
	}

	// Set host call state
	vm.hostCall = true
	vm.host_func_id = int(hostFuncID)

	// Invoke Go host function with panic recovery
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Host function %d panicked: %v\n", hostFuncID, r)
			// Set VM to panic state
			vm.MachineState = PANIC
			vm.ResultCode = PANIC
			vm.terminated = true
		}
		vm.hostCall = false
		vm.host_func_id = 0
	}()

	// Call the host function
	vm.InvokeHostCall(int(hostFuncID))

	// Check if VM was terminated by host function
	if vm.terminated {
		return C.PVM_HOST_TERMINATE
	}

	// Check if host function caused panic
	if vm.MachineState == PANIC {
		return C.PVM_HOST_ERROR
	}

	return C.PVM_HOST_CONTINUE
}

// Execute runs VM execution using the C interpreter
func (vm *VM) Execute() error {
	// ***** Execute ****
	result := C.pvm_execute(vm.cVM, C.uint32_t(vm.EntryPoint), 0)

	// Get post-execution state
	vm.ResultCode = uint8(C.pvm_get_result_code(vm.cVM))
	vm.MachineState = uint8(C.pvm_get_machine_state(vm.cVM))
	vm.terminated = C.pvm_is_terminated(vm.cVM) != 0

	//fmt.Printf("===> Post-execution  Gas=%d, ResultCode=%d\n", vm.GetGas(), vm.ResultCode)

	switch result {
	case C.PVM_RESULT_OK:
		return nil
	case C.PVM_RESULT_PANIC:
		return nil
	case C.PVM_RESULT_HOST_CALL:
		return nil
	case C.PVM_RESULT_OOG:
		return nil
	default:
		return fmt.Errorf("VM execution failed with result code %d", result)
	}
}

// ReadRegister reads a register value from the C VM
func (vm *VM) ReadRegister(idx int) uint64 {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return 0
	}
	return uint64(C.pvm_get_register(vm.cVM, C.int(idx)))
}

// WriteRegister writes a register value to the C VM
func (vm *VM) WriteRegister(idx int, value uint64) {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return
	}
	C.pvm_set_register(vm.cVM, C.int(idx), C.uint64_t(value))
}

// ReadRegisters returns all register values from the C VM as an array
func (vm *VM) ReadRegisters() [13]uint64 {
	var registers [13]uint64
	if vm.cVM == nil {
		return registers
	}

	for i := 0; i < 13; i++ {
		registers[i] = uint64(C.pvm_get_register(vm.cVM, C.int(i)))
	}
	return registers
}

func (ram *VM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	if length == 0 {
		return []byte{}, OK
	}

	buffer := make([]byte, length)
	var errCode C.int
	result := C.pvm_read_ram_bytes(ram.cVM, C.uint32_t(address), (*C.uint8_t)(&buffer[0]), C.uint32_t(length), &errCode)

	if errCode != 0 {
		return nil, uint64(errCode)
	}

	return buffer, uint64(result)
}

func (ram *VM) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}

	// VM must be created before writing
	if ram.cVM == nil {
		return OOB // Return error if VM not ready
	}

	return uint64(C.pvm_write_ram_bytes(ram.cVM, C.uint32_t(address), (*C.uint8_t)(&data[0]), C.uint32_t(len(data))))
}

func (vm *VM) GetGas() int64 {
	if vm.cVM != nil {
		return int64(C.pvm_get_gas(vm.cVM))
	}
	return 0
}

func (vm *VM) SetMemoryBounds(rwAddr, rwEnd, roAddr, roEnd, outputAddr, outputEnd, stackAddr, stackEnd uint32) {
	C.pvm_set_memory_bounds(vm.cVM,
		C.uint32_t(rwAddr), C.uint32_t(rwEnd),
		C.uint32_t(roAddr), C.uint32_t(roEnd),
		C.uint32_t(outputAddr), C.uint32_t(outputEnd),
		C.uint32_t(stackAddr), C.uint32_t(stackEnd))
}

// Destroy cleans up the C VM resources
func (vm *VM) Destroy() {
	if vm.cVM != nil {
		delete(vmMap, vm.cVM)
		C.pvm_destroy(vm.cVM)
		vm.cVM = nil
	}
}

var RecordTime = true

func (vm *VM) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VM) GetServiceIndex() uint32 {
	return vm.Service_index
}

func (vm *VM) SetCore(coreIndex uint16) {
	vm.CoreIndex = coreIndex
}

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = ModeRefine

	workitem := workPackage.WorkItems[workitemIndex]

	// ADD IN 0.6.6
	a := types.E(uint64(workitemIndex))
	//fmt.Printf("ExecuteRefine  workitemIndex %d bytes - %x\n", len(a), a)
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
	switch vm.Backend {
	case BackendInterpreter:
		if len(argumentData) == 0 {
			argumentData = []byte{0}
		}

		// argument
		argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
		vm.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
		vm.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
		vm.WriteRegister(7, uint64(argAddr))
		vm.WriteRegister(8, uint64(uint32(len(argumentData))))
		vm.WriteRAMBytes(argAddr, argumentData)

		// fmt.Printf("Standard Program Initialization: %s=%x %s=%x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)))

		vm.EntryPoint = entryPoint
		vm.IsChild = false
		err := vm.Execute()
		if err != nil {
			log.Error(vm.logging, "C VM execution failed", "error", err)
		}

	case BackendCompiler:
		// rvm, err := NewCompilerVM(vm)
		// if err != nil {
		// 	log.Error(vm.logging, "CompilerVM creation failed", "error", err)
		// 	return
		// }
		// vm.initializationTime = common.Elapsed(startTime)
		// startTime = time.Now()
		// if err = rvm.Standard_Program_Initialization(argumentData); err != nil {
		// 	log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
		// 	return
		// }
		// vm.standardInitTime = common.Elapsed(startTime)
		// rvm.Execute(entryPoint)
	default:
		log.Crit(vm.logging, "Unknown VM mode", "mode", vm.Backend)
		panic(0)
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
