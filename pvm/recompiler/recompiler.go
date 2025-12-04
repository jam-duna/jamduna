package recompiler

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/sync/singleflight"
	"golang.org/x/sys/unix"
)

var ALWAYS_COMPILE = false
var compiler_usage = compiler_go

const (
	GasModeBasicBlock  = iota
	GasModeInstruction = 1
	GasMode            = GasModeInstruction
)

const (
	PageInaccessible = unix.PROT_NONE
	PageMutable      = unix.PROT_READ | unix.PROT_WRITE
	PageImmutable    = unix.PROT_READ
)

const (
	dumpSize   = 0x100000
	PageSize   = 4096                   // 4 KiB
	TotalMem   = 4 * 1024 * 1024 * 1024 // 4 GiB
	TotalPages = TotalMem / PageSize    // 1,048,576 pages
)

const (
	HOST  = 4
	PANIC = 2
)

/*
0 ---------------------- dumpAddr

	Register * 13 , 13 * 8 = 104

14 ---------------------

	Gas * 8 , 8 bytes

15 ---------------------

	pcBytes * 4 , 4 bytes
	empty 4 bytes for alignment

16 ---------------------

	blockCounter * 8 , 8 bytes
*/
const (
	gasSlotIndex          = 14 // Gas is at index 14
	pcSlotIndex           = 15 // PC is at index 15
	blockCounterSlotIndex = 16 // Block counter is at index 16

	indirectJumpPointSlot = 20 // Indirect jump point is at index 20
	nextx86SlotIndex      = 21 // Next x86 instruction address is at index 29 : for sbrk and ecalli

	sbrkAIndex = 22 // Sbrk A is at index 22
	sbrkDIndex = 23 // Sbrk D is at index 23

	vmStateSlotIndex = 30 // VM state is at index 30
	hostFuncIdIndex  = 31 // Host function ID is at index 31
	ripSlotIndex     = 32 // RIP is at index 32
	heapPointerSlot  = 33 // Heap pointer is at index 33

	opcodeDebugSlot = 34 // Opcode debug info is at index 34
)

type RecompilerVM struct {
	*program.Program
	*RecompilerRam
	compiler Compiler
	HostFunc
	pc uint64
	//
	Gas          int64
	IsChild      bool
	MachineState uint8
	ResultCode   int
	// standard program initialization parameters

	hostCall     bool // ̵h in GP
	host_func_id int  // h in GP

	o_size          uint32
	w_size          uint32
	z               uint32
	s               uint32
	o_byte          []byte
	w_byte          []byte
	code            []byte
	ServiceMetadata []byte
	Service_index   uint32

	realCode []byte
	codeAddr uintptr

	InstMapPVMToX86 map[uint32]int // maps PVM PC to the x86 PC index
	InstMapX86ToPVM map[int]uint32 // maps x86 PC to the PVM PC

	// Timing measurements using time.Duration for accuracy
	initializationTime time.Duration // time taken to initialize the VM
	standardInitTime   time.Duration
	compileTime        time.Duration
	executionTime      time.Duration // total time in x86 execution (excluding host calls)
	hostcallTime       time.Duration // total time in host calls
	allExecutionTime   time.Duration // total wall-clock time from Execute() start to finish

	//	snapshot *EmulatorSnapShot

	basicBlockExecutionCounter map[uint64]int // PVM PC to execution count

	OP_tally map[string]*X86InstTally `json:"tally"`

	vmBasicBlock int

	x86Code   []byte
	djumpAddr uintptr // address of the jump table in x86Code

	reuseCode bool // whether to reuse the existing x86Code buffer
}

type X86Compiler struct {
	JSize   uint64
	Z       uint8
	J       []uint32
	code    []byte
	bitmask []byte

	startCode []byte
	exitCode  []byte

	x86Blocks map[uint64]*BasicBlock // by x86 PC
	x86PC     uint64

	JumpTableOffset  uint64         // offset for the jump table in x86Code
	JumpTableOffset2 uint64         // offset for the jump table in x86Code
	JumpTableMap     []uint64       // maps PVM PC to the index of the x86code (djump only)
	InstMapPVMToX86  map[uint32]int // maps PVM PC to the x86 PC index
	InstMapX86ToPVM  map[int]uint32 // maps x86 PC to the PVM PC

	djumpTableFunc []byte
	djumpAddr      uintptr // address of the jump table in x86Code

	//debug tool
	x86Instructions map[int]x86asm.Inst
	pc_addr         uint64
	r12             uint64
	isChargingGas   bool
	isPCCounting    bool
	IsBlockCounting bool // whether to count basic blocks

	basicBlocks map[uint64]*BasicBlock // by PVM PC
	x86Code     []byte
}

type Compiler interface {
	SetJumpTable(j []uint32) error
	SetBitMask(bitmask []byte) error
	CompileX86Code(startPC uint64) (x86code []byte, djumpAddr uintptr, InstMapPVMToX86 map[uint32]int, InstMapX86ToPVM map[int]uint32)
	GetBasicBlock(pvmPC uint64) *BasicBlock
}

func NewX86Compiler(code []byte) *X86Compiler {
	return &X86Compiler{
		code:            code,
		x86Blocks:       make(map[uint64]*BasicBlock),
		JumpTableMap:    make([]uint64, 0),
		InstMapPVMToX86: make(map[uint32]int),
		InstMapX86ToPVM: make(map[int]uint32),
		x86Instructions: make(map[int]x86asm.Inst),
		basicBlocks:     make(map[uint64]*BasicBlock),
		isChargingGas:   true,  // default to charging gas
		isPCCounting:    false, // default to counting PC
		IsBlockCounting: false, // default to not counting basic blocks
		Z:               0,
	}
}

func (compiler *X86Compiler) SetJumpTable(j []uint32) error {
	compiler.J = j
	compiler.JSize = uint64(len(j))
	return nil
}

func (compiler *X86Compiler) SetBitMask(bitmask []byte) error {
	compiler.bitmask = bitmask
	return nil
}

type HostFunc interface {
	InvokeHostCall(host_fn int) (bool, error)
	GetResultCode() uint8
	GetMachineState() uint8
}

