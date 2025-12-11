package statedb

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
)

// #cgo CFLAGS: -I${SRCDIR}/../pvm/include
// #cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lpvm.linux_amd64
// #cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lpvm.linux_arm64
// #cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lpvm.mac_amd64
// #cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lpvm.mac_arm64
// #cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lpvm.windows_amd64
/*
#include "pvm.h"
extern pvm_host_result_t goInvokeHostFunction(pvm_vm_t* vm, int hostFuncID);
*/
import "C"

var (
	vmMap   = make(map[*C.pvm_vm_t]*VM)
	vmMapMu sync.RWMutex
)

type Interpreter struct {
	cVM  *C.pvm_vm_t // FFI VM handle
	code []byte      // Program code
}

func NewInterpreter(Service_index uint32, p *Program, initialRegs []uint64, initialPC uint64, initialGas uint64, vm *VM) *Interpreter {
	// Create VM using FFI API with integrated setup
	var bitmaskPtr *C.uint8_t
	var jumpTablePtr *C.uint32_t
	if len(p.K) > 0 {
		bitmaskPtr = (*C.uint8_t)(unsafe.Pointer(&p.K[0]))
	}

	if len(p.J) > 0 {
		jumpTablePtr = (*C.uint32_t)(unsafe.Pointer(&p.J[0]))
	}

	cVM := C.pvm_create(
		C.uint32_t(vm.Service_index),
		(*C.uint8_t)(unsafe.Pointer(&p.Code[0])),
		C.size_t(len(p.Code)),
		(*C.uint64_t)(unsafe.Pointer(&initialRegs[0])),
		bitmaskPtr,
		C.size_t(len(p.K)),
		jumpTablePtr,
		C.size_t(len(p.J)),
		C.uint64_t(initialGas))
	// TODO: correct error handling
	if cVM == nil {
		return nil
	}

	vmMapMu.Lock()
	vmMap[cVM] = vm
	vmMapMu.Unlock()
	return &Interpreter{
		cVM:  cVM,
		code: p.Code,
	}
}

func (vm *Interpreter) GetFaultAddress() uint64 {
	return 0 //TODO
}

func (vm *Interpreter) SetPagesAccessRange(startPage, pageCount int, access int) error {
	return nil //TODO
}

func ExecuteAsChild(entryPoint uint32) error {
	return nil
}

func (vm *Interpreter) GetPC() uint64 {
	// if vm.cVM == nil {
	// 	return 0
	// }
	// return uint64(C.pvm_get_pc(vm.cVM))
	return 0 //TODO
}

func (vm *Interpreter) SetMemoryBounds(rwAddr, rwEnd, roAddr, roEnd, outputAddr, outputEnd, stackAddr, stackEnd uint32) {
	C.pvm_set_memory_bounds(vm.cVM,
		C.uint32_t(rwAddr), C.uint32_t(rwEnd),
		C.uint32_t(roAddr), C.uint32_t(roEnd),
		C.uint32_t(outputAddr), C.uint32_t(outputEnd),
		C.uint32_t(stackAddr), C.uint32_t(stackEnd))
}

// Destroy cleans up the C VM resources
func (vm *Interpreter) Destroy() {
	if vm.cVM != nil {
		vmMapMu.Lock()
		delete(vmMap, vm.cVM)
		vmMapMu.Unlock()
		C.pvm_destroy(vm.cVM)
		vm.cVM = nil
	}
}

func (ram *Interpreter) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}

	// VM must be created before writing
	if ram.cVM == nil {
		return OOB // Return error if VM not ready
	}

	return uint64(C.pvm_write_ram_bytes(ram.cVM, C.uint32_t(address), (*C.uint8_t)(&data[0]), C.uint32_t(len(data))))
}

func (vm *Interpreter) SetPage(uint32, uint32, uint8) {

}

func (vm *Interpreter) SetHeapPointer(pointer uint32) {
	C.pvm_set_heap_pointer(vm.cVM, C.uint32_t(pointer))
}

func (vm *Interpreter) SetHostResultCode(c uint64) {
	C.pvm_set_result_code(vm.cVM, C.uint64_t(c))
}

