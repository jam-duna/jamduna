package pvm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
)

func TestX86MemAddStore(t *testing.T) {
	memSize := 4096
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("failed to mmap memory: %v", err)
	}

	binary.LittleEndian.PutUint64(mem[0:], 1)
	binary.LittleEndian.PutUint64(mem[8:], 5)

	// x86code:
	// mov rbx, memAddr
	// mov rax, [rbx]
	// add rax, [rbx+8]
	// mov [rbx+16], rax
	// ret
	x86code := []byte{
		0x48, 0xbb, /* addr 8 bytes */ // mov rbx, memAddr
		0x48, 0x8b, 0x03, // mov rax, [rbx]
		0x48, 0x03, 0x43, 0x08, // add rax, [rbx+8]
		0x48, 0x89, 0x43, 0x10, // mov [rbx+16], rax
		0xc3, // ret
	}

	memAddr := uintptr(unsafe.Pointer(&mem[0]))
	addrBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(addrBuf, uint64(memAddr))
	x86code = append(x86code[:2], append(addrBuf, x86code[2:]...)...)

	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("failed to mmap exec code: %v", err)
	}
	copy(codeAddr, x86code)

	type execFunc func() int
	fnPtr := (uintptr)(unsafe.Pointer(&codeAddr))
	fn := *(*execFunc)(unsafe.Pointer(&fnPtr))
	result := fn()

	sum := binary.LittleEndian.Uint64(mem[16:])

	fmt.Printf("Result = %d (should be 6)\n", result)
	fmt.Printf("Memory[16] = %d (should be 6)\n", sum)
}

var familyToOpcodes = map[string][]string{
	"Trap":             {"TRAP"},
	"Fallthrough":      {"FALLTHROUGH"},
	"DirectJump":       {"JUMP", "LOAD_IMM_JUMP"},
	"IndirectJump":     {"JUMP_IND", "LOAD_IMM_JUMP_IND"},
	"CompareBranch":    {"BRANCH_EQ", "BRANCH_NE", "BRANCH_LT_U", "BRANCH_LT_S", "BRANCH_GE_U", "BRANCH_GE_S"},
	"BranchImm":        {"BRANCH_EQ_IMM", "BRANCH_NE_IMM", "BRANCH_LT_U_IMM", "BRANCH_LE_U_IMM", "BRANCH_GE_U_IMM", "BRANCH_GT_U_IMM", "BRANCH_LT_S_IMM", "BRANCH_LE_S_IMM", "BRANCH_GE_S_IMM", "BRANCH_GT_S_IMM"},
	"LoadImm64":        {"LOAD_IMM_64"},
	"StoreImmGeneric":  {"STORE_IMM_U8", "STORE_IMM_U16", "STORE_IMM_U32"},
	"StoreImmU64":      {"STORE_IMM_U64"},
	"LoadImm32":        {"LOAD_IMM"},
	"LoadWithBase":     {"LOAD_U8", "LOAD_I8", "LOAD_U16", "LOAD_I16", "LOAD_U32", "LOAD_I32", "LOAD_U64"},
	"StoreWithBase":    {"STORE_U8", "STORE_U16", "STORE_U32", "STORE_U64"},
	"StoreImmIndirect": {"STORE_IMM_IND_U8", "STORE_IMM_IND_U16", "STORE_IMM_IND_U32", "STORE_IMM_IND_U64"},
	"MoveAndBitManip":  {"MOVE_REG", "COUNT_SET_BITS_64", "COUNT_SET_BITS_32", "LEADING_ZERO_BITS_64", "LEADING_ZERO_BITS_32", "TRAILING_ZERO_BITS_64", "TRAILING_ZERO_BITS_32", "SIGN_EXTEND_8", "SIGN_EXTEND_16", "ZERO_EXTEND_16", "REVERSE_BYTES"},
	"StoreIndirect":    {"STORE_IND_U8", "STORE_IND_U16", "STORE_IND_U32", "STORE_IND_U64"},
	"LoadIndirect":     {"LOAD_IND_U8", "LOAD_IND_I8", "LOAD_IND_U16", "LOAD_IND_I16", "LOAD_IND_U32", "LOAD_IND_I32", "LOAD_IND_U64"},
	"BinaryImm32":      {"ADD_IMM_32", "NEG_ADD_IMM_32"},
	"ImmBinaryOp64":    {"AND_IMM", "XOR_IMM", "OR_IMM", "ADD_IMM_64", "NEG_ADD_IMM_64"},
	"ImmMulOp":         {"MUL_IMM_32", "MUL_IMM_64"},
	"ImmSetCondOp":     {"SET_LT_U_IMM", "SET_LT_S_IMM", "SET_GT_U_IMM", "SET_GT_S_IMM"},
	"ImmShiftOp":       {"SHLO_L_IMM_32", "SHLO_R_IMM_32", "SHLO_L_IMM_ALT_32", "SHLO_R_IMM_ALT_32", "SHAR_R_IMM_ALT_32", "SHAR_R_IMM_32", "SHLO_L_IMM_64", "SHLO_R_IMM_64", "SHAR_R_IMM_64", "SHLO_L_IMM_ALT_64", "SHLO_R_IMM_ALT_64", "SHAR_R_IMM_ALT_64", "ROT_R_64_IMM", "ROT_R_64_IMM_ALT", "ROT_R_32_IMM_ALT", "ROT_R_32_IMM"},
	"CmovImm":          {"CMOV_IZ_IMM", "CMOV_NZ_IMM"},
	"BinaryOp32":       {"ADD_32", "SUB_32"},
	"MulDivRem32":      {"MUL_32", "DIV_U_32", "DIV_S_32", "REM_U_32", "REM_S_32"},
	"ShiftOp32":        {"SHLO_L_32", "SHLO_R_32", "SHAR_R_32", "ROT_L_32", "ROT_R_32"},
	"BinaryOp64":       {"ADD_64", "SUB_64", "AND", "XOR", "OR"},
	"MulDivRem64":      {"MUL_64", "DIV_U_64", "DIV_S_64", "REM_U_64", "REM_S_64"},
	"ShiftOp64":        {"SHLO_L_64", "SHLO_R_64", "SHAR_R_64", "ROT_L_64", "ROT_R_64"},
	"MulUpperOp64":     {"MUL_UPPER_S_S", "MUL_UPPER_U_U", "MUL_UPPER_S_U"},
	"SetCondOp64":      {"SET_LT_U", "SET_LT_S"},
	"CmovOp64":         {"CMOV_IZ", "CMOV_NZ"},
	"BitwiseLogicOp64": {"AND_INV", "OR_INV", "XNOR"},
	"MinMaxOp":         {"MAX", "MAX_U", "MIN", "MIN_U"},
}