// vm.cVM = C.pvm_create(
//
//	C.uint32_t(vm.Service_index),
//	(*C.uint8_t)(unsafe.Pointer(&p.Code[0])),
//	C.size_t(len(p.Code)),
//	(*C.uint64_t)(unsafe.Pointer(&initialRegs[0])),
//	C.size_t(len(initialRegs)),
//	C.uint64_t(initialPC))
func NewRecompilerVM(serviceIndex uint32, code []byte, initialRegs []uint64, initialPC uint64) (*RecompilerVM, error) {
	ram, err := NewRecompilerRam() // 1MB for register dump
	if err != nil {
		return nil, fmt.Errorf("failed to create RecompilerRam: %w", err)
	}
	// Assemble the VM
	rvm := &RecompilerVM{
		pc:              initialPC,
		Service_index:   serviceIndex,
		InstMapX86ToPVM: make(map[int]uint32),
		InstMapPVMToX86: make(map[uint32]int),
		code:            code,
	}
	rvm.RecompilerRam = ram
	for i := range initialRegs {
		rvm.WriteRegister(i, initialRegs[i])
	}

	err = rvm.GetX86FromPVMX(code)
	if err != nil || ALWAYS_COMPILE {
		fmt.Printf("GetX86FromPVMX failed: %v\n", err)
		if compiler_usage == compiler_go {
			rvm.compiler = NewX86Compiler(code)
		} else if compiler_usage == compiler_c {
			rvm.compiler = NewRecompilerC(code)
		}
	} else {
		rvm.reuseCode = true
	}
	return rvm, nil
}

const (
	compiler_go = "compiler_go"
	compiler_c  = "compiler_c"
)

// Global flag to enable/disable debug tracing of PVM instructions
var EnableDebugTracing = false

func (rvm *RecompilerVM) SetPC(pc uint64) {
	rvm.pc = pc
}

func (rvm *RecompilerVM) GetPC() uint64 {
	return rvm.pc
}
func (rvm *RecompilerVM) SetBitMask(bitmask []byte) error {
	if rvm.compiler != nil {
		rvm.compiler.SetBitMask(bitmask)
	}
	return nil
}

func (rvm *RecompilerVM) SetJumpTable(j []uint32) error {
	if rvm.compiler != nil {
		rvm.compiler.SetJumpTable(j)
	}
	return nil
}

func (rvm *RecompilerVM) SetGas(gas int64) {
	rvm.Gas = gas
	rvm.WriteContextSlot(gasSlotIndex, uint64(gas), 8)
}

func (rvm *RecompilerVM) GetGas() int64 {
	return rvm.Gas
}

func (rvm *RecompilerVM) GetResultCode() uint8 {
	state, _ := rvm.ReadContextSlot(vmStateSlotIndex)
	return uint8(state)
}

func (rvm *RecompilerVM) Panic(uint64) {
	rvm.MachineState = PANIC
	rvm.WriteContextSlot(vmStateSlotIndex, uint64(PANIC), 8)
}

func (rvm *RecompilerVM) SetHostResultCode(errCode uint64) {
	// TODO
}

func (rvm *RecompilerVM) SetHeapPointer(addr uint32) {
	rvm.RecompilerRam.current_heap_pointer = addr
}

func (rvm *RecompilerVM) SetMemoryBounds(o_size uint32,
	w_size uint32,
	z uint32,
	s uint32,
	o_byte []byte,
	w_byte []byte) {
	// set memory bounds
	rvm.o_size = o_size
	rvm.w_size = w_size
	rvm.z = z
	rvm.s = s
	rvm.o_byte = o_byte
	rvm.w_byte = w_byte
}

// add jump indirects
const entryPatch = 0x99999999
const regDumpMemPatch = 0x8888_8888_8888_8888

