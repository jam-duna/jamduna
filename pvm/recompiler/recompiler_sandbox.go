//go:build unicorn
// +build unicorn

package recompiler

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"sort"
	"time"

	"github.com/unicorn-engine/unicorn/bindings/go/unicorn"
	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
)

// {"rax", 0, 0}, // Commonly used as return value register
// {"rcx", 1, 0}, // Used for loop counters or intermediates
// {"rdx", 2, 0}, // Often paired with rax for mul/div
// {"rbx", 3, 0},
// {"rsi", 6, 0}, // Often used as function argument

// {"rdi", 7, 0}, // Often used as function argument
// {"r8", 0, 1},  // Typically function argument #5
// {"r9", 1, 1},
// {"r10", 2, 1},
// {"r11", 3, 1},

// {"r13", 5, 1},
// {"r14", 6, 1},
// {"r15", 7, 1},

var sandBoxRegInfoList = []int{
	uc.X86_REG_RAX,
	uc.X86_REG_RCX,
	uc.X86_REG_RDX,
	uc.X86_REG_RBX,
	uc.X86_REG_RSI,
	uc.X86_REG_RDI,
	uc.X86_REG_R8,
	uc.X86_REG_R9,
	uc.X86_REG_R10,
	uc.X86_REG_R11,
	uc.X86_REG_R13,
	uc.X86_REG_R14,
	uc.X86_REG_R15,
}

const (
	pageSize = uint64(0x1000) // 4 KiB

	guestBase = uint64(0x10000000)             // guest RAM start
	guestSize = uint64(4 * 1024 * 1024 * 1024) // 4 GiB
)

func SetUseEcalli500(enabled bool) {
	useEcalli500 = enabled
}

var skipSaveLog = false

func SetDebugRecompiler(enabled bool) {
	debugRecompiler = enabled
}

type RecompilerSandboxVM struct {
	RecompilerVM
	sandBox *Emulator

	savedRegisters bool
	savedMemory    bool
	ecallAddr      uint64
	sbrkOffset     uint64
	stepNumber     int
	snapshot       *EmulatorSnapShot
	post_register  []uint64 // registers after execution

	recordGeneratedCode bool   // record generated code for debugging
	genreatedCode       []byte // generated x86 code
}

func (rvm *RecompilerSandboxVM) WriteRAMBytes(address uint32, data []byte) (resultCode uint64) {
	return rvm.sandBox.WriteRAMBytes(address, data)
}

func (rvm *RecompilerSandboxVM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	return rvm.sandBox.ReadRAMBytes(address, length)
}

func (rvm *RecompilerSandboxVM) GetCurrentHeapPointer() uint32 {
	return rvm.sandBox.current_heap_pointer
}

func (rvm *RecompilerSandboxVM) SetHeapPointer(ptr uint32) {
	rvm.sandBox.current_heap_pointer = ptr
}

func (rvm *RecompilerSandboxVM) SetMemAccess(address uint32, length uint32, access int) error {
	return rvm.sandBox.SetMemAccessSandBox(address, length, access)
}

func (rvm *RecompilerSandboxVM) GetMemAccess(address uint32, length uint32) (int, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return PageInaccessible, fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}
	return rvm.sandBox.pageAccess[pageIndex], nil
}

func (rvm *RecompilerSandboxVM) ReadRegister(index int) uint64 {
	// read register from the context slot
	val, _ := rvm.sandBox.ReadRegister(index)
	return val
}

func (rvm *RecompilerSandboxVM) ReadRegisters() []uint64 {
	return rvm.sandBox.ReadRegisters()
}

func (rvm *RecompilerSandboxVM) ReadRegistersFromX86() []uint64 {
	post_register := make([]uint64, len(sandBoxRegInfoList))
	for i, reg := range sandBoxRegInfoList {
		val, _ := rvm.sandBox.RegRead(reg)
		post_register[i] = val
	}
	return post_register
}

func (rvm *RecompilerSandboxVM) SetGas(gas uint64) {
	rvm.WriteContextSlot(gasSlotIndex, gas, 8)
}

func (rvm *RecompilerSandboxVM) WriteRegister(index int, value uint64) {
	// write register to the context slot
	if index < 0 || index >= len(regInfoList) {
		return
	}
	rvm.WriteContextSlot(index, value, 8)
}

type Emulator struct {
	uc.Unicorn
	pageAccess           map[int]int
	dirtyPages           map[int]bool // pages that were modified
	current_heap_pointer uint32
	regDumpMem           []byte  // buffer to copy registers
	regDumpAddr          uintptr // address to dump registers
	realMemory           []byte  // actual memory for the guest
	realMemAddr          uintptr // address of the real memory

	guestCodeAddr uintptr
	guestCodeLen  uintptr
}

