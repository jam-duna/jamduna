package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
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
	pageSize  = uint64(0x1000)                 // 4 KiB
	guestBase = uint64(0x10000000)             // guest RAM start
	guestSize = uint64(4 * 1024 * 1024 * 1024) // 4 GiB
)

var debugRecompiler = false
var showDisassembly = false
var UseTally = false     // use tally for x86 instructions
var useEcalli500 = false // use ecalli500 for log check in x86
func SetUseEcalli500(enabled bool) {
	useEcalli500 = enabled
}

var isSaveLog = false

func SetDebugRecompiler(enabled bool) {
	debugRecompiler = enabled
}

type RecompilerSandboxVM struct {
	RecompilerVM
	sandBox        uc.Unicorn
	pageAccess     map[int]int
	dirtyPages     map[int]bool // pages that were modified
	savedRegisters bool
	savedMemory    bool
	ecallAddr      uint64
	sbrkOffset     uint64
	stepNumber     int
	snapshot       *EmulatorSnapShot
	post_register  []uint64 // registers after execution
}

// NewRecompilerVM_SandBox creates a Unicorn sandbox with 4 GiB guest RAM
// and an extra dump page at the very end of that RAM region.
func NewRecompilerSandboxVM(vm *VM) (*RecompilerSandboxVM, error) {

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

	ecallAddr := guestBase + guestMainSize
	if err := mu.MemMap(ecallAddr, 0x1000); err != nil {
		mu.Close()
		return nil, fmt.Errorf("map ecall page: %w", err)
	}
	if err := mu.MemProtect(ecallAddr, 0x1000, uc.PROT_ALL); err != nil {
		mu.Close()
		return nil, fmt.Errorf("protect ecall page: %w", err)
	}

	rvm := &RecompilerSandboxVM{
		RecompilerVM: RecompilerVM{
			VM: vm,

			regDumpMem:      make([]byte, dumpSize), // buffer to copy registers
			regDumpAddr:     uintptr(dumpAddr),
			realMemory:      nil, // defer heavy allocation
			realMemAddr:     uintptr(guestBase),
			startCode:       make([]byte, 0),
			exitCode:        make([]byte, 0),
			JumpTableMap:    make([]uint64, 0),
			InstMapX86ToPVM: make(map[int]uint32),
			InstMapPVMToX86: make(map[uint32]int),
		},
		sandBox:    mu,
		pageAccess: make(map[int]int),
		dirtyPages: make(map[int]bool),
		ecallAddr:  ecallAddr,
		sbrkOffset: 0x1,
	}
	rvm.current_heap_pointer = rvm.RecompilerVM.VM.Ram.GetCurrentHeapPointer()
	rvm.RecompilerVM.VM.Ram = rvm

	rvm.sandBox.HookAdd(uc.HOOK_MEM_READ, func(mu uc.Unicorn, access int,
		addr uint64, size int, value int64) {
		// compute the page index
		if addr < guestBase || addr >= guestBase+guestMainSize {
			return // ignore accesses outside guest RAM
		}
		offset := addr - guestBase
		pageIndex := int(offset / pageSize)
		if access, ok := rvm.pageAccess[pageIndex]; !ok || access == PageInaccessible {
			fmt.Printf("❌ Access denied for page %d at address 0x%X\n", pageIndex, offset)
			rvm.ResultCode = types.WORKRESULT_PANIC
			rvm.terminated = true
			for i := range vm.Ram.ReadRegisters() {
				val, _ := rvm.sandBox.RegRead(sandBoxRegInfoList[i])
				vm.Ram.WriteRegister(i, val)
			}
			rvm.saveLogs()
			rvm.saveRegistersOnceSandBox()
			rvm.sandBox.Stop()
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
		if debugRecompiler {
			fmt.Printf("✍️ WRITE @ 0x%X (%d bytes) = 0x%X\n", addr, size, value)
		}
		// ignore writes outside guest RAM
		baseRegValue, err := rvm.sandBox.RegRead(uc.X86_REG_R12)
		if err != nil {
			fmt.Printf("❌ Failed to read R12: %v\n", err)
			return
		}

		if addr < guestBase || addr >= guestBase+guestMainSize {
			return
		} else {
			if baseRegValue != guestBase {
				panic("R12 base register is not set to guestBase")
			}
		}
		// compute the page index
		offset := addr - guestBase
		pageIndex := int(offset / pageSize)
		if access, ok := rvm.pageAccess[pageIndex]; !ok || access == PageInaccessible || access == PageImmutable {
			if !ok {
				fmt.Printf("🔒 Page %d at address 0x%X is not initialized\n", pageIndex, offset)
			} else if access == PageInaccessible {
				fmt.Printf("🔒 Page %d at address 0x%X is inaccessible\n", pageIndex, offset)
			} else if access == PageImmutable {
				fmt.Printf("🔒 Page %d at address 0x%X is immutable\n", pageIndex, offset)
			}
			fmt.Printf("❌ Access denied for page %d at address 0x%X\n", pageIndex, offset)
			rvm.ResultCode = types.WORKRESULT_PANIC
			rvm.terminated = true
			rvm.saveLogs()
			rvm.saveRegistersOnceSandBox()
			rvm.saveMemoryOnceSandBox()
		} else {
			rvm.dirtyPages[pageIndex] = true // mark page as dirty
		}
	}, 1, 0, 0)
	rvm.sandBox.HookAdd(unicorn.HOOK_MEM_INVALID, func(uc unicorn.Unicorn, access int, address uint64, size int, value int64) bool {
		fmt.Printf("⚠️ Invalid memory access: access=%d addr=0x%X size=%d value=%d\n",
			access, address, size, value)
		return false // return false to stop the emulation
	}, 1, 0, 0)
	rvm.sandBox.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
		if addr == uint64(rvm.ecallAddr) {

			rvm_address, err := rvm.sandBox.RegRead(uc.X86_REG_RDI)
			if err != nil {
				fmt.Printf("❌ Failed to read RDI: %v\n", err)
				return
			}
			//read esi for the opcode
			opcode, err := rvm.sandBox.RegRead(uc.X86_REG_ESI)
			if err != nil {
				fmt.Printf("❌ Failed to read ESI: %v\n", err)
				return
			}
			// call ecalli function
			EcalliSandBox(unsafe.Pointer(uintptr(rvm_address)), int32(opcode))
			// fmt.Printf("🧭 Ecalli called at 0x%X with opcode %d\n", addr, opcode)
			esp, _ := mu.RegRead(uc.X86_REG_RSP)
			retAddrBytes, _ := mu.MemRead(esp, 8)
			retAddr := binary.LittleEndian.Uint64(retAddrBytes)
			mu.RegWrite(uc.X86_REG_RIP, retAddr)
			mu.RegWrite(uc.X86_REG_RSP, esp+8)
		}
		if addr == uint64(rvm.ecallAddr+rvm.sbrkOffset) {
			//fmt.Printf("🧭 Sbrk called at 0x%X\n", addr)
			rvm_address, err := rvm.sandBox.RegRead(uc.X86_REG_RDI)
			if err != nil {
				fmt.Printf("❌ Failed to read RDI: %v\n", err)
				return
			}
			//read esi for regA
			regIdxA, err := rvm.sandBox.RegRead(uc.X86_REG_ESI)
			if err != nil {
				fmt.Printf("❌ Failed to read ESI: %v\n", err)
				return
			}
			//read edx for regD
			regIdxD, err := rvm.sandBox.RegRead(uc.X86_REG_EDX)
			if err != nil {
				fmt.Printf("❌ Failed to read EDI: %v\n", err)
				return
			}
			// call sbrk function
			SbrkSandBox(unsafe.Pointer(uintptr(rvm_address)), uint32(regIdxA), uint32(regIdxD))
			esp, _ := mu.RegRead(uc.X86_REG_RSP)
			retAddrBytes, _ := mu.MemRead(esp, 8)
			retAddr := binary.LittleEndian.Uint64(retAddrBytes)
			mu.RegWrite(uc.X86_REG_RIP, retAddr)
			mu.RegWrite(uc.X86_REG_RSP, esp+8)
		}
	}, 1, 0, 0) // 1~0 == entire range

	return rvm, nil
}

