package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"testing"
	"unsafe"
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

func TestRecompiler(t *testing.T) {
	testcases := map[string]string{
		// A.5.1. Instructions without Arguments
		"TRAP":        "inst_trap",
		"FALLTHROUGH": "inst_fallthrough",

		// A.5.2. Instructions with Arguments of One Immediate.
		// "ECALLI":                "",

		// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
		"LOAD_IMM_64": "inst_load_imm_64,inst_load_imm",

		// A.5.4. Instructions with Arguments of Two Immediates.
		// "STORE_IMM_U8": "inst_store_imm_u8,inst_store_imm_u8_trap_read_only",
		// "STORE_IMM_U16": "inst_store_imm_u16",
		// "STORE_IMM_U32": "inst_store_imm_u32",
		// "STORE_IMM_U64": "inst_store_imm_u64",

		// A.5.5. Instructions with Arguments of One Offset.
		// "JUMP": "inst_jump",

		// A.5.6. Instructions with Arguments of One Register & Two Immediates.
		// "JUMP_IND": "inst_ret_halt",
		// "LOAD_IMM":              "",
		//"LOAD_U8": "inst_load_u8,inst_load_u8_nok",
		//"LOAD_I8":  "inst_load_i8",
		//"LOAD_U16": "inst_load_u16",
		//"LOAD_I16":  "inst_load_i16",
		//"LOAD_U32": "inst_load_u32",
		//"LOAD_I32":  "inst_load_i32",
		//"LOAD_U64":  "inst_load_u64",
		//"STORE_U8": "inst_store_u8,inst_store_u8_trap_inaccessible,inst_store_u8_trap_read_only",
		//"STORE_U16": "inst_store_u16",
		//"STORE_U32": "inst_store_u32",
		//"STORE_U64": "inst_store_u64",

		// A.5.7. Instructions with Arguments of One Register and Two Immediates.
		"STORE_IMM_IND_U8":  "inst_store_imm_indirect_u8_with_offset_ok,inst_store_imm_indirect_u8_with_offset_nok,inst_store_imm_indirect_u8_without_offset_ok",
		"STORE_IMM_IND_U16": "inst_store_imm_indirect_u16_with_offset_ok,inst_store_imm_indirect_u16_with_offset_nok,inst_store_imm_indirect_u16_without_offset_ok",
		//"STORE_IMM_IND_U32": "inst_store_imm_indirect_u32_with_offset_ok,inst_store_imm_indirect_u32_with_offset_nok,inst_store_imm_indirect_u32_without_offset_ok",
		//"STORE_IMM_IND_U64": "inst_store_imm_indirect_u64_with_offset_ok,inst_store_imm_indirect_u64_with_offset_nok,inst_store_imm_indirect_u64_without_offset_ok",

		// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
		//"LOAD_IMM_JUMP": "inst_load_imm_and_jump",
		"BRANCH_EQ_IMM": "inst_branch_eq_imm_ok,inst_branch_eq_imm_nok",
		// "BRANCH_NE_IMM": "inst_branch_not_eq_imm_ok,inst_branch_not_eq_imm_nok",
		// "BRANCH_LT_U_IMM": "inst_branch_less_unsigned_imm_ok,inst_branch_less_unsigned_imm_nok",
		// "BRANCH_LE_U_IMM": "inst_branch_less_or_equal_unsigned_imm_ok,inst_branch_less_or_equal_unsigned_imm_nok",
		// "BRANCH_GE_U_IMM": "inst_branch_greater_or_equal_unsigned_imm_ok,inst_branch_greater_or_equal_unsigned_imm_nok",
		// "BRANCH_GT_U_IMM": "inst_branch_greater_unsigned_imm_ok,inst_branch_greater_unsigned_imm_nok",
		// "BRANCH_LT_S_IMM": "inst_branch_less_signed_imm_ok,inst_branch_less_signed_imm_nok",
		// "BRANCH_LE_S_IMM": "inst_branch_less_or_equal_signed_imm_ok,inst_branch_less_or_equal_signed_imm_nok",
		// "BRANCH_GE_S_IMM": "inst_branch_greater_or_equal_signed_imm_ok,inst_branch_greater_or_equal_signed_imm_nok",
		// "BRANCH_GT_S_IMM": "inst_branch_greater_signed_imm_ok,inst_branch_greater_signed_imm_nok",

		// A.5.9. Instructions with Arguments of Two Registers.
		// "SBRK":                  "",
		// "MOVE_REG": 			    "",
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
		// "STORE_IND_U8": "inst_store_indirect_u8_with_offset_ok,inst_store_indirect_u8_without_offset_ok,inst_store_indirect_u8_with_offset_nok",
		// "STORE_IND_U16": "inst_store_indirect_u16_with_offset_ok,inst_store_indirect_u16_with_offset_nok,inst_store_indirect_u16_without_offset_ok",
		// "STORE_IND_U32": "inst_store_indirect_u32_with_offset_ok,inst_store_indirect_u32_with_offset_nok,inst_store_indirect_u32_without_offset_ok",
		// "STORE_IND_U64": "inst_store_indirect_u64_with_offset_ok,inst_store_indirect_u64_without_offset_ok,inst_store_indirect_u64_with_offset_nok",
		// "LOAD_IND_U8":   "inst_load_indirect_u8_with_offset,inst_load_indirect_u8_without_offset",
		// "LOAD_IND_I8":   "inst_load_indirect_i8_with_offset,inst_load_indirect_i8_without_offset",
		// "LOAD_IND_U16":  "inst_load_indirect_u16_with_offset,inst_load_indirect_u16_without_offset",
		// "LOAD_IND_I16":  "inst_load_indirect_i16_with_offset,inst_load_indirect_i16_without_offset",
		// "LOAD_IND_U32":  "inst_load_indirect_u32_with_offset,inst_load_indirect_u32_without_offset",
		// "LOAD_IND_I32":  "inst_load_indirect_i32_with_offset,inst_load_indirect_i32_without_offset",
		// "LOAD_IND_U64":  "inst_load_indirect_u64_with_offset,inst_load_indirect_u64_without_offset",
		"ADD_IMM_32": "inst_add_imm_32,inst_sub_imm_32",
		"AND_IMM":    "inst_and_imm",
		"XOR_IMM":    "inst_xor_imm",
		"OR_IMM":     "inst_or_imm",
		"MUL_IMM_32": "inst_mul_imm_32",
		//"SET_LT_U_IMM": "inst_set_less_than_unsigned_imm_0,inst_set_less_than_unsigned_imm_1",
		//"SET_LT_S_IMM": "inst_set_less_than_signed_imm_0,inst_set_less_than_signed_imm_1",
		"SHLO_L_IMM_32":  "inst_shift_logical_left_imm_32",
		"SHLO_R_IMM_32":  "inst_shift_logical_right_imm_32",
		"SHAR_R_IMM_32":  "inst_shift_arithmetic_right_imm_32",
		"NEG_ADD_IMM_32": "inst_negate_and_add_imm_32",
		// "SET_GT_U_IMM":      "inst_set_greater_than_unsigned_imm_0,inst_set_greater_than_unsigned_imm_1",
		// "SET_GT_S_IMM":      "inst_set_greater_than_signed_imm_0,inst_set_greater_than_signed_imm_1",
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
		// "BRANCH_EQ": "inst_branch_eq_ok,inst_branch_eq_nok",
		// "BRANCH_NE":   "inst_branch_not_eq_ok,inst_branch_not_eq_nok",
		// "BRANCH_LT_U": "inst_branch_less_unsigned_ok,inst_branch_less_unsigned_nok",
		// "BRANCH_LT_S": "inst_branch_less_signed_ok,inst_branch_less_signed_nok",
		// "BRANCH_GE_U": "inst_branch_greater_or_equal_unsigned_ok,inst_branch_greater_or_equal_unsigned_nok",
		// "BRANCH_GE_S": "inst_branch_greater_or_equal_signed_ok,inst_branch_greater_or_equal_signed_nok",

		// A.5.12. Instruction with Arguments of Two Registers and Two Immediates. (180)
		//"LOAD_IMM_JUMP_IND": "inst_load_imm_and_jump_indirect_same_regs_with_offset_ok,inst_load_imm_and_jump_indirect_different_regs_with_offset_ok,inst_load_imm_and_jump_indirect_same_regs_without_offset_ok,inst_load_imm_and_jump_indirect_different_regs_without_offset_ok",

		// A.5.13. Instructions with Arguments of Three Registers. (190-230)
		"ADD_32": "inst_add_32",
		"SUB_32": "inst_sub_32,inst_sub_32_with_overflow",
		"MUL_32": "inst_mul_32",
		//"DIV_U_32":  "inst_div_unsigned_32,inst_div_unsigned_32_by_zero",
		//"DIV_S_32":  "inst_div_signed_32,inst_div_signed_32_by_zero",
		//"REM_U_32":  "inst_rem_unsigned_32,inst_rem_unsigned_32_by_zero",
		//"REM_S_32":  "inst_rem_signed_32,inst_rem_signed_32_by_zero",
		"SHLO_L_32": "inst_shift_logical_left_32",
		"SHLO_R_32": "inst_shift_logical_right_32",
		"SHAR_R_32": "inst_shift_arithmetic_right_32",
		"ADD_64":    "inst_add_64",
		"SUB_64":    "inst_sub_64,inst_sub_64_with_overflow",
		// "DIV_U_64": "inst_div_unsigned_64,inst_div_unsigned_64_by_zero,inst_div_unsigned_64_with_overflow"
		// "DIV_S_64": "inst_div_signed_64,inst_div_signed_64_by_zero"
		// "REM_U_64": "inst_rem_unsigned_64,inst_rem_unsigned_64_by_zero",
		// "REM_S_64": "inst_rem_signed_64,inst_rem_signed_64_by_zero",
		"SHLO_L_64": "inst_shift_logical_left_64",
		"SHLO_R_64": "inst_shift_logical_right_64",
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

	PvmLogging = true
	PvmTrace = true
	RecompilerFlag = true

	keys := make([]string, 0, len(testcases))
	for k := range testcases {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if testcases[key] == "" {
			continue
		}
		entries := strings.Split(testcases[key], ",")
		for _, name := range entries {
			filePath := "../jamtestvectors/pvm/programs/" + name + ".json"
			// filePath := "../jamtestvectors/pvm/programs/inst_add_64.json"
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
	}
}

func TestSingleRecompile(t *testing.T) {
	name := "inst_sub_32_with_overflow"
	filePath := "../jamtestvectors/pvm/programs/" + name + ".json"
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