func NewEmulator() (*Emulator, error) {
	mu, err := uc.NewUnicorn(uc.ARCH_X86, uc.MODE_64)
	if err != nil {
		return nil, fmt.Errorf("create unicorn: %w", err)
	}

	dumpSize := uint64(0x100000) // 1 MiB dump page size
	if dumpSize < 104 {
		dumpSize = 10
	}
	dumpSize = (dumpSize + pageSize - 1) & ^(pageSize - 1) // round-up
	guestMainSize := guestSize                             // main RAM size
	dumpAddr := guestBase - dumpSize                       // dump page start

	if err := mu.MemMap(guestBase, guestMainSize); err != nil {
		mu.Close()
		return nil, fmt.Errorf("map guest RAM: %w", err)
	}

	if err := mu.MemProtect(guestBase, guestMainSize, uc.PROT_READ|uc.PROT_WRITE); err != nil {
		mu.Close()
		return nil, fmt.Errorf("protect guest RAM: %w", err)
	}

	if err := mu.MemMap(dumpAddr, dumpSize); err != nil {
		mu.Close()
		return nil, fmt.Errorf("map dump page: %w", err)
	}
	if err := mu.MemProtect(dumpAddr, dumpSize, uc.PROT_READ|uc.PROT_WRITE); err != nil {
		mu.Close()
		return nil, fmt.Errorf("protect dump page: %w", err)
	}
	access := make(map[int]int)
	for i := 0; i < TotalPages; i++ {
		access[i] = PageInaccessible // initialize all pages as inaccessible
	}
	return &Emulator{
		Unicorn:     mu,
		pageAccess:  make(map[int]int),
		dirtyPages:  make(map[int]bool),
		regDumpMem:  make([]byte, dumpSize), // buffer to copy registers
		regDumpAddr: uintptr(dumpAddr),
		realMemory:  nil, // defer heavy allocation
		realMemAddr: uintptr(guestBase),
	}, nil
}

func (emu *Emulator) Close() error {
	if emu.Unicorn != nil {
		return emu.Unicorn.Close()
	}
	return nil
}

// NewRecompilerVM_SandBox creates a Unicorn sandbox with 4 GiB guest RAM
// and an extra dump page at the very end of that RAM region.
func NewRecompilerSandboxVM(vm *RecompilerVM) (*RecompilerSandboxVM, error) {
	mu, err := NewEmulator()
	if err != nil {
		return nil, fmt.Errorf("create emulator: %w", err)
	}
	return NewRecompilerSandboxVMFromEmulator(vm, mu)
}

func NewRecompilerSandboxVMFromEmulator(vm *RecompilerVM, mu *Emulator) (*RecompilerSandboxVM, error) {

	ecallAddr := guestBase + guestSize
	if err := mu.MemMap(ecallAddr, 0x1000); err != nil {
		mu.Close()
		return nil, fmt.Errorf("map ecall page: %w", err)
	}
	if err := mu.MemProtect(ecallAddr, 0x1000, uc.PROT_ALL); err != nil {
		mu.Close()
		return nil, fmt.Errorf("protect ecall page: %w", err)
	}
	dumpSize := uint64(0x100000) // 1 MiB dump page size
	if dumpSize < 104 {
		dumpSize = 10
	}
	dumpSize = (dumpSize + pageSize - 1) & ^(pageSize - 1) // round-up
	rvm := &RecompilerSandboxVM{
		RecompilerVM: *vm,
		sandBox:      mu,

		ecallAddr:  ecallAddr,
		sbrkOffset: 0x1,
	}
	rvm.RecompilerVM.regDumpAddr = mu.regDumpAddr
	rvm.RecompilerVM.regDumpMem = mu.regDumpMem
	rvm.RecompilerVM.realMemAddr = mu.realMemAddr

	rvm.sandBox.current_heap_pointer = rvm.RecompilerVM.GetCurrentHeapPointer()
	rvm.sandBox.HookAdd(uc.HOOK_MEM_READ, func(mu uc.Unicorn, access int,
		addr uint64, size int, value int64) {
		// compute the page index
		if addr < guestBase || addr >= guestBase+guestSize {
			return // ignore accesses outside guest RAM
		}
		offset := addr - guestBase
		pageIndex := int(offset / pageSize)
		if access, ok := rvm.sandBox.pageAccess[pageIndex]; !ok || access == PageInaccessible {
			fmt.Printf("❌ Access denied for page %d at address 0x%X\n", pageIndex, offset)
			rvm.ResultCode = PANIC
			rvm.saveRegistersOnceSandBox()
			rvm.sandBox.Stop()
			rvm.MachineState = PANIC
		} else {

			data, err := rvm.sandBox.MemRead(addr, uint64(size))
			if err != nil {
				fmt.Printf("    (failed to read: %v)\n", err)
			} else if debugRecompiler {
				fmt.Printf("🔍 READ  @ 0x%X (%d bytes):0x%X\n", addr, size, data)
			}
		}
	}, 1, 0, 0)

	rvm.sandBox.HookAdd(uc.HOOK_MEM_WRITE, func(mu uc.Unicorn, access int,
		addr uint64, size int, value int64) {
		// Track writes to divergence address range [0x10032800 - 0x10032900]
		if addr >= guestBase+0x32800 && addr <= guestBase+0x32900 && size == 4 {
			rip, _ := rvm.sandBox.RegRead(uc.X86_REG_RIP)
			offset := uint32(addr - guestBase)
			fmt.Printf("[SANDBOX_WRITE_32_HOOK] RIP=0x%x addr=0x%x data=0x%x\n", rip, offset, uint32(value))
		}
		if debugRecompiler {
			fmt.Printf("✍️ WRITE @ 0x%X (%d bytes) = 0x%X", addr, size, value)
			// For 1-byte writes to problematic region, show ALL register state
			if size == 1 && addr >= 0x10130040 && addr <= 0x10130060 {
				rax, _ := rvm.sandBox.RegRead(uc.X86_REG_RAX)
				rcx, _ := rvm.sandBox.RegRead(uc.X86_REG_RCX)
				r9, _ := rvm.sandBox.RegRead(uc.X86_REG_R9)
				r10, _ := rvm.sandBox.RegRead(uc.X86_REG_R10)
				r11, _ := rvm.sandBox.RegRead(uc.X86_REG_R11)
				r12, _ := rvm.sandBox.RegRead(uc.X86_REG_R12)
				r13, _ := rvm.sandBox.RegRead(uc.X86_REG_R13)
				r14, _ := rvm.sandBox.RegRead(uc.X86_REG_R14)
				fmt.Printf("\n  [RAX=0x%x RCX=0x%x R9=0x%x R10=0x%x R11=0x%x R12=0x%x R13=0x%x R14=0x%x]", rax, rcx, r9, r10, r11, r12, r13, r14)
			}
			fmt.Printf("\n")
		}
		// ignore writes outside guest RAM
		baseRegValue, err := rvm.sandBox.RegRead(uc.X86_REG_R12)
		if err != nil {
			fmt.Printf("❌ Failed to read R12: %v\n", err)
			return
		}

		if addr < guestBase || addr >= guestBase+guestSize {
			return
		} else {
			if baseRegValue != guestBase {
				panic("R12 base register is not set to guestBase")
			}
		}
		// compute the page index
		offset := addr - guestBase
		pageIndex := int(offset / pageSize)
		if access, ok := rvm.sandBox.pageAccess[pageIndex]; !ok || access == PageInaccessible || access == PageImmutable {
			if !ok {
				fmt.Printf("🔒 Page %d at address 0x%X is not initialized\n", pageIndex, offset)
			} else if access == PageInaccessible {
				fmt.Printf("🔒 Page %d at address 0x%X is inaccessible\n", pageIndex, offset)
			} else if access == PageImmutable {
				fmt.Printf("🔒 Page %d at address 0x%X is immutable\n", pageIndex, offset)
			}
			fmt.Printf("❌ Access denied for page %d at address 0x%X\n", pageIndex, offset)
			rvm.ResultCode = PANIC
			rvm.saveRegistersOnceSandBox()
			rvm.saveMemoryOnceSandBox()
		} else {
			rvm.sandBox.dirtyPages[pageIndex] = true // mark page as dirty
		}
	}, 1, 0, 0)
	rvm.sandBox.HookAdd(unicorn.HOOK_MEM_INVALID, func(uc unicorn.Unicorn, access int, address uint64, size int, value int64) bool {
		fmt.Printf("⚠️ Invalid memory access: access=%d addr=0x%X size=%d value=%d\n",
			access, address, size, value)
		return false // return false to stop the emulation
	}, 1, 0, 0)

	return rvm, nil
}