func (vm *RecompilerSandboxVM) Close() {
	vm.RecompilerVM.Close()
	if vm.sandBox != nil {
		if err := vm.sandBox.Close(); err != nil {
			log.Error(vm.logging, "Failed to close Unicorn sandbox", "error", err)
		}
		vm.sandBox = nil
	}
}
func (vm *RecompilerSandboxVM) Compile(startStep uint64) {
	// init the recompiler
	vm.basicBlocks = make(map[uint64]*BasicBlock)
	vm.x86Blocks = make(map[uint64]*BasicBlock)
	vm.x86PC = 0
	vm.x86Code = make([]byte, 0)

	// emit start block
	block := NewBasicBlock(vm.x86PC)
	block.X86Code = vm.startCode
	block.JumpType = FALLTHROUGH_JUMP
	vm.basicBlocks[0] = block
	vm.appendBlock(block)

	pc := startStep
	for pc < uint64(len(vm.code)) {
		block := vm.translateBasicBlock(pc)
		if block == nil {
			break
		}
		pc = block.PVMNextPC
	}

	// add exitCode
	block = NewBasicBlock(vm.x86PC)
	block.JumpType = TERMINATED
	vm.basicBlocks[block.X86PC] = block
	vm.appendBlock(block)
}
func generateGasCheck(memAddr uint64, gasCharge uint32) []byte {
	var code []byte
	// 4G + 14*8
	offset := int64(14*8 - 0x100000)
	// fmt.Printf("generateGasCheck: memAddr=0x%X, gasCharge=%d, offset=%d\n", memAddr, gasCharge, offset)
	code = append(code, generateSubMem64Imm32(BaseReg, offset, gasCharge)...)

	// JNS skip_trap
	jumpPos := len(code)
	code = append(code, 0x79, 0x00) // JNS rel8
	// INT3 (trap)
	code = append(code, 0xCC)

	// Patch rel8
	code[jumpPos+1] = byte(len(code) - (jumpPos + 2)) // calculate relative jump distance

	// fmt.Printf("disassembled gas check code: %s\n", Disassemble(code))
	return code
}
func generateSubMem64Imm32(reg X86Reg, offset int64, gasCost uint32) []byte {
	var code []byte

	// --- REX Prefix ---
	rex := byte(0x48) // REX.W = 1
	if reg.REXBit == 1 {
		rex |= 0x01 // REX.B = 1
	}
	code = append(code, rex)

	// --- Opcode: 81 /5 (SUB r/m64, imm32) ---
	code = append(code, 0x81)

	// --- ModRM ---
	// mod = 10 (disp32), reg = 5 (SUB), rm = reg.RegBits
	modrm := byte(0x80 | (5 << 3) | (reg.RegBits & 0x07))
	code = append(code, modrm)

	// --- SIB (required for r12/r13/r14/r15) ---
	if reg.RegBits&0x07 == 4 {
		// SIB: scale=0, index=none(100), base=100 (RSP/r12)
		sib := byte(0x24)
		code = append(code, sib)
	}

	// --- disp32 ---
	disp := make([]byte, 4)
	binary.LittleEndian.PutUint32(disp, uint32(offset))
	code = append(code, disp...)

	// --- imm32 ---
	imm := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm, uint32(gasCost))
	code = append(code, imm...)

	return code
}

