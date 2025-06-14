package pvm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/colorfulnotion/jam/types"
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

// TestRecompilerSuccess runs all test cases expected to succeed (no division by zero or overflow conditions).
var errorNames = []string{
	"by_zero", "overflow", "trap", "nok", "inaccessible", "read_only",
}

func TestRecompilerSuccess(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	RecompilerFlag = true

	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm/programs"

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
	for i := 0; i < 3; i++ {
		mismatchs[i] = make([]string, 0)
	}
	// 183 tests pass, these 22 tests fail
	suppress := []string{
		// SOURABH TODO
		"inst_branch_eq_imm_nok.json",
		"inst_jump_indirect_with_offset_ok.json",
		"inst_jump_indirect_without_offset_ok.json",
		"inst_load_imm_and_jump.json",
		"inst_load_imm_and_jump_indirect_same_regs_with_offset_ok.json",
		"inst_load_imm_and_jump_indirect_same_regs_without_offset_ok.json",
		//"inst_store_imm_indirect_u16_with_offset_nok.json",
		//"inst_store_imm_indirect_u32_with_offset_nok.json",
		//"inst_store_imm_indirect_u64_with_offset_nok.json",
		//"inst_store_imm_indirect_u8_with_offset_nok.json",
		//"inst_store_indirect_u16_with_offset_nok.json",
		//"inst_store_indirect_u32_with_offset_nok.json",
		//"inst_store_indirect_u64_with_offset_nok.json",
		//"inst_store_indirect_u8_with_offset_nok.json",
		//"inst_store_u8_trap_inaccessible.json",
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
			err = recompiler_test(tc)
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
				} else {
					t.Errorf("Unexpected error: %v", err)
				}

			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
			count++
		})
	}
	fmt.Printf("Recompiler Test Summary:===========================================\n")
	fmt.Printf("Total tests run: %d\n", count)
	fmt.Printf("Memory mismatch: %d (%.2f%%)\n", len(mismatchs[memory_mismatch]), float64(len(mismatchs[memory_mismatch]))/float64(count)*100)
	fmt.Printf("Register mismatch: %d (%.2f%%)\n", len(mismatchs[register_mismatch]), float64(len(mismatchs[register_mismatch]))/float64(count)*100)
	fmt.Printf("Result code mismatch: %d (%.2f%%)\n", len(mismatchs[result_code_mismatch]), float64(len(mismatchs[result_code_mismatch]))/float64(count)*100)
	success := count - len(mismatchs[memory_mismatch]) - len(mismatchs[register_mismatch]) - len(mismatchs[result_code_mismatch])
	fmt.Printf("Success: %d (%.2f%%)\n", success, float64(success)/float64(count)*100)
	for i, mismatch := range mismatchs {
		switch i {
		case memory_mismatch:
			fmt.Printf("Memory Mismatches: %d\n", len(mismatch))

		case register_mismatch:
			fmt.Printf("Register Mismatches: %d\n", len(mismatch))

		case result_code_mismatch:
			fmt.Printf("Result Code Mismatches: %d\n", len(mismatch))
		}
		for _, name := range mismatch {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Printf("===================================================================\n")
}

func recompiler_test(tc TestCase) error {
	var num_mismatch int

	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{})
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		pvm.Ram.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	rvm, err := NewRecompilerVM(pvm)
	if err != nil {
		return fmt.Errorf("failed to create recompiler VM: %w", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAssess(pm.Address, pm.Length, PageMutable)
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

	_, _, x86Code := rvm.Compile(rvm.pc)
	str := Disassemble(x86Code)
	fmt.Printf("ALL COMBINED Disassembled x86 code:\n%s\n", str)
	if err := rvm.ExecuteX86Code(x86Code); err != nil {
		return fmt.Errorf("error executing x86 code: %w", err)
	}

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
	for i, reg := range rvm.register {
		if reg != tc.ExpectedRegs[i] {
			num_mismatch++
			fmt.Printf("MISMATCH expected %v got %v\n", tc.ExpectedRegs, rvm.register)
			return fmt.Errorf("register mismatch for test %s at index %d: expected %d, got %d", tc.Name, i, tc.ExpectedRegs[i], reg)
		}
	}
	resultCode = rvm.ResultCode
	rvm.Close()

	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}

	resultCodeStr := types.ResultCodeToString[resultCode]
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
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.register)
}

func TestSinglePVM(t *testing.T) {

	PvmLogging = true
	PvmTrace = true
	RecompilerFlag = true

	name := "inst_branch_eq_ok" // inst_branch_eq_nok
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
		err = recompiler_test(testCase)
		if err != nil {
			t.Errorf("❌ [%s] Test failed: %v", name, err)
		} else {
			t.Logf("✅ [%s] Test passed", name)
		}
	})
}

func TestHostFuncExpose(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	RecompilerFlag = true
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := "../jamtestvectors/pvm/programs/" + name + ".json"
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
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{})
	rvm, err := NewRecompilerVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create RecompilerVM: %v", err)
	}

	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAssess(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemory(mem.Address, mem.Data)
	}

	rvm.initStartCode()
	rvm.x86Code = rvm.startCode
	ecallicode, err := rvm.EcalliCode(INFO)
	if err != nil {
		t.Fatalf("Failed to generate Ecalli code: %v", err)
	}
	rvm.x86Code = append(rvm.x86Code, ecallicode...)
	rvm.x86Code = append(rvm.x86Code, 0xC3) // ret
	fmt.Printf("Disassembled x86 code:\n%s\n", Disassemble(rvm.x86Code))
	err = rvm.ExecuteX86Code(rvm.x86Code)
	if err != nil {
		t.Fatalf("Failed to execute x86 code: %v", err)
	}
}