func (vm *X86Compiler) initStartCode() {

	vm.startCode = append(vm.startCode, encodeMovImm(BaseRegIndex, regDumpMemPatch)...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < regSize; i++ {
		offset := byte(i * 8)
		code := encodeMem64ToReg(i, BaseRegIndex, offset)
		vm.startCode = append(vm.startCode, code...)
	}
	// Adjust R12 from regDumpAddr to realMemAddr: R12 = regDumpAddr + regMemsize
	vm.startCode = append(vm.startCode, emitAddR12Imm32(regMemsize)...)
	// padding with jump to the entry point
	vm.startCode = append(vm.startCode, X86_OP_JMP_REL32) // JMP rel32
	// use entryPatch as a placeholder 0x99999999
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	vm.startCode = append(vm.startCode, patch...)

	// Build exit code in temporary buffer
	exitCode := emitSubR12Imm32(regMemsize)
	for i := 0; i < len(regInfoList); i++ {
		if i == BaseRegIndex {
			continue // skip R12 into [R12]
		}
		offset := byte(i * 8)
		exitCode = append(exitCode, encodeMovRegToMem(i, BaseRegIndex, offset)...)
	}
	vm.exitCode = append(exitCode, X86_OP_RET)
}

// initDJumpFunc initializes the dynamic jump table function for indirect jumps.
// It generates x86 code that performs a series of checks and dispatches to the correct handler
// based on the value in jumpIndTempReg (typically RCX). The generated code handles the following cases:
//
//  1. If jumpIndTempReg == 0xFFFF0000, return (ret_stub).
//  2. If jumpIndTempReg == 0, trigger a panic (panic_stub).
//  3. If jumpIndTempReg > threshold, trigger a panic (panic_stub).
//  4. If jumpIndTempReg is odd (jumpIndTempReg % 2 != 0), trigger a panic (panic_stub).
//
// If all checks pass, it computes the handler address and jumps to the appropriate handler.
// The function also patches the generated code with the correct relative offsets for jumps and handlers.
func (vm *X86Compiler) initDJumpFunc(x86CodeLen int) {
	type pending struct {
		jeOff   int
		handler uintptr
	}

	code := make([]byte, 0)
	var pendings []pending

	// ==== Pre-checks: CMP/JE placeholders ====

	// (a) If jumpIndTempReg == 0xFFFF0000, jump to ret_stub
	rex := byte(X86_REX_BASE)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B // REX.B
	}
	modrm := byte(X86_MOD_REGISTER<<6 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP1_RM_IMM32, // CMP r/m32, imm32
		modrm,
		0x00, 0x00, 0xFF, 0xFF, // 0xFFFF0000
	)
	offJEret := len(code)
	code = append(code, emitJeInitDJump()...)

	// (b) If jumpIndTempReg == 0, jump to panic_stub
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP1_RM_IMM8, modrm, X86_IMM_0)
	offJE0 := len(code)
	code = append(code, emitJeInitDJump()...)

	// (c) If jumpIndTempReg > threshold, jump to panic_stub
	threshold := uint32(len(vm.JumpTableMap)) * uint32(Z_A)
	var thr [4]byte
	binary.LittleEndian.PutUint32(thr[:], threshold)
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(0xF8 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP1_RM_IMM32, modrm)
	code = append(code, thr[:]...)
	offJA := len(code)
	code = append(code, emitJaInitDJump()...)

	// (d) If jumpIndTempReg is odd (jumpIndTempReg % 2 != 0), jump to panic_stub
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_GROUP3_RM, modrm, X86_IMM_1, 0, 0, 0)
	offJNZ := len(code)
	code = append(code, emitJneInitDJump()...)

	// Collect panic jump offsets for later patching
	panicOffs := []int{offJE0 + 2, offJA + 2, offJNZ + 2}

	// ==== Continue execution after checks ====
	// Compute (jumpIndTempReg / 2 - 1) * 7, then load current PC into RAX

	// 1. SAR jumpIndTempReg, 1    ; Arithmetic right shift by 1 (signed division by 2)
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (7 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP2_RM_IMM8,
		modrm,
		X86_IMM_1,
	)

	// 2. SUB jumpIndTempReg, 1
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modr := byte(X86_MOD_REGISTER<<6 | (5 << 3) | jumpIndTempReg.RegBits)
	code = append(code,
		rex,
		X86_OP_GROUP1_RM_IMM8,
		modr,
		X86_IMM_1,
	)

	// 3. IMUL jumpIndTempReg, jumpIndTempReg, 7
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= (X86_REX_R | X86_REX_B)
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (jumpIndTempReg.RegBits << 3) | jumpIndTempReg.RegBits)
	imm := []byte{0x07, 0x00, 0x00, 0x00}
	code = append(code, rex, X86_OP_IMUL_R_RM, modrm)
	code = append(code, imm...)

	// 4. PUSH RAX
	code = append(code, emitPushReg(RAX)...)

	// 5. LEA RAX, [RIP+0] ; Load current instruction address into RAX
	code = append(code, emitLeaRaxRipInitDJump()...)

	// 6. ADD jumpIndTempReg, RAX
	rex = byte(X86_REX_W_PREFIX)
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = byte(X86_MOD_REGISTER<<6 | (0 << 3) | jumpIndTempReg.RegBits)
	code = append(code, rex, X86_OP_ADD_RM_R, modrm)

	// 7. POP RAX
	code = append(code, emitPopReg(RAX)...)

	// 8. ADD jumpIndTempReg, imm32 (handler address offset, to be patched later)
	handlerAddOff := len(code) + 3
	code = append(code, rex, X86_OP_GROUP1_RM_IMM32, modrm, 0xEF, 0xBE, 0xAD, 0xDE)

	// 9. JMP jumpIndTempReg
	rex = X86_REX_W_PREFIX
	if jumpIndTempReg.REXBit == 1 {
		rex |= X86_REX_B
	}
	modrm = 0xE0 | jumpIndTempReg.RegBits
	code = append(code, rex, 0xFF, modrm)

	// Collect handler jump locations for patching
	for _, idx := range vm.JumpTableMap {
		pendings = append(pendings, pending{
			handler: uintptr(idx),
		})
	}

	// Patch panic jumps to point to the panic stub
	panicStubAddr := uintptr(len(code))
	for _, off := range panicOffs {
		rel := int32(int64(panicStubAddr) - int64(off) - 4)
		binary.LittleEndian.PutUint32(code[off:], uint32(rel))
	}

	// Panic stub: POP jumpIndTempReg, then UD2 (undefined instruction)
	code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits))
	code = append(code, emitUd2InitDJump()...)

	// Patch JE ret stub to point to the return stub
	retStubAddr := uintptr(len(code))
	relRet := int32(int64(retStubAddr) - int64(offJEret) - 6)
	binary.LittleEndian.PutUint32(code[offJEret+2:offJEret+6], uint32(relRet))

	// Return stub: POP jumpIndTempReg, then exit code
	code = append(code, emitPopReg(jumpIndTempReg)...)
	code = append(code, vm.exitCode...)

	// Patch handler jumps for each pending handler
	for i, p := range pendings {
		// POP jumpIndTempReg then JMP handler
		pendings[i].jeOff = len(code)
		code = append(code, rex, byte(0x58|jumpIndTempReg.RegBits))
		toMinus := x86CodeLen + len(code) - int(p.handler)
		num := int32(-toMinus - 5)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(num))
		code = append(code, emitJmpE9InitDJump()...)
		code = append(code, buf...)
	}

	if len(pendings) == 0 {
		fmt.Println("No pending handlers found, skipping handler patching.")
		vm.djumpTableFunc = code
		return
	}
	firstPending := pendings[0]
	toAdd := firstPending.jeOff - handlerAddOff + 7
	// Patch the handler address offset into the ADD instruction
	uint64ToAdd := uint64(toAdd)
	binary.LittleEndian.PutUint32(code[handlerAddOff:], uint32(uint64ToAdd))
	vm.djumpTableFunc = code
}

func (vm *RecompilerVM) Close() error {
	var errs []error
	if len(errs) > 0 {
		return fmt.Errorf("Close encountered errors: %v", errs)
	}

	return nil
}
func Disassemble(code []byte) string {
	var sb strings.Builder
	offset := 0
	for offset < len(code) {
		inst, err := x86asm.Decode(code[offset:], 64)
		length := inst.Len
		if err != nil {
			sb.WriteString(fmt.Sprintf("0x%04x: db 0x%02x\n", offset, code[offset]))
			offset++
			continue
		}

		var hexBytes []string
		for i := 0; i < length; i++ {
			hexBytes = append(hexBytes, fmt.Sprintf("%02x", code[offset+i]))
		}
		sb.WriteString(fmt.Sprintf(
			"0x%04x: %-16s %s\n",
			offset,
			strings.Join(hexBytes, " "),
			inst.String(),
		))
		offset += length
	}
	return sb.String()
}

type X86Instr struct {
	Offset      int
	Instruction x86asm.Inst
}

func DisassembleInstructions(code []byte) []X86Instr {
	instructions := make([]X86Instr, 0)
	offset := 0
	for offset < len(code) {
		inst, err := x86asm.Decode(code[offset:], 64)
		length := inst.Len
		if err != nil {
			offset++
			continue
		}
		instructions = append(instructions, X86Instr{
			Offset:      offset,
			Instruction: inst,
		})
		offset += length
	}
	return instructions
}