func (vm *RecompilerSandboxVM) translateBasicBlock(startPC uint64) *BasicBlock {
	pc := startPC
	block := NewBasicBlock(vm.x86PC)

	hitBasicBlock := false
	for pc < uint64(len(vm.code)) {
		op := vm.code[pc]

		olen := vm.skip(pc)
		operands := vm.code[pc+1 : pc+1+olen]
		if pc == 0 && op == JUMP && debugRecompiler {
			fmt.Printf("JUMP at PC %d with operands %x\n", pc, operands)
			fmt.Printf("operands length: %d\n", olen)
			fmt.Printf("code hash %v", common.Blake2Hash(vm.code))
		}
		if op == ECALLI {
			lx := uint32(types.DecodeE_l(operands))
			host_func_id := int(lx)
			block.GasUsage += int64(vm.chargeGas(host_func_id))
			if debugRecompiler && false {
				fmt.Printf("ECALLI at PC %d with operands %x\n", pc, operands)
			}
		}
		block.AddInstruction(op, operands, int(pc), pc)
		pc0 := pc
		pc += uint64(olen) + 1
		block.GasUsage += 1
		if IsBasicBlockInstruction(op) {
			vm.setJumpMetadata(block, op, operands, pc0)

			hitBasicBlock = true

			break
		}
	}

	if len(block.Instructions) == 0 {
		return nil
	}

	// translate the instructions to x86 code
	var code []byte
	for i, inst := range block.Instructions {
		pvm_opcode := inst.Opcode
		if pvm_opcode == ECALLI {
			// 1. Dump registers to memory.
			// 2. Set up C ABI registers (rdi, esi).
			opcode := uint32(types.DecodeE_l(inst.Args))
			Ecallcode := append(vm.DumpRegisterToMemory(true), EmitCallToEcalliStubSandBox(uintptr(unsafe.Pointer(vm)), int(opcode), uint64(vm.ecallAddr))...)
			Ecallcode = append(Ecallcode, vm.RestoreRegisterInX86()...)
			codeLen := len(code)
			code = append(code, Ecallcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if pvm_opcode == SBRK {
			dstIdx, srcIdx := extractTwoRegisters(inst.Args)
			Sbrkcode := append(vm.DumpRegisterToMemory(true), EmitCallToSbrkStubSandBox(uintptr(unsafe.Pointer(vm)), uint32(srcIdx), uint32(dstIdx), uint64(vm.ecallAddr+vm.sbrkOffset))...)
			// Sbrkcode = append(Sbrkcode, vm.RestoreRegisterInX86()...)
			codeLen := len(code)
			code = append(code, Sbrkcode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		} else if translateFunc, ok := pvmByteCodeToX86Code[pvm_opcode]; ok {
			codeLen := len(code)
			if i == 0 {
				gasMemAddr := uint64(vm.regDumpAddr + uintptr(len(regInfoList)*8))
				code = append(code, generateGasCheck(gasMemAddr, uint32(block.GasUsage))...)
			}
			if i == len(block.Instructions)-1 {
				block.LastInstructionOffset = len(code)
			}
			additionalCode := translateFunc(inst)
			//disassemble the additionalCode and build the tally map
			if UseTally {
				vm.DisassembleAndTally(pvm_opcode, additionalCode)
			}
			code = append(code, additionalCode...)
			block.pvmPC_TO_x86Index[uint32(inst.Pc)] = codeLen
		}
	}
	// add a trap if we didn't hit a basic block instruction at the end
	if !hitBasicBlock {
		code = append(code, generateTrap(Instruction{
			Opcode: TRAP,
			Args:   nil,
		})...)
		block.JumpType = TRAP_JUMP
	}

	block.X86Code = code
	block.PVMNextPC = pc
	vm.basicBlocks[startPC] = block
	vm.x86Blocks[block.X86PC] = block

	vm.appendBlock(block)
	str := vm.Disassemble(block.X86Code)
	if showDisassembly {
		fmt.Printf("Translated block at PVM PC %d to x86 PC %x with %d instructions: %s\n%s", startPC, block.X86PC, len(block.Instructions), block.String(), str)
	}
	return block
}

const stackSize = uint64(0x400000) //4mb
const stackTop = uint64(0x1A0000000)

func (vm *RecompilerSandboxVM) ExecuteX86Code_SandBox(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.terminated = true
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()

	// --------------------------------------------------------------------
	// 1. Prepare jump-table and compute aligned memory region
	// --------------------------------------------------------------------
	vm.initDJumpFunc(len(x86code))
	codeLen := len(x86code) + len(vm.djumpTableFunc)
	codeLenAligned := (uint64(codeLen) + pageSize - 1) & ^(pageSize - 1)

	// --------------------------------------------------------------------
	// 2. Allocate a new memory region for code
	// --------------------------------------------------------------------
	if err := vm.sandBox.MemMap(codeBase, codeLenAligned); err != nil {
		return fmt.Errorf("MemMap failed: %w", err)
	}
	if err := vm.sandBox.MemProtect(codeBase, codeLenAligned, uc.PROT_ALL); err != nil {
		return fmt.Errorf("MemProtect failed: %w", err)
	}

	vm.codeAddr = uintptr(codeBase)
	vm.djumpAddr = uintptr(codeBase + uint64(len(x86code)))
	vm.finalizeJumpTargets(vm.J)
	// --------------------------------------------------------------------
	// 3. Write code and jump table into the allocated memory
	// --------------------------------------------------------------------
	if err := vm.sandBox.MemWrite(codeBase, x86code); err != nil {
		return fmt.Errorf("write main code: %w", err)
	}
	if err := vm.sandBox.MemWrite(codeBase+uint64(len(x86code)), vm.djumpTableFunc); err != nil {
		return fmt.Errorf("write jump table: %w", err)
	}

	// read the code out and print the disassembled instructions
	codeBytes, err := vm.sandBox.MemRead(codeBase, uint64(codeLen))
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	// Print disassembled code for debugging
	str := vm.Disassemble(codeBytes)
	if showDisassembly {
		fmt.Printf("disassembled code:\n")
		fmt.Printf("%s\n", str)
	}
	// --------------------------------------------------------------------
	// 4. Add basic-block hook
	// --------------------------------------------------------------------
	if _, err = vm.sandBox.HookAdd(
		uc.HOOK_BLOCK,
		func(mu uc.Unicorn, addr uint64, size uint32) {
			// fmt.Printf("🧱 Basic Block: 0x%X (%d bytes)\n", addr, size)
		},
		0, // userData
		codeBase, int(codeBase+codeLenAligned-1),
	); err != nil {
		return fmt.Errorf("add hook: %w", err)
	}
	vm.sandBox.HookAdd(
		uc.HOOK_MEM_WRITE_UNMAPPED,
		func(mu uc.Unicorn, access int, addr uint64, size int, value int64) bool {
			fmt.Printf("❌ WRITE_UNMAPPED @ 0x%X (size=%d, value=0x%X)\n", addr, size, value)
			return false
		},
		1, 0, 0,
	)
	_, err = vm.sandBox.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
		offset := addr - codeBase
		instruction := vm.x86Instructions[int(offset)]
		if debugRecompiler {
			fmt.Printf("🧭 Executing @ 0x%X %s\n", addr, instruction.String())
		}
		if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
			vm.saveRegisters()
			basicBlock, ok2 := vm.basicBlocks[uint64(pvm_pc)]
			if !ok2 {
				// fmt.Printf("⚠️ No basic block found for PVM PC %d at 0x%X ...\n", pvm_pc, offset)
				return
			}
			//vm.Gas -= basicBlock.GasUsage
			fmt.Printf("PVM PC: %d [%s] reg:%v  BasicBlock: %s ⛽️ Gas remaining: %d, GasUsage %d\n", pvm_pc, opcode_str(vm.code[pvm_pc]), vm.Ram.ReadRegisters(), basicBlock.String(), vm.Gas, basicBlock.GasUsage)
			if vm.Gas < 0 {
				fmt.Printf("⛔️ Gas limit exceeded at 0x%X\n",
					addr)
				vm.ResultCode = types.RESULT_OOG
				vm.terminated = true
				vm.saveRegistersOnceSandBox()
				vm.saveMemoryOnceSandBox()
				return
			}
			fmt.Printf("n")
		} else {
			// fmt.Printf("⚠️ No basic block found for instruction at 0x%X\n", offset)s
		}
	}, 1, 0, 0) // 1~0 == entire range
	if err != nil {
		return fmt.Errorf("add code hook: %w", err)
	}
	vm.sandBox.HookAdd(uc.HOOK_MEM_READ_UNMAPPED|uc.HOOK_MEM_WRITE_UNMAPPED|uc.HOOK_MEM_FETCH_UNMAPPED,
		func(uc uc.Unicorn, access int, addr uint64, size int, value int64) bool {
			fmt.Printf("[Unmapped Access] Access=%d Addr=0x%X Size=%d\n", access, addr, size)
			return false // return false to stop the emulation
		},
		1, 0, 0)

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
	fmt.Printf("RSP = 0x%X\n", rsp)
	// --------------------------------------------------------------------
	// 5. Run the code
	// --------------------------------------------------------------------
	if err := vm.sandBox.Start(codeBase, codeBase+uint64(codeLen)); err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.terminated = true
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

	for i := range vm.Ram.ReadRegisters() {
		val := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = 0x%X\n", regInfoList[i].Name, val)
		vm.Ram.WriteRegister(i, val)
	}
	return nil
}