func (rvm *RecompilerSandboxVM) SetRecoredGeneratedCode(enabled bool) {
	rvm.recordGeneratedCode = enabled
	if rvm.recordGeneratedCode {
		rvm.genreatedCode = make([]byte, 0)
	}
}

func (vm *RecompilerSandboxVM) Close() {
	vm.RecompilerVM.Close()
	if vm.sandBox != nil {
		if err := vm.sandBox.Close(); err != nil {
			fmt.Printf("Error closing sandbox: %v\n", err)
		}
		vm.sandBox = nil
	}
}

const stackSize = uint64(0x400000) //4mb
const stackTop = uint64(0x1A0000000)

const (
	codeBase = uint64(0x190000000) // separate memory area outside 4GiB RAM
)

var saveCode = true

func (vm *RecompilerSandboxVM) ExecuteX86Code_SandBox_WithEntry(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			vm.ResultCode = PANIC
			vm.MachineState = PANIC
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()
	codeLen := len(x86code)
	codeLenAligned := (uint64(codeLen) + pageSize - 1) & ^(pageSize - 1)
	vm.codeAddr = uintptr(codeBase)
	vm.djumpAddr += vm.codeAddr
	// --------------------------------------------------------------------
	// 2. Allocate a new memory region for code
	// --------------------------------------------------------------------
	if err := vm.sandBox.MemMap(codeBase, codeLenAligned); err != nil {
		return fmt.Errorf("MemMap failed: %w", err)
	}
	if err := vm.sandBox.MemProtect(codeBase, codeLenAligned, uc.PROT_ALL); err != nil {
		return fmt.Errorf("MemProtect failed: %w", err)
	}
	// --------------------------------------------------------------------
	// 3. Write code and jump table into the allocated memory
	// --------------------------------------------------------------------
	if err := vm.sandBox.MemWrite(codeBase, x86code); err != nil {
		return fmt.Errorf("write main code: %w", err)
	}

	codeBytes, err := vm.sandBox.MemRead(codeBase, uint64(codeLen))
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	vm.sandBox.guestCodeAddr = uintptr(codeBase)
	vm.sandBox.guestCodeLen = uintptr(codeLen)
	vm.realCode = codeBytes
	if saveCode {
		codeName := "hash_service.bin"
		err := os.WriteFile(codeName, codeBytes, 0644)
		if err != nil {
			fmt.Printf("Failed to save code to %s: %v\n", codeName, err)
		} else {
			fmt.Printf("Saved generated code to %s\n", codeName)
		}

		instrunction_map_name := "x86_to_pvm_map_go.json"
		instr_map := vm.InstMapX86ToPVM
		jsonData, err := json.MarshalIndent(instr_map, "", "  ")
		if err != nil {
			fmt.Printf("Failed to marshal instruction map to JSON: %v\n", err)
		} else {
			err = os.WriteFile(instrunction_map_name, jsonData, 0644)
			if err != nil {
				fmt.Printf("Failed to save instruction map to %s: %v\n", instrunction_map_name, err)
			} else {
				fmt.Printf("Saved instruction map to %s\n", instrunction_map_name)
			}
		}
	}
	// Print disassembled code for debugging
	str := Disassemble(codeBytes)
	if showDisassembly {
		fmt.Printf("disassembled code: %s\n", str)
	}
	vm.sandBox.HookAdd(
		uc.HOOK_MEM_WRITE_UNMAPPED,
		func(mu uc.Unicorn, access int, addr uint64, size int, value int64) bool {
			fmt.Printf("❌ WRITE_UNMAPPED @ 0x%X (size=%d, value=0x%X)\n", addr, size, value)
			return false
		},
		1, 0, 0,
	)
	// _, err = vm.sandBox.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
	// 	offset := addr - codeBase
	// 	code := vm.x86Code[offset : offset+size]
	// 	instruction := Disassemble(code)
	// 	if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
	// 		opcode := vm.code[pvm_pc]
	// 		if debugRecompiler {
	// 			fmt.Printf("[DEBUG_UC]Start PVM PC: %d [%s] reg:%v\n", pvm_pc, opcode_str(opcode), vm.ReadRegistersFromX86())
	// 		}
	// 	}

	// 	baseRegValue, err := mu.RegRead(uc.X86_REG_R12)
	// 	if err != nil {
	// 		fmt.Printf("❌ Failed to read R12: %v\n", err)
	// 		return
	// 	}
	// 	rcxValue, err := mu.RegRead(uc.X86_REG_RCX)
	// 	if err != nil {
	// 		fmt.Printf("❌ Failed to read RBP: %v\n", err)
	// 		return
	// 	}
	// 	if debugRecompiler {
	// 		fmt.Printf("🧭 Executing @ 0x%X >%s< [baseReg 0x%X][RCX 0x%X]\n", addr, instruction.String(), baseRegValue, rcxValue)
	// 	}
	// 	if instruction.String() == "RET" {
	// 		// Handle RET instruction
	// 		// This is a special case where we need to restore the registers and memory
	// 		//fmt.Printf("🧭 RET instruction encountered at 0x%X\n", addr)
	// 		vm.saveRegistersOnceSandBox()
	// 		vm.saveMemoryOnceSandBox()
	// 		machineState, _ := vm.ReadContextSlot(vmStateSlotIndex)
	// 		vm.MachineState = uint8(machineState)
	// 		hostId, _ := vm.ReadContextSlot(hostFuncIdIndex)
	// 		vm.host_func_id = int(hostId)
	// 		// Stop the sandbox
	// 		if machineState == HOST {
	// 			vm.hostCall = true
	// 		}
	// 		vm.sandBox.Stop()
	// 		return
	// 	}
	// if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
	// 	basicBlock, ok2 := vm.basicBlocks[uint64(pvm_pc)]
	// 	if !ok2 {
	// 		// fmt.Printf("⚠️ No basic block found for PVM PC %d at 0x%X ...\n", pvm_pc, offset)
	// 		return
	// 	}
	// 	if debugRecompiler {
	// 		gas, _ := vm.ReadContextSlot(gasSlotIndex)
	// 		vm.Gas = int64(gas)
	// 		fmt.Printf("PVMX PC: %d [%s] reg:%v  BasicBlock: %s ⛽️ %d \n", pvm_pc, opcode_str(vm.code[pvm_pc]),
	// 			vm.ReadRegisters(), basicBlock.String(), vm.Gas)
	// 	}
	// 	//vm.Gas -= basicBlock.GasUsage
	// } else {
	// 	// fmt.Printf("⚠️ No basic block found for instruction at 0x%X\n", offset)s
	// }
	// }, 1, 0, 0) // 1~0 == entire range
	if err != nil {
		return fmt.Errorf("add code hook: %w", err)
	}

	stackBase := (stackTop - stackSize) &^ (pageSize - 1)

	if err := vm.sandBox.MemMap(stackBase, stackSize); err != nil {
		return fmt.Errorf("stack MemMap: %w", err)
	}
	if err := vm.sandBox.MemProtect(stackBase, stackSize, uc.PROT_ALL); err != nil {
		return fmt.Errorf("stack MemProtect: %w", err)
	}
	if err := vm.sandBox.RegWrite(uc.X86_REG_RSP, stackTop); err != nil {
		return fmt.Errorf("set RSP: %w", err)
	}
	rsp, _ := vm.sandBox.RegRead(uc.X86_REG_RSP)
	if debugRecompiler {
		fmt.Printf("RSP = 0x%X\n", rsp)
	}
	// --------------------------------------------------------------------
	// 5. Run the code
	// --------------------------------------------------------------------
	if err := vm.sandBox.Start(codeBase, codeBase+uint64(codeLen)); err != nil {
		vm.ResultCode = PANIC
		vm.MachineState = PANIC
		vm.saveRegistersOnceSandBox()
		vm.saveMemoryOnceSandBox()
		return fmt.Errorf("emulation failed: %w", err)
	}

	// --------------------------------------------------------------------
	// 6. Read out results
	// --------------------------------------------------------------------
	vm.regDumpMem, err = vm.sandBox.MemRead(uint64(vm.regDumpAddr), uint64(len(vm.regDumpMem)))
	if err != nil {
		return fmt.Errorf("read reg dump: %w", err)
	}

	vm.realMemory, err = vm.sandBox.MemRead(uint64(vm.realMemAddr), uint64(len(vm.realMemory)))
	if err != nil {
		return fmt.Errorf("read guest RAM: %w", err)
	}

	for i := range vm.ReadRegisters() {
		val := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		if debugRecompiler {
			fmt.Printf("%s = 0x%X\n", regInfoList[i].Name, val)
		}
		vm.WriteRegister(i, val)
	}
	return nil
}

