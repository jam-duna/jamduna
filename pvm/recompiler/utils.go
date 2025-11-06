package recompiler

import (
	"fmt"
	"slices"

	"github.com/colorfulnotion/jam/types"
)

func opcode_str(opcode byte) string {
	opcodeMap := map[byte]string{
		0:   "TRAP",
		1:   "FALLTHROUGH",
		3:   "UNLIKELY",
		10:  "ECALLI",
		20:  "LOAD_IMM_64",
		30:  "STORE_IMM_U8",
		31:  "STORE_IMM_U16",
		32:  "STORE_IMM_U32",
		33:  "STORE_IMM_U64",
		40:  "JUMP",
		50:  "JUMP_IND",
		51:  "LOAD_IMM",
		52:  "LOAD_U8",
		53:  "LOAD_I8",
		54:  "LOAD_U16",
		55:  "LOAD_I16",
		56:  "LOAD_U32",
		57:  "LOAD_I32",
		58:  "LOAD_U64",
		59:  "STORE_U8",
		60:  "STORE_U16",
		61:  "STORE_U32",
		62:  "STORE_U64",
		70:  "STORE_IMM_IND_U8",
		71:  "STORE_IMM_IND_U16",
		72:  "STORE_IMM_IND_U32",
		73:  "STORE_IMM_IND_U64",
		80:  "LOAD_IMM_JUMP",
		81:  "BRANCH_EQ_IMM",
		82:  "BRANCH_NE_IMM",
		83:  "BRANCH_LT_U_IMM",
		84:  "BRANCH_LE_U_IMM",
		85:  "BRANCH_GE_U_IMM",
		86:  "BRANCH_GT_U_IMM",
		87:  "BRANCH_LT_S_IMM",
		88:  "BRANCH_LE_S_IMM",
		89:  "BRANCH_GE_S_IMM",
		90:  "BRANCH_GT_S_IMM",
		100: "MOVE_REG",
		101: "SBRK",
		102: "COUNT_SET_BITS_64",
		103: "COUNT_SET_BITS_32",
		104: "LEADING_ZERO_BITS_64",
		105: "LEADING_ZERO_BITS_32",
		106: "TRAILING_ZERO_BITS_64",
		107: "TRAILING_ZERO_BITS_32",
		108: "SIGN_EXTEND_8",
		109: "SIGN_EXTEND_16",
		110: "ZERO_EXTEND_16",
		111: "REVERSE_BYTES",
		120: "STORE_IND_U8",
		121: "STORE_IND_U16",
		122: "STORE_IND_U32",
		123: "STORE_IND_U64",
		124: "LOAD_IND_U8",
		125: "LOAD_IND_I8",
		126: "LOAD_IND_U16",
		127: "LOAD_IND_I16",
		128: "LOAD_IND_U32",
		129: "LOAD_IND_I32",
		130: "LOAD_IND_U64",
		131: "ADD_IMM_32",
		132: "AND_IMM",
		133: "XOR_IMM",
		134: "OR_IMM",
		135: "MUL_IMM_32",
		136: "SET_LT_U_IMM",
		137: "SET_LT_S_IMM",
		138: "SHLO_L_IMM_32",
		139: "SHLO_R_IMM_32",
		140: "SHAR_R_IMM_32",
		141: "NEG_ADD_IMM_32",
		142: "SET_GT_U_IMM",
		143: "SET_GT_S_IMM",
		144: "SHLO_L_IMM_ALT_32",
		145: "SHLO_R_IMM_ALT_32",
		146: "SHAR_R_IMM_ALT_32",
		147: "CMOV_IZ_IMM",
		148: "CMOV_NZ_IMM",
		149: "ADD_IMM_64",
		150: "MUL_IMM_64",
		151: "SHLO_L_IMM_64",
		152: "SHLO_R_IMM_64",
		153: "SHAR_R_IMM_64",
		154: "NEG_ADD_IMM_64",
		155: "SHLO_L_IMM_ALT_64",
		156: "SHLO_R_IMM_ALT_64",
		157: "SHAR_R_IMM_ALT_64",
		158: "ROT_R_64_IMM",
		159: "ROT_R_64_IMM_ALT",
		160: "ROT_R_32_IMM",
		161: "ROT_R_32_IMM_ALT",
		170: "BRANCH_EQ",
		171: "BRANCH_NE",
		172: "BRANCH_LT_U",
		173: "BRANCH_LT_S",
		174: "BRANCH_GE_U",
		175: "BRANCH_GE_S",
		180: "LOAD_IMM_JUMP_IND",
		190: "ADD_32",
		191: "SUB_32",
		192: "MUL_32",
		193: "DIV_U_32",
		194: "DIV_S_32",
		195: "REM_U_32",
		196: "REM_S_32",
		197: "SHLO_L_32",
		198: "SHLO_R_32",
		199: "SHAR_R_32",
		200: "ADD_64",
		201: "SUB_64",
		202: "MUL_64",
		203: "DIV_U_64",
		204: "DIV_S_64",
		205: "REM_U_64",
		206: "REM_S_64",
		207: "SHLO_L_64",
		208: "SHLO_R_64",
		209: "SHAR_R_64",
		210: "AND",
		211: "XOR",
		212: "OR",
		213: "MUL_UPPER_S_S",
		214: "MUL_UPPER_U_U",
		215: "MUL_UPPER_S_U",
		216: "SET_LT_U",
		217: "SET_LT_S",
		218: "CMOV_IZ",
		219: "CMOV_NZ",
		220: "ROT_L_64",
		221: "ROT_L_32",
		222: "ROT_R_64",
		223: "ROT_R_32",
		224: "AND_INV",
		225: "OR_INV",
		226: "XNOR",
		227: "MAX",
		228: "MAX_U",
		229: "MIN",
		230: "MIN_U",
	}

	if name, exists := opcodeMap[opcode]; exists {
		return name
	}
	return fmt.Sprintf("OPCODE %d", opcode)
}