const entryOffset = 82
const regDumpOffset = 2

func (rvm *X86Compiler) Patch() {
	rvm.djumpAddr = uintptr(len(rvm.x86Code))
	rvm.initDJumpFunc(len(rvm.x86Code))
	rvm.finalizeJumpTargets(rvm.J)
	rvm.x86Code = append(rvm.x86Code, rvm.djumpTableFunc...)
}

func (vm *RecompilerVM) ExecuteX86CodeWithEntry(entry uint32) (err error) {
	codeAddr, err := syscall.Mmap(
		-1, 0, len(vm.x86Code),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return fmt.Errorf("failed to mmap exec code: %w", err)
	}

	vm.realCode = codeAddr
	vm.codeAddr = uintptr(unsafe.Pointer(&vm.realCode[0]))
	if err != nil {
		return fmt.Errorf("failed to mmap djumpTableFunc: %w", err)
	}
	// find the real memory placeholder and patch it

	binary.LittleEndian.PutUint64(vm.x86Code[regDumpOffset:regDumpOffset+8], uint64(vm.regDumpAddr))
	// use entryPatch as a placeholder 0x99999999
	//get the x86 pc
	x86PC, ok := vm.InstMapPVMToX86[entry]
	if !ok && entry != 0 {
		return fmt.Errorf("entry %d not found in InstMapPVMToX86, isChild %v", entry, vm.IsChild)
	}
	if debugRecompiler {
		fmt.Printf("Executing code at x86 PC: %d (PVM PC: %d)\n", x86PC, entry)
	}
	patch := make([]byte, 4)
	binary.LittleEndian.PutUint32(patch, entryPatch)
	binary.LittleEndian.PutUint32(vm.x86Code[entryOffset+1:entryOffset+5], uint32(x86PC-entryOffset-5))
	vm.djumpAddr += vm.codeAddr
	vm.WriteContextSlot(indirectJumpPointSlot, uint64(vm.djumpAddr), 8)

	// if patchInstIdx == -1 {
	// 	return fmt.Errorf("no entry patch placeholder found in x86 code")
	// }
	copy(codeAddr, vm.x86Code)

	// Keep PROT_WRITE|PROT_EXEC for fast patching during Resume()
	// Security note: This allows self-modifying code but improves performance

	if showDisassembly {
		str := Disassemble(vm.realCode)
		fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	}
	crashed, _, err := ExecuteX86(codeAddr, vm.regDumpMem)

	if err != nil {
		return fmt.Errorf("ExecuteX86 failed: %w", err)
	}
	gas, err := vm.ReadContextSlot(gasSlotIndex)
	if err != nil {
		return fmt.Errorf("failed to read gas from context slot: %w", err)
	}
	vm.Gas = int64(gas)
	if crashed == -1 || err != nil {
		vm.ResultCode = PANIC
		vm.MachineState = PANIC
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		fmt.Printf("codeAddr: 0x%x\n", vm.codeAddr)
		fmt.Printf("djumpAddr: 0x%x\n", vm.djumpAddr)
		fmt.Printf("realMemory address: 0x%x\n", vm.realMemAddr)
		rip, _ := vm.ReadContextSlot(ripSlotIndex)
		if rip >= uint64(vm.codeAddr) && rip < uint64(len(vm.realCode))+uint64(vm.codeAddr) {
			// get the code offset out
			codeAddr := vm.codeAddr
			var pvm_pc uint32
			var ok bool
			offset := int(rip - uint64(codeAddr))
			if rip > uint64(vm.djumpAddr) {
				pvm_pc_64, _ := vm.ReadContextSlot(pcSlotIndex)
				pvm_pc = uint32(pvm_pc_64)
				fmt.Printf("Recovered PVM PC: %d from RIP1: %d\n", pvm_pc, rip)
				vm.WriteContextSlot(pcSlotIndex, uint64(pvm_pc), 8)
				vm.SetPC(uint64(pvm_pc))
			} else {
				fmt.Printf("RIP at crash: %d, code offset: %d\n", rip, offset)

				// get the pvm pc out
				for i := offset; i >= 0; i-- {
					pvm_pc, ok = vm.InstMapX86ToPVM[i]
					if ok {
						fmt.Printf("Recovered PVM PC: %d from RIP: %d\n", pvm_pc, rip)
						vm.WriteContextSlot(pcSlotIndex, uint64(pvm_pc), 8)
						vm.SetPC(uint64(pvm_pc))
						break
					}
				}
			}

		}
		return fmt.Errorf("ExecuteX86 crash detected (return -1) gas = %d", vm.Gas)
	}
	// get the pc out
	vm.pc, _ = vm.ReadContextSlot(pcSlotIndex)
	// vm state
	vmState, _ := vm.ReadContextSlot(vmStateSlotIndex)
	if vmState == HOST {
		vm.hostCall = true
		host_id, _ := vm.ReadContextSlot(hostFuncIdIndex)
		vm.host_func_id = int(host_id) // reset host function ID
	}
	// get the vmstate out
	vm.MachineState = uint8(vmState)
	return nil
}