func (rvm *RecompilerSandboxVM) ExecuteSandBox(entryPoint uint64) error {
	compileStart := time.Now()
	rvm.x86Code, rvm.djumpAddr, rvm.InstMapPVMToX86, rvm.InstMapX86ToPVM = rvm.compiler.CompileX86Code(entryPoint)
	rvm.compileTime = time.Since(compileStart)

	execStart := time.Now()
	if execErr := rvm.ExecuteX86Code_SandBox_WithEntry(rvm.x86Code); execErr != nil {
		fmt.Printf("ExecuteX86 crash detected: %v\n", execErr)
	}
	rvm.pc, _ = rvm.ReadContextSlot(pcSlotIndex)
	fmt.Printf("After ExecuteX86, PVM PC: %d, MachineState: %d, Gas: %d\n", rvm.pc, rvm.MachineState, rvm.Gas)
	for rvm.MachineState == HOST || rvm.MachineState == SBRK {
		if rvm.MachineState == HOST {
			err := rvm.HandleEcalli()
			if err != nil {
				fmt.Printf("HandleEcalli failed: %v\n", err)
				break
			}
		} else if rvm.MachineState == SBRK {
			err := rvm.HandleSbrk()
			if err != nil {
				fmt.Printf("HandleSbrk failed: %v\n", err)
				break
			}
		}
		if rvm.MachineState == PANIC {
			break
		}
		err := rvm.Resume()
		if err != nil {
			fmt.Printf("Resume after host call failed: %v\n", err)
			rvm.WriteContextSlot(vmStateSlotIndex, uint64(PANIC), 8)
			rvm.MachineState = PANIC
			break
		}
		rvm.pc, _ = rvm.ReadContextSlot(pcSlotIndex)
		fmt.Printf("After Resume, PVM PC: %d, MachineState: %d, Gas: %d\n", rvm.pc, rvm.MachineState, rvm.Gas)

	}

	gas, err := rvm.ReadContextSlot(gasSlotIndex)
	if err != nil {
		fmt.Printf("Failed to read gas from context slot: %v\n", err)
	} else {
		rvm.Gas = int64(gas)
	}
	rvm.allExecutionTime = time.Since(execStart)
	return nil
}
func (vm *RecompilerSandboxVM) Resume() error {
	patchInstIdx := entryOffset
	u64x86PC, err := vm.ReadContextSlot(nextx86SlotIndex)
	codeAddr := vm.realCode
	if err != nil && vm.pc != 0 {
		fmt.Printf("post-host call: pc %d not found in InstMapPVMToX86, isChild %v\n", vm.pc, vm.IsChild)
		return fmt.Errorf("post-host call: pc %d not found in InstMapPVMToX86, isChild %v", vm.pc, vm.IsChild)
	}
	x86PC := int(u64x86PC)
	// compute rel32 from the next instruction after JMP
	rel := uint32(x86PC - patchInstIdx - 5)
	var imm [4]byte
	binary.LittleEndian.PutUint32(imm[:], rel)
	copy(codeAddr[patchInstIdx+1:patchInstIdx+5], imm[:])
	codeLen := (uint64(len(codeAddr)) + pageSize - 1) & ^(pageSize - 1)
	if err := vm.sandBox.MemUnmap(codeBase, codeLen); err != nil {
		return fmt.Errorf("MemUnmap failed: %w. codeBase %x, codeLen %x", err, codeBase, codeLen)
	}
	if err := vm.sandBox.MemMap(codeBase, codeLen); err != nil {
		return fmt.Errorf("MemMap failed: %w", err)
	}
	if err := vm.sandBox.MemProtect(codeBase, codeLen, uc.PROT_ALL); err != nil {
		return fmt.Errorf("MemProtect failed: %w", err)
	}
	if err := vm.sandBox.MemWrite(codeBase, codeAddr); err != nil {
		return fmt.Errorf("write main code: %w", err)
	}
	codeBytes, err := vm.sandBox.MemRead(codeBase, uint64(len(codeAddr)))
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	str := Disassemble(codeBytes)
	if showDisassembly {
		fmt.Printf("disassembled code: %s\n", str)
	}

	vm.WriteContextSlot(vmStateSlotIndex, uint64(0), 8) // reset vm state
	vm.MachineState = 0

	gas, err := vm.ReadContextSlot(gasSlotIndex)
	if err != nil {
		return fmt.Errorf("failed to read gas from context slot: %w", err)
	}
	vm.Gas = int64(gas)
	if err := vm.sandBox.Start(codeBase, codeBase+uint64(len(codeAddr))); err != nil {
		vm.ResultCode = PANIC
		vm.MachineState = PANIC
		vm.saveRegistersOnceSandBox()
		vm.saveMemoryOnceSandBox()
		return fmt.Errorf("emulation failed: %w", err)
	}
	// get the pc out
	vm.pc, _ = vm.ReadContextSlot(pcSlotIndex)
	// vm state
	vmState, _ := vm.ReadContextSlot(vmStateSlotIndex)
	host_id, _ := vm.ReadContextSlot(hostFuncIdIndex)
	if vmState == HOST {
		vm.hostCall = true
		vm.host_func_id = int(host_id) // reset host function ID
	}
	vm.MachineState = uint8(vmState)
	return nil
}
func (mu *Emulator) SetMemAccessSandBox(address uint32, length uint32, access int) error {
	if length == 0 {
		return nil
	}

	// Check for overflow in address + length - 1
	if address > ^uint32(0)-(length-1) {
		return fmt.Errorf("invalid address/length: address=0x%x, length=0x%x", address, length)
	}

	// Calculate start and end page indices
	startPage := int(address / PageSize)
	endAddress := address + length - 1
	endPage := int(endAddress / PageSize)

	// Validate page indices
	if startPage < 0 || startPage >= TotalPages || endPage < 0 || endPage >= TotalPages {
		return fmt.Errorf(
			"invalid address range: 0x%x(len=0x%x) spans pages %d~%d, total pages=%d",
			address, length, startPage, endPage, TotalPages,
		)
	}

	// Set access for each page in the range
	for page := startPage; page <= endPage; page++ {
		// fmt.Printf("Setting access for page %d: %d\n", page, access)
		// fmt.Printf("Address: 0x%x, Length: 0x%x, Page: %d\n", address, length, page)
		if err := mu.SetPageAccessSandBox(page, access); err != nil {
			return fmt.Errorf("failed to set access on page %d: %w", page, err)
		}
	}
	return nil
}