func opcode_str_lower(opcode byte) string {
	opcodeMap := map[byte]string{
		0:   "trap",
		1:   "fallthrough",
		10:  "ecalli",
		20:  "load_imm_64",
		30:  "store_imm_u8",
		31:  "store_imm_u16",
		32:  "store_imm_u32",
		33:  "store_imm_u64",
		40:  "jump",
		50:  "jump_ind",
		51:  "load_imm",
		52:  "load_u8",
		53:  "load_i8",
		54:  "load_u16",
		55:  "load_i16",
		56:  "load_u32",
		57:  "load_i32",
		58:  "load_u64",
		59:  "store_u8",
		60:  "store_u16",
		61:  "store_u32",
		62:  "store_u64",
		70:  "store_imm_ind_u8",
		71:  "store_imm_ind_u16",
		72:  "store_imm_ind_u32",
		73:  "store_imm_ind_u64",
		80:  "load_imm_jump",
		81:  "branch_eq_imm",
		82:  "branch_ne_imm",
		83:  "branch_lt_u_imm",
		84:  "branch_le_u_imm",
		85:  "branch_ge_u_imm",
		86:  "branch_gt_u_imm",
		87:  "branch_lt_s_imm",
		88:  "branch_le_s_imm",
		89:  "branch_ge_s_imm",
		90:  "branch_gt_s_imm",
		100: "move_reg",
		101: "sbrk",
		102: "count_set_bits_64",
		103: "count_set_bits_32",
		104: "leading_zero_bits_64",
		105: "leading_zero_bits_32",
		106: "trailing_zero_bits_64",
		107: "trailing_zero_bits_32",
		108: "sign_extend_8",
		109: "sign_extend_16",
		110: "zero_extend_16",
		111: "reverse_bytes",
		120: "store_ind_u8",
		121: "store_ind_u16",
		122: "store_ind_u32",
		123: "store_ind_u64",
		124: "load_ind_u8",
		125: "load_ind_i8",
		126: "load_ind_u16",
		127: "load_ind_i16",
		128: "load_ind_u32",
		129: "load_ind_i32",
		130: "load_ind_u64",
		131: "add_imm_32",
		132: "and_imm",
		133: "xor_imm",
		134: "or_imm",
		135: "mul_imm_32",
		136: "set_lt_u_imm",
		137: "set_lt_s_imm",
		138: "shlo_l_imm_32",
		139: "shlo_r_imm_32",
		140: "shar_r_imm_32",
		141: "neg_add_imm_32",
		142: "set_gt_u_imm",
		143: "set_gt_s_imm",
		144: "shlo_l_imm_alt_32",
		145: "shlo_r_imm_alt_32",
		146: "shar_r_imm_alt_32",
		147: "cmov_iz_imm",
		148: "cmov_nz_imm",
		149: "add_imm_64",
		150: "mul_imm_64",
		151: "shlo_l_imm_64",
		152: "shlo_r_imm_64",
		153: "shar_r_imm_64",
		154: "neg_add_imm_64",
		155: "shlo_l_imm_alt_64",
		156: "shlo_r_imm_alt_64",
		157: "shar_r_imm_alt_64",
		158: "rot_r_64_imm",
		159: "rot_r_64_imm_alt",
		160: "rot_r_32_imm",
		161: "rot_r_32_imm_alt",
		170: "branch_eq",
		171: "branch_ne",
		172: "branch_lt_u",
		173: "branch_lt_s",
		174: "branch_ge_u",
		175: "branch_ge_s",
		180: "load_imm_jump_ind",
		190: "add_32",
		191: "sub_32",
		192: "mul_32",
		193: "div_u_32",
		194: "div_s_32",
		195: "rem_u_32",
		196: "rem_s_32",
		197: "shlo_l_32",
		198: "shlo_r_32",
		199: "shar_r_32",
		200: "add_64",
		201: "sub_64",
		202: "mul_64",
		203: "div_u_64",
		204: "div_s_64",
		205: "rem_u_64",
		206: "rem_s_64",
		207: "shlo_l_64",
		208: "shlo_r_64",
		209: "shar_r_64",
		210: "and",
		211: "xor",
		212: "or",
		213: "mul_upper_s_s",
		214: "mul_upper_u_u",
		215: "mul_upper_s_u",
		216: "set_lt_u",
		217: "set_lt_s",
		218: "cmov_iz",
		219: "cmov_nz",
		220: "rot_l_64",
		221: "rot_l_32",
		222: "rot_r_64",
		223: "rot_r_32",
		224: "and_inv",
		225: "or_inv",
		226: "xnor",
		227: "max",
		228: "max_u",
		229: "min",
		230: "min_u",
	}

	if name, exists := opcodeMap[opcode]; exists {
		return name
	}
	return fmt.Sprintf("opcode %d", opcode)
}
func P_func(x uint32) uint32 {
	return Z_P * CeilingDivide(x, Z_P)
}