func (vm *RecompilerVM) Resume() error {

	u64x86PC, err := vm.ReadContextSlot(nextx86SlotIndex)
	patchInstIdx := entryOffset
	codeAddr := vm.realCode
	if err != nil && vm.pc != 0 {
		fmt.Printf("post-host call: pc %d not found in InstMapPVMToX86, isChild %v\n", vm.pc, vm.IsChild)
		return fmt.Errorf("post-host call: pc %d not found in InstMapPVMToX86, isChild %v", vm.pc, vm.IsChild)
	}
	x86PC := int(u64x86PC)
	// Direct patch - no mprotect needed (code already has PROT_WRITE|PROT_EXEC)
	binary.LittleEndian.PutUint32(codeAddr[patchInstIdx+1:patchInstIdx+5], uint32(x86PC-patchInstIdx-5))
	vm.WriteContextSlot(vmStateSlotIndex, uint64(0), 8) // reset vm state
	vm.MachineState = 0
	crashed, _, err := ExecuteX86(codeAddr, vm.regDumpMem)
	if err != nil {
		return fmt.Errorf("ExecuteX86 failed: %w", err)
	}

	// Batch read all needed context slots for better performance
	slots, err := vm.ReadContextSlots(gasSlotIndex, pcSlotIndex, vmStateSlotIndex, hostFuncIdIndex)
	if err != nil {
		return fmt.Errorf("failed to read context slots: %w", err)
	}
	vm.Gas = int64(slots[0])
	vm.pc = slots[1]
	vmState := slots[2]
	host_id := slots[3]

	if crashed == -1 || err != nil {
		vm.ResultCode = PANIC
		vm.MachineState = PANIC
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		fmt.Printf("codeAddr: 0x%x\n", vm.codeAddr)
		fmt.Printf("djumpAddr: 0x%x\n", vm.djumpAddr)
		fmt.Printf("realMemory address: 0x%x\n", vm.realMemAddr)

		// restore the gas calculation
		rip, _ := vm.ReadContextSlot(ripSlotIndex)
		if rip >= uint64(vm.codeAddr) && rip < uint64(len(vm.realCode))+uint64(vm.codeAddr) {
			// get the code offset out
			codeAddr := vm.codeAddr
			var pvm_pc uint32
			var ok bool
			offset := int(rip - uint64(codeAddr))
			if rip > uint64(vm.djumpAddr) {
				pvm_pc_64, _ := vm.ReadContextSlot(pcSlotIndex)
				pvm_pc = uint32(pvm_pc_64)
				fmt.Printf("Recovered PVM PC: %d from RIP1: %d\n", pvm_pc, rip)
				vm.WriteContextSlot(pcSlotIndex, uint64(pvm_pc), 8)
				vm.SetPC(uint64(pvm_pc))
			} else {
				fmt.Printf("RIP at crash: %d, code offset: %d\n", rip, offset)

				// get the pvm pc out
				for i := offset; i >= 0; i-- {
					pvm_pc, ok = vm.InstMapX86ToPVM[i]
					if ok {
						fmt.Printf("Recovered PVM PC: %d from RIP: %d\n", pvm_pc, rip)
						vm.WriteContextSlot(pcSlotIndex, uint64(pvm_pc), 8)
						vm.SetPC(uint64(pvm_pc))
						fmt.Printf("code = %s\n", opcode_str(vm.code[pvm_pc]))
						break
					}
				}
			}

		}
		return fmt.Errorf("ExecuteX86 crash detected (return -1) gas = %d", vm.Gas)
	}

	// Update machine state based on execution result
	if vmState == HOST {
		vm.hostCall = true
		vm.host_func_id = int(host_id) // reset host function ID
	}
	vm.MachineState = uint8(vmState)
	return nil
}

func (compiler *X86Compiler) CompileX86Code(startPC uint64) (x86code []byte, djumpAddr uintptr, InstMapPVMToX86 map[uint32]int, InstMapX86ToPVM map[int]uint32) {
	compiler.initStartCode()
	compiler.Compile(startPC)
	if GasMode == GasModeBasicBlock {
		// panic check for this trap
		gas_check_code := generateGasCheck(2)
		offsetPanic := len(compiler.x86Code)
		pc := len(compiler.code)
		compiler.InstMapPVMToX86[uint32(pc)] = offsetPanic
		compiler.InstMapX86ToPVM[offsetPanic] = uint32(pc)
		compiler.x86Code = append(compiler.x86Code, gas_check_code...)
	}
	compiler.x86Code = append(compiler.x86Code, emitTrap()...) // in case direct fallthrough
	compiler.Patch()
	return compiler.x86Code, compiler.djumpAddr, compiler.InstMapPVMToX86, compiler.InstMapX86ToPVM
}

func (rvm *RecompilerVM) Execute(entry uint32) {
	compileStart := time.Now()
	rvm.pc = 0
	rvm.WriteContextSlot(gasSlotIndex, uint64(rvm.Gas), 8)

	if !rvm.reuseCode {
		rvm.x86Code, rvm.djumpAddr, rvm.InstMapPVMToX86, rvm.InstMapX86ToPVM = rvm.compiler.CompileX86Code(rvm.pc)
		err1 := rvm.SavePVMX()
		if err1 != nil {
			fmt.Printf("SavePVMX failed: %v\n", err1)
		}
	}
	rvm.compileTime = time.Since(compileStart)

	hardStart := time.Now()
	execStart := time.Now()
	if err := rvm.ExecuteX86CodeWithEntry(entry); err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
	}
	// executionTime captures the time spent in ExecuteX86CodeWithEntry
	// This includes mmap setup + actual x86 execution
	rvm.executionTime = time.Since(execStart)

	for rvm.MachineState == HOST || rvm.MachineState == SBRK {
		hostStart := time.Now()
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
		if rvm.HostFunc.GetMachineState() == PANIC {
			fmt.Printf("PANIC after host call\n")
			break
		}
		rvm.hostcallTime += time.Since(hostStart)

		resumeStart := time.Now()
		err := rvm.Resume()
		resumeTime := time.Since(resumeStart)
		rvm.executionTime += resumeTime
		if err != nil {
			fmt.Printf("Resume after host call failed: %v\n", err)
			rvm.WriteContextSlot(vmStateSlotIndex, uint64(PANIC), 8)
			break
		}
	}
	rvm.allExecutionTime = time.Since(hardStart)
}