func (vm *Interpreter) Panic(errCode uint64) {
	C.pvm_panic(vm.cVM, C.uint64_t(errCode))
}
func (vm *Interpreter) GetGas() uint64 {
	// not sure why the oog state is corrected be set but can't get the gas correctly
	if vm.cVM == nil {
		return 0
	}
	// If the machine is out-of-gas, normalize to 0 to match expected semantics
	// even if the native side didn't explicitly zero the counter.
	ms := C.pvm_get_machine_state(vm.cVM)
	if uint8(ms) == OOG {
		return 0
	}
	return uint64(C.pvm_get_gas(vm.cVM))
}

// ReadRegister reads a register value from the C VM
func (vm *Interpreter) ReadRegister(idx int) uint64 {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return 0
	}
	return uint64(C.pvm_get_register(vm.cVM, C.int(idx)))
}

// WriteRegister writes a register value to the C VM
func (vm *Interpreter) WriteRegister(idx int, value uint64) {
	if vm.cVM == nil || idx < 0 || idx >= 13 {
		return
	}
	C.pvm_set_register(vm.cVM, C.int(idx), C.uint64_t(value))
}

// ReadRegisters returns all register values from the C VM as an array
func (vm *Interpreter) ReadRegisters() [13]uint64 {
	var registers [13]uint64
	if vm.cVM == nil {
		return registers
	}

	for i := 0; i < 13; i++ {
		registers[i] = uint64(C.pvm_get_register(vm.cVM, C.int(i)))
	}
	return registers
}

func (ram *Interpreter) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	if length == 0 {
		return []byte{}, OK
	}
	buffer := make([]byte, length)
	var errCode C.int
	result := C.pvm_read_ram_bytes(ram.cVM, C.uint32_t(address), (*C.uint8_t)(&buffer[0]), C.uint32_t(length), &errCode)
	if errCode != 0 {
		return nil, uint64(errCode)
	}
	//fmt.Printf("ReadRAMBytes: address=0x%X, length=%d buffer: %x\n", address, length, buffer)
	return buffer, uint64(result)
}

func (vm *Interpreter) SetHostCallBack() {
	// Set host function callback
	C.pvm_set_host_callback(vm.cVM, C.pvm_host_callback_t(C.goInvokeHostFunction))
}
func (vm *Interpreter) SetLogging() {
	// Set logging and tracing based on PvmLogging
	if PvmLogging {
		C.pvm_set_logging(vm.cVM, C.int(1))
	} else {
		C.pvm_set_logging(vm.cVM, C.int(0))
	}
	C.pvm_set_tracing(vm.cVM, C.int(0))
}

// Execute runs VM execution using the C interpreter
func (vm *Interpreter) Execute(pvm *VM, EntryPoint uint32) error {
	// ***** Execute ****
	result := C.pvm_execute(vm.cVM, C.uint32_t(EntryPoint), 0)

	//fmt.Printf("===> Post-execution  Gas=%d, ResultCode=%d\n", vm.GetGas(), vm.ResultCode)

	// Get post-execution state
	pvm.ResultCode = vm.GetResultCode()
	pvm.MachineState = vm.GetMachineState()
	pvm.terminated = vm.IsTerminated()
	switch pvm.ResultCode {
	case C.PVM_RESULT_OK:
		return nil
	case C.PVM_RESULT_PANIC:
		// get the pc
		// pc := C.pvm_get_pc(vm.cVM)
		// opcode := vm.code[pc]
		// log.Warn(log.GeneralAuthoring, fmt.Sprintf("pvm panic at pc 0x%x, opcode = %s", pc, recompiler.GetOpcodeStr(opcode)))
		return nil
	case C.PVM_RESULT_HOST_CALL:
		// pc := C.pvm_get_pc(vm.cVM)
		// opcode := vm.code[pc]
		// log.Warn(log.GeneralAuthoring, fmt.Sprintf("pvm host call at pc 0x%x, opcode = %s", pc, recompiler.GetOpcodeStr(opcode)))

		return nil
	case C.PVM_RESULT_OOG:
		// pc := C.pvm_get_pc(vm.cVM)
		// opcode := vm.code[pc]
		// log.Warn(log.GeneralAuthoring, fmt.Sprintf("pvm out of gas at pc 0x%x, opcode = %s", pc, recompiler.GetOpcodeStr(opcode)))
		return nil
	default:
		return fmt.Errorf("VM execution failed with result code %d", result)
	}
}
func (vm *Interpreter) IsTerminated() bool {
	return C.pvm_is_terminated(vm.cVM) != 0
}
func (vm *Interpreter) GetResultCode() uint8 {
	return uint8(C.pvm_get_result_code(vm.cVM))
}
func (vm *Interpreter) GetMachineState() uint8 {
	// Get post-execution state
	return uint8(C.pvm_get_machine_state(vm.cVM))
}

