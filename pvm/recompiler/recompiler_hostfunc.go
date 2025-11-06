package recompiler

import "C"
import (
	"fmt"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

//export goDebugPrintInstruction
func goDebugPrintInstruction(opcode uint32, pc uint64, regDumpAddr unsafe.Pointer) {
	// Read 13 registers from the register dump area
	// Registers are stored at [regDumpAddr + 0] to [regDumpAddr + 104] (13 * 8 bytes)
	regs := (*[13]uint64)(regDumpAddr)

	fmt.Printf("[DEBUG] PC=%d Opcode=0x%02x (%s) | ", pc, opcode, opcode_str(byte(opcode)))
	for i := 0; i < 13; i++ {
		fmt.Printf("r%d=%d ", i, regs[i])
	}
	fmt.Printf("\n")
}

// encodeMovRdiImm64 encodes 'mov rdi, imm64'.
func encodeMovRdiImm64(imm uint64) []byte {
	return emitMovImmToReg64(RDI, imm) // RDI is at index 5
}
func encodeMovEdxImm32(val uint32) []byte {
	return emitMovImm32ToReg32(RDX, val) // RDX is at index 2
}

// encodeMovEsiImm32 encodes 'mov esi, imm32'.
func encodeMovEsiImm32(imm uint32) []byte {
	return emitMovImm32ToReg32(RSI, imm) // RSI is at index 4
}

// encodeMovabsRaxImm64 encodes 'movabs rax, imm64'.
func encodeMovabsRaxImm64(imm uint64) []byte {
	return emitMovImmToReg64(RAX, imm) // RAX is at index 0
}

// encodeCallRax encodes 'call rax'.
func encodeCallRax() []byte {
	return emitCallReg(RAX) // RAX is at index 0
}

func (vm *X86Compiler) chargeGas(host_fn int) uint64 {
	chargedGas := uint64(10) // We deduct 10 here

	switch host_fn {
	case LOG:
		chargedGas = 0
	}
	return chargedGas
}

func (vm *RecompilerVM) InvokeSbrk(registerA uint32, registerIndexD uint32) (result uint32) {
	valueA := vm.ReadRegister(int(registerA))
	currentHeapPointer := vm.GetCurrentHeapPointer()
	if valueA == 0 {
		vm.WriteRegister(int(registerIndexD), uint64(currentHeapPointer))
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

	vm.WriteRegister(int(registerIndexD), uint64(result))
	return result
}

type HostFuncVM interface {
	// CPU registers
	ReadRegister(int) uint64
	// Memory
	ReadRAMBytes(offset uint32, length uint32) ([]byte, uint64)
}

type DummyHostFunc struct {
	vm HostFuncVM
}

func NewDummyHostFunc(vm HostFuncVM) *DummyHostFunc {
	return &DummyHostFunc{vm: vm}
}

func (d *DummyHostFunc) InvokeHostCall(host_fn int) (bool, error) {
	fmt.Printf("RecompilerVM: DUMMY InvokeHostCall: host_fn=%d\n", host_fn)
	if host_fn == LOG {
		vm := d.vm
		// JIP-1 https://hackmd.io/@polkadot/jip1
		level := vm.ReadRegister(7)
		message := vm.ReadRegister(10)
		messagelen := vm.ReadRegister(11)

		messageBytes, errCode := vm.ReadRAMBytes(uint32(message), uint32(messagelen))
		if errCode != OK {
			log.Error("x86", "DummyHostFunc: LOG: failed to read message from RAM", "error", errCode)
			return true, nil
		}
		switch level {
		case 0: // 0: User agent displays as fatal error
			fmt.Printf("[CRIT] DUMMY HOSTLOG: %s\n", string(messageBytes))
		case 1: // 1: User agent displays as warning
			fmt.Printf("[WARN] DUMMY HOSTLOG: %s\n", string(messageBytes))
		case 2: // 2: User agent displays as important information
			fmt.Printf("[INFO] DUMMY HOSTLOG: %s\n", string(messageBytes))
		case 3: // 3: User agent displays as helpful information
			fmt.Printf("[DEBUG] DUMMY HOSTLOG: %s\n", string(messageBytes))
		case 4: // 4: User agent displays as pedantic information
			fmt.Printf("[TRACE] DUMMY HOSTLOG: %s\n", string(messageBytes))
		}
	}
	return true, nil
}

func (vm *RecompilerVM) HandleEcalli() error {
	if !vm.hostCall || vm.MachineState != HOST {
		return fmt.Errorf("not in host call state")
	}

	if vm.host_func_id == TRANSFER {
		gas, err := vm.ReadContextSlot(gasSlotIndex)
		if err != nil {
			return fmt.Errorf("failed to read gas slot: %w", err)
		}
		transferGas := vm.ReadRegister(9)
		fmt.Printf("transferGas %d, before gas deduction gas = %d\n", transferGas, gas)
		gas -= transferGas
		fmt.Printf("after gas deduction gas = %d\n", gas)
		if gas < 0 {
			vm.MachineState = PANIC
			vm.ResultCode = types.WORKDIGEST_OOG
			return fmt.Errorf("out of gas in transfer")
		}
		err = vm.WriteContextSlot(gasSlotIndex, gas, 8)
		if err != nil {
			return fmt.Errorf("failed to write gas slot: %w", err)
		}
		vm.SetGas(int64(gas))
	}

	ok, err := vm.InvokeHostCall(vm.host_func_id)
	if err != nil || !ok {
		return fmt.Errorf("InvokeHostCall failed: %w", err)
	}
	if ok {
		// if the host call handled the state change, we reset it to normal
		if vm.MachineState == PANIC {
			vm.ResultCode = PANIC
			fmt.Printf("PANIC in host call\n")
			return fmt.Errorf("PANIC in host call")
		}
	}
	vm.hostCall = false
	vm.MachineState = 0
	return nil
}

func (vm *RecompilerVM) HandleDebug() error {

	opcodeDebug, err := vm.ReadContextSlot(opcodeDebugSlot)
	if err != nil {
		return fmt.Errorf("Debug call at PC %d missing opcode debug info", vm.pc)
	}
	fmt.Printf("[%s][%v]\n", opcode_str(byte(opcodeDebug)), vm.ReadRegisters())
	return nil
}

func (vm *RecompilerVM) HandleSbrk() error {
	regAint, err1 := vm.ReadContextSlot(sbrkAIndex)
	regDint, err2 := vm.ReadContextSlot(sbrkDIndex)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("Sbrk call at PC %d missing register info", vm.pc)
	}
	regA := uint32(regAint)
	regD := uint32(regDint)
	vm.InvokeSbrk(regA, regD)
	return nil
}

func (d *DummyHostFunc) GetResultCode() uint8 {
	return 0
}

func (d *DummyHostFunc) GetMachineState() uint8 {
	return 0
}