var testcases = map[string]string{
	// A.5.1. Instructions without Arguments
	"TRAP":        "inst_trap",
	"FALLTHROUGH": "inst_fallthrough",

	// A.5.2. Instructions with Arguments of One Immediate.
	// "ECALLI":                "",

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
	"LOAD_IMM_64": "inst_load_imm_64",

	// A.5.4. Instructions with Arguments of Two Immediates.
	"STORE_IMM_U8":  "inst_store_imm_u8,inst_store_imm_u8_trap_read_only,inst_store_imm_u8_trap_inaccessible,inst_store_imm_u8_trap_read_only",
	"STORE_IMM_U16": "inst_store_imm_u16",
	"STORE_IMM_U32": "inst_store_imm_u32",
	"STORE_IMM_U64": "inst_store_imm_u64",

	// A.5.5. Instructions with Arguments of One Offset.
	"JUMP": "inst_jump",

	// A.5.6. Instructions with Arguments of One Register & Two Immediates.
	"JUMP_IND":  "inst_ret_halt,inst_ret_invalid,inst_jump_indirect_invalid_djump_to_zero_nok,inst_jump_indirect_misaligned_djump_with_offset_nok,inst_jump_indirect_misaligned_djump_without_offset_nok,inst_jump_indirect_with_offset_ok,inst_jump_indirect_without_offset_ok",
	"LOAD_IMM":  "inst_load_imm",
	"LOAD_U8":   "inst_load_u8,inst_load_u8_nok",
	"LOAD_I8":   "inst_load_i8",
	"LOAD_U16":  "inst_load_u16",
	"LOAD_I16":  "inst_load_i16",
	"LOAD_U32":  "inst_load_u32",
	"LOAD_I32":  "inst_load_i32",
	"LOAD_U64":  "inst_load_u64",
	"STORE_U8":  "inst_store_u8,inst_store_u8_trap_inaccessible,inst_store_u8_trap_read_only",
	"STORE_U16": "inst_store_u16",
	"STORE_U32": "inst_store_u32",
	"STORE_U64": "inst_store_u64",

	// A.5.7. Instructions with Arguments of One Register and Two Immediates.
	"STORE_IMM_IND_U8":  "inst_store_imm_indirect_u8_with_offset_ok,inst_store_imm_indirect_u8_with_offset_nok,inst_store_imm_indirect_u8_without_offset_ok",
	"STORE_IMM_IND_U16": "inst_store_imm_indirect_u16_with_offset_ok,inst_store_imm_indirect_u16_with_offset_nok,inst_store_imm_indirect_u16_without_offset_ok",
	"STORE_IMM_IND_U32": "inst_store_imm_indirect_u32_with_offset_ok,inst_store_imm_indirect_u32_with_offset_nok,inst_store_imm_indirect_u32_without_offset_ok",
	"STORE_IMM_IND_U64": "inst_store_imm_indirect_u64_with_offset_ok,inst_store_imm_indirect_u64_with_offset_nok,inst_store_imm_indirect_u64_without_offset_ok",

	// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
	"LOAD_IMM_JUMP":   "inst_load_imm_and_jump",
	"BRANCH_EQ_IMM":   "inst_branch_eq_imm_ok,inst_branch_eq_imm_nok",
	"BRANCH_NE_IMM":   "inst_branch_not_eq_imm_ok,inst_branch_not_eq_imm_nok",
	"BRANCH_LT_U_IMM": "inst_branch_less_unsigned_imm_ok,inst_branch_less_unsigned_imm_nok",
	"BRANCH_LE_U_IMM": "inst_branch_less_or_equal_unsigned_imm_ok,inst_branch_less_or_equal_unsigned_imm_nok",
	"BRANCH_GE_U_IMM": "inst_branch_greater_or_equal_unsigned_imm_ok,inst_branch_greater_or_equal_unsigned_imm_nok",
	"BRANCH_GT_U_IMM": "inst_branch_greater_unsigned_imm_ok,inst_branch_greater_unsigned_imm_nok",
	"BRANCH_LT_S_IMM": "inst_branch_less_signed_imm_ok,inst_branch_less_signed_imm_nok",
	"BRANCH_LE_S_IMM": "inst_branch_less_or_equal_signed_imm_ok,inst_branch_less_or_equal_signed_imm_nok",
	"BRANCH_GE_S_IMM": "inst_branch_greater_or_equal_signed_imm_ok,inst_branch_greater_or_equal_signed_imm_nok",
	"BRANCH_GT_S_IMM": "inst_branch_greater_signed_imm_ok,inst_branch_greater_signed_imm_nok",

	// A.5.9. Instructions with Arguments of Two Registers.
	// "SBRK":                  "",
	"MOVE_REG": "inst_move_reg",
	// "COUNT_SET_BITS_64":     "",
	// "COUNT_SET_BITS_32":     "",
	// "LEADING_ZERO_BITS_64":  "",
	// "LEADING_ZERO_BITS_32":  "",
	// "TRAILING_ZERO_BITS_64": "",
	// "TRAILING_ZERO_BITS_32": "",
	// "SIGN_EXTEND_8":         "",
	// "SIGN_EXTEND_16":        "",
	// "ZERO_EXTEND_16":        "",
	// "REVERSE_BYTES":         "",

	// A.5.10 Instructions with Arguments of Two Registers and One Immediate. (120-161)
	"STORE_IND_U8":      "inst_store_indirect_u8_with_offset_ok,inst_store_indirect_u8_without_offset_ok,inst_store_indirect_u8_with_offset_nok",
	"STORE_IND_U16":     "inst_store_indirect_u16_with_offset_ok,inst_store_indirect_u16_with_offset_nok,inst_store_indirect_u16_without_offset_ok",
	"STORE_IND_U32":     "inst_store_indirect_u32_with_offset_ok,inst_store_indirect_u32_with_offset_nok,inst_store_indirect_u32_without_offset_ok",
	"STORE_IND_U64":     "inst_store_indirect_u64_with_offset_ok,inst_store_indirect_u64_without_offset_ok,inst_store_indirect_u64_with_offset_nok",
	"LOAD_IND_U8":       "inst_load_indirect_u8_with_offset,inst_load_indirect_u8_without_offset",
	"LOAD_IND_I8":       "inst_load_indirect_i8_with_offset,inst_load_indirect_i8_without_offset",
	"LOAD_IND_U16":      "inst_load_indirect_u16_with_offset,inst_load_indirect_u16_without_offset",
	"LOAD_IND_I16":      "inst_load_indirect_i16_with_offset,inst_load_indirect_i16_without_offset",
	"LOAD_IND_U32":      "inst_load_indirect_u32_with_offset,inst_load_indirect_u32_without_offset",
	"LOAD_IND_I32":      "inst_load_indirect_i32_with_offset,inst_load_indirect_i32_without_offset",
	"LOAD_IND_U64":      "inst_load_indirect_u64_with_offset,inst_load_indirect_u64_without_offset",
	"ADD_IMM_32":        "inst_add_imm_32,inst_add_imm_32_with_truncation,inst_add_imm_32_with_truncation_and_sign_extension,inst_sub_imm_32",
	"AND_IMM":           "inst_and_imm",
	"XOR_IMM":           "inst_xor_imm",
	"OR_IMM":            "inst_or_imm",
	"MUL_IMM_32":        "inst_mul_imm_32",
	"SET_LT_U_IMM":      "inst_set_less_than_unsigned_imm_0,inst_set_less_than_unsigned_imm_1",
	"SET_LT_S_IMM":      "inst_set_less_than_signed_imm_0,inst_set_less_than_signed_imm_1",
	"SHLO_L_IMM_32":     "inst_shift_logical_left_imm_32",
	"SHLO_R_IMM_32":     "inst_shift_logical_right_imm_32",
	"SHAR_R_IMM_32":     "inst_shift_arithmetic_right_imm_32",
	"NEG_ADD_IMM_32":    "inst_negate_and_add_imm_32",
	"SET_GT_U_IMM":      "inst_set_greater_than_unsigned_imm_0,inst_set_greater_than_unsigned_imm_1",
	"SET_GT_S_IMM":      "inst_set_greater_than_signed_imm_0,inst_set_greater_than_signed_imm_1",
	"SHLO_L_IMM_ALT_32": "inst_shift_logical_left_imm_alt_32",
	"SHLO_R_IMM_ALT_32": "inst_shift_logical_right_imm_alt_32",
	"SHAR_R_IMM_ALT_32": "inst_shift_arithmetic_right_imm_alt_32",
	"CMOV_IZ_IMM":       "inst_cmov_if_zero_imm_ok,inst_cmov_if_zero_imm_nok",
	// "CMOV_NZ_IMM":           "",
	"ADD_IMM_64":        "inst_add_imm_64,inst_sub_imm_64",
	"MUL_IMM_64":        "inst_mul_imm_64",
	"SHLO_L_IMM_64":     "inst_shift_logical_left_imm_64",
	"SHLO_R_IMM_64":     "inst_shift_logical_right_imm_64",
	"SHAR_R_IMM_64":     "inst_shift_arithmetic_right_imm_64",
	"NEG_ADD_IMM_64":    "inst_negate_and_add_imm_64",
	"SHLO_L_IMM_ALT_64": "inst_shift_logical_left_imm_alt_64",
	"SHLO_R_IMM_ALT_64": "inst_shift_logical_right_imm_alt_64",
	"SHAR_R_IMM_ALT_64": "inst_shift_arithmetic_right_imm_alt_64",
	// "ROT_R_64_IMM":          "",
	// "ROT_R_64_IMM_ALT":      "",
	// "ROT_R_32_IMM":          "",
	// "ROT_R_32_IMM_ALT":      "",

	// A.5.11. Instructions with Arguments of One Register, One Immediate and One Offset. (170-175)
	"BRANCH_EQ":   "inst_branch_eq_ok,inst_branch_eq_nok",
	"BRANCH_NE":   "inst_branch_not_eq_ok,inst_branch_not_eq_nok",
	"BRANCH_LT_U": "inst_branch_less_unsigned_ok,inst_branch_less_unsigned_nok",
	"BRANCH_LT_S": "inst_branch_less_signed_ok,inst_branch_less_signed_nok",
	"BRANCH_GE_U": "inst_branch_greater_or_equal_unsigned_ok,inst_branch_greater_or_equal_unsigned_nok",
	"BRANCH_GE_S": "inst_branch_greater_or_equal_signed_ok,inst_branch_greater_or_equal_signed_nok",

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates. (180)
	"LOAD_IMM_JUMP_IND": "inst_load_imm_and_jump,inst_load_imm_and_jump_indirect_different_regs_with_offset_ok,inst_load_imm_and_jump_indirect_different_regs_without_offset_ok,inst_load_imm_and_jump_indirect_invalid_djump_to_zero_different_regs_without_offset_nok,inst_load_imm_and_jump_indirect_invalid_djump_to_zero_same_regs_without_offset_nok,inst_load_imm_and_jump_indirect_misaligned_djump_different_regs_with_offset_nok,inst_load_imm_and_jump_indirect_misaligned_djump_different_regs_without_offset_nok,inst_load_imm_and_jump_indirect_misaligned_djump_same_regs_with_offset_nok,inst_load_imm_and_jump_indirect_misaligned_djump_same_regs_without_offset_nok,inst_load_imm_and_jump_indirect_same_regs_with_offset_ok,inst_load_imm_and_jump_indirect_same_regs_without_offset_ok",

	// A.5.13. Instructions with Arguments of Three Registers. (190-230)
	"ADD_32":    "inst_add_32,inst_add_32_with_overflow,inst_add_32_with_truncation,inst_add_32_with_truncation_and_sign_extension",
	"SUB_32":    "inst_sub_32,inst_sub_32_with_overflow",
	"MUL_32":    "inst_mul_32",
	"DIV_U_32":  "inst_div_unsigned_32,inst_div_unsigned_32_by_zero,inst_div_unsigned_32_with_overflow",
	"DIV_S_32":  "inst_div_signed_32,inst_div_signed_32_by_zero,inst_div_signed_32_with_overflow",
	"REM_U_32":  "inst_rem_unsigned_32,inst_rem_unsigned_32_by_zero",
	"REM_S_32":  "inst_rem_signed_32,inst_rem_signed_32_by_zero,inst_rem_signed_32_with_overflow",
	"SHLO_L_32": "inst_shift_logical_left_32,inst_shift_logical_left_32_with_overflow",
	"SHLO_R_32": "inst_shift_logical_right_32,inst_shift_logical_right_32_with_overflow",
	"SHAR_R_32": "inst_shift_arithmetic_right_32,inst_shift_arithmetic_right_32_with_overflow",
	"ADD_64":    "inst_add_64,inst_add_64_with_overflow",
	"SUB_64":    "inst_sub_64,inst_sub_64_with_overflow",
	"DIV_U_64":  "inst_div_unsigned_64,inst_div_unsigned_64_by_zero,inst_div_unsigned_64_with_overflow",
	"DIV_S_64":  "inst_div_signed_64,inst_div_signed_64_by_zero,inst_div_signed_64_with_overflow",
	"REM_U_64":  "inst_rem_unsigned_64,inst_rem_unsigned_64_by_zero",
	"REM_S_64":  "inst_rem_signed_64,inst_rem_signed_64_by_zero",
	"SHLO_L_64": "inst_shift_logical_left_64,inst_shift_logical_left_64_with_overflow",
	"SHLO_R_64": "inst_shift_logical_right_64,inst_shift_logical_right_64_with_overflow",
	"SHAR_R_64": "inst_shift_arithmetic_right_64",
	"AND":       "inst_and",
	"XOR":       "inst_xor",
	"OR":        "inst_or",
	//"MUL_UPPER_S_S":         "",
	//"MUL_UPPER_U_U":         "",
	//"MUL_UPPER_S_U":         "",
	"SET_LT_U": "inst_set_less_than_unsigned_0,inst_set_less_than_unsigned_1",
	"SET_LT_S": "inst_set_less_than_signed_0,inst_set_less_than_signed_1",
	"CMOV_IZ":  "inst_cmov_if_zero_ok,inst_cmov_if_zero_nok",
	//"CMOV_NZ":               "",
	//"ROT_L_64":              "",
	//"ROT_L_32":              "",
	"MUL_64": "inst_mul_64",
	//"ROT_R_64":              "",
	//"ROT_R_32":              "",
	//"AND_INV":               "",
	//"OR_INV":                "",
	//"XNOR": "",
	//"MAX":                   "",
	//"MAX_U":                 "",
	//"MIN":                   "",
	//"MIN_U":                 "",
}