//export goInvokeHostFunction
func goInvokeHostFunction(cvm *C.pvm_vm_t, hostFuncID C.int) C.pvm_host_result_t {
	// Look up the Go VM from our mapping
	vmMapMu.RLock()
	vm, ok := vmMap[cvm]
	vmMapMu.RUnlock()
	if !ok {
		// VM not found, this shouldn't happen - log error and return
		fmt.Printf("Error: goInvokeHostFunction called with unknown C VM pointer\n")
		return C.PVM_HOST_ERROR
	}

	// Invoke Go host function with panic recovery
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Host function %d panicked: %v\n", hostFuncID, r)
			// Set VM to panic state
			vm.MachineState = PANIC
			vm.ResultCode = PANIC
			vm.terminated = true

			// print stack trace
			stackBuf := make([]byte, 1024*8)
			n := runtime.Stack(stackBuf, false)
			fmt.Printf("Stack trace:\n%s\n", string(stackBuf[:n]))
		}
	}()

	// Call the host function
	_, err := vm.InvokeHostCall(int(hostFuncID))
	if err != nil {
		log.Error(vm.logging, fmt.Sprintf("HostCall %s (%d) ERROR: %v", HostFnToName(int(hostFuncID)), hostFuncID, err))
		return C.PVM_HOST_TERMINATE
	}
	// Check if host function caused panic
	if vm.MachineState == PANIC {
		vm.Panic(PANIC)
		//log.Error(vm.logging, fmt.Sprintf("HostCall %s (%d) PANIC!", HostFnToName(int(hostFuncID)), hostFuncID))
		return C.PVM_HOST_ERROR
	}

	// Check if VM was terminated by host function
	if vm.terminated {
		return C.PVM_HOST_TERMINATE
	}

	return C.PVM_HOST_CONTINUE
}

func (vm *Interpreter) Init(argumentData []byte) (err error) {
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

	return nil
}

func (vm *Interpreter) SetGas(gas uint64) {
	// STUB
}

func (vm *Interpreter) SetPC(pc uint64) {
	// STUB
}

func (vm *Interpreter) ExecuteAsChild(entryPoint uint32) error {
	// STUB
	return nil
}

func (vm *Interpreter) GetHostID() uint64 {
	// STUB
	return 0
}

func (vm *Interpreter) InitStepwise(pvm *VM, entryPoint uint32) error {
	// Call C pvm_init_stepwise to initialize VM for stepping
	result := C.pvm_init_stepwise(vm.cVM, C.uint32_t(entryPoint), 0)

	if result != C.PVM_RESULT_OK {
		return fmt.Errorf("pvm_init_stepwise failed with result %d", result)
	}

	return nil
}

func (vm *Interpreter) ExecuteStep(pvm *VM) []byte {
	// Allocate log buffer (369 bytes):
	// 1 byte opcode + 4 bytes pc + 8 bytes gas + 104 bytes registers + 256 bytes mem_op
	logBuf := make([]byte, 369)

	// Call C pvm_step function
	C.pvm_step(vm.cVM, (*C.uint8_t)(unsafe.Pointer(&logBuf[0])))

	// Update VM state from C
	pvm.ResultCode = vm.GetResultCode()
	pvm.MachineState = vm.GetMachineState()
	pvm.terminated = vm.IsTerminated()

	return logBuf
}
