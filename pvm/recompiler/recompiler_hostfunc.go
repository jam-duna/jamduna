package recompiler

import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

// Per-VM verifier storage keyed by regDumpAddr
// regDumpAddr is stable and unique per VM instance (allocated in regDumpMem)
var (
	verifierStorageMu sync.RWMutex
	verifierStorage   = make(map[uintptr]*RecompilerTraceVerifier)
)

// SetCurrentVerifier sets the verifier for a VM identified by its regDumpAddr
func SetCurrentVerifier(regDumpAddr uintptr, v *RecompilerTraceVerifier) {
	verifierStorageMu.Lock()
	verifierStorage[regDumpAddr] = v
	verifierStorageMu.Unlock()
}

// ClearCurrentVerifier clears the verifier for a VM identified by its regDumpAddr
func ClearCurrentVerifier(regDumpAddr uintptr) {
	verifierStorageMu.Lock()
	delete(verifierStorage, regDumpAddr)
	verifierStorageMu.Unlock()
}

// getCurrentVerifier retrieves the verifier for a VM by its regDumpAddr
func getCurrentVerifier(regDumpAddr uintptr) *RecompilerTraceVerifier {
	verifierStorageMu.RLock()
	v := verifierStorage[regDumpAddr]
	verifierStorageMu.RUnlock()
	return v
}

