package pvm

import "fmt"

// Appendix A - Instuctions

// A.5.1. Instructions without Arguments.
const (
	TRAP        = 0
	FALLTHROUGH = 1
)

// A.5.2. Instructions with Arguments of One Immediate.
const (
	ECALLI = 10 // 0x0a
)

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
const (
	LOAD_IMM_64 = 20 // 0x14
)

// A.5.4. Instructions with Arguments of Two Immediates.
const (
	STORE_IMM_U8  = 30
	STORE_IMM_U16 = 31
	STORE_IMM_U32 = 32
	STORE_IMM_U64 = 33 // NEW, 32-bit twin = store_imm_u32
)

// A.5.5. Instructions with Arguments of One Offset.
const (
	JUMP = 40 // 0x28
)

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
const (
	JUMP_IND  = 50
	LOAD_IMM  = 51
	LOAD_U8   = 52
	LOAD_I8   = 53
	LOAD_U16  = 54
	LOAD_I16  = 55
	LOAD_U32  = 56
	LOAD_I32  = 57
	LOAD_U64  = 58
	STORE_U8  = 59
	STORE_U16 = 60
	STORE_U32 = 61
	STORE_U64 = 62
)

// A.5.7. Instructions with Arguments of One Register & Two Immediates.
const (
	STORE_IMM_IND_U8  = 70 // 0x46
	STORE_IMM_IND_U16 = 71 // 0x47
	STORE_IMM_IND_U32 = 72 // 0x48
	STORE_IMM_IND_U64 = 73 // 0x49 NEW, 32-bit twin = store_imm_ind_u32
)

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
const (
	LOAD_IMM_JUMP   = 80
	BRANCH_EQ_IMM   = 81 // 0x51
	BRANCH_NE_IMM   = 82
	BRANCH_LT_U_IMM = 83
	BRANCH_LE_U_IMM = 84
	BRANCH_GE_U_IMM = 85
	BRANCH_GT_U_IMM = 86
	BRANCH_LT_S_IMM = 87
	BRANCH_LE_S_IMM = 88
	BRANCH_GE_S_IMM = 89
	BRANCH_GT_S_IMM = 90
)

// A.5.9. Instructions with Arguments of Two Registers.
const (
	MOVE_REG              = 100
	SBRK                  = 101
	COUNT_SET_BITS_64     = 102
	COUNT_SET_BITS_32     = 103
	LEADING_ZERO_BITS_64  = 104
	LEADING_ZERO_BITS_32  = 105
	TRAILING_ZERO_BITS_64 = 106
	TRAILING_ZERO_BITS_32 = 107
	SIGN_EXTEND_8         = 108
	SIGN_EXTEND_16        = 109
	ZERO_EXTEND_16        = 110
	REVERSE_BYTES         = 111
)

// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
const (
	STORE_IND_U8      = 120
	STORE_IND_U16     = 121
	STORE_IND_U32     = 122
	STORE_IND_U64     = 123
	LOAD_IND_U8       = 124
	LOAD_IND_I8       = 125
	LOAD_IND_U16      = 126
	LOAD_IND_I16      = 127
	LOAD_IND_U32      = 128
	LOAD_IND_I32      = 129
	LOAD_IND_U64      = 130
	ADD_IMM_32        = 131
	AND_IMM           = 132
	XOR_IMM           = 133
	OR_IMM            = 134
	MUL_IMM_32        = 135
	SET_LT_U_IMM      = 136
	SET_LT_S_IMM      = 137
	SHLO_L_IMM_32     = 138
	SHLO_R_IMM_32     = 139
	SHAR_R_IMM_32     = 140
	NEG_ADD_IMM_32    = 141
	SET_GT_U_IMM      = 142
	SET_GT_S_IMM      = 143
	SHLO_L_IMM_ALT_32 = 144
	SHLO_R_IMM_ALT_32 = 145
	SHAR_R_IMM_ALT_32 = 146
	CMOV_IZ_IMM       = 147
	CMOV_NZ_IMM       = 148
	ADD_IMM_64        = 149
	MUL_IMM_64        = 150
	SHLO_L_IMM_64     = 151
	SHLO_R_IMM_64     = 152
	SHAR_R_IMM_64     = 153
	NEG_ADD_IMM_64    = 154
	SHLO_L_IMM_ALT_64 = 155
	SHLO_R_IMM_ALT_64 = 156
	SHAR_R_IMM_ALT_64 = 157
	ROT_R_64_IMM      = 158
	ROT_R_64_IMM_ALT  = 159
	ROT_R_32_IMM      = 160
	ROT_R_32_IMM_ALT  = 161
)

// A.5.11. Instructions with Arguments of Two Registers & One Offset.
const (
	BRANCH_EQ   = 170
	BRANCH_NE   = 171
	BRANCH_LT_U = 172
	BRANCH_LT_S = 173
	BRANCH_GE_U = 174
	BRANCH_GE_S = 175
)

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
const (
	LOAD_IMM_JUMP_IND = 180
)