const (
	codeBase = uint64(0x190000000) // separate memory area outside 4GiB RAM
)

func (vm *RecompilerSandboxVM) Patch(x86code []byte, entry uint32) (err error) {
	// --------------------------------------------------------------------
	// 1. Prepare jump-table and compute aligned memory region
	// --------------------------------------------------------------------
	vm.initDJumpFunc(len(x86code))

	vm.codeAddr = uintptr(codeBase)
	vm.djumpAddr = uintptr(codeBase + uint64(len(x86code)))
	vm.finalizeJumpTargets(vm.J)
	var patchInstIdx = -1
	entryPatchImm := entryPatch
	// use entryPatch as a placeholder 0x99999999
	//get the x86 pc
	x86PC, ok := vm.InstMapPVMToX86[entry]
	if !ok {
		return fmt.Errorf("entry %d not found in InstMapPVMToX86", entry)
	}
	if debugRecompiler {
		fmt.Printf("Executing code at x86 PC: %d (PVM PC: %d)\n", x86PC, entry)
	}
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	for i := 0; i < len(x86code)-5; i++ {
		if x86code[i] == 0xE9 && // JMP rel32
			x86code[i+1] == 0x99 &&
			x86code[i+2] == 0x99 &&
			x86code[i+3] == 0x99 &&
			x86code[i+4] == 0x99 {
			// found a placeholder for the entry patch
			patchInstIdx = i
			// replace it with the actual entry patch
			binary.LittleEndian.PutUint32(x86code[i+1:i+5], uint32(x86PC-i-5))
			fmt.Printf("Patching entry point at index %d with 0x%X\n",
				patchInstIdx, entryPatchImm)
			break
		}
	}
	if patchInstIdx == -1 {
		return fmt.Errorf("no entry patch placeholder found in x86 code")
	}
	vm.x86Code = x86code
	return nil
}
func (vm *RecompilerSandboxVM) ExecuteX86Code_SandBox_WithEntry(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.terminated = true
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()
	codeLen := len(x86code) + len(vm.djumpTableFunc)
	codeLenAligned := (uint64(codeLen) + pageSize - 1) & ^(pageSize - 1)

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
	if err := vm.sandBox.MemWrite(codeBase+uint64(len(x86code)), vm.djumpTableFunc); err != nil {
		return fmt.Errorf("write jump table: %w", err)
	}

	codeBytes, err := vm.sandBox.MemRead(codeBase, uint64(codeLen))
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	// Print disassembled code for debugging
	str := vm.Disassemble(codeBytes)
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
	var lastPc = -1
	_, err = vm.sandBox.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
		offset := addr - codeBase
		instruction, ok_x86 := vm.x86Instructions[int(offset)] // should get the op_code here
		if !ok_x86 {
			// fmt.Printf("❌ Invalid instruction at offset 0x%X\n", offset)
		}
		if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
			opcode := vm.code[pvm_pc]
			fmt.Printf("!!! Start PVM PC: %d [%s] reg:%v\n", pvm_pc, opcode_str(opcode), vm.Ram.ReadRegisters())
		}
		baseRegValue, err := mu.RegRead(uc.X86_REG_R12)
		if err != nil {
			fmt.Printf("❌ Failed to read R12: %v\n", err)
			return
		}
		if debugRecompiler {
			fmt.Printf("🧭 Executing @ 0x%X >%s< [baseReg 0x%X]\n", addr, instruction.String(), baseRegValue)
		}
		if instruction.String() == "RET" {
			// Handle RET instruction
			// This is a special case where we need to restore the registers and memory
			//fmt.Printf("🧭 RET instruction encountered at 0x%X\n", addr)
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()

			// Stop the sandbox
			vm.sandBox.Stop()
			return
		}
		if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
			if VMsCompare {
				if lastPc != -1 {
					pc := uint64(lastPc)
					if vm.code[pc] != ECALLI {
						vm.saveRegisters()
					}
					opcode := vm.code[pc] // this is the opCode
					olen := vm.skip(pc)
					operands := vm.code[pc+1 : pc+1+olen]
					vm.LogCurrentState(opcode, operands, pc, vm.Gas)

					// if vm.stepNumber >= maxPVMSteps {
					// 	fmt.Printf("Reached max PVM steps (%d), stopping execution.\n", maxPVMSteps)
					// 	vm.saveLogs()
					// 	os.Exit(0)
					// }
					vm.stepNumber++
				}
				lastPc = int(pvm_pc)
			}
			basicBlock, ok2 := vm.basicBlocks[uint64(pvm_pc)]
			if !ok2 {
				// fmt.Printf("⚠️ No basic block found for PVM PC %d at 0x%X ...\n", pvm_pc, offset)
				return
			} else {
				vm.basicBlockExecutionCounter[uint64(pvm_pc)]++
			}
			if debugRecompiler {
				fmt.Printf("PVMX PC: %d [%s] reg:%v  BasicBlock: %s ⛽️ Gas remaining: %d, GasUsage %d\n", pvm_pc, opcode_str(vm.code[pvm_pc]),
					vm.Ram.ReadRegisters(), basicBlock.String(), vm.Gas, basicBlock.GasUsage)
			}
			//vm.Gas -= basicBlock.GasUsage
			if vm.Gas < 0 {
				fmt.Printf("⛔️ Gas limit exceeded at 0x%X\n",
					addr)
				vm.ResultCode = types.RESULT_OOG
				vm.terminated = true
				vm.saveRegistersOnceSandBox()
				vm.saveMemoryOnceSandBox()
				return
			}
		} else {
			// fmt.Printf("⚠️ No basic block found for instruction at 0x%X\n", offset)s
		}
	}, 1, 0, 0) // 1~0 == entire range
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
	fmt.Printf("RSP = 0x%X\n", rsp)
	// --------------------------------------------------------------------
	// 5. Run the code
	// --------------------------------------------------------------------
	if err := vm.sandBox.Start(codeBase, codeBase+uint64(codeLen)); err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.terminated = true
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

	for i := range vm.Ram.ReadRegisters() {
		val := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = 0x%X\n", regInfoList[i].Name, val)
		vm.Ram.WriteRegister(i, val)
	}
	return nil
}

