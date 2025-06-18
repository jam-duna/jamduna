package pvm

import (
	"encoding/binary"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/arch/x86/x86asm"
)

const (
	PageInaccessible = 0
	PageMutable      = 1
	PageImmutable    = 2
)

const (
	PageSize   = 4096                   // 4 KiB
	TotalMem   = 4 * 1024 * 1024 * 1024 // 4 GiB
	TotalPages = TotalMem / PageSize    // 1,048,576 pages
)

func NewRecompilerVM(vm *VM) (*RecompilerVM, error) {
	// Allocate 4GB virtual memory region (not accessed directly here)
	const memSize = 4 * 1024 * 1024 * 1024 // 4GB
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap memory: %v", err)
	}

	// Allocate memory to dump registers into
	dumpSize := len(regInfoList) * 8
	if dumpSize < 104 {
		dumpSize = 104
	}
	dumpMem, err := syscall.Mmap(
		-1, 0, dumpSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap regDump memory: %v", err)
	}
	regDumpAddr := uintptr(unsafe.Pointer(&dumpMem[0]))

	// Assemble the VM
	rvm := &RecompilerVM{
		VM:          vm,
		realMemory:  mem,
		regDumpMem:  dumpMem,
		regDumpAddr: regDumpAddr,

		JumpTableMap: make(map[int]int),
	}
	rvm.initStartCode()

	return rvm, nil
}

// add jump indirects

func (vm *RecompilerVM) initStartCode() {
	// vm.startCode, vm.JumpTableOffset, vm.JumpTableOffset2 = encodeJumpIndirects(vm.J)

	vm.startCode = append(vm.startCode, encodeMovImm(BaseRegIndex, uint64(uintptr(unsafe.Pointer(&vm.realMemory[0]))))...)
	// initialize registers: mov rX, imm from vm.register
	for i := 0; i < len(vm.register); i++ {
		immVal := vm.register[i]
		code := encodeMovImm(i, immVal)
		fmt.Printf("Initialize %s = %d\n", regInfoList[i].Name, immVal)
		vm.startCode = append(vm.startCode, code...)
	}

	// Build exit code in temporary buffer
	exitCode := encodeMovImm(BaseRegIndex, uint64(vm.regDumpAddr))
	for i := 0; i < len(regInfoList); i++ {
		if i == BaseRegIndex {
			continue // skip R12 into [R12]
		}
		offset := byte(i * 8)
		exitCode = append(exitCode, encodeMovRegToMem(i, BaseRegIndex, offset)...)
	}
	vm.exitCode = append(exitCode, 0xC3)
}