// TestCompilerSuccess runs all test cases expected to succeed (no division by zero or overflow conditions).
// var errorNames = []string{
// 	"by_zero", "overflow", "trap", "nok", "inaccessible", "read_only",
// }

func TestCompilerFull(t *testing.T) {
	PvmLogging = true
	PvmTrace = true

	// Directory containing the JSON files
	dir := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs")

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	mismatchs := make(map[int][]string)
	memory_mismatch := 0
	register_mismatch := 1
	result_code_mismatch := 2
	crashed := 4
	for i := 0; i < 3; i++ {
		mismatchs[i] = make([]string, 0)
	}
	// 183 tests pass, these 22 tests fail
	suppress := []string{
		// "inst_ret_halt.json",            // RET_HALT
		"inst_cmov_if_zero_imm_ok.json", // this one is the weird case
		// "inst_load_imm_and_jump.json", // LOAD_IMM_JUMP
		// "inst_load_imm_and_jump_indirect_different_regs_with_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_different_regs_without_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_same_regs_with_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_same_regs_without_offset_ok.json",
	}

	for _, file := range files {
		name := file.Name()
		fmt.Println(name)
		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		if slices.Contains(suppress, name) {
			continue // skip suppressed tests
		}
		filePath := dir + "/" + name
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var tc TestCase
		err = json.Unmarshal(data, &tc)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		t.Run(name, func(t *testing.T) {
			fmt.Printf("Running test: %s\n", name)
			err = compiler_test(tc)
			if err != nil {
				//Memory mismatch
				//result code mismatch
				//register mismatch
				t.Errorf("❌ [%s] Test failed: %v", name, err)
				if strings.Contains(err.Error(), "Memory mismatch") {
					mismatchs[memory_mismatch] = append(mismatchs[memory_mismatch], name)
				} else if strings.Contains(err.Error(), "result code mismatch") {
					mismatchs[result_code_mismatch] = append(mismatchs[result_code_mismatch], name)
				} else if strings.Contains(err.Error(), "register mismatch") {
					mismatchs[register_mismatch] = append(mismatchs[register_mismatch], name)
				} else if strings.Contains(err.Error(), "ExecuteX86 crash detected") {
					mismatchs[crashed] = append(mismatchs[crashed], name)
				} else {
					t.Errorf("Unexpected error: %v", err)
				}

			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
			count++
		})
	}
	fmt.Printf("Compiler Test Summary:===========================================\n")
	fmt.Printf("Total tests run: %d\n", count)
	fmt.Printf("Memory mismatch: %d (%.2f%%)\n", len(mismatchs[memory_mismatch]), float64(len(mismatchs[memory_mismatch]))/float64(count)*100)
	fmt.Printf("Register mismatch: %d (%.2f%%)\n", len(mismatchs[register_mismatch]), float64(len(mismatchs[register_mismatch]))/float64(count)*100)
	fmt.Printf("Result code mismatch: %d (%.2f%%)\n", len(mismatchs[result_code_mismatch]), float64(len(mismatchs[result_code_mismatch]))/float64(count)*100)
	fmt.Printf("Crashed: %d (%.2f%%)\n", len(mismatchs[crashed]), float64(len(mismatchs[crashed]))/float64(count)*100)
	success := count - len(mismatchs[memory_mismatch]) - len(mismatchs[register_mismatch]) - len(mismatchs[result_code_mismatch]) - len(mismatchs[crashed])
	fmt.Printf("Success: %d (%.2f%%)\n", success, float64(success)/float64(count)*100)
	for i, mismatch := range mismatchs {
		if len(mismatch) == 0 {
			continue
		}
		switch i {
		case memory_mismatch:
			fmt.Printf("Memory Mismatches: %d\n", len(mismatch))

		case register_mismatch:
			fmt.Printf("Register Mismatches: %d\n", len(mismatch))

		case result_code_mismatch:
			fmt.Printf("Result Code Mismatches: %d\n", len(mismatch))
		case crashed:
			fmt.Printf("Crashed: %d\n", len(mismatch))
		}

		for _, name := range mismatch {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Printf("===================================================================\n")
}

func compiler_test(tc TestCase) error {
	var num_mismatch int
	pvmBackend := BackendCompiler
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, pvmBackend)
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		pvm.Ram.WriteRAMBytes(mem.Address, mem.Data[:])
	}
	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	pvm.Gas = 100000000000
	rvm, err := NewCompilerVM(pvm)
	if err != nil {
		return fmt.Errorf("failed to create compiler VM: %w", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccess(pm.Address, pm.Length, PageMutable)
			if err != nil {
				return fmt.Errorf("failed to set memory access for address %x: %w", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemory(mem.Address, mem.Data)
	}
	rvm.pc = 0
	for i, reg := range tc.InitialRegs {
		if err := rvm.Ram.WriteRegister(i, reg); err == OOB {
			return fmt.Errorf("failed to write initial register %d: %v", i, err)
		} else {
			fmt.Printf("Register %d initialized to %d\n", i, reg)
		}
	}
	rvm.Execute(0)
	// check the memory
	for _, mem := range tc.ExpectedMemory {
		data, err := rvm.ReadMemory(mem.Address, uint32(len(mem.Data)))
		if err != nil {
			return fmt.Errorf("failed to read memory at address %x: %w", mem.Address, err)
		}
		if !bytes.Equal(data, mem.Data) {
			num_mismatch++
			return fmt.Errorf("Memory mismatch for test %s at address %x: expected %x, got %x \n", tc.Name, mem.Address, mem.Data, data)
		} else {
			fmt.Printf("Memory match for test %s at address %x \n", tc.Name, mem.Address)
		}
	}
	for i, reg := range rvm.Ram.ReadRegisters() {
		if reg != tc.ExpectedRegs[i] {
			num_mismatch++
			v, _ := rvm.Ram.ReadRegister(i)
			fmt.Printf("MISMATCH expected %v got %v\n", tc.ExpectedRegs, v)
			return fmt.Errorf("register mismatch for test %s at index %d: expected %d, got %d", tc.Name, i, tc.ExpectedRegs[i], reg)
		}
	}
	resultCode = rvm.ResultCode
	// Check the registers
	if equalIntSlices(pvm.Ram.ReadRegisters(), tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}
	rvm.Close()

	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}
	if resultCodeStr == expectedCodeStr {
		fmt.Printf("Result code match for test %s: %s\n", tc.Name, resultCodeStr)
	} else {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.Ram.ReadRegisters())
}

func TestSingleInterpreter(t *testing.T) {

	PvmLogging = true
	PvmTrace = true

	name := "inst_add_32"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), fmt.Sprintf("pvm/programs/%s.json", name))
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var testCase TestCase
	err = json.Unmarshal(data, &testCase)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}

	t.Run(name, func(t *testing.T) {
		err = pvm_test(testCase)
		if err != nil {
			t.Errorf("❌ [%s] Test failed: %v", name, err)
		} else {
			t.Logf("✅ [%s] Test passed", name)
		}
	})
}

