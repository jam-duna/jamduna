package pvm

import "C"
import (
	"encoding/binary"
	"fmt"
	"os"
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
	for i := range vm.Ram.ReadRegisters() {
		vm.Ram.WriteRegister(i, binary.LittleEndian.Uint64(vm.regDumpMem[i*8:]))
	}
	// Invoke the host logic, e.g., gas charging and actual operation
	// fmt.Fprintf(os.Stderr, "Ecalli called with opcode: %d, gas: %d\n", opcode, vm.Gas)
	if vm.isChargingGas {
		gas, err := vm.ReadContextSlot(gasSlotIndex)
		if err != nil {
			log.Error("x86", "Ecalli: failed to read gas from context slot", "error", err)
		}
		vm.Gas = int64(gas)
	} else {
		vm.Gas = 100000
	}
	if opcode == 500 {
		var err error
		if useEcalli500 {
			if vm.isPCCounting {
				// read 4 bytes from vm.pc_addr
				vm.pc, err = vm.ReadContextSlot(pcSlotIndex)
				if err != nil {
					log.Error("x86", "Ecalli: failed to read pc from context slot", "error", err)
				}
			}

			// fmt.Printf("Ecalli: pc=%d, operands=%v, gas=%d\n", vm.pc, operands, vm.Gas)
			// fmt.Fprintf(os.Stderr, "Ecalli: pc=%d, gas=%d\n", vm.pc, vm.Gas)
			var blockCounter uint64
			if vm.IsBlockCounting {
				blockCounter, err = vm.ReadContextSlot(blockCounterSlotIndex)
				if err != nil {
					log.Error("x86", "Ecalli: failed to read blockCounter from context slot", "error", err)
				}
			}
			olen := vm.skip(vm.pc)
			operands := vm.code[vm.pc+1 : vm.pc+1+olen]
			// get the block counter from the register dump memory
			vm.vmBasicBlock = int(blockCounter)
			vm.basicBlockExecutionCounter[vm.pc]++
			if blockCounter%100_000_000 == 0 {
				if blockCounter >= 999_000_000_000_000 {
					vm.LogCurrentState(vm.code[vm.pc], operands, vm.pc, vm.Gas)
					fmt.Fprintf(os.Stderr, "+++ Ecalli: blockCounter=%d, pc=%d, gas=%d\n", blockCounter, vm.pc, vm.Gas)
				} else if blockCounter > 0 {
					fmt.Fprintf(os.Stderr, "--- Ecalli: blockCounter=%d, pc=%d, gas=%d\n", blockCounter, vm.pc, vm.Gas)
				}

			}
		}
		return
	}
	vm.InvokeHostCall(int(opcode))
	if opcode == 20 {
		vm.WriteContextSlot(gasSlotIndex, uint64(vm.Gas), 8)
	}

}
func (vm *RecompilerVM) LogCurrentState(opcode byte, operands []byte, currentPC uint64, gas int64) {
	if opcode == ECALLI {
		return
	}
	recordLog := false
	if gas >= hiResGasRangeStart && gas <= hiResGasRangeEnd {
		recordLog = true
	}
	// fmt.Printf("IsBasicBlockInstruction: %t vmBasicBlock: %d Gas: %d PC: %d Opcode: %s\n", IsBasicBlockInstruction(opcode), vm.vmBasicBlock, gas, currentPC, opcode_str(opcode))
	if (vm.vmBasicBlock)%50000 == 0 { // every 50k basic blocks, take a snapshot
		registers := vm.Ram.ReadRegisters()
		//fmt.Printf("vmBasicBlock: %d Gas: %d PC: %d Opcode: %s Registers: %v\n", vm.vmBasicBlock, gas, currentPC, opcode_str(opcode), registers)
		snapshot := vm.TakeSnapShot(fmt.Sprintf("BB%d", vm.vmBasicBlock), uint32(currentPC), registers, uint64(vm.Gas), 268435456, vm.r12, uint64(vm.vmBasicBlock))
		// this snapshot + memory of what was just executed, but we want the NEXT PC, not the currentPC.  The Gas is not clear
		vm.SaveSnapShot(snapshot)
		// vm.snapshot = snapshot
	}
	if vm.vmBasicBlock%10000 == 0 { // every 10000 basic blocks, take a snapshot
		recordLog = true
	}

	if recordLog {
		log := VMLog{
			Opcode:   opcode,
			OpStr:    opcode_str(opcode),
			Operands: operands,
			PvmPc:    currentPC,
			Gas:      gas,
		}

		log.Registers = make([]uint64, len(vm.Ram.ReadRegisters()))
		for i := 0; i < regSize; i++ {
			log.Registers[i], _ = vm.Ram.ReadRegister(i)
		}
		vm.Logs = append(vm.Logs, log)
		if (len(vm.Logs) > 10 && (gas < hiResGasRangeStart || gas > hiResGasRangeEnd)) || len(vm.Logs) > 1000 {
			vm.saveLogs()
		}
	}
}

