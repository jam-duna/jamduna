package pvm

import "C"
import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
)

// Ecalli is the host call invoked by the recompiled x86 code. It updates the VM state.
//
//export Ecalli
func Ecalli(rvmPtr unsafe.Pointer, opcode int32) {
	vm := (*RecompilerVM)(rvmPtr)
	// Acquire lock to ensure thread-safety
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Reload registers from memory before executing host call
	for i := range vm.register {
		vm.register[i] = binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
	}
	// Invoke the host logic, e.g., gas charging and actual operation
	vm.InvokeHostCall(int(opcode))
}

// Ecalli is the host call invoked by the recompiled x86 code. It updates the VM state.
//
//export Sbrk
func Sbrk(rvmPtr unsafe.Pointer, valueA uint32, registerIndexD int32) {
	vm := (*RecompilerVM)(rvmPtr)
	// Acquire lock to ensure thread-safety
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Reload registers from memory before executing host call
	for i := range vm.register {
		vm.register[i] = binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
	}
	// Invoke the host logic, e.g., gas charging and actual operation
	vm.InvokeSbrk(valueA, registerIndexD)
}

// EcalliCode generates the x86_64 machine code snippet that:
// 1. Dumps registers to memory.
// 2. Sets up C ABI registers (rdi, esi).
// 3. Calls the Ecalli function.
// It returns the combined machine code bytes.
func (rvm *RecompilerVM) EcalliCode(opcode int) ([]byte, error) {
	// 1. Dump registers to memory
	code := rvm.DumpRegisterToMemory(true)

	// 2. Generate call stub to Ecalli
	stub := EmitCallToEcalliStub(uintptr(unsafe.Pointer(rvm)), opcode)

	// 3. Append stub to code
	code = append(code, stub...)
	return code, nil
}

// EmitCallToEcalliStub creates the machine code stub that sets up the arguments
// and calls the Ecalli function using an absolute indirect call via RAX.
func EmitCallToEcalliStub(rvmPtr uintptr, opcode int) []byte {
	var stub []byte
	stub = append(stub, 0x50) // push rax
	stub = append(stub, 0x57) // push rdi
	// mov rdi, rvmPtr
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...)
	// mov esi, opcode
	stub = append(stub, encodeMovEsiImm32(uint32(opcode))...)
	// movabs rax, <address of Ecalli>
	addr := GetEcalliAddress()
	stub = append(stub, encodeMovabsRaxImm64(uint64(addr))...)
	// call rax
	stub = append(stub, encodeCallRax()...)
	stub = append(stub, 0x5F) // pop rdi
	stub = append(stub, 0x58) // pop rax
	return stub
}

// encodeMovRdiImm64 encodes 'mov rdi, imm64'.
func encodeMovRdiImm64(imm uint64) []byte {
	buf := []byte{0x48, 0xBF}
	for i := 0; i < 8; i++ {
		buf = append(buf, byte(imm>>uint(8*i)))
	}
	return buf
}

// encodeMovEsiImm32 encodes 'mov esi, imm32'.
func encodeMovEsiImm32(imm uint32) []byte {
	buf := []byte{0xBE}
	for i := 0; i < 4; i++ {
		buf = append(buf, byte(imm>>uint(8*i)))
	}
	return buf
}

// encodeMovabsRaxImm64 encodes 'movabs rax, imm64'.
func encodeMovabsRaxImm64(imm uint64) []byte {
	buf := []byte{0x48, 0xB8}
	for i := 0; i < 8; i++ {
		buf = append(buf, byte(imm>>uint(8*i)))
	}
	return buf
}

// encodeCallRax encodes 'call rax'.
func encodeCallRax() []byte {
	return []byte{0xFF, 0xD0}
}

