package recompiler

import (
	"github.com/colorfulnotion/jam/pvm/program"
)

const (
	regSize = 13

	M   = 128
	V   = 1023
	Z_A = 2

	Z_P = (1 << 12)
	Z_Q = (1 << 16)
	Z_I = (1 << 24)
	Z_Z = (1 << 16)
)

var (
	PvmLogging = false
	PvmTrace   = false
	PvmTrace2  = false

	showDisassembly = false
	useEcalli500    = false
	debugRecompiler = false
	UseTally        = false
)

func SetShowDisassembly(show bool) {
	showDisassembly = show
}

const (
	NONE = (1 << 64) - 1 // 2^32 - 1 15
	WHAT = (1 << 64) - 2 // 2^32 - 2 14
	OOB  = (1 << 64) - 3 // 2^32 - 3 13
	WHO  = (1 << 64) - 4 // 2^32 - 4 12
	FULL = (1 << 64) - 5 // 2^32 - 5 11
	CORE = (1 << 64) - 6 // 2^32 - 6 10
	CASH = (1 << 64) - 7 // 2^32 - 7 9
	LOW  = (1 << 64) - 8 // 2^32 - 8 8
	HUH  = (1 << 64) - 9 // 2^32 - 9 7
	OK   = 0             // 0
)