// Ecalli is the host call invoked by the recompiled x86 code. It updates the VM state.
//
//export Sbrk
func Sbrk(rvmPtr unsafe.Pointer, registerIndexA uint32, registerIndexD uint32) {
	vm := (*RecompilerVM)(rvmPtr)
	// Acquire lock to ensure thread-safety
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Reload registers from memory before executing host call
	for i := range vm.Ram.ReadRegisters() {
		vm.Ram.WriteRegister(i, binary.LittleEndian.Uint64(vm.regDumpMem[i*8:]))
	}
	// Invoke the host logic, e.g., gas charging and actual operation
	// fmt.Fprintf(os.Stderr, "Sbrk: registerIndexA=%d, registerIndexD=%d\n", registerIndexA, registerIndexD)
	vm.InvokeSbrk(registerIndexA, registerIndexD)
}
func EmitCallToSbrkStub(rvmPtr uintptr, registerIndexA uint32, registerIndexD uint32) []byte {
	var stub []byte
	stub = append(stub, 0x50)                                 // push rax
	stub = append(stub, 0x57)                                 // push rdi
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...) // mov rdi, rvmPtr
	stub = append(stub, encodeMovEsiImm32(registerIndexA)...) // mov esi, valueA
	stub = append(stub, encodeMovEdxImm32(registerIndexD)...) // mov edx, registerIndexD

	// movabs rax, &Sbrk
	addr := GetSbrkAddress()
	stub = append(stub, encodeMovabsRaxImm64(uint64(addr))...)

	stub = append(stub, encodeCallRax()...) // call rax
	stub = append(stub, 0x5F)               // pop rdi
	stub = append(stub, 0x58)               // pop rax
	return stub
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
	// stub = append(stub, 0x50) // push rax
	// stub = append(stub, 0x57) // push rdi
	// mov rdi, rvmPtr
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...)
	// mov esi, opcode
	stub = append(stub, encodeMovEsiImm32(uint32(opcode))...)
	// movabs rax, <address of Ecalli>
	addr := GetEcalliAddress()
	// fmt.Printf("EmitCallToEcalliStub: addr=0x%x, opcode=%d\n", addr, opcode)

	stub = append(stub, encodeMovabsRaxImm64(uint64(addr))...)
	// call rax
	stub = append(stub, encodeCallRax()...)
	// stub = append(stub, 0x5F) // pop rdi
	// stub = append(stub, 0x58) // pop rax
	return stub
}

// EmitCallToEcalliStub creates the machine code stub that sets up the arguments
// and calls the Ecalli function using an absolute indirect call via RAX.
func EmitCallToEcalliStubPushPop(rvmPtr uintptr, opcode int) []byte {
	var stub []byte
	stub = append(stub, 0x50) // push rax
	stub = append(stub, 0x57) // push rdi
	// mov rdi, rvmPtr
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...)
	// mov esi, opcode
	stub = append(stub, encodeMovEsiImm32(uint32(opcode))...)
	// movabs rax, <address of Ecalli>
	addr := GetEcalliAddress()
	fmt.Printf("EmitCallToEcalliStub: addr=0x%x, opcode=%d\n", addr, opcode)

	stub = append(stub, encodeMovabsRaxImm64(uint64(addr))...)
	// call rax
	stub = append(stub, encodeCallRax()...)
	stub = append(stub, 0x5F) // pop rdi
	stub = append(stub, 0x58) // pop rax
	return stub
}

func EcalliSandBox(rvmPtr unsafe.Pointer, opcode int32) {
	vm := (*RecompilerSandboxVM)(rvmPtr)
	regMem, err := vm.sandBox.MemRead(uint64(vm.regDumpAddr), uint64(len(vm.Ram.ReadRegisters())*8))
	if err != nil {
		log.Error("x86", "EcalliSandBox: failed to read register memory", "error", err)
		return
	}
	for i := range vm.Ram.ReadRegisters() {
		val := binary.LittleEndian.Uint64(regMem[i*8 : (i+1)*8])
		vm.Ram.WriteRegister(i, val)
	}

	gas, err := vm.ReadContextSlot(gasSlotIndex)
	if err != nil {
		log.Error("x86", "EcalliSandBox: failed to read gas from context slot", "error", err)
		return
	}
	vm.Gas = int64(gas)
	// Invoke the host logic, e.g., gas charging and actual operation
	vm.InvokeHostCall(int(opcode))

	vm.WriteContextSlot(gasSlotIndex, uint64(vm.Gas), 8)
}