// Standard_Program_Initialization initializes the program memory and registers
func (vm *RecompilerVM) Init(argument_data_a []byte) (err error) {

	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	//1)
	// o_byte
	o_len := len(vm.o_byte)

	if err = vm.SetMemAccess(Z_Z, uint32(o_len), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess1 failed o_len=%d (o_byte): %w", o_len, err)
	}
	if err = vm.WriteMemory(Z_Z, vm.o_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (o_byte): %w", err)
	}
	if err = vm.SetMemAccess(Z_Z, uint32(o_len), PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess2 failed (o_byte): %w", err)
	}
	//2)
	// Padding after o_byte: the gap between o_len and its page-aligned size
	// This padding should be immutable (read-only zeros)
	p_o_len := P_func(uint32(o_len))
	padding_after_o := p_o_len - uint32(o_len)
	if padding_after_o > 0 {
		if err = vm.SetMemAccess(Z_Z+uint32(o_len), padding_after_o, PageImmutable); err != nil {
			return fmt.Errorf("SetMemAccess failed (o_byte padding): %w", err)
		}
	}

	z_o := Z_func(vm.o_size)
	z_w := Z_func(vm.w_size + vm.z*Z_P)
	z_s := Z_func(vm.s)
	requiredMemory := uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
	if requiredMemory > math.MaxUint32 {
		return
	}
	// 3)
	// w_byte
	w_addr := 2*Z_Z + z_o
	w_len := uint32(len(vm.w_byte))
	if err = vm.SetMemAccess(w_addr, w_len, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (w_byte): %w", err)
	}
	if err = vm.WriteMemory(w_addr, vm.w_byte); err != nil {
		return fmt.Errorf("WriteMemory failed (w_byte): %w", err)
	}
	// 4)
	addr4 := 2*Z_Z + z_o + w_len
	little_z := vm.z
	len4 := P_func(w_len) + little_z*Z_P - w_len
	if err = vm.SetMemAccess(addr4, len4, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr4): %w", err)
	}
	// 5)
	addr5 := 0xFFFFFFFF + 1 - 2*Z_Z - Z_I - P_func(vm.s)
	len5 := P_func(vm.s)
	if err = vm.SetMemAccess(addr5, len5, PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr5): %w", err)
	}
	// 6)
	argAddr := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	if err = vm.SetMemAccess(argAddr, uint32(len(argument_data_a)), PageMutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr): %w", err)
	}
	if err = vm.WriteMemory(argAddr, argument_data_a); err != nil {
		return fmt.Errorf("WriteMemory failed (argAddr): %w", err)
	}
	// set it back to immutable
	if err = vm.SetMemAccess(argAddr+uint32(len(argument_data_a)), Z_I, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (argAddr+len): %w", err)
	}
	// 7)
	addr7 := argAddr + uint32(len(argument_data_a))
	len7 := argAddr + P_func(uint32(len(argument_data_a))) - addr7
	if err = vm.SetMemAccess(addr7, len7, PageImmutable); err != nil {
		return fmt.Errorf("SetMemAccess failed (addr7): %w", err)
	}

	vm.WriteRegister(0, uint64(0xFFFFFFFF-(1<<16)+1))
	vm.WriteRegister(1, uint64(0xFFFFFFFF-2*Z_Z-Z_I+1))
	vm.WriteRegister(7, uint64(argAddr))
	vm.WriteRegister(8, uint64(uint32(len(argument_data_a))))
	return nil
}

func (vm *RecompilerVM) WriteContextSlot(slotIndex int, value uint64, size int) error {
	if vm.regDumpAddr == 0 {
		return fmt.Errorf("regDumpAddr is not initialized")
	}
	start := slotIndex * 8
	if start+size > len(vm.regDumpMem) {
		return fmt.Errorf("out of bounds: slot=%d size=%d len=%d", slotIndex, size, len(vm.regDumpMem))
	}

	switch size {
	case 4:
		binary.LittleEndian.PutUint32(vm.regDumpMem[start:start+4], uint32(value))
	case 8:
		binary.LittleEndian.PutUint64(vm.regDumpMem[start:start+8], value)
	default:
		return fmt.Errorf("unsupported size: %d", size)
	}
	return nil
}

// BuildWriteContextSlotCode emits x86-64 machine code that, when executed,
// stores `value` into vm.regDumpAddr + slot_index*8.
// size must be 4 or 8 (bytes).
func BuildWriteContextSlotCode(slotIndex int, value uint64, size int) ([]byte, error) {
	if slotIndex < 0 {
		return nil, fmt.Errorf("invalid slot index: %d", slotIndex)
	}
	if size != 4 && size != 8 {
		return nil, fmt.Errorf("unsupported size: %d", size)
	}

	var code []byte
	// Helpers
	emit := func(b ...byte) { code = append(code, b...) }
	emitImm64 := func(x uint64) {
		var t [8]byte
		binary.LittleEndian.PutUint64(t[:], x)
		code = append(code, t[:]...)
	}
	emitImm32 := func(x uint32) {
		var t [4]byte
		binary.LittleEndian.PutUint32(t[:], x)
		code = append(code, t[:]...)
	}
	code = append(code, emitPushReg(RAX)...)

	slotOffset := uint32(slotIndex * 8)
	dumpOffset := uint32(dumpSize)

	switch size {
	case 8:
		// 48 B8 imm64         ; mov rax, imm64
		emit(0x48, 0xB8)
		emitImm64(value)

	case 4:
		// B8 imm32            ; mov eax, imm32
		emit(0xB8)
		emitImm32(uint32(value))
	}

	// Temporarily shift R12 from the real memory base to the register-dump slot.
	code = append(code, emitSubR12Imm32(dumpOffset)...)

	if slotOffset != 0 {
		code = append(code, emitAddR12Imm32(slotOffset)...)
	}

	switch size {
	case 8:
		// 49 89 04 24        ; mov [r12], rax
		emit(0x49, 0x89, 0x04, 0x24)
	case 4:
		// 41 89 04 24        ; mov [r12], eax
		emit(0x41, 0x89, 0x04, 0x24)
	}

	if slotOffset != 0 {
		code = append(code, emitSubR12Imm32(slotOffset)...)
	}

	// Restore R12 to the real memory base
	code = append(code, emitAddR12Imm32(dumpOffset)...)

	// Restore RAX and return
	code = append(code, emitPopReg(RAX)...)

	return code, nil
}

func (cmp *X86Compiler) GetBasicBlock(pvmPC uint64) *BasicBlock {
	block, exists := cmp.basicBlocks[pvmPC]
	if !exists {
		return nil
	}
	return block
}

func (vm *RecompilerVM) BuildWriteRipToContextSlotCode(slotIndex int) ([]byte, error) {
	if vm.regDumpAddr == 0 {
		return nil, fmt.Errorf("regDumpAddr is not initialized")
	}
	if slotIndex < 0 {
		return nil, fmt.Errorf("invalid slot index: %d", slotIndex)
	}

	var code []byte
	emit := func(b ...byte) { code = append(code, b...) }

	// Save RAX
	code = append(code, emitPushReg(RAX)...)

	// 48 8D 05 00 00 00 00    ; lea rax, [rip+0]
	// This loads the address of the next instruction into RAX.
	emit(0x48, 0x8D, 0x05, 0x00, 0x00, 0x00, 0x00)

	slotOffset := uint32(slotIndex * 8)
	dumpOffset := uint32(dumpSize)

	// Temporarily repoint R12 to the register dump buffer.
	code = append(code, emitSubR12Imm32(dumpOffset)...)

	if slotOffset != 0 {
		code = append(code, emitAddR12Imm32(slotOffset)...)
	}

	// 49 89 04 24          ; mov [r12], rax
	emit(0x49, 0x89, 0x04, 0x24)

	if slotOffset != 0 {
		code = append(code, emitSubR12Imm32(slotOffset)...)
	}

	// Restore R12 to the real memory base
	code = append(code, emitAddR12Imm32(dumpOffset)...)

	// Restore RAX
	code = append(code, emitPopReg(RAX)...)

	return code, nil
}