func (mu *Emulator) allocatePages(startPage uint32, count uint32) {
	for i := uint32(0); i < count; i++ {
		pageIndex := startPage + i
		mu.SetPageAccessSandBox(int(pageIndex), PageMutable)
	}
}

// saveRegistersOnceSandBox saves the current state of the registers to the VM's RAM, including the gasUsed.
func (vm *RecompilerSandboxVM) saveRegistersOnceSandBox() {
	if vm.savedRegisters {
		return
	}
	for i := range vm.ReadRegisters() {
		val, err := vm.sandBox.RegRead(sandBoxRegInfoList[i])
		if err != nil {
			val = 0
		}
		vm.WriteRegister(i, val)
	}
	vm.saveGasFromMemory()
	vm.savedRegisters = true
}

// saveRegisters saves the current state of the registers to the VM's RAM, including the gasUsed.
func (vm *RecompilerSandboxVM) saveRegisters() {
	for i := range vm.ReadRegisters() {
		val, err := vm.sandBox.RegRead(sandBoxRegInfoList[i])
		// fmt.Printf("Saving register %d: 0x%X\n", i, val)
		if err != nil {
			val = 0
		}
		vm.WriteRegister(i, val)
	}
	vm.saveGasFromMemory()
}

func (rvm *RecompilerSandboxVM) saveGasFromMemory() {
	gasRegMemAddr := uint64(rvm.regDumpAddr) + uint64(len(regInfoList)*8)
	rawGasByte, _ := rvm.sandBox.MemRead(gasRegMemAddr, 8)
	rawGasUsed := int64(binary.LittleEndian.Uint64(rawGasByte))
	//fmt.Printf("!Saving gas used from memory: %d (%x @ Addr:%x)\n", rawGasUsed, rawGasByte, gasRegMemAddr)
	rvm.Gas = rawGasUsed
}