func (rvm *RecompilerSandboxVM) EcalliCodeSandBox(opcode int) ([]byte, error) {
	// 1. Dump registers to memory
	code := rvm.DumpRegisterToMemory(true)

	// 2. Generate call stub to Ecalli
	fmt.Printf("EcalliCodeSandBox: opcode=%d\n", opcode)
	stub := EmitCallToEcalliStubSandBox(uintptr(unsafe.Pointer(rvm)), opcode, rvm.ecallAddr)

	// 3. Append stub to code
	code = append(code, stub...)
	for i, reg := range regInfoList {
		code = append(code, generateLoadMemToReg(reg, uint64(rvm.regDumpAddr)+uint64(i*8))...)
	}
	return code, nil
}

// EmitCallToEcalliStub creates the machine code stub that sets up the arguments
// and calls the Ecalli function using an absolute indirect call via RAX.
func EmitCallToEcalliStubSandBox(rvmPtr uintptr, opcode int, addr uint64) []byte {
	var stub []byte
	// stub = append(stub, 0x50) // push rax
	// stub = append(stub, 0x57) // push rdi
	// stub = append(stub, 0x56) // push rsi
	// mov rdi, rvmPtr
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...)
	// mov esi, opcode
	stub = append(stub, encodeMovEsiImm32(uint32(opcode))...)
	// movabs rax, <address of Ecalli>
	//fmt.Printf("EmitCallToEcalliStubSandBox: addr=0x%x\n", addr)
	stub = append(stub, encodeMovabsRaxImm64(addr)...)
	// call rax
	stub = append(stub, encodeCallRax()...)
	// Clean up the stack
	//pop rsi, pop rdi, pop rax
	// stub = append(stub, 0x5E) // pop rsi
	// stub = append(stub, 0x5F) // pop rdi
	// stub = append(stub, 0x58) // pop rax

	return stub
}

func SbrkSandBox(rvmPtr unsafe.Pointer, registerIndexA uint32, registerIndexD uint32) {
	vm := (*RecompilerSandboxVM)(rvmPtr)
	// Acquire lock to ensure thread-safety
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Reload registers from memory before executing host call
	for i := range vm.Ram.ReadRegisters() {
		regAddr := vm.regDumpAddr
		regMem, errr := vm.sandBox.MemRead(uint64(regAddr+uintptr(i*8)), 8)
		if errr != nil {
			log.Error("x86", "SbrkSandBox: failed to read register memory", "index", i, "error", errr)
			return
		}
		v := binary.LittleEndian.Uint64(regMem)
		vm.Ram.WriteRegister(i, v)
		//fmt.Printf("SbrkSandBox: register[%d] = %d\n", i, v)
	}
	// Invoke the host logic, e.g., gas charging and actual operation
	result := vm.InvokeSbrkSandBox(registerIndexA, registerIndexD)
	// Write the result back to the specified register
	vm.Ram.WriteRegister(int(registerIndexD), uint64(result))
	// todo : use memory to separate registers and memory
	vm.writeBackRegisters()
}

func (vm *RecompilerSandboxVM) writeBackRegisters() {
	for i := range vm.Ram.ReadRegisters() {
		v, _ := vm.Ram.ReadRegister(i)
		if err := vm.sandBox.RegWrite(sandBoxRegInfoList[i], v); err != nil {
			log.Error("x86", "writeBackRegisters: failed to write back register", "index", i, "error", err)
			return
		}
		// fmt.Printf("writeBackRegisters: register[%d] = %d\n", i, v)
		// log.Debug("x86", "writeBackRegisters", "index", i, "value", v, "name", sandBoxRegInfoList[i].Name)
	}
}