func TestHostFuncExposeCompiler(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs", name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendCompiler)
	pvm.Gas = 100000000000
	rvm, err := NewCompilerVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create CompilerVM: %v", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccess(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemory(mem.Address, mem.Data)
	}

	// rvm.initStartCode()

	// rvm.x86Code = rvm.startCode
	ecallicode, err := rvm.EcalliCode(INFO)
	if err != nil {
		t.Fatalf("Failed to generate Ecalli code: %v", err)
	}
	rvm.x86Code = append(rvm.x86Code, ecallicode...)
	rvm.x86Code = append(rvm.x86Code, ecallicode...)

	rvm.x86Code = append(rvm.x86Code, 0xC3) // ret
	// fmt.Printf("Disassembled x86 code:\n%s\n", Disassemble(rvm.x86Code))
	err = rvm.ExecuteX86CodeWithEntry(rvm.x86Code, 0)
	if err != nil {
		t.Fatalf("Failed to execute x86 code: %v", err)
	}
}

func TestSBRK(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs", name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendCompiler)

	//ramdom set the reg to do the sbrk test
	pvm.Ram.WriteRegister(5, 8888)
	pvm.Ram.WriteRegister(6, 6)

	rvm, err := NewCompilerVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create CompilerVM: %v", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccess(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemory(mem.Address, mem.Data)
	}

	rvm.x86Code = rvm.startCode
	code := append(rvm.DumpRegisterToMemory(true), EmitCallToSbrkStub(uintptr(unsafe.Pointer(rvm)), uint32(5), uint32(6))...)
	code = append(code, rvm.RestoreRegisterInX86()...)
	rvm.x86Code = append(rvm.x86Code, code...)
	rvm.x86Code = append(rvm.x86Code, rvm.DumpRegisterToMemory(false)...)
	rvm.x86Code = append(rvm.x86Code, 0xC3) // ret
	// fmt.Printf("Disassembled x86 code:\n%s\n", Disassemble(rvm.x86Code))
	err = rvm.ExecuteX86Code(rvm.x86Code)
	if err != nil {
		t.Fatalf("Failed to execute x86 code: %v", err)
	}
}