func (rvm *RecompilerSandboxVM) ExecuteSandBox(entryPoint uint64) error {
	rvm.initStartCode()
	rvm.Compile(rvm.pc)

	start := time.Now()
	rvm.Patch(rvm.x86Code, uint32(entryPoint))
	if execErr := rvm.ExecuteX86Code_SandBox_WithEntry(rvm.x86Code); execErr != nil {
		fmt.Printf("ExecuteX86 crash detected: %v\n", execErr)
	}
	if UseTally {
		// timestamp suffix
		ts := time.Now().UnixMilli()
		jsonFile := fmt.Sprintf("test/%s_%d.json", VM_MODE, ts)

		// ensure the directory exists (mkdir -p)
		dir := filepath.Dir(jsonFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("failed to create directory %q: %v\n", dir, err)
		}
		// dump JSON tally
		if err := rvm.TallyJSON(jsonFile); err != nil {
			fmt.Printf("failed to write JSON tally: %v\n", err)
		}

	}

	fmt.Printf(
		"**** ExecuteSandBox %s finished, ResultCode: %d Time: %s\n",
		rvm.Mode,
		rvm.ResultCode,
		time.Since(start),
	)
	return nil
}

func (rvm *RecompilerSandboxVM) SetMemAccessSandBox(address uint32, length uint32, access int) error {
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
		if err := rvm.SetPageAccessSandBox(page, access); err != nil {
			return fmt.Errorf("failed to set access on page %d: %w", page, err)
		}
	}
	return nil
}

func (rvm *RecompilerSandboxVM) allocatePages(startPage uint32, count uint32) {
	for i := uint32(0); i < count; i++ {
		pageIndex := startPage + i
		rvm.SetPageAccessSandBox(int(pageIndex), PageMutable)
	}
}

// saveRegistersOnceSandBox saves the current state of the registers to the VM's RAM, including the gasUsed.
func (vm *RecompilerSandboxVM) saveRegistersOnceSandBox() {
	if vm.savedRegisters {
		return
	}
	for i := range vm.Ram.ReadRegisters() {
		val, err := vm.sandBox.RegRead(sandBoxRegInfoList[i])
		if err != nil {
			val = 0
		}
		vm.Ram.WriteRegister(i, val)
	}
	vm.saveGasFromMemory()
	vm.savedRegisters = true
}