func (vm *RecompilerVM) ReadContextSlot(slot_index int) (uint64, error) {
	if vm.regDumpAddr == 0 {
		return 0, fmt.Errorf("regDumpAddr is not initialized")
	}

	start := slot_index * 8
	if start+8 > len(vm.regDumpMem) {
		return 0, fmt.Errorf("not enough data to read from regDumpMem at index %d", slot_index)
	}

	return *(*uint64)(unsafe.Pointer(&vm.regDumpMem[start])), nil
}

// ReadContextSlots reads multiple context slots in a single batch operation for better performance.
// This reduces overhead compared to multiple individual ReadContextSlot calls.
func (vm *RecompilerVM) ReadContextSlots(slotIndices ...int) ([]uint64, error) {
	if vm.regDumpAddr == 0 {
		return nil, fmt.Errorf("regDumpAddr is not initialized")
	}

	results := make([]uint64, len(slotIndices))
	for i, slotIdx := range slotIndices {
		start := slotIdx * 8
		if start+8 > len(vm.regDumpMem) {
			return nil, fmt.Errorf("not enough data to read from regDumpMem at index %d", slotIdx)
		}
		results[i] = *(*uint64)(unsafe.Pointer(&vm.regDumpMem[start]))
	}
	return results, nil
}

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1

func generateTrap(inst Instruction) []byte {
	// rel := 0xDEADBEEF
	return emitTrap()
}

func generateFallthrough(inst Instruction) []byte {
	return emitNop()
}

func generateJump(inst Instruction) []byte {
	// For direct jumps, we can just append a jump instruction to a X86 PC
	// The displacement 0xFEFEFEFE will be patched by the VM
	return emitJmpWithPlaceholder()
}

// LOAD_IMM_JUMP (only the “LOAD_IMM” portion)
func generateLoadImmJump(inst Instruction) []byte {
	dstIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
	r := regInfoList[dstIdx]

	var code []byte
	// Generate MOV instruction using matching helper
	code = append(code, emitMovImmToReg64(r, vx)...)

	// Generate JMP instruction using matching helper
	code = append(code, emitJmpWithPlaceholder()...)
	return code
}

var jumpIndTempReg = RCX

func generateJumpIndirect(inst Instruction) []byte {
	// 1) Extract baseIdx and vx
	operands := slices.Clone(inst.Args)
	baseIdx := min(12, int(operands[0])&0x0F)
	lx := min(4, max(0, len(operands))-1)
	if lx == 0 {
		lx = 1
		operands = append(operands, 0)
	}
	vx := x_encode(types.DecodeE_l(operands[1:1+lx]), uint32(lx))
	base := regInfoList[baseIdx]
	buf := make([]byte, 0, 64)
	if GasMode == GasModeBasicBlock {
		pc_code, _ := BuildWriteContextSlotCode(pcSlotIndex, inst.Pc, 8)
		buf = append(buf, pc_code...)
	}
	// 1) PUSH jumpIndTempReg with exact manual construction
	buf = append(buf, emitPushRegJumpIndirect(jumpIndTempReg)...)

	// 2) MOV jumpIndTempReg, [base] with exact manual construction
	buf = append(buf, emitMovRegFromMemJumpIndirect(jumpIndTempReg, base)...)

	// 3) ADD jumpIndTempReg, vx with exact manual construction
	buf = append(buf, emitAddRegImm32_100(jumpIndTempReg, uint32(vx))...)

	// 4) JMP [BaseReg + offset] (already uses emit helper)
	buf = append(buf, generateJumpRegMem(BaseReg, indirectJumpPointSlot*8-dumpSize)...)

	return buf
}

// LOAD_IMM_JUMP_IND with jumpIndTempReg
func generateLoadImmJumpIndirect(inst Instruction) []byte {

	dstIdx, indexRegIdx, vx, vy := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	indexReg := regInfoList[indexRegIdx]

	var buf []byte
	if GasMode == GasModeBasicBlock {
		pcCode, _ := BuildWriteContextSlotCode(pcSlotIndex, inst.Pc, 8)
		buf = append(buf, pcCode...)
	}
	// 1) PUSH jumpIndTempReg
	buf = append(buf, emitPushRegLoadImmJumpIndirect(jumpIndTempReg)...)

	// 2) MOV jumpIndTempReg, indexReg
	buf = append(buf, emitMovRegToRegLoadImmJumpIndirect(jumpIndTempReg, indexReg)...)

	// 3) MOV r64 (dst) <- imm64 (vx)
	buf = append(buf, emitMovImmToRegLoadImmJumpIndirect(r, vx)...)

	// 4) ADD jumpIndTempReg, vy
	buf = append(buf, emitAddRegImm32LoadImmJumpIndirect(jumpIndTempReg, uint32(vy))...)

	// 5) AND jumpIndTempReg, 0xFFFFFFFF (zero-extend)
	buf = append(buf, emitAndRegImm32LoadImmJumpIndirect(jumpIndTempReg)...)

	// 6) JMP [TempReg + indirectJumpPointSlot*8 - dumpSize]
	buf = append(buf, generateJumpRegMem(BaseReg, indirectJumpPointSlot*8-dumpSize)...)
	return buf
}

// generateBranchImm emits:
//
//	REX.W + CMP r64, imm32         (7 bytes total)
//	0x0F, jcc                      (2 bytes)
//	rel32 placeholder (true‐target) (4 bytes)
//
// Total = 13 bytes
func generateBranchImm(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regIdx, imm, _ := extractOneRegOneImmOneOffset(inst.Args)
		r := regInfoList[regIdx]

		var buf []byte

		// Generate CMP r64, imm32 using exact matching helper
		buf = append(buf, emitCmpRegImm32Force81(r, int32(imm))...)

		// Generate conditional jump using exact matching helper
		buf = append(buf, emitJccWithPlaceholder(jcc)...)

		return buf
	}
}