func (rvm *RecompilerSandboxVM) saveMemoryOnceSandBox() {
	if rvm.savedMemory {
		return
	}
	rvm.realMemory, _ = rvm.sandBox.MemRead(uint64(rvm.realMemAddr), uint64(len(rvm.realMemory)))
	rvm.savedMemory = true
}

// SetPageAccess sets the memory protection of a single page using BaseReg as memory base.
func (mu *Emulator) SetPageAccessSandBox(pageIndex int, access int) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}
	mu.pageAccess[pageIndex] = access
	return nil
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (mu *Emulator) ReadMemorySandBox(address uint32, length uint32) ([]byte, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return nil, fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}
	godMode := false
	godMap := make(map[int]int)
	tmpMap := mu.pageAccess
	if godMode {
		for i := 0; i < TotalPages; i++ {
			godMap[i] = PageMutable // allow all pages to be readable
		}
		mu.pageAccess = godMap
		data, err := mu.MemRead(uint64(mu.realMemAddr+uintptr(address)), uint64(length))
		mu.pageAccess = tmpMap // restore original page access
		if err != nil {
			return nil, fmt.Errorf("read memory at address %x: %w", address, err)
		}
		return data, nil
	}
	if access, ok := mu.pageAccess[pageIndex]; !ok || access == PageInaccessible {
		return nil, fmt.Errorf("read access denied for page %d at address 0x%X", pageIndex, address)
	}
	return mu.MemRead(uint64(mu.realMemAddr+uintptr(address)), uint64(length))
}

// WriteMemory writes data to a specific address in the memory if it's writable.
func (mu *Emulator) WriteMemorySandBox(address uint32, data []byte) error {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}
	if access, ok := mu.pageAccess[pageIndex]; !ok || access != PageMutable {
		return fmt.Errorf("write access denied for page %d at address 0x%X", pageIndex, address)
	}
	return mu.MemWrite(uint64(mu.realMemAddr+uintptr(address)), data)
}