func (vm *RecompilerVM) initDJumpFunc(x86CodeLen int) {
	type pending struct {
		jeOff   int
		handler uintptr
	}
	code := make([]byte, 0)
	var pendings []pending

	// 1. POP return address into RAX
	code = append(code, 0x58) // POP RAX

	// ==== Pre-checks: All use CMP RAX, imm32 + Jcc placeholder ====

	// (a) RAX == 0 → panic_stub
	code = append(code, 0x48, 0x83, 0xF8, 0x00) // CMP RAX, 0
	offJE0 := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0) // JE rel32 placeholder

	// (b) RAX > threshold → panic_stub
	threshold := uint32(len(vm.JumpTableMap)) * uint32(Z_A)
	var thr [4]byte
	binary.LittleEndian.PutUint32(thr[:], threshold)
	code = append(code,
		0x48, 0x81, 0xF8, // CMP RAX, imm32
		thr[0], thr[1], thr[2], thr[3],
	)
	offJA := len(code)
	code = append(code, 0x0F, 0x87, 0, 0, 0, 0) // JA rel32 placeholder

	// (c) RAX%2 != 0 → panic_stub
	code = append(code, 0x48, 0xF7, 0xC0, 0x01, 0, 0, 0) // TEST RAX, 1
	offJNZ := len(code)
	code = append(code, 0x0F, 0x85, 0, 0, 0, 0) // JNZ rel32 placeholder

	// (d) RAX == retval(0xFFFF0000) → ret_stub
	retval := uint32((1 << 32) - (1 << 16))
	var retv [4]byte
	binary.LittleEndian.PutUint32(retv[:], retval)
	code = append(code,
		0x48, 0x81, 0xF8, // CMP RAX, imm32
		retv[0], retv[1], retv[2], retv[3],
	)
	offJEret := len(code)
	code = append(code, 0x0F, 0x84, 0, 0, 0, 0) // JE rel32 placeholder

	// Collect all placeholders that should jump to panic (start from imm32)
	panicOffs := []int{offJE0 + 2, offJA + 2, offJNZ + 2}

	// ==== After passing all checks, execution will naturally fall through here and continue =====

	// 2. Divide RAX by Z_A then subtract 1, using RCX as a temporary register
	za := uint32(Z_A)
	var zaBytes [4]byte
	binary.LittleEndian.PutUint32(zaBytes[:], za)

	code = append(code,
		0x51, // PUSH RCX
		0xB9, // MOV ECX, imm32(Z_A)
		zaBytes[0], zaBytes[1], zaBytes[2], zaBytes[3],
		0x48, 0x99, // CQO (sign-extend RAX → RDX:RAX)
		0x48, 0xF7, 0xF9, // IDIV RCX
		0x48, 0x83, 0xE8, 0x01, // SUB RAX, 1
		0x59, // POP RCX
	)
	// 3. Generate dynamic CMP/JE chain
	for djumpVal, idx := range vm.JumpTableMap {
		code = append(code, 0x48, 0x81, 0xF8)
		var imm [4]byte
		binary.LittleEndian.PutUint32(imm[:], uint32(djumpVal))
		code = append(code, imm[:]...)

		offJE := len(code)
		code = append(code, 0x0F, 0x84, 0, 0, 0, 0) // JE placeholder
		pendings = append(pendings, pending{
			jeOff:   offJE,
			handler: vm.codeAddr + uintptr(idx),
		})
	}

	// ==== Finally, append stubs and patch all placeholders at once ====

	// 4. panic_stub
	panicStubAddr := vm.codeAddr + uintptr(len(code))
	for _, off := range panicOffs {
		rel := int32(int64(panicStubAddr) - int64(vm.codeAddr) - int64(off) - 4)
		binary.LittleEndian.PutUint32(code[off:], uint32(rel))
	}
	code = append(code,
		0x58,       // POP RAX
		0x0F, 0x0B, // UD2
	)

	// 5. ret_stub
	retStubAddr := vm.codeAddr + uintptr(len(code))
	relRet := int32(int64(retStubAddr) - int64(vm.codeAddr) - int64(offJEret) - 6)
	binary.LittleEndian.PutUint32(code[offJEret+2:offJEret+6], uint32(relRet))
	code = append(code, 0x58)
	code = append(code, vm.exitCode...) // Use the previously defined exitCode here

	// 6. Patch dynamic JE → stub, and generate stub body
	for _, p := range pendings {
		stubAddr := vm.codeAddr + uintptr(len(code))
		relJE := int32(int64(stubAddr) - int64(vm.codeAddr) - int64(p.jeOff) - 6)
		binary.LittleEndian.PutUint32(code[p.jeOff+2:p.jeOff+6], uint32(relJE))

		code = append(code, 0x58) // POP RAX
		toMinus := x86CodeLen + len(code) - int(p.handler)
		num := int32(-toMinus - 5)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(num))
		code = append(code, append([]byte{0xE9}, buf...)...) // JMP rel32
	}

	vm.djumpTableFunc = code
}