// generateCompareBranch creates a function to handle conditional branches between two registers.
// It uses the classic, robust two-jump sequence which works regardless of block layout.
// This predictable 14-byte output allows a separate patching pass to safely optimize it.
func generateCompareBranch(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, _ := extractTwoRegsOneOffset(inst.Args)
		rA := regInfoList[aIdx]
		rB := regInfoList[bIdx]

		// Use helper function for CMP rA, rB (order: CMP b, a due to parameter convention)
		cmpBytes := emitCmpReg64(rB, rA)

		// This robust sequence uses a conditional jump to the true case
		// and an unconditional jump to the false case. It is 14 bytes long.
		code := make([]byte, 0, 14)
		code = append(code, cmpBytes...) // CMP rA, rB
		code = append(code,
			0x0F, jcc, // Jcc rel32 (jump to the "true" branch)
			0, 0, 0, 0, // Placeholder for relTrue dd
		)
		return code
	}
}

type X86InstTally struct {
	PVM_OP           string                       `json:"pvm_op"`
	X86_Map          map[string]*X86InternalTally `json:"x86_map"`
	TotalX86         int                          `json:"total_x86_insts"`
	TotalPVM         int                          `json:"total_pvm_insts"`    // number of times PVM_OP appeared in code
	ExeCount         int                          `json:"execution_count"`    // number of times PVM_OP executed
	AverageX86Insts  float64                      `json:"average_x86_insts"`  // TotalX86 / TotalPVM
	WeightedX86Insts float64                      `json:"weighted_x86_insts"` // ExeCount * AverageX86Insts
}

type X86InternalTally struct {
	X86_OP string `json:"x86_op"`
	Count  int    `json:"count"`
}

func (vm *RecompilerVM) AddPVMCount(pvm_OP string) {
	if vm.OP_tally == nil {
		vm.OP_tally = make(map[string]*X86InstTally)
	}
	entry, ok := vm.OP_tally[pvm_OP]
	if !ok {
		entry = &X86InstTally{
			PVM_OP:  pvm_OP,
			X86_Map: make(map[string]*X86InternalTally),
		}
		vm.OP_tally[pvm_OP] = entry
	}
	entry.TotalPVM++
}

type PVMX struct {
	DjumpEntry      uint64 `json:"djump_entry"`
	SavingX86Entry0 uint64 `json:"saving_x86_entry0"`
	SavingX86Entry5 uint64 `json:"saving_x86_entry5"`
	X86Code         []byte `json:"x86_code"`
}

func EncodePVMX(p *PVMX) ([]byte, error) {
	x86Len := uint32(len(p.X86Code))
	size := 8 + 8 + 8 + 4 + len(p.X86Code) // 3 uint64 + length(uint32) + data
	buf := make([]byte, size)
	pos := 0

	binary.LittleEndian.PutUint64(buf[pos:], p.DjumpEntry)
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], p.SavingX86Entry0)
	pos += 8
	binary.LittleEndian.PutUint64(buf[pos:], p.SavingX86Entry5)
	pos += 8
	binary.LittleEndian.PutUint32(buf[pos:], x86Len)
	pos += 4
	copy(buf[pos:], p.X86Code)

	return buf, nil
}

func DecodePVMX(data []byte) (*PVMX, error) {
	if len(data) < 28 {
		return nil, fmt.Errorf("data too short")
	}
	pos := 0
	p := &PVMX{}
	p.DjumpEntry = binary.LittleEndian.Uint64(data[pos:])
	pos += 8
	p.SavingX86Entry0 = binary.LittleEndian.Uint64(data[pos:])
	pos += 8
	p.SavingX86Entry5 = binary.LittleEndian.Uint64(data[pos:])
	pos += 8

	x86Len := binary.LittleEndian.Uint32(data[pos:])
	pos += 4
	if len(data[pos:]) < int(x86Len) {
		return nil, fmt.Errorf("truncated x86_code: want %d bytes, got %d", x86Len, len(data[pos:]))
	}
	// Use slicing instead of allocate+copy for better performance
	// This is safe because the data buffer is not reused after this function returns
	p.X86Code = data[pos : pos+int(x86Len)]
	return p, nil
}

// tmp directory
const tmpDir = "/tmp/pvmx_tmp"

func (vm *RecompilerVM) SavePVMX() error {
	pvmx := PVMX{
		DjumpEntry:      uint64(vm.djumpAddr),
		SavingX86Entry0: uint64(vm.InstMapPVMToX86[0]),
		SavingX86Entry5: uint64(vm.InstMapPVMToX86[5]),
		X86Code:         vm.x86Code,
	}
	// use codec to encode
	data, err := EncodePVMX(&pvmx)
	if err != nil {
		return fmt.Errorf("failed to encode PVMX: %w", err)
	}
	// write to file
	// check if the dir exists
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		err = os.Mkdir(tmpDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create tmp dir: %w", err)
		}
	}
	// get the pvm code and compute the hash
	pvm_code := vm.code
	pvm_hash := common.Blake2Hash(pvm_code)
	filename := fmt.Sprintf("%s/%v.pvmx", tmpDir, pvm_hash)
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write PVMX to file: %w", err)
	}
	if debugRecompiler {
		fmt.Printf("Saved PVMX to %s\n", filename)
	}
	return nil
}

var (
	pvmxCache sync.Map
	sfGroup   singleflight.Group
)

func loadPVMXOnce(filename string) (*PVMX, error) {
	// check cache first
	if v, ok := pvmxCache.Load(filename); ok {
		return v.(*PVMX), nil
	}
	// singleflight to avoid duplicate loads
	v, err, _ := sfGroup.Do(filename, func() (any, error) {
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read PVMX from file: %w", err)
		}
		pvmx, err := DecodePVMX(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode PVMX: %w", err)
		}
		pvmxCache.Store(filename, pvmx)
		return pvmx, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*PVMX), nil
}

func (vm *RecompilerVM) GetX86FromPVMX(code []byte) error {
	// return fmt.Errorf("ALWAYS COMPILE")
	pvmHash := common.Blake2Hash(code)
	filename := filepath.Join(tmpDir, pvmHash.String()+".pvmx")

	pvmx, err := loadPVMXOnce(filename)
	if err != nil {
		return err
	}

	vm.djumpAddr = uintptr(pvmx.DjumpEntry)

	if vm.InstMapPVMToX86 == nil {
		vm.InstMapPVMToX86 = make(map[uint32]int)
	}
	vm.InstMapPVMToX86[0] = int(pvmx.SavingX86Entry0)
	vm.InstMapPVMToX86[5] = int(pvmx.SavingX86Entry5)

	vm.x86Code = pvmx.X86Code

	if debugRecompiler {
		fmt.Printf("Loaded PVMX from %s (cached)\n", filename)
	}
	return nil
}