func TestUnicornBasicEmulation(t *testing.T) {
	const (
		ADDRESS = 0x1000
	)
	code := []byte{0xB8, 0xD2, 0x04, 0x00, 0x00} // mov eax, 1234

	emu, err := uc.NewUnicorn(uc.ARCH_X86, uc.MODE_64)
	if err != nil {
		t.Fatalf("Failed to create emulator: %v", err)
	}
	defer emu.Close()

	if err := emu.MemMap(ADDRESS, 0x1000); err != nil {
		t.Fatalf("Memory map failed: %v", err)
	}

	if err := emu.MemWrite(ADDRESS, code); err != nil {
		t.Fatalf("Failed to write machine code: %v", err)
	}

	if err := emu.Start(ADDRESS, ADDRESS+uint64(len(code))); err != nil {
		t.Fatalf("Emulation failed: %v", err)
	}

	eax, err := emu.RegRead(uc.X86_REG_EAX)
	if err != nil {
		t.Fatalf("Failed to read EAX: %v", err)
	}

	if eax != 1234 {
		t.Fatalf("EAX should be 1234, got %d", eax)
	} else {
		fmt.Printf("EAX is correctly set to %d\n", eax)
	}
}

func compiler_sandbox_test(tc TestCase) error {
	var num_mismatch int

	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendSandbox)
	// Set the initial memory
	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	rvm, err := NewCompilerSandboxVM(pvm)
	if err != nil {
		return fmt.Errorf("failed to create compiler VM: %w", err)
	}
	rvm.Gas = 1000000 // set a high gas limit for the sandbox
	codeLines := rvm.DisassemblePVM()
	for _, line := range codeLines {
		fmt.Printf("%s\n", line)
	}
	_, codeLines = rvm.DisassemblePVMOfficial()
	for _, line := range codeLines {
		fmt.Printf("%s\n", line)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.sandBox.SetMemAccessSandBox(pm.Address, pm.Length, PageMutable)
			if err != nil {
				return fmt.Errorf("failed to set memory access for address %x: %w", pm.Address, err)
			}
		}
	}

	for i, regV := range tc.InitialRegs {
		// Write the initial register values
		rvm.Ram.WriteRegister(i, regV)
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.sandBox.WriteMemorySandBox(mem.Address, mem.Data)
	}
	rvm.pc = 0
	err = rvm.ExecuteSandBox(0)
	if err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
		return fmt.Errorf("ExecuteX86 crash detected: %w", err)
	}
	// check the memory
	for _, mem := range tc.ExpectedMemory {
		data, err := rvm.sandBox.ReadMemorySandBox(mem.Address, uint32(len(mem.Data)))
		if err != nil {
			return fmt.Errorf("failed to read memory at address %x: %w", mem.Address, err)
		}
		if !bytes.Equal(data, mem.Data) {
			num_mismatch++
			return fmt.Errorf("Memory mismatch for test %s at address %x: expected %x, got %x \n", tc.Name, mem.Address, mem.Data, data)
		} else {
			fmt.Printf("Memory match for test %s at address %x \n", tc.Name, mem.Address)
		}
	}
	for i, reg := range rvm.Ram.ReadRegisters() {
		if reg != tc.ExpectedRegs[i] {
			num_mismatch++
			fmt.Printf("MISMATCH expected %v got %v\n", tc.ExpectedRegs, rvm.Ram.ReadRegisters())
			return fmt.Errorf("register mismatch for test %s at index %d: expected %d, got %d", tc.Name, i, tc.ExpectedRegs[i], reg)
		}
	}
	resultCode = rvm.ResultCode

	// Check the registers
	if equalIntSlices(pvm.Ram.ReadRegisters(), tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}
	rvm.Close()
	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}
	if resultCodeStr == expectedCodeStr {
		fmt.Printf("Result code match for test %s: %s\n", tc.Name, resultCodeStr)
	} else {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.Ram.ReadRegisters())
}