func (vm *RecompilerVM) Close() error {
	var errs []error

	if vm.realMemory != nil {
		if err := syscall.Munmap(vm.realMemory); err != nil {
			errs = append(errs, fmt.Errorf("realMemory: %w", err))
		}
		vm.realMemory = nil
	}

	if vm.regDumpMem != nil {
		if err := syscall.Munmap(vm.regDumpMem); err != nil {
			errs = append(errs, fmt.Errorf("regDumpMem: %w", err))
		}
		vm.regDumpMem = nil
	}

	if vm.x86Code != nil {
		if err := syscall.Munmap(vm.x86Code); err != nil {
			errs = append(errs, fmt.Errorf("x86Code: %w", err))
		}
		vm.x86Code = nil
	}

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
func (vm *RecompilerVM) ExecuteX86Code(x86code []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(vm.logging, "RecompilerVM ExecuteX86Code panic", "error", r)
			debug.PrintStack()
			vm.ResultCode = types.WORKRESULT_PANIC
			vm.terminated = true
			err = fmt.Errorf("ExecuteX86Code panic: %v", r)
		}
	}()
	vm.initDJumpFunc(len(x86code))
	fmt.Printf("\ninitDJumpFunc: \n%s\n", Disassemble(vm.djumpTableFunc))
	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code)+len(vm.djumpTableFunc),
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
	ca := make([]byte, 8)
	binary.LittleEndian.PutUint64(ca, uint64(vm.codeAddr))
	for c := 0; c < int(vm.JumpTableOffset2); c++ {
		if x86code[c] == 0xba && x86code[c+1] == 0xef && x86code[c+2] == 0xef && x86code[c+3] == 0xef {
			copy(x86code[c+1:c+9], ca)
		}
	}
	vm.djumpAddr = vm.codeAddr + uintptr(len(x86code))
	vm.finalizeJumpTargets(vm.J)

	copy(codeAddr, x86code)
	copy(codeAddr[len(x86code):], vm.djumpTableFunc)
	err = syscall.Mprotect(codeAddr, syscall.PROT_READ|syscall.PROT_EXEC)
	if err != nil {
		return fmt.Errorf("failed to mprotect exec code: %w", err)
	}

	fmt.Printf("codeAddr: 0x%x\n", vm.codeAddr)
	fmt.Printf("regDumpAddr: 0x%x\n", vm.regDumpAddr)
	fmt.Printf("x86code (%d bytes):\n%x\n", len(x86code), x86code)
	str := Disassemble(vm.realCode)
	fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)

	crashed, err := ExecuteX86(codeAddr, vm.regDumpMem)
	for i := 0; i < len(vm.register); i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.register[i] = regValue
	}
	if crashed == -1 || err != nil {
		vm.ResultCode = types.WORKRESULT_PANIC
		vm.terminated = true
		fmt.Printf("PANIC in ExecuteX86Code: %v\n", err)
		return fmt.Errorf("ExecuteX86 crash detected (return -1)")
	}

	for i := 0; i < len(vm.register); i++ {
		regValue := binary.LittleEndian.Uint64(vm.regDumpMem[i*8:])
		fmt.Printf("%s = %d\n", regInfoList[i].Name, regValue)
		vm.register[i] = regValue
	}
	return nil
}

// A.5.2. Instructions with Arguments of One Immediate. InstructionI1

func generateTrap(inst Instruction) []byte {
	rel := 0xDEADBEEF
	return []byte{
		0xE9,                      // opcode
		byte(rel), byte(rel >> 8), // disp[0..1]
		byte(rel >> 16), byte(rel >> 24),
	}
}

func generateFallthrough(inst Instruction) []byte {
	return []byte{0x90}
}

func generateJump(inst Instruction) []byte {
	// For direct jumps, we can just append a jump instruction to a X86 PC
	return append([]byte{0xE9}, 0xFE, 0xFE, 0xFE, 0xFE)
}

// LOAD_IMM_JUMP (only the “LOAD_IMM” portion)
func generateLoadImmJump(inst Instruction) []byte {
	dstIdx, vx, _ := extractOneRegOneImmOneOffset(inst.Args)
	r := regInfoList[dstIdx]
	rex := byte(0x49)
	if r.REXBit == 1 {
		rex |= 0x01 // set REX.B
	}
	movOp := byte(0xB8 + r.RegBits)
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)
	code := append([]byte{rex, movOp}, immBytes...)
	return append(code, 0xE9, 0xFE, 0xFE, 0xFE, 0xFE) // this will be patched
}

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
	tmp := regInfoList[9] // r11 scratch

	buf := make([]byte, 0, 32)
	buf = append(buf,
		0x50,       // push rax
		0x41, 0x53, // push r11
	)
	// mov r11, [base]
	rex := byte(0x48) // REX.W
	if tmp.REXBit == 1 {
		rex |= 0x04
	} // REX.R
	if base.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x8B, byte(0xC0|(tmp.RegBits<<3)|base.RegBits))

	// add r11, vx
	rex = 0x48
	if tmp.REXBit == 1 {
		rex |= 0x01
	} // REX.B
	buf = append(buf, rex, 0x81, byte(0xC0|(0<<3)|tmp.RegBits))
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vx))
	buf = append(buf, imm32...)
	// mov rax , r11
	buf = append(buf, 0x4c, 0x89, 0xD8) // mov rax, r11
	// pop r11
	buf = append(buf, 0x41, 0x5B) // pop r11
	// push rax
	buf = append(buf, 0x50) // push rax

	// MOV RAX, base_address  Load base into RAX
	buf = append(buf, 0x48, 0xB8, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE)
	buf = append(buf,
		0xFF, 0xE0, // JMP RAX
	)
	return buf
}