func EmitCallToSbrkStubSandBox(rvmPtr uintptr, registerIndexA uint32, registerIndexD uint32, addr uint64) []byte {
	var stub []byte
	stub = append(stub, 0x50)                                 // push rax
	stub = append(stub, 0x57)                                 // push rdi
	stub = append(stub, 0x56)                                 // push rsi
	stub = append(stub, 0x52)                                 // push rdx
	stub = append(stub, encodeMovRdiImm64(uint64(rvmPtr))...) // mov rdi, rvmPtr
	stub = append(stub, encodeMovEsiImm32(registerIndexA)...) // mov esi, registerIndexA
	stub = append(stub, encodeMovEdxImm32(registerIndexD)...) // mov edx, registerIndexD

	// movabs rax, &Sbrk
	stub = append(stub, encodeMovabsRaxImm64(addr)...)

	stub = append(stub, encodeCallRax()...) // call rax
	// Clean up the stack
	stub = append(stub, 0x5A) // pop rdx
	stub = append(stub, 0x5E) // pop rsi
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
func encodeMovEdxImm32(val uint32) []byte {
	buf := make([]byte, 5)
	buf[0] = 0xBA // opcode
	binary.LittleEndian.PutUint32(buf[1:], val)
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

func (vm *RecompilerVM) chargeGas(host_fn int) uint64 {
	beforeGas := vm.Gas
	chargedGas := uint64(10) // We deduct 10 here
	exp := fmt.Sprintf("HOSTFUNC %d", host_fn)

	switch host_fn {
	case TRANSFER:
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
		/*
			case ZERO:
				exp = "ZERO"
			case VOID:
				exp = "VOID"
		*/
	case INVOKE:
		exp = "INVOKE"
	case EXPUNGE:
		exp = "EXPUNGE"
	case LOG:
		exp = "LOG"
		chargedGas = 0
	}
	if false {
		fmt.Fprintf(os.Stderr, "RecompilerVM: chargeGas: host_fn=%d, beforeGas=%d, chargedGas=%d, exp=%s\n",
			host_fn,
			beforeGas,
			chargedGas,
			exp)
	}
	return chargedGas
}

func (vm *RecompilerVM) InvokeSbrk(registerA uint32, registerIndexD uint32) (result uint32) {
	valueA, _ := vm.Ram.ReadRegister(int(registerA))
	currentHeapPointer := vm.GetCurrentHeapPointer()
	if valueA == 0 {
		vm.Ram.WriteRegister(int(registerIndexD), uint64(currentHeapPointer))
		return currentHeapPointer
	}
	result = currentHeapPointer
	//fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: current_heap_pointer=%d, valueA=%d, registerIndexD=%d\n", currentHeapPointer, valueA, registerIndexD)
	next_page_boundary := P_func(currentHeapPointer)
	new_heap_pointer := currentHeapPointer + uint32(valueA)
	if new_heap_pointer > uint32(next_page_boundary) {

		final_boundary := P_func(uint32(new_heap_pointer))
		// fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: new_heap_pointer=%d, final_boundary=%d\n", new_heap_pointer, final_boundary)
		idx_start := next_page_boundary / Z_P
		idx_end := final_boundary / Z_P
		page_count := idx_end - idx_start

		vm.allocatePages(idx_start, page_count)
		// fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: Allocated %d pages from %d to %d\n", page_count, idx_start, idx_end)
	}
	vm.SetCurrentHeapPointer(new_heap_pointer)
	return result
}

func (vm *RecompilerSandboxVM) InvokeSbrkSandBox(registerA uint32, registerIndexD uint32) (result uint32) {
	valueA, _ := vm.Ram.ReadRegister(int(registerA))
	currentHeapPointer := vm.GetCurrentHeapPointer()
	if valueA == 0 {
		vm.Ram.WriteRegister(int(registerIndexD), uint64(currentHeapPointer))
		return currentHeapPointer
	}
	result = currentHeapPointer
	//fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: current_heap_pointer=%d, valueA=%d, registerIndexD=%d\n", currentHeapPointer, valueA, registerIndexD)
	next_page_boundary := P_func(currentHeapPointer)
	new_heap_pointer := currentHeapPointer + uint32(valueA)
	if new_heap_pointer > uint32(next_page_boundary) {

		final_boundary := P_func(uint32(new_heap_pointer))
		//fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: new_heap_pointer=%d, final_boundary=%d\n", new_heap_pointer, final_boundary)
		idx_start := next_page_boundary / Z_P
		idx_end := final_boundary / Z_P
		page_count := idx_end - idx_start

		vm.allocatePages(idx_start, page_count)
		//fmt.Fprintf(os.Stderr, "RecompilerVM: Sbrk: Allocated %d pages from %d to %d\n", page_count, idx_start, idx_end)
	}
	vm.SetCurrentHeapPointer(new_heap_pointer)
	return result
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *RecompilerVM) InvokeHostCall(host_fn int) (bool, error) {

	// if vm.Gas-g < 0 {
	// 	vm.ResultCode = types.PVM_OOG
	// 	return true, fmt.Errorf("out of gas")
	// }
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
		gas, _ := vm.Ram.ReadRegister(9)
		vm.Gas = vm.Gas - int64(gas)
		fmt.Printf("InvokeHostCall: TRANSFER gas=%d, vm.Gas=%d\n", gas, vm.Gas)
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

	case INVOKE:
		vm.hostInvoke()
		return true, nil

	case EXPUNGE:
		vm.hostExpunge()
		return true, nil

	case LOG:
		vm.hostLog()
		return true, nil
		/*
			case ZERO:
				vm.hostZero()
				return true, nil

			case VOID:
				vm.hostVoid()
				return true, nil

			case MANIFEST:
				vm.hostManifest()
				return true, nil
		*/
	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d", host_fn)
	}
}