// saveRegisters saves the current state of the registers to the VM's RAM, including the gasUsed.
func (vm *RecompilerSandboxVM) saveRegisters() {
	for i := range vm.Ram.ReadRegisters() {
		val, err := vm.sandBox.RegRead(sandBoxRegInfoList[i])
		// fmt.Printf("Saving register %d: 0x%X\n", i, val)
		if err != nil {
			val = 0
		}
		vm.Ram.WriteRegister(i, val)
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
func (rvm *RecompilerSandboxVM) SetPageAccessSandBox(pageIndex int, access int) error {
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid page index")
	}
	rvm.pageAccess[pageIndex] = access
	return nil
}

// ReadMemory reads data from a specific address in the memory if it's readable.
func (rvm *RecompilerSandboxVM) ReadMemorySandBox(address uint32, length uint32) ([]byte, error) {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return nil, fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}
	godMode := false
	godMap := make(map[int]int)
	tmpMap := rvm.pageAccess
	if godMode {
		for i := 0; i < TotalPages; i++ {
			godMap[i] = PageMutable // allow all pages to be readable
		}
		rvm.pageAccess = godMap
		data, err := rvm.sandBox.MemRead(uint64(rvm.realMemAddr+uintptr(address)), uint64(length))
		rvm.pageAccess = tmpMap // restore original page access
		if err != nil {
			return nil, fmt.Errorf("read memory at address %x: %w", address, err)
		}
		return data, nil
	}
	return rvm.sandBox.MemRead(uint64(rvm.realMemAddr+uintptr(address)), uint64(length))

}

// WriteMemory writes data to a specific address in the memory if it's writable.
func (rvm *RecompilerSandboxVM) WriteMemorySandBox(address uint32, data []byte) error {
	pageIndex := int(address / PageSize)
	if pageIndex < 0 || pageIndex >= TotalPages {
		return fmt.Errorf("invalid address %x for page index %d", address, pageIndex)
	}

	return rvm.sandBox.MemWrite(uint64(rvm.realMemAddr+uintptr(address)), data)
}

// Standard_Program_Initialization initializes the program memory and registers
func (vm *RecompilerSandboxVM) Standard_Program_Initialization_SandBox(argument_data_a []byte) (err error) {

	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	//1)
	// o_byte
	o_len := len(vm.o_byte)
	if err = vm.SetMemAccessSandBox(Z_Z, uint32(o_len), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (o_byte): %w", err)
	}
	if err = vm.WriteMemorySandBox(Z_Z, vm.o_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (o_byte): %w", err)
	}
	if err = vm.SetMemAccessSandBox(Z_Z, uint32(o_len), PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (o_byte): %w", err)
	}
	//2)
	//p|o|
	p_o_len := P_func(uint32(o_len))
	if err = vm.SetMemAccessSandBox(Z_Z+uint32(o_len), p_o_len, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (p_o_byte): %w", err)
	}

	z_o := Z_func(vm.o_size)
	z_w := Z_func(vm.w_size + vm.z*Z_P)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return
	}
	// 3)
	// w_byte
	w_addr := 2*Z_Z + z_o
	w_len := uint32(len(vm.w_byte))
	if err = vm.SetMemAccessSandBox(w_addr, w_len, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (w_byte): %w", err)
	}
	if err = vm.WriteMemorySandBox(w_addr, vm.w_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (w_byte): %w", err)
	}
	// 4)
	addr4 := 2*Z_Z + z_o + w_len
	little_z := vm.z
	len4 := P_func(w_len) + little_z*Z_P - w_len
	if err = vm.SetMemAccessSandBox(addr4, len4, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr4): %w", err)
	}
	// 5)
	addr5 := 0xFFFFFFFF + 1 - 2*Z_Z - Z_I - P_func(vm.s)
	len5 := P_func(vm.s)
	if err = vm.SetMemAccessSandBox(addr5, len5, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr5): %w", err)
	}
	// 6)
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	if err = vm.SetMemAccessSandBox(argAddr, uint32(len(argument_data_a)), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr): %w", err)
	}
	if err = vm.WriteMemorySandBox(argAddr, argument_data_a); err != nil {
		return fmt.Errorf("WriteMemory failed (argAddr): %w", err)
	}
	// set it back to immutable
	if err = vm.SetMemAccessSandBox(argAddr+uint32(len(argument_data_a)), Z_I, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr+len): %w", err)
	}
	// 7)
	addr7 := argAddr + uint32(len(argument_data_a))
	len7 := argAddr + P_func(uint32(len(argument_data_a))) - addr7
	if err = vm.SetMemAccessSandBox(addr7, len7, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr7): %w", err)
	}

	vm.Ram.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.Ram.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.Ram.WriteRegister(7, uint64(argAddr))
	vm.Ram.WriteRegister(8, uint64(uint32(len(argument_data_a))))
	return nil
}

// WriteRAMBytes(address uint32, data []byte) uint64
// ReadRAMBytes(address uint32, length uint32) ([]byte, uint64)
// allocatePages(startPage uint32, count uint32)

func (vm *RecompilerSandboxVM) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OOB
	}
	if err := vm.WriteMemorySandBox(address, data); err != nil {
		return OOB
	}
	return OK
}

func (vm *RecompilerSandboxVM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	if length == 0 {
		return nil, OOB
	}
	data, err := vm.ReadMemorySandBox(address, length)
	if err != nil {
		fmt.Printf("ReadRAMBytes error: %v\n", err)
		return nil, OOB
	}
	return data, OK
}

func (rvm *RecompilerSandboxVM) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= regSize {
		return 0, OOB // Out of bounds
	}
	value_bytes, err := rvm.sandBox.MemRead(uint64(rvm.regDumpAddr+uintptr(index*8)), 8)
	if err != nil {
		fmt.Printf("ReadRegister error: %v\n", err)
		return 0, OOB // Out of bounds
	}
	value := binary.LittleEndian.Uint64(value_bytes)
	return value, OK
}