// LOAD_IMM_JUMP_IND
func generateLoadImmJumpIndirect(inst Instruction) []byte {

	dstIdx, indexRegIdx, vx, vy := extractTwoRegsAndTwoImmediates(inst.Args)
	r := regInfoList[dstIdx]
	fmt.Printf("**** LOAD_IMM_JUMP_IND generateLoadImmJumpIndirect dstIdx %d vx=%d\n", dstIdx, vx)
	fmt.Printf("dstIdx = %d, indexRegIdx = %d, vx = %d, vy = %d\n", dstIdx, indexRegIdx, vx, vy)
	// Build REX prefix: REX.W plus REX.B if dst is r8–r15
	MovCode := []byte{0x50} // push rax
	indexReg := regInfoList[indexRegIdx]
	// MOV RAX, indexReg
	rex := byte(0x48) // REX.W
	if indexReg.REXBit == 1 {
		rex |= 0x01 // REX.B
	}
	modrm := byte(0xC0 | (0 << 3) | indexReg.RegBits) // MOV RAX, indexReg
	MovCode = append(MovCode, rex, 0x8B, modrm)

	rex = byte(0x48) // 01001000b = REX.W=1
	if r.REXBit == 1 {
		rex |= 0x01 // set REX.B
	}

	// Opcode B8+rd  = MOV r64, imm64
	movOp := byte(0xB8 + r.RegBits)

	// Little‐endian encode the 64-bit immediate
	immBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(immBytes, vx)

	// [REX][MOV-opcode][imm64]
	code := append(MovCode, append([]byte{rex, movOp}, immBytes...)...)

	// ADD RAX, vy
	rex = 0x48 // REX.W
	code = append(code, rex, 0x05)
	imm32 := make([]byte, 4)
	binary.LittleEndian.PutUint32(imm32, uint32(vy))
	code = append(code, imm32...)
	// AND RAX, 0xFFFFFFFF  → 直接 zero-extend RAX 至 32 bit
	code = append(code,
		0x48, 0x81, 0xE0, // AND RAX, imm32
		0xFF, 0xFF, 0xFF, 0xFF,
	)
	// push rax
	code = append(code, 0x50) // push rax
	// MOV RAX
	code = append(code, 0x48, 0xB8, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE, 0xFE)
	code = append(code,
		0xFF, 0xE0, // JMP RAX
	)

	return code
}

// generateBranchImm emits:
//
//	REX.W + CMP r64, imm32         (7 bytes total)
//	0x0F, jcc                      (2 bytes)
//	rel32 placeholder (true‐target) (4 bytes)
//	0xE9                           (1 byte JMP opcode)
//	rel32 placeholder (false‐target)(4 bytes)
//
// Total = 18 bytes
func generateBranchImm(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		regIdx, imm, _ := extractOneRegOneImmOneOffset(inst.Args)
		r := regInfoList[regIdx]

		// REX.W + REX.B if needed
		rex := byte(0x48)
		if r.REXBit == 1 {
			rex |= 0x01
		}

		// CMP r64, imm32 → opcode 0x81 /7 id
		modrm := byte(0xC0 | (0x7 << 3) | r.RegBits)
		disp := int32(imm)

		return []byte{
			// 7-byte compare
			rex, 0x81, modrm,
			byte(disp), byte(disp >> 8), byte(disp >> 16), byte(disp >> 24),

			// 2-byte conditional jump
			0x0F, jcc,

			// 4-byte placeholder for true target
			0, 0, 0, 0,

			// 1-byte unconditional JMP opcode
			0xE9,

			// 4-byte placeholder for false target
			0, 0, 0, 0,
		}
	}
}

func generateCompareBranch(jcc byte) func(inst Instruction) []byte {
	return func(inst Instruction) []byte {
		aIdx, bIdx, _ := extractTwoRegsOneOffset(inst.Args)
		rA := regInfoList[aIdx]
		rB := regInfoList[bIdx]

		// REX.W + REX.R + REX.B
		rex := byte(0x48)
		if rB.REXBit == 1 {
			rex |= 0x04
		}
		if rA.REXBit == 1 {
			rex |= 0x01
		}

		// CMP r/m64, r64 (0x39 /r)
		modrm := byte(0xC0 | (rB.RegBits << 3) | rA.RegBits)

		return []byte{
			rex, 0x39, modrm, // 00–02
			0x0F, jcc, // 03–04
			0, 0, 0, 0, // 05–08 (relTrue placeholder)
			0xE9,       // 09 (JMP opcode)
			0, 0, 0, 0, // 10–13 (relFalse placeholder)
		}
	}
}