// Import all instruction constants from the unified pvm/program package
const (
	// A.5.1. Instructions without Arguments
	TRAP        = program.TRAP
	FALLTHROUGH = program.FALLTHROUGH
	UNLIKELY    = program.UNLIKELY

	// A.5.2. Instructions with Arguments of One Immediate
	ECALLI = program.ECALLI

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
	LOAD_IMM_64 = program.LOAD_IMM_64

	// A.5.4. Instructions with Arguments of Two Immediates
	STORE_IMM_U8  = program.STORE_IMM_U8
	STORE_IMM_U16 = program.STORE_IMM_U16
	STORE_IMM_U32 = program.STORE_IMM_U32
	STORE_IMM_U64 = program.STORE_IMM_U64

	// A.5.5. Instructions with Arguments of One Offset
	JUMP = program.JUMP

	// A.5.6. Instructions with Arguments of One Register & One Immediate
	JUMP_IND  = program.JUMP_IND
	LOAD_IMM  = program.LOAD_IMM
	LOAD_U8   = program.LOAD_U8
	LOAD_I8   = program.LOAD_I8
	LOAD_U16  = program.LOAD_U16
	LOAD_I16  = program.LOAD_I16
	LOAD_U32  = program.LOAD_U32
	LOAD_I32  = program.LOAD_I32
	LOAD_U64  = program.LOAD_U64
	STORE_U8  = program.STORE_U8
	STORE_U16 = program.STORE_U16
	STORE_U32 = program.STORE_U32
	STORE_U64 = program.STORE_U64

	// A.5.7. Instructions with Arguments of One Register & Two Immediates
	STORE_IMM_IND_U8  = program.STORE_IMM_IND_U8
	STORE_IMM_IND_U16 = program.STORE_IMM_IND_U16
	STORE_IMM_IND_U32 = program.STORE_IMM_IND_U32
	STORE_IMM_IND_U64 = program.STORE_IMM_IND_U64

	// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
	LOAD_IMM_JUMP   = program.LOAD_IMM_JUMP
	BRANCH_EQ_IMM   = program.BRANCH_EQ_IMM
	BRANCH_NE_IMM   = program.BRANCH_NE_IMM
	BRANCH_LT_U_IMM = program.BRANCH_LT_U_IMM
	BRANCH_LE_U_IMM = program.BRANCH_LE_U_IMM
	BRANCH_GE_U_IMM = program.BRANCH_GE_U_IMM
	BRANCH_GT_U_IMM = program.BRANCH_GT_U_IMM
	BRANCH_LT_S_IMM = program.BRANCH_LT_S_IMM
	BRANCH_LE_S_IMM = program.BRANCH_LE_S_IMM
	BRANCH_GE_S_IMM = program.BRANCH_GE_S_IMM
	BRANCH_GT_S_IMM = program.BRANCH_GT_S_IMM

	// A.5.9. Instructions with Arguments of Two Registers
	MOVE_REG              = program.MOVE_REG
	SBRK                  = program.SBRK
	COUNT_SET_BITS_64     = program.COUNT_SET_BITS_64
	COUNT_SET_BITS_32     = program.COUNT_SET_BITS_32
	LEADING_ZERO_BITS_64  = program.LEADING_ZERO_BITS_64
	LEADING_ZERO_BITS_32  = program.LEADING_ZERO_BITS_32
	TRAILING_ZERO_BITS_64 = program.TRAILING_ZERO_BITS_64
	TRAILING_ZERO_BITS_32 = program.TRAILING_ZERO_BITS_32
	SIGN_EXTEND_8         = program.SIGN_EXTEND_8
	SIGN_EXTEND_16        = program.SIGN_EXTEND_16
	ZERO_EXTEND_16        = program.ZERO_EXTEND_16
	REVERSE_BYTES         = program.REVERSE_BYTES

	// A.5.10. Instructions with Arguments of Two Registers & One Immediate
	STORE_IND_U8      = program.STORE_IND_U8
	STORE_IND_U16     = program.STORE_IND_U16
	STORE_IND_U32     = program.STORE_IND_U32
	STORE_IND_U64     = program.STORE_IND_U64
	LOAD_IND_U8       = program.LOAD_IND_U8
	LOAD_IND_I8       = program.LOAD_IND_I8
	LOAD_IND_U16      = program.LOAD_IND_U16
	LOAD_IND_I16      = program.LOAD_IND_I16
	LOAD_IND_U32      = program.LOAD_IND_U32
	LOAD_IND_I32      = program.LOAD_IND_I32
	LOAD_IND_U64      = program.LOAD_IND_U64
	ADD_IMM_32        = program.ADD_IMM_32
	AND_IMM           = program.AND_IMM
	XOR_IMM           = program.XOR_IMM
	OR_IMM            = program.OR_IMM
	MUL_IMM_32        = program.MUL_IMM_32
	SET_LT_U_IMM      = program.SET_LT_U_IMM
	SET_LT_S_IMM      = program.SET_LT_S_IMM
	SHLO_L_IMM_32     = program.SHLO_L_IMM_32
	SHLO_R_IMM_32     = program.SHLO_R_IMM_32
	SHAR_R_IMM_32     = program.SHAR_R_IMM_32
	NEG_ADD_IMM_32    = program.NEG_ADD_IMM_32
	SET_GT_U_IMM      = program.SET_GT_U_IMM
	SET_GT_S_IMM      = program.SET_GT_S_IMM
	SHLO_L_IMM_ALT_32 = program.SHLO_L_IMM_ALT_32
	SHLO_R_IMM_ALT_32 = program.SHLO_R_IMM_ALT_32
	SHAR_R_IMM_ALT_32 = program.SHAR_R_IMM_ALT_32
	CMOV_IZ_IMM       = program.CMOV_IZ_IMM
	CMOV_NZ_IMM       = program.CMOV_NZ_IMM
	ADD_IMM_64        = program.ADD_IMM_64
	MUL_IMM_64        = program.MUL_IMM_64
	SHLO_L_IMM_64     = program.SHLO_L_IMM_64
	SHLO_R_IMM_64     = program.SHLO_R_IMM_64
	SHAR_R_IMM_64     = program.SHAR_R_IMM_64
	NEG_ADD_IMM_64    = program.NEG_ADD_IMM_64
	SHLO_L_IMM_ALT_64 = program.SHLO_L_IMM_ALT_64
	SHLO_R_IMM_ALT_64 = program.SHLO_R_IMM_ALT_64
	SHAR_R_IMM_ALT_64 = program.SHAR_R_IMM_ALT_64
	ROT_R_64_IMM      = program.ROT_R_64_IMM
	ROT_R_64_IMM_ALT  = program.ROT_R_64_IMM_ALT
	ROT_R_32_IMM      = program.ROT_R_32_IMM
	ROT_R_32_IMM_ALT  = program.ROT_R_32_IMM_ALT

	// A.5.11. Instructions with Arguments of Two Registers & One Offset
	BRANCH_EQ   = program.BRANCH_EQ
	BRANCH_NE   = program.BRANCH_NE
	BRANCH_LT_U = program.BRANCH_LT_U
	BRANCH_LT_S = program.BRANCH_LT_S
	BRANCH_GE_U = program.BRANCH_GE_U
	BRANCH_GE_S = program.BRANCH_GE_S

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
	LOAD_IMM_JUMP_IND = program.LOAD_IMM_JUMP_IND

	// A.5.13. Instructions with Arguments of Three Registers
	ADD_32        = program.ADD_32
	SUB_32        = program.SUB_32
	MUL_32        = program.MUL_32
	DIV_U_32      = program.DIV_U_32
	DIV_S_32      = program.DIV_S_32
	REM_U_32      = program.REM_U_32
	REM_S_32      = program.REM_S_32
	SHLO_L_32     = program.SHLO_L_32
	SHLO_R_32     = program.SHLO_R_32
	SHAR_R_32     = program.SHAR_R_32
	ADD_64        = program.ADD_64
	SUB_64        = program.SUB_64
	MUL_64        = program.MUL_64
	DIV_U_64      = program.DIV_U_64
	DIV_S_64      = program.DIV_S_64
	REM_U_64      = program.REM_U_64
	REM_S_64      = program.REM_S_64
	SHLO_L_64     = program.SHLO_L_64
	SHLO_R_64     = program.SHLO_R_64
	SHAR_R_64     = program.SHAR_R_64
	AND           = program.AND
	XOR           = program.XOR
	OR            = program.OR
	MUL_UPPER_S_S = program.MUL_UPPER_S_S
	MUL_UPPER_U_U = program.MUL_UPPER_U_U
	MUL_UPPER_S_U = program.MUL_UPPER_S_U
	SET_LT_U      = program.SET_LT_U
	SET_LT_S      = program.SET_LT_S
	CMOV_IZ       = program.CMOV_IZ
	CMOV_NZ       = program.CMOV_NZ
	ROT_L_64      = program.ROT_L_64
	ROT_L_32      = program.ROT_L_32
	ROT_R_64      = program.ROT_R_64
	ROT_R_32      = program.ROT_R_32
	AND_INV       = program.AND_INV
	OR_INV        = program.OR_INV
	XNOR          = program.XNOR
	MAX           = program.MAX
	MAX_U         = program.MAX_U
	MIN           = program.MIN
	MIN_U         = program.MIN_U
)

// Appendix B - Host function
// Host function indexes
const (
	GAS               = 0
	FETCH             = 1 // NEW
	LOOKUP            = 2
	READ              = 3
	WRITE             = 4
	INFO              = 5 // ADJ
	HISTORICAL_LOOKUP = 6
	EXPORT            = 7
	MACHINE           = 8
	PEEK              = 9
	POKE              = 10
	PAGES             = 11 // NEW
	INVOKE            = 12
	EXPUNGE           = 13
	BLESS             = 14
	ASSIGN            = 15
	DESIGNATE         = 16
	CHECKPOINT        = 17
	NEW               = 18
	UPGRADE           = 19
	TRANSFER          = 20
	EJECT             = 21
	QUERY             = 22
	SOLICIT           = 23
	FORGET            = 24
	YIELD             = 25
	PROVIDE           = 26

	MANIFEST = 64 // Not in 0.6.7

	LOG = 100
)