func (rvm *RecompilerSandboxVM) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= regSize {
		return OOB // Out of bounds
	}
	if debugRecompiler {
		fmt.Printf("WriteRegister index: %d, value: 0x%X\n", index, value)
	}
	value_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(value_bytes, value)
	start := index * 8
	if start+8 > len(rvm.regDumpMem) {
		return OOB // Out of bounds
	}

	if err := rvm.sandBox.MemWrite(uint64(rvm.regDumpAddr+uintptr(start)), value_bytes); err != nil {
		fmt.Printf("WriteRegister error: %v\n", err)
		return OOB // Out of bounds
	}

	return OK // Success
}

// ReadRegisters returns a copy of the current register values.
func (rvm *RecompilerSandboxVM) ReadRegisters() []uint64 {
	registersCopy := make([]uint64, regSize)
	for i := 0; i < regSize; i++ {
		value, ok := rvm.ReadRegister(i)
		if ok != OK {
			fmt.Printf("ReadRegisters error: %v\n", ok)
			return nil // Error occurred, return nil
		}
		registersCopy[i] = value
	}
	return registersCopy
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
		InitialPageMap:   vm.pageAccess,
		InitialMemory:    vm.GetMemory(),
		InitialGas:       gas,
		Code:             vm.x86Code,
		BaseRegValue:     BaseRegValue,
		BasicBlockNumber: basicBlockNumber,
	}
	return snapshot
}

func (vm *VM) LoadSnapshot(name string) (snapshot *EmulatorSnapShot, err error) {
	filePath := fmt.Sprintf("interpreter/%s.json", name)
	fmt.Printf("Loading snapshot from %s\n", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot file %s: %w", filePath, err)
	}

	err = json.Unmarshal(data, &snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	fmt.Printf("Snapshot loaded from %s [PC=%d, Gas=%d, Regs=%v, BasicBlock=%d]\n", filePath, snapshot.InitialPC, snapshot.InitialGas, snapshot.InitialRegs, snapshot.BasicBlockNumber)
	return snapshot, nil
}

func (vm *RecompilerSandboxVM) SaveSnapShot(snapshot *EmulatorSnapShot) error {
	filePath := fmt.Sprintf("recompiler_sandbox/BB%d.json", snapshot.BasicBlockNumber)
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write snapshot to file %s: %w", filePath, err)
	}
	fmt.Printf("Snapshot %s saved [PC: %d, BasicBlock: %d, Gas: %d Registers: %v]\n", filePath, snapshot.InitialPC, snapshot.BasicBlockNumber, snapshot.InitialGas, snapshot.InitialRegs)
	return nil
}

func (vm *RecompilerSandboxVM) GetMemory() map[int][]byte {
	memory := make(map[int][]byte)

	// 1) collect all the dirty, accessible pages
	var pages []int
	for idx, access := range vm.pageAccess {
		if access != PageInaccessible && vm.dirtyPages[idx] {
			pages = append(pages, idx)
		}
	}
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
			}
			bytesSaved += (runEnd - runStart + 1) * pageSize
		}

		// move to the next run
		i = j
	}

	return memory
}

func (vm *RecompilerSandboxVM) ExecuteFromSnapshot(snapshot *EmulatorSnapShot) error {
	fmt.Println("ExecuteFromSnapshot RECOMPILING")
	vm.initStartCode()
	tm := time.Now()
	vm.Compile(0)
	fmt.Printf("Compile took %s\n", time.Since(tm))
	vm.Patch(vm.x86Code, uint32(0))
	vm.x86Code = append(vm.x86Code, vm.djumpTableFunc...)
	// Load the snapshot data into the VM
	vm.pc = uint64(snapshot.InitialPC)
	vm.Gas = int64(snapshot.InitialGas)
	vm.vmBasicBlock = int(snapshot.BasicBlockNumber)
	GasAddress := uint64(vm.regDumpAddr) + uint64(len(regInfoList)*8)
	vm.sandBox.MemWrite(uint64(GasAddress), encodeU64(snapshot.InitialGas)) // Set the gas address
	vm.pageAccess = snapshot.InitialPageMap

	// Set the initial registers
	fmt.Println("Setting initial registers...")
	for i, regValue := range snapshot.InitialRegs {
		vm.sandBox.RegWrite(sandBoxRegInfoList[i], regValue)
	}
	vm.sandBox.RegWrite(uc.X86_REG_R12, guestBase) // Set the base register value
	vm.post_register = make([]uint64, regSize)
	for i, regV := range snapshot.InitialRegs {
		// Write the initial register values
		vm.Ram.WriteRegister(i, regV)
		vm.post_register[i] = regV
	}

	fmt.Println("TestDoomSnapshot write Initial Memory")
	for pageIndex, mem := range snapshot.InitialMemory {
		address := uint32((pageIndex) * PageSize)
		// Write the initial memory contents
		vm.WriteMemorySandBox(address, mem)
		//fmt.Printf("Writing initial memory at page %d (address 0x%X) %s\n", pageIndex, address, common.Blake2Hash(mem))
		for i := 0; i < len(mem)/PageSize; i++ {
			p := pageIndex + i
			address := uint32(p * PageSize)
			vm.dirtyPages[p] = true // mark page as dirty
			err := vm.SetMemAccessSandBox(address, PageSize, PageMutable)
			if err != nil {
				return err
			}
		}
	}

	for pageIndex, access := range snapshot.InitialPageMap {
		vm.pageAccess[pageIndex] = access
		//	fmt.Printf("p=%d %d len(vm.pageAccess) = %d\n", pageIndex, access, len(vm.pageAccess))
	}
	// print the disabled code (required for now -- do not disable yet)
	str := vm.Disassemble(vm.x86Code)
	if showDisassembly {
		fmt.Printf("Disabled code:\n%s\n", str)
	}

	// vm.x86Code = snapshot.Code
	fmt.Printf("Initial PC: %d, Gas: %d\n", snapshot.InitialPC, snapshot.InitialGas)
	breakpoint := uint64(vm.InstMapPVMToX86[snapshot.InitialPC]) // REVIEW!!!
	fmt.Printf("found breakpoint at 0x%Xs\n", breakpoint)
	breakpoint = breakpoint + codeBase
	return vm.ExecuteX86CodeFromBreakPoint(vm.x86Code, breakpoint)
}