func Z_func(x uint32) uint32 {
	return Z_Z * CeilingDivide(x, Z_Z)
}

func CeilingDivide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

// A.5.4. Instructions with Arguments of Two Immediates.
func extractTwoImm(oargs []byte) (vx uint64, vy uint64) {
	args := slices.Clone(oargs)
	lx := min(4, int(args[0])%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value

	return
}

// A.5.5. Instructions with Arguments of One Offset. (JUMP)
func extractOneOffset(oargs []byte) (vx int64) {
	args := slices.Clone(oargs)
	lx := min(4, len(args))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = z_encode(types.DecodeE_l(args[0:lx]), uint32(lx))
	return vx
}

// A.5.6. Instructions with Arguments of One Register & One Immediate. (JUMP_IND)
func extractOneRegOneImm(oargs []byte) (reg1 int, vx uint64) {
	args := slices.Clone(oargs)
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, max(0, len(args))-1)
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return int(registerIndexA), vx
}

// A.5.7. Instructions with Arguments of One Register and Two Immediates.
func extractOneReg2Imm(oargs []byte) (reg1 int, vx uint64, vy uint64) {
	args := slices.Clone(oargs)

	reg1 = min(12, int(args[0])%16)
	lx := min(4, (int(args[0])/16)%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return int(reg1), vx, vy
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset. (LOAD_IMM_JUMP, BRANCH_{EQ/NE/...}_IMM)
func extractOneRegOneImmOneOffset(oargs []byte) (registerIndexA int, vx uint64, vy int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	lx := min(4, (int(args[0]) / 16 % 8))
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return registerIndexA, vx, vy
}

// A.5.9. Instructions with Arguments of Two Registers.
func extractTwoRegisters(args []byte) (regD, regA int) {
	regD = min(12, int(args[0]&0x0F))
	regA = min(12, int(args[0]>>4))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func extractTwoRegsOneImm(args []byte) (reg1, reg2 int, imm uint64) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func extractTwoRegsOneImm64(args []byte) (reg1, reg2 int, imm int64, uint64imm uint64) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = int64(x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	uint64imm = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.11. Instructions with Arguments of Two Registers and One Offset. (BRANCH_{EQ/NE/...})
func extractTwoRegsOneOffset(oargs []byte) (registerIndexA, registerIndexB int, vx int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = int64(z_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	return registerIndexA, registerIndexB, vx
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
// This instruction is used to load an immediate value into a register and then jump to an address.
func extractTwoRegsAndTwoImmediates(oargs []byte) (registerIndexA, registerIndexB int, vx, vy uint64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, (int(args[1]) % 8))
	ly := min(4, max(0, len(args)-lx-2))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[2:2+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[2+lx:2+lx+ly]), uint32(ly))
	return registerIndexA, registerIndexB, vx, vy
}

// A.5.13. Instructions with Arguments of Three Registers.
func extractThreeRegs(args []byte) (reg1, reg2, dst int) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	dst = min(12, int(args[1]))
	return
}
func x_encode(x uint64, n uint32) uint64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 8*n - 1
	q := x >> shift
	if n == 8 {
		return x
	}
	mask := (uint64(1) << (8 * n)) - 1
	factor := ^mask
	return x + q*factor
}
func z_encode(a uint64, n uint32) int64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 64 - 8*n
	return int64(a<<shift) >> shift
}

// skip function calculates the distance to the next instruction
func (vm *X86Compiler) skip(pc uint64) uint64 {
	n := uint64(len(vm.bitmask))
	end := pc + 25
	if end > n {
		end = n
	}
	for i := pc + 1; i < end; i++ {
		if vm.bitmask[i] > 0 {
			return i - pc - 1
		}
	}
	if end < pc+25 {
		return end - pc - 1
	}
	return 24
}

func Skip(bitmask []byte, pc uint64) uint64 {
	n := uint64(len(bitmask))
	end := pc + 25
	if end > n {
		end = n
	}
	for i := pc + 1; i < end; i++ {
		if bitmask[i] == 0x01 {
			return i - pc - 1
		}
	}
	if end < pc+25 {
		return end - pc - 1
	}
	return 24
}