func (vm *RecompilerVM) chargeGas(host_fn int) {
	beforeGas := vm.Gas
	chargedGas := uint64(10)
	exp := fmt.Sprintf("HOSTFUNC %d", host_fn)

	switch host_fn {
	case TRANSFER:
		omega_9, _ := vm.ReadRegister(9)
		chargedGas = omega_9 + 10
		exp = "TRANSFER"
	case READ:
		exp = "READ"
	case WRITE:
		exp = "WRITE"
	case NEW:
		exp = "NEW"
	case FETCH:
		exp = "FETCH"
	case EXPORT:
		exp = "EXPORT"
	case GAS:
		exp = "GAS"
	case LOOKUP:
		exp = "LOOKUP"
	case INFO:
		exp = "INFO"
	case BLESS:
		exp = "BLESS"
	case ASSIGN:
		exp = "ASSIGN"
	case DESIGNATE:
		exp = "DESIGNATE"
	case CHECKPOINT:
		exp = "CHECKPOINT"
	case UPGRADE:
		exp = "UPGRADE"
	case EJECT:
		exp = "EJECT"
	case QUERY:
		exp = "QUERY"
	case SOLICIT:
		exp = "SOLICIT"
	case FORGET:
		exp = "FORGET"
	case YIELD:
		exp = "YIELD"
	case PROVIDE:
		exp = "PROVIDE"
	case HISTORICAL_LOOKUP:
		exp = "HISTORICAL_LOOKUP"
	case MACHINE:
		exp = "MACHINE"
	case PEEK:
		exp = "PEEK"
	case POKE:
		exp = "POKE"
	case ZERO:
		exp = "ZERO"
	case VOID:
		exp = "VOID"
	case INVOKE:
		exp = "INVOKE"
	case EXPUNGE:
		exp = "EXPUNGE"
	case LOG:
		exp = "LOG"
		chargedGas = 0
	}

	vm.Gas = beforeGas - int64(chargedGas)
	log.Trace(vm.logging, exp, "reg", vm.ReadRegisters(), "gasCharged", chargedGas, "beforeGas", beforeGas, "afterGas", vm.Gas)
}

func (vm *RecompilerVM) InvokeSbrk(valueA uint32, registerIndexD int32) (result uint32) {
	if valueA == 0 {
		vm.WriteRegister(int(registerIndexD), uint64(vm.Ram.current_heap_pointer))
		return vm.Ram.current_heap_pointer
	}
	result = vm.Ram.current_heap_pointer
	next_page_boundary := P_func(vm.Ram.current_heap_pointer)
	new_heap_pointer := vm.Ram.current_heap_pointer + valueA
	if new_heap_pointer > uint32(next_page_boundary) {
		final_boundary := P_func(uint32(new_heap_pointer))
		idx_start := next_page_boundary / Z_P
		idx_end := final_boundary / Z_P
		page_count := idx_end - idx_start

		vm.Ram.allocatePages(idx_start, page_count)
	}
	vm.Ram.current_heap_pointer = uint32(new_heap_pointer)
	return result
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *RecompilerVM) InvokeHostCall(host_fn int) (bool, error) {

	// if vm.Gas-g < 0 {
	// 	vm.ResultCode = types.PVM_OOG
	// 	return true, fmt.Errorf("out of gas")
	// }
	vm.chargeGas(host_fn)
	switch host_fn {
	case GAS:
		vm.hostGas()
		return true, nil

	case LOOKUP:
		vm.hostLookup()
		return true, nil

	case READ:
		vm.hostRead()
		return true, nil

	case WRITE:
		vm.hostWrite()
		return true, nil

	case INFO:
		vm.hostInfo()
		return true, nil

	case BLESS:
		vm.hostBless()
		return true, nil

	case ASSIGN:
		vm.hostAssign()
		return true, nil

	case DESIGNATE:
		vm.hostDesignate()
		return true, nil

	case CHECKPOINT:
		vm.hostCheckpoint()
		return true, nil

	case NEW:
		vm.hostNew()
		return true, nil

	case UPGRADE:
		vm.hostUpgrade()
		return true, nil

	case TRANSFER:
		vm.hostTransfer()
		return true, nil

	case EJECT:
		vm.hostEject()
		return true, nil

	case QUERY:
		vm.hostQuery()
		return true, nil

	case SOLICIT:
		vm.hostSolicit()
		return true, nil

	case FORGET:
		// t := vm.hostenv.GetTimeslot()
		vm.hostForget()
		return true, nil

	case YIELD:
		vm.hostYield()
		return true, nil

	case PROVIDE:
		vm.hostProvide()
		return true, nil

	// Refine functions
	case HISTORICAL_LOOKUP:
		vm.hostHistoricalLookup()
		return true, nil

	case FETCH:
		vm.hostFetch()
		return true, nil

	case EXPORT:
		vm.hostExport()
		return true, nil

	case MACHINE:
		vm.hostMachine()
		return true, nil

	case PEEK:
		vm.hostPeek()
		return true, nil

	case POKE:
		vm.hostPoke()
		return true, nil

	case ZERO:
		vm.hostZero()
		return true, nil

	case VOID:
		vm.hostVoid()
		return true, nil

	case INVOKE:
		vm.hostInvoke()
		return true, nil

	case EXPUNGE:
		vm.hostExpunge()
		return true, nil

	case LOG:
		vm.hostLog()
		return true, nil

	case MANIFEST:
		vm.hostManifest()
		return true, nil

	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d", host_fn)
	}
}