func (vm *RecompilerSandboxVM) ExecuteX86CodeFromBreakPoint(x86code []byte, breakpoint uint64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.terminated = true
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()

	// --------------------------------------------------------------------
	// 1. Prepare jump-table and compute aligned memory region
	// --------------------------------------------------------------------
	codeLen := len(x86code)
	codeLenAligned := (uint64(codeLen) + pageSize - 1) & ^(pageSize - 1)

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
	// --------------------------------------------------------------------
	// 4. Add basic-block hook
	// --------------------------------------------------------------------
	vm.sandBox.HookAdd(
		uc.HOOK_MEM_WRITE_UNMAPPED,
		func(mu uc.Unicorn, access int, addr uint64, size int, value int64) bool {
			fmt.Printf("❌ WRITE_UNMAPPED @ 0x%X (size=%d, value=0x%X)\n", addr, size, value)
			return false
		},
		1, 0, 0,
	)
	var lastPc = -1
	_, err = vm.sandBox.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
		offset := addr - codeBase
		instruction := vm.x86Instructions[int(offset)]
		baseRegValue, err := mu.RegRead(uc.X86_REG_R12)
		if err != nil {
			fmt.Printf("❌ Failed to read R12: %v\n", err)
			return
		}
		if debugRecompiler {
			fmt.Printf("🧭 Executing @ 0x%X %s [baseReg 0x%X]\n", addr, instruction.String(), baseRegValue)
		}
		if instruction.String() == "RET" {
			// Handle RET instruction
			// This is a special case where we need to restore the registers and memory
			//fmt.Printf("🧭 RET instruction encountered at 0x%X\n", addr)
			vm.saveRegistersOnceSandBox()
			vm.saveMemoryOnceSandBox()

			// Stop the sandbox
			vm.sandBox.Stop()
			return
		}
		if pvm_pc, ok := vm.InstMapX86ToPVM[int(offset)]; ok {
			if VMsCompare {
				if lastPc != -1 {
					pc := uint64(lastPc)
					if vm.code[pc] != ECALLI {
						vm.saveRegisters()
					}
					opcode := vm.code[pc]
					olen := vm.skip(pc)
					operands := vm.code[pc+1 : pc+1+olen]
					vm.LogCurrentState(opcode, operands, pc, vm.Gas)
					// if vm.stepNumber >= maxPVMSteps {
					// 	fmt.Printf("Reached max PVM steps (%d), stopping execution.\n", maxPVMSteps)
					// 	vm.saveLogs()
					// 	os.Exit(0)
					// }
					vm.stepNumber++
				}
				lastPc = int(pvm_pc)
			}
			basicBlock, ok2 := vm.basicBlocks[uint64(pvm_pc)]
			if !ok2 {
				// fmt.Printf("⚠️ No basic block found for PVM PC %d at 0x%X ...\n", pvm_pc, offset)
				return
			}
			if debugRecompiler {
				fmt.Printf("PVMX PC: %d [%s] reg:%v  BasicBlock: %s ⛽️ Gas remaining: %d, GasUsage %d\n", pvm_pc, opcode_str(vm.code[pvm_pc]),
					vm.Ram.ReadRegisters(), basicBlock.String(), vm.Gas, basicBlock.GasUsage)
			}
			//vm.Gas -= basicBlock.GasUsage
			if vm.Gas < 0 {
				fmt.Printf("⛔️ Gas limit exceeded at 0x%X\n",
					addr)
				vm.ResultCode = types.RESULT_OOG
				vm.terminated = true
				vm.saveRegistersOnceSandBox()
				vm.saveMemoryOnceSandBox()
				return
			}
		} else {
			// fmt.Printf("⚠️ No basic block found for instruction at 0x%X\n", offset)s
		}
	}, 1, 0, 0) // 1~0 == entire range
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
	// init from the breakpoint
	fmt.Printf("running from breakpoint 0x%X\n", breakpoint)
	if err := vm.sandBox.Start(breakpoint, codeBase+uint64(codeLen)); err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.terminated = true
		vm.saveRegistersOnceSandBox()
		vm.saveMemoryOnceSandBox()
		return fmt.Errorf("emulation failed: %w", err)
	}
	vm.regDumpMem, err = vm.sandBox.MemRead(uint64(vm.regDumpAddr), uint64(len(vm.regDumpMem)))
	if err != nil {
		return fmt.Errorf("read reg dump: %w", err)
	}

	vm.realMemory, err = vm.sandBox.MemRead(uint64(vm.realMemAddr), uint64(len(vm.realMemory)))
	if err != nil {
		return fmt.Errorf("read guest RAM: %w", err)
	}

	for i := range vm.Ram.ReadRegisters() {
		val := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = 0x%X\n", regInfoList[i].Name, val)
		vm.Ram.WriteRegister(i, val)
	}
	return nil
}
func (vm *RecompilerSandboxVM) WriteRegisters() {
	for i, reg := range vm.Ram.ReadRegisters() {
		if debugRecompiler {
			fmt.Printf("WriteRegistersSandbox index: %d, value: 0x%X\n", i, reg)
		}
		if err := vm.sandBox.RegWrite(sandBoxRegInfoList[i], reg); err != nil {
			fmt.Printf("WriteRegistersSandbox error: %v\n", err)
			return
		}
	}
}