//export goDebugPrintInstruction
func goDebugPrintInstruction(opcode uint32, pc uint64, regDumpAddr unsafe.Pointer) {
	// Read 13 registers from the register dump area
	// Registers are stored at [regDumpAddr + 0] to [regDumpAddr + 104] (13 * 8 bytes)
	regs := (*[13]uint64)(regDumpAddr)

	// If verification mode is enabled, verify this step
	verifier := getCurrentVerifier(uintptr(regDumpAddr))
	if EnableVerifyMode && verifier != nil {
		var preRegs [13]uint64
		for i := 0; i < 13; i++ {
			preRegs[i] = regs[i]
		}
		mismatch := verifier.VerifyStepPreRegisters(byte(opcode), pc, preRegs)
		if mismatch != nil {
			fmt.Printf("❌ [RecompilerVerify] %s\n", mismatch.String())
			fmt.Printf("   Opcode: %s (0x%02x), PC: %d\n", opcode_str(byte(opcode)), opcode, pc)
			fmt.Printf("   Pre-Registers: ")
			for i := 0; i < 13; i++ {
				fmt.Printf("r%d=%d ", i, preRegs[i])
			}
			fmt.Printf("\n")
			panic("Recompiler verification failed")
			// Note: We don't terminate here - the verifier's StopOnFirstMismatch controls behavior
		} else {
			// fmt.Printf("✅ [RecompilerVerify] Step verified for Opcode: %s (0x%02x), PC: %d\n", opcode_str(byte(opcode)), opcode, pc)
		}
		return // Don't print debug output when verifying
	}

	// Original debug print behavior
	if EnableDebugTracing {
		fmt.Printf("[DEBUG] PC=%d Opcode=0x%02x (%s) | ", pc, opcode, opcode_str(byte(opcode)))
		for i := 0; i < 13; i++ {
			fmt.Printf("r%d=%d ", i, regs[i])
		}
		fmt.Printf("\n")
	}
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

// NOTE: add "time" to the file imports (import "time")
// Debug counters for DummyHostFunc (only used in tests)
// Protected by mutex to prevent races in parallel test execution
var (
	dummyHostFuncMu     sync.Mutex
	lastFrameTime       time.Time
	frameCount          uint64
	totalFrameDuration  time.Duration
	totalCalled         = 0
)

func (d *DummyHostFunc) InvokeHostCall(host_fn int) (bool, error) {
	vm := d.vm
	if host_fn == LOG {

		// JIP-1 https://hackmd.io/@polkadot/jip1
		level := vm.ReadRegister(7)
		message := vm.ReadRegister(10)
		messagelen := vm.ReadRegister(11)

		messageBytes, errCode := vm.ReadRAMBytes(uint32(message), uint32(messagelen))
		if errCode != OK {
			log.Error("x86", "DummyHostFunc: LOG: failed to read message from RAM", "error", errCode)
			return true, nil
		}
		log := false
		if !log {
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
	if host_fn == EXPORT {
		// EXPORT == frame out. Timestamp and compute instant & average frame rate.
		// Lock to protect shared debug counters in parallel test execution
		dummyHostFuncMu.Lock()
		defer dummyHostFuncMu.Unlock()

		now := time.Now()
		if lastFrameTime.IsZero() {
			lastFrameTime = now
			fmt.Printf("RecompilerVM: DUMMY EXPORT called (first frame timestamp recorded)\n")
		} else if totalCalled%250 == 0 {

			dur := now.Sub(lastFrameTime)
			lastFrameTime = now
			frameCount++
			totalFrameDuration += dur
			avgDur := totalFrameDuration / time.Duration(frameCount)
			fps := 0.0
			avgFps := 0.0
			if dur > 0 {
				fps = 1.0 / dur.Seconds()
			}
			if avgDur > 0 {
				avgFps = 1.0 / avgDur.Seconds()
			}
			p := vm.ReadRegister(7) // a0 = 7
			z := vm.ReadRegister(8) // a1 = 8
			z = min(z, types.SegmentSize)
			x, _ := vm.ReadRAMBytes(uint32(p), uint32(z))
			fmt.Printf("RecompilerVM: DUMMY EXPORT called - frame #%d duration=%v fps=%.2f avgFps=%.2f, data len %d\n", totalCalled, dur, fps, avgFps, len(x))
		}

		// x = slices.Clone(x)
		totalCalled++
	}
	return true, nil
}

func (vm *RecompilerVM) HandleEcalli() error {
	if !vm.hostCall || vm.MachineState != HOST {
		return fmt.Errorf("not in host call state")
	}

	ok, err := vm.InvokeHostCall(vm.host_func_id)
	if err != nil || !ok {
		return fmt.Errorf("InvokeHostCall failed: %w", err)
	}
	if ok {
		// if the host call handled the state change, we reset it to normal
		if vm.MachineState == PANIC {
			vm.ResultCode = types.WORKDIGEST_PANIC
			fmt.Printf("PANIC in host call\n")
			return fmt.Errorf("PANIC in host call")
		}
	}
	if vm.host_func_id == TRANSFER && vm.ReadRegister(7) == OK {
		gasU64, err := vm.ReadContextSlot(gasSlotIndex)
		if err != nil {
			return fmt.Errorf("failed to read gas slot: %w", err)
		}
		gas := int64(gasU64)
		transferGas := int64(vm.ReadRegister(9))
		gas -= transferGas

		err = vm.WriteContextSlot(gasSlotIndex, uint64(gas), 8)
		if err != nil {
			return fmt.Errorf("failed to write gas slot: %w", err)
		}
		vm.SetGas(gas)
		if gas < 0 {
			vm.MachineState = PANIC
			vm.ResultCode = types.WORKDIGEST_OOG
			vm.WriteContextSlot(vmStateSlotIndex, uint64(types.WORKDIGEST_OOG), 8)
			return fmt.Errorf("out of gas in transfer")
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

	// Verify SBRK instruction before execution if verify mode is enabled
	regDumpAddr := uintptr(unsafe.Pointer(&vm.regDumpMem[0]))
	verifier := getCurrentVerifier(regDumpAddr)
	if EnableVerifyMode && verifier != nil {
		preRegs := vm.ReadRegisters()
		var preRegsArray [13]uint64
		for i := 0; i < 13 && i < len(preRegs); i++ {
			preRegsArray[i] = preRegs[i]
		}
		mismatch := verifier.VerifyStepPreRegisters(SBRK, vm.pc, preRegsArray)
		if mismatch != nil {
			fmt.Printf("❌ [RecompilerVerify] SBRK %s\n", mismatch.String())
			fmt.Printf("   Opcode: SBRK (0x%02x), PC: %d\n", SBRK, vm.pc)
			fmt.Printf("   Pre-Registers: ")
			for i := 0; i < 13; i++ {
				fmt.Printf("r%d=%d ", i, preRegsArray[i])
			}
			fmt.Printf("\n")
			panic("Recompiler verification failed at SBRK")
		}
	}

	vm.InvokeSbrk(regA, regD)
	return nil
}

func (d *DummyHostFunc) GetResultCode() uint8 {
	return 0
}

func (d *DummyHostFunc) GetMachineState() uint8 {
	return 0
}