func TestSingleSandbox(t *testing.T) {

	PvmLogging = false
	PvmTrace = false
	debugCompiler = false
	showDisassembly = false

	name := "inst_store_imm_indirect_u16_with_offset_ok"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs", name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var testCase TestCase
	err = json.Unmarshal(data, &testCase)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}

	t.Run(name, func(t *testing.T) {
		err = compiler_sandbox_test(testCase)
		if err != nil {
			t.Errorf("❌ [%s] Test failed: %v", name, err)
		} else {
			t.Logf("✅ [%s] Test passed", name)
		}
	})
}

func TestCompilerFamilies(t *testing.T) {
	familyNames := []string{"BranchImm", "CompareBranch"}
	for _, familyName := range familyNames {
		t.Run(familyName, func(t *testing.T) {
			testCompilerFamily(t, familyName)
		})
	}
}

// testCompilerFamily runs all tests associated with a specific, hard-coded generator family.
func testCompilerFamily(t *testing.T, familyName string) {
	// To test a different group of instructions, change the value of this variable.
	showDisassembly = true
	opcodes, ok := familyToOpcodes[familyName]
	if !ok {
		t.Fatalf("Unknown family specified in test: %s", familyName)
	}

	fmt.Printf("--- Running tests for family: %s ---\n", familyName)

	// Collect all test case files for the specified family.
	var testsToRun [][2]string // Store pairs of [opcodeName, testName]
	for _, opcodeName := range opcodes {
		testCaseString, found := testcases[opcodeName]
		if !found {
			t.Logf("  Warning: No test cases found for opcode %s", opcodeName)
			continue
		}
		// The testcases map has comma-separated values.
		for _, testName := range strings.Split(testCaseString, ",") {
			testsToRun = append(testsToRun, [2]string{opcodeName, testName})
		}
	}

	if len(testsToRun) == 0 {
		t.Fatalf("No test files found for family: %s", familyName)
	}

	fmt.Printf("\nFound %d total test cases to run for family %s\n", len(testsToRun), familyName)

	// --- Standard Test Execution Logic ---
	dir := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs") // Adjust this path if necessary

	for _, testInfo := range testsToRun {
		opcodeName := testInfo[0]
		testName := testInfo[1]
		fileName := testName + ".json"
		filePath := dir + "/" + fileName

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Logf("Warning: Test file not found, skipping: %s", filePath)
			continue
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", filePath, err)
			continue
		}

		var tc TestCase
		err = json.Unmarshal(data, &tc)
		if err != nil {
			t.Errorf("Failed to unmarshal JSON from file %s: %v", filePath, err)
			continue
		}

		// Run the individual test case as a subtest.
		// The subtest name now includes the opcode for better logging.
		t.Run(fmt.Sprintf("%s/%s", opcodeName, testName), func(t *testing.T) {
			t.Logf("Test: [%s - %s] %s\n", familyName, opcodeName, testName)
			// Assuming compiler_sandbox_test exists and is the entry point for a single test case.
			err := compiler_sandbox_test(tc)
			if err != nil {
				t.Errorf("❌ Test failed: %v", err)
			} else {
				t.Logf("✅ Test passed")
			}
		})
	}
}
func TestCompilerSandBox(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	// Directory containing the JSON files
	dir := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs")

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	mismatchs := make(map[int][]string)
	memory_mismatch := 0
	register_mismatch := 1
	result_code_mismatch := 2
	crashed := 4
	for i := 0; i < 3; i++ {
		mismatchs[i] = make([]string, 0)
	}
	// 183 tests pass, these 22 tests fail
	suppress := []string{
		// "inst_load_imm_and_jump.json", // LOAD_IMM_JUMP
		// "inst_load_imm_and_jump_indirect_different_regs_with_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_different_regs_without_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_same_regs_with_offset_ok.json",
		// "inst_load_imm_and_jump_indirect_same_regs_without_offset_ok.json",
	}

	for _, file := range files {
		name := file.Name()
		fmt.Println(name)
		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		if slices.Contains(suppress, name) {
			continue // skip suppressed tests
		}
		filePath := dir + "/" + name
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var tc TestCase
		err = json.Unmarshal(data, &tc)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		t.Run(name, func(t *testing.T) {
			err = compiler_sandbox_test(tc)
			if err != nil {
				//Memory mismatch
				//result code mismatch
				//register mismatch
				t.Errorf("❌ [%s] Test failed: %v", name, err)
				if strings.Contains(err.Error(), "Memory mismatch") {
					mismatchs[memory_mismatch] = append(mismatchs[memory_mismatch], name)
				} else if strings.Contains(err.Error(), "result code mismatch") {
					mismatchs[result_code_mismatch] = append(mismatchs[result_code_mismatch], name)
				} else if strings.Contains(err.Error(), "register mismatch") {
					mismatchs[register_mismatch] = append(mismatchs[register_mismatch], name)
				} else if strings.Contains(err.Error(), "ExecuteX86 crash detected") {
					mismatchs[crashed] = append(mismatchs[crashed], name)
				} else {
					t.Errorf("Unexpected error: %v", err)
				}

			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
			count++
		})
	}
	fmt.Printf("Compiler Test Summary:===========================================\n")
	fmt.Printf("Total tests run: %d\n", count)
	fmt.Printf("Memory mismatch: %d (%.2f%%)\n", len(mismatchs[memory_mismatch]), float64(len(mismatchs[memory_mismatch]))/float64(count)*100)
	fmt.Printf("Register mismatch: %d (%.2f%%)\n", len(mismatchs[register_mismatch]), float64(len(mismatchs[register_mismatch]))/float64(count)*100)
	fmt.Printf("Result code mismatch: %d (%.2f%%)\n", len(mismatchs[result_code_mismatch]), float64(len(mismatchs[result_code_mismatch]))/float64(count)*100)
	fmt.Printf("Crashed: %d (%.2f%%)\n", len(mismatchs[crashed]), float64(len(mismatchs[crashed]))/float64(count)*100)
	success := count - len(mismatchs[memory_mismatch]) - len(mismatchs[register_mismatch]) - len(mismatchs[result_code_mismatch]) - len(mismatchs[crashed])
	fmt.Printf("Success: %d (%.2f%%)\n", success, float64(success)/float64(count)*100)
	for i, mismatch := range mismatchs {
		if len(mismatch) == 0 {
			continue
		}
		switch i {
		case memory_mismatch:
			fmt.Printf("Memory Mismatches: %d\n", len(mismatch))

		case register_mismatch:
			fmt.Printf("Register Mismatches: %d\n", len(mismatch))

		case result_code_mismatch:
			fmt.Printf("Result Code Mismatches: %d\n", len(mismatch))
		case crashed:
			fmt.Printf("Crashed: %d\n", len(mismatch))
		}

		for _, name := range mismatch {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Printf("===================================================================\n")
}
func TestHostFuncExposeSandBox(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs", name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendSandbox)
	rvm, err := NewCompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create CompilerVM: %v", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.sandBox.SetMemAccessSandBox(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.sandBox.WriteMemorySandBox(mem.Address, mem.Data)
	}

	rvm.initStartCode()
	rvm.x86Code = rvm.startCode
	ecallicode, err := rvm.EcalliCodeSandBox(INFO)
	if err != nil {
		t.Fatalf("Failed to generate Ecalli code: %v", err)
	}
	rvm.x86Code = append(rvm.x86Code, ecallicode...)
	rvm.x86Code = append(rvm.x86Code, 0xCC)
	// fmt.Printf("Disassembled x86 code:\n%s\n", Disassemble(rvm.x86Code))
	rvm.ExecuteX86Code_SandBox(rvm.x86Code)

}

func TestSBRKSandBox(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs", name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendSandbox)

	//ramdom set the reg to do the sbrk test
	pvm.Ram.WriteRegister(5, 8888)
	pvm.Ram.WriteRegister(6, 6)

	rvm, err := NewCompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create CompilerVM: %v", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.sandBox.SetMemAccessSandBox(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.sandBox.WriteMemorySandBox(mem.Address, mem.Data)
	}

	rvm.x86Code = rvm.startCode
	code := append(rvm.DumpRegisterToMemory(true), EmitCallToSbrkStubSandBox(uintptr(unsafe.Pointer(rvm)), uint32(5), uint32(6), rvm.ecallAddr+rvm.sbrkOffset)...)
	code = append(code, rvm.RestoreRegisterInX86()...)
	rvm.x86Code = append(rvm.x86Code, code...)
	rvm.x86Code = append(rvm.x86Code, rvm.DumpRegisterToMemory(false)...)
	rvm.x86Code = append(rvm.x86Code, 0xC3) // ret
	// fmt.Printf("Disassembled x86 code:\n%s\n", Disassemble(rvm.x86Code))
	rvm.ExecuteX86Code_SandBox(rvm.x86Code)

}

func TestCodeIsSame(t *testing.T) {
	PvmLogging = false
	PvmTrace = false

	useRawRam = true // use raw RAM for this test
	debugCompiler = true

	// set up the code for the Doom self-playing test case
	fp := "../services/algo.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	// set up the VM with the test case
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	metadata := "algo"
	pvm := NewVM(DoomServiceID, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendSandbox)
	rvm, err := NewCompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create CompilerSandboxVM: %v", err)
	}
	a := make([]byte, 0)
	rvm.Standard_Program_Initialization_SandBox(a)
	rvm.SetRecoredGeneratedCode(true)
	rvm.initStartCode()
	rvm.Compile(0)
	// Get the three hashes we want to test
	x86CodeHash := common.Blake2Hash(rvm.genreatedCode)
	// use the generated code for the x])
	rvm.Patch(rvm.x86Code, 0)
	djumpTableHash := common.Blake2Hash(rvm.djumpTableFunc)

	// Combined code hash (for backward compatibility)

	// x86Code hash: 0x32544f520aa21576f7fc5224275cecafeb522393e3c0fc2e8c011884f352f0b8
	// djumpTableFunc hash: 0x76d1a0c42becc37e9b8f0be067e3f19b2afd008934f201e8e4293eccf0f69209
	// Combined code hash: 0xae58870518c960d2a0bbe05e25c58798c977ddfc24363424c35c1eab3b3a1d44
	fmt.Printf("x86Code hash: %s\n", x86CodeHash.Hex())
	fmt.Printf("djumpTableFunc hash: %s\n", djumpTableHash.Hex())
	// fmt.Printf("Combined code hash: %s\n", combinedHash.Hex())

	// Test x86Code hash
	expectedX86CodeHash := "0x32544f520aa21576f7fc5224275cecafeb522393e3c0fc2e8c011884f352f0b8"
	expectedDjumpTableHash := "0x76d1a0c42becc37e9b8f0be067e3f19b2afd008934f201e8e4293eccf0f69209"
	// expectedCombinedHash := "0xae58870518c960d2a0bbe05e25c58798c977ddfc24363424c35c1eab3b3a1d44"
	if x86CodeHash.Hex() != expectedX86CodeHash {
		t.Errorf("x86Code hash mismatch: expected %s, got %s", expectedX86CodeHash, x86CodeHash.Hex())
	} else {
		fmt.Printf("✅ x86Code hash matches expected value\n")
	}

	// Test djumpTableFunc hash
	if djumpTableHash.Hex() != expectedDjumpTableHash {
		t.Errorf("djumpTableFunc hash mismatch: expected %s, got %s", expectedDjumpTableHash, djumpTableHash.Hex())
	} else {
		fmt.Printf("✅ djumpTableFunc hash matches expected value\n")
	}
}