// A.5.13. Instructions with Arguments of Three Registers.
const (
	ADD_32        = 190
	SUB_32        = 191
	MUL_32        = 192
	DIV_U_32      = 193
	DIV_S_32      = 194
	REM_U_32      = 195
	REM_S_32      = 196
	SHLO_L_32     = 197
	SHLO_R_32     = 198
	SHAR_R_32     = 199
	ADD_64        = 200
	SUB_64        = 201
	MUL_64        = 202
	DIV_U_64      = 203
	DIV_S_64      = 204
	REM_U_64      = 205
	REM_S_64      = 206
	SHLO_L_64     = 207
	SHLO_R_64     = 208
	SHAR_R_64     = 209
	AND           = 210
	XOR           = 211
	OR            = 212
	MUL_UPPER_S_S = 213
	MUL_UPPER_U_U = 214
	MUL_UPPER_S_U = 215
	SET_LT_U      = 216
	SET_LT_S      = 217
	CMOV_IZ       = 218
	CMOV_NZ       = 219
	ROT_L_64      = 220
	ROT_L_32      = 221
	ROT_R_64      = 222
	ROT_R_32      = 223
	AND_INV       = 224
	OR_INV        = 225
	XNOR          = 226
	MAX           = 227
	MAX_U         = 228
	MIN           = 229
	MIN_U         = 230
)

var PvmTrace = true

func branchCondSymbol(name string) string {
	switch name {
	case "BRANCH_EQ", "BRANCH_EQ_IMM":
		return "=="
	case "BRANCH_NE", "BRANCH_NE_IMM":
		return "!="
	case "BRANCH_LT_U", "BRANCH_LT_U_IMM":
		return "<u"
	case "BRANCH_LE_U_IMM":
		return "<=u"
	case "BRANCH_GE_U", "BRANCH_GE_U_IMM":
		return ">=u"
	case "BRANCH_GT_U_IMM":
		return ">u"
	case "BRANCH_LT_S", "BRANCH_LT_S_IMM":
		return "<s"
	case "BRANCH_LE_S_IMM":
		return "<=s"
	case "BRANCH_GE_S", "BRANCH_GE_S_IMM":
		return ">=s"
	case "BRANCH_GT_S_IMM":
		return ">s"
	default:
		return "??"
	}
}

func dumpStoreGeneric(_ string, addr uint64, regOrSrc string, value uint64, bits int) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\tu%d [0x%x] = %s = 0x%x\n", bits, addr, regOrSrc, value)
}

func dumpLoadGeneric(_ string, regA int, addrOrVx uint64, value uint64, bits int, signed bool) {
	if !PvmTrace {
		return
	}
	prefix := "u"
	if signed {
		prefix = "i"
	}
	fmt.Printf("\t%s = %s%d[0x%x] = 0x%x\n", reg(regA), prefix, bits, addrOrVx, value)
	fmt.Printf("\t%s = 0x%x\n", reg(regA), value)
}

func dumpMov(regD, regA int, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = %s \n", reg(regD), reg(regA))
	fmt.Printf("\t%s = 0x%x\n", reg(regD), result)
}

func dumpThreeRegOp(opname string, regD, regA, regB int, _, _, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = %s %s %s\n", reg(regD), reg(regA), opname, reg(regB))
	fmt.Printf("\t%s = 0x%x\n", reg(regD), result)
}

func dumpBinOp(name string, regA, regB int, vx, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = %s %s 0x%x\n", reg(regA), reg(regB), name, vx)
	fmt.Printf("\t%s = 0x%x\n", reg(regA), result)
}

func dumpCmpOp(name string, regA, regB int, vx, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = %s %s 0x%x\n", reg(regA), name, reg(regB), vx)
	fmt.Printf("\t%s = 0x%x\n", reg(regA), result)
}

func dumpShiftOp(name string, regA, regB int, shift, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = %s %s %x\n", reg(regA), reg(regB), name, shift)
	fmt.Printf("\t%s = 0x%x\n", reg(regA), result)
}

func dumpRotOp(_ string, regDst, src string, shift, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\t%s = rotR %s by %x = %x\n", regDst, src, shift, result)
}

func dumpCmovOp(name string, regA, regB int, _, _, result uint64, _ bool) {
	if !PvmTrace {
		return
	}

	fmt.Printf("\t%s = if %s %s\n", reg(regA), reg(regB), name)
	fmt.Printf("\t%s = 0x%x\n", reg(regA), result)
}

func dumpJumpOffset(_ string, offset int64, pc uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("\tjump %d\n", uint64(int64(pc)+int64(offset)))
}

func dumpBranch(name string, regA, regB int, valueA, valueB, vx uint64, taken bool) {
	if !PvmTrace {
		return
	}
	cond := branchCondSymbol(name)
	fmt.Printf("\tjump %d if %s (%x) %s %s (%x)\n", vx, reg(regA), valueA, cond, reg(regB), valueB)
	if taken {
		fmt.Printf("\t*** jumped to %d\n", vx)
	}
}

func dumpBranchImm(name string, regA int, _, vx, vy uint64, _, taken bool) {
	if !PvmTrace {
		return
	}
	cond := branchCondSymbol(name)
	fmt.Printf("\tjump %d if %s %s 0x%x\n", vy, reg(regA), cond, vx)
	if taken {
		fmt.Printf("\t*** jumped to %d\n", vy)
	}
}

func boolToUint(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}