// Standard_Program_Initialization initializes the program memory and registers
func (vm *RecompilerSandboxVM) Init(argument_data_a []byte) (err error) {

	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	//1)
	// o_byte
	o_len := len(vm.o_byte)
	if err = vm.sandBox.SetMemAccessSandBox(Z_Z, uint32(o_len), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (o_byte): %w", err)
	}
	if err = vm.sandBox.WriteMemorySandBox(Z_Z, vm.o_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (o_byte): %w", err)
	}
	if err = vm.sandBox.SetMemAccessSandBox(Z_Z, uint32(o_len), PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (o_byte): %w", err)
	}
	//2)
	//p|o|
	p_o_len := P_func(uint32(o_len))
	if err = vm.sandBox.SetMemAccessSandBox(Z_Z+uint32(o_len), p_o_len, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (p_o_byte): %w", err)
	}

	z_o := Z_func(vm.o_size)
	z_w := Z_func(vm.w_size + vm.z*Z_P)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		fmt.Printf("Required memory exceeds 4GB limit: %d bytes\n", requiredMemory)
		return
	}

	// Calculate memory bounds for debugging output
	ro_data_address := uint32(Z_Z)
	ro_data_address_end := ro_data_address + P_func(vm.o_size)

	// 3)
	// w_byte
	w_addr := 2*Z_Z + z_o
	w_len := uint32(len(vm.w_byte))
	rw_data_address := w_addr
	rw_data_address_end := rw_data_address + P_func(vm.w_size) + vm.z*Z_P

	if err = vm.sandBox.SetMemAccessSandBox(w_addr, w_len, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (w_byte): %w", err)
	}
	if err = vm.sandBox.WriteMemorySandBox(w_addr, vm.w_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (w_byte): %w", err)
	}
	fmt.Printf("[SANDBOX] Wrote w_byte: len=%d to address 0x%x (w_addr=0x%x, z_o=0x%x, o_size=%d, w_size=%d, z=%d)\n",
		len(vm.w_byte), w_addr, w_addr, z_o, vm.o_size, vm.w_size, vm.z)
	if len(vm.w_byte) > 0 {
		fmt.Printf("[SANDBOX] w_byte first 40 bytes: %x\n", vm.w_byte[:min(40, len(vm.w_byte))])
	}
	// 4)
	addr4 := 2*Z_Z + z_o + w_len
	little_z := vm.z
	len4 := P_func(w_len) + little_z*Z_P - w_len
	if err = vm.sandBox.SetMemAccessSandBox(addr4, len4, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr4): %w", err)
	}
	// 5)
	addr5 := uint32(uint64(1)<<32 - uint64(2*Z_Z) - uint64(Z_I) - uint64(P_func(vm.s)))
	len5 := P_func(vm.s)
	stack_address := addr5
	stack_address_end := uint32(uint64(1)<<32 - uint64(2*Z_Z) - uint64(Z_I))
	if err = vm.sandBox.SetMemAccessSandBox(addr5, len5, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr5): %w", err)
	}
	// 6)
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	output_address := argAddr
	output_end := uint32(0xFFFFFFFF)

	if err = vm.sandBox.SetMemAccessSandBox(argAddr, uint32(len(argument_data_a)), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr): %w", err)
	}
	if err = vm.sandBox.WriteMemorySandBox(argAddr, argument_data_a); err != nil {
		return fmt.Errorf("WriteMemory failed (argAddr): %w", err)
	}
	// set it back to immutable
	if err = vm.sandBox.SetMemAccessSandBox(argAddr+uint32(len(argument_data_a)), Z_I, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr+len): %w", err)
	}
	// 7)
	addr7 := argAddr + uint32(len(argument_data_a))
	len7 := argAddr + P_func(uint32(len(argument_data_a))) - addr7
	if err = vm.sandBox.SetMemAccessSandBox(addr7, len7, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr7): %w", err)
	}

	// Print memory bounds like interpreter does
	fmt.Printf("Memory bounds set: RW [0x%x - 0x%x], RO [0x%x - 0x%x], Output [0x%x - 0x%x], Stack [0x%x - 0x%x]\n",
		rw_data_address, rw_data_address_end, ro_data_address, ro_data_address_end, output_address, output_end, stack_address, stack_address_end)

	vm.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.WriteRegister(7, uint64(argAddr))
	vm.WriteRegister(8, uint64(uint32(len(argument_data_a))))
	return nil
}

// WriteRAMBytes(address uint32, data []byte) uint64
// ReadRAMBytes(address uint32, length uint32) ([]byte, uint64)
// allocatePages(startPage uint32, count uint32)

func (mu *Emulator) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}
	if len(data) == 0 {
		return OOB
	}
	if err := mu.WriteMemorySandBox(address, data); err != nil {
		return OOB
	}
	return OK
}

func (mu *Emulator) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	if length == 0 {
		return nil, OOB
	}
	if length == 0 {
		return []byte{}, OK
	}
	data, err := mu.ReadMemorySandBox(address, length)
	if err != nil {
		fmt.Printf("ReadRAMBytes error: %v\n", err)
		return nil, OOB
	}
	return data, OK
}

func (mu *Emulator) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= regSize {
		return 0, OOB // Out of bounds
	}
	value_bytes, err := mu.MemRead(uint64(mu.regDumpAddr+uintptr(index*8)), 8)
	if err != nil {
		fmt.Printf("ReadRegister error: %v\n", err)
		return 0, OOB // Out of bounds
	}
	value := binary.LittleEndian.Uint64(value_bytes)
	return value, OK
}

func (mu *Emulator) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= regSize {
		return OOB // Out of bounds
	}
	if debugRecompiler {
		//fmt.Printf("WriteRegister index: %d, value: 0x%X\n", index, value)
	}
	value_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(value_bytes, value)
	start := index * 8
	if start+8 > len(mu.regDumpMem) {
		return OOB // Out of bounds
	}

	if err := mu.MemWrite(uint64(mu.regDumpAddr+uintptr(start)), value_bytes); err != nil {
		fmt.Printf("WriteRegister error: %v\n", err)
		return OOB // Out of bounds
	}

	return OK // Success
}

// ReadRegisters returns a copy of the current register values.
func (mu *Emulator) ReadRegisters() []uint64 {
	registersCopy := make([]uint64, regSize)
	for i := 0; i < regSize; i++ {
		value, ok := mu.ReadRegister(i)
		if ok != OK {
			fmt.Printf("ReadRegisters error: %v\n", ok)
			return nil // Error occurred, return nil
		}
		registersCopy[i] = value
	}
	return registersCopy
}

func (mu *Emulator) GetDirtyPages() []int {
	var dirtyPages []int
	for i := 0; i < TotalPages; i++ {
		if mu.dirtyPages[i] {
			dirtyPages = append(dirtyPages, i)
		}
	}
	return dirtyPages
}

func (mu *Emulator) GetCurrentHeapPointer() uint32 {
	return mu.current_heap_pointer
}

func (mu *Emulator) SetCurrentHeapPointer(pointer uint32) {
	mu.current_heap_pointer = pointer
}

type EmulatorSnapShot struct {
	Name           string         `json:"name"`
	InitialRegs    []uint64       `json:"initial-regs"`
	InitialPC      uint32         `json:"initial-pc"`
	InitialPageMap map[int]int    `json:"initial-page-map"`
	InitialMemory  map[int][]byte `json:"initial-memory"`
	InitialGas     uint64         `json:"initial-gas"`
	Code           []byte         `json:"program"`
	FailAddress    uint64         `json:"fail-address,omitempty"`
	BaseRegValue   uint64         `json:"base-reg-value,omitempty"`

	BasicBlockNumber uint64 `json:"basic-block-number,omitempty"`
}

func (vm *RecompilerSandboxVM) TakeSnapShot(name string, pc uint32, registers []uint64, gas uint64, failAddress uint64, BaseRegValue uint64, basicBlockNumber uint64) *EmulatorSnapShot {
	snapshot := &EmulatorSnapShot{
		Name:             name,
		InitialRegs:      registers,
		InitialPC:        pc,
		FailAddress:      failAddress,
		InitialPageMap:   vm.sandBox.pageAccess,
		InitialMemory:    vm.GetMemory(),
		InitialGas:       gas,
		Code:             vm.x86Code,
		BaseRegValue:     BaseRegValue,
		BasicBlockNumber: basicBlockNumber,
	}
	return snapshot
}

func (vm *RecompilerSandboxVM) GetMemory() map[int][]byte {
	memory := make(map[int][]byte)

	// 1) collect all the dirty, accessible pages
	var pages []int
	if len(pages) == 0 {
		fmt.Println("No writable memory found in the snapshot.")
		return memory
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
		chunk, errCode := vm.ReadRAMBytes(byteOffset, byteLen)
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
			}
			bytesSaved += (runEnd - runStart + 1) * pageSize
		}

		// move to the next run
		i = j
	}

	return memory
}

func (vm *RecompilerSandboxVM) WriteRegisters() {
	for i, reg := range vm.ReadRegisters() {
		if debugRecompiler {
			fmt.Printf("WriteRegistersSandbox index: %d, value: 0x%X\n", i, reg)
		}
		if err := vm.sandBox.RegWrite(sandBoxRegInfoList[i], reg); err != nil {
			fmt.Printf("WriteRegistersSandbox error: %v\n", err)
			return
		}
	}
}

func (vm *RecompilerSandboxVM) WriteContextSlot(slot_index int, value uint64, size int) error {
	if vm.regDumpAddr == 0 {
		return fmt.Errorf("regDumpAddr is not initialized")
	}
	addr := vm.regDumpAddr + uintptr(slot_index*8)
	switch size {
	case 4:
		var buf [8]byte
		binary.LittleEndian.PutUint32(buf[:4], uint32(value))
		return vm.sandBox.MemWrite(uint64(addr), buf[:4])
	case 8:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], value)
		return vm.sandBox.MemWrite(uint64(addr), buf[:])
	default:
		return fmt.Errorf("unsupported size: %d", size)
	}
}

func (vm *RecompilerSandboxVM) ReadContextSlot(slot_index int) (uint64, error) {
	if vm.regDumpAddr == 0 {
		return 0, fmt.Errorf("regDumpAddr is not initialized")
	}
	addr := vm.regDumpAddr + uintptr(slot_index*8)
	var value uint64
	// just read it out
	data, err := vm.sandBox.MemRead(uint64(addr), 8)
	if err != nil {
		return 0, fmt.Errorf("failed to read from regDumpMem at index %d: %w", slot_index, err)
	}
	if len(data) < 8 {
		return 0, fmt.Errorf("not enough data to read from regDumpMem at index %d", slot_index)
	}
	value = binary.LittleEndian.Uint64(data)
	return value, nil
}

func (vm *RecompilerSandboxVM) InvokeSbrk(registerA uint32, registerIndexD uint32) (result uint32) {
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
	vm.SetHeapPointer(new_heap_pointer)
	return result
}
func (vm *RecompilerSandboxVM) HandleSbrk() error {
	regAint, err1 := vm.ReadContextSlot(sbrkAIndex)
	regDint, err2 := vm.ReadContextSlot(sbrkDIndex)
	if err1 != nil || err2 != nil {
		return fmt.Errorf("Sbrk call at PC %d missing register info", vm.pc)
	}
	regA := uint32(regAint)
	regD := uint32(regDint)
	vm.InvokeSbrk(regA, regD)
	fmt.Printf("%d RecompilerVM: HandleSbrk at PC %d: regA=%d, regD=%d, registers:%v\n",
		vm.Service_index, vm.GetPC(), regA, regD, vm.ReadRegisters())
	return nil
}

func (rvm *RecompilerSandboxVM) allocatePages(startPage uint32, count uint32) {
	for i := uint32(0); i < count; i++ {
		pageIndex := startPage + i
		rvm.sandBox.SetPageAccessSandBox(int(pageIndex), PageMutable)
	}
}
func (rvm *RecompilerSandboxVM) Panic(uint64) {
	rvm.MachineState = PANIC
	rvm.WriteContextSlot(vmStateSlotIndex, uint64(PANIC), 8)
}
