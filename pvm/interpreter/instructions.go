package interpreter

import (
	"fmt"

	"github.com/jam-duna/jamduna/pvm/program"
	"github.com/jam-duna/jamduna/pvm/trace"
)

// Import all instruction constants from the unified pvm/program package
const (
	// A.5.1. Instructions without Arguments
	TRAP        = program.TRAP
	FALLTHROUGH = program.FALLTHROUGH

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

const (
	prefixTrace = "TRACE polkavm::interpreter"
)

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

var PvmInterpretation = false

// Global variables to track memory operation for current instruction
var lastMemAddrRead uint32
var lastMemValueRead uint64
var lastMemAddrWrite uint32
var lastMemValueWrite uint64

func dumpStoreGeneric(_ string, addr uint64, regOrSrc string, value uint64, bits int) {
	// Track memory writes for both trace mode and verify mode
	if PvmTraceMode || PvmVerifyBaseDir != "" || PvmVerifyDir != "" {
		lastMemAddrWrite = uint32(addr)
		lastMemValueWrite = value
	}
	if !PvmTrace {
		return
	}
	valueStr := fmt.Sprintf("0x%x", value)
	if valueStr == regOrSrc {
		fmt.Printf("%s u%d [0x%x] = 0x%x\n", prefixTrace, bits, addr, value)
	} else {
		fmt.Printf("%s u%d [0x%x] = %s = 0x%x\n", prefixTrace, bits, addr, regOrSrc, value)
	}
}

func dumpLoadImmJump(_ string, registerIndexA int, vx uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(registerIndexA), vx)
}

func dumpLoadGeneric(_ string, regA int, addrOrVx uint64, value uint64, bits int, signed bool) {
	// Track memory reads for both trace mode and verify mode
	if PvmTraceMode || PvmVerifyBaseDir != "" || PvmVerifyDir != "" {
		lastMemAddrRead = uint32(addrOrVx)
		lastMemValueRead = value
	}
	if !PvmTrace {
		return
	}
	prefix := "u"
	if signed {
		prefix = "i"
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s%d[0x%x] = 0x%x\n", reg(regA), prefix, bits, addrOrVx, value)
	}
	fmt.Printf("%s %s = %s%d [0x%x] = 0x%x\n", prefixTrace, reg(regA), prefix, bits, addrOrVx, value)
}

func dumpLoadImm(_ string, regA int, addrOrVx uint64, value uint64, bits int, signed bool) {
	// Track memory reads for both trace mode and verify mode
	if PvmTraceMode || PvmVerifyBaseDir != "" || PvmVerifyDir != "" {
		lastMemAddrRead = uint32(addrOrVx)
		lastMemValueRead = value
	}

	if !PvmTrace {
		return
	}
	prefix := "u"
	if signed {
		prefix = "i"
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s%d[0x%x] = 0x%x\n", reg(regA), prefix, bits, addrOrVx, value)
	}

	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regA), value)
}

func dumpMov(regD, regA int, result uint64) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s \n", reg(regD), reg(regA))
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regD), result)
}

func dumpThreeRegOp(opname string, regD, regA, regB int, _, _, result uint64) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s %s %s\n", reg(regD), reg(regA), opname, reg(regB))
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regD), result)
}

func dumpBinOp(name string, regA, regB int, vx, result uint64) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s %s 0x%x\n", reg(regA), reg(regB), name, vx)
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regA), result)
}

func dumpCmpOp(name string, regA, regB int, vx, result uint64) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s %s 0x%x\n", reg(regA), name, reg(regB), vx)
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regA), result)
}

func dumpShiftOp(name string, regA, regB int, shift, result uint64) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = %s %s %x\n", reg(regA), reg(regB), name, shift)
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regA), result)
}

func dumpRotOp(_ string, regDst, src string, shift, result uint64) {
	if !PvmTrace {
		return
	}
	fmt.Printf("%s %s = rotR %s by %x = %x\n", prefixTrace, regDst, src, shift, result)
}

func dumpCmovOp(name string, regA, regB int, _, _, result uint64, _ bool) {
	if !PvmTrace {
		return
	}
	if PvmInterpretation {
		fmt.Printf("\t%s = if %s %s\n", reg(regA), reg(regB), name)
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(regA), result)
}

func dumpJumpOffset(_ string, offset int64, pc uint64, pc_label map[int]string) {
	if !PvmTrace {
		return
	}
	if _, ok := pc_label[int(pc)+int(offset)]; ok {
		//fmt.Printf("*** jump %d (%s)\n", int64(int64(pc)+int64(offset)), label)
	} else {
		//fmt.Printf("*** jump %d\n", int64(int64(pc)+int64(offset)))
	}
}

func dumpBranch(name string, regA, regB int, valueA, valueB, vx uint64, taken bool) {
	if !PvmTrace {
		return
	}
	cond := branchCondSymbol(name)
	if PvmInterpretation {
		fmt.Printf("\tjump %d if %s (%x) %s %s (%x)\n", vx, reg(regA), valueA, cond, reg(regB), valueB)
	}
	if taken {
		fmt.Printf("*** jumped to %x\n", vx)
	}
}

func dumpBranchImm(name string, regA int, _, vx, vy uint64, _, taken bool) {
	if !PvmTrace {
		return
	}
	cond := branchCondSymbol(name)
	if PvmInterpretation {
		fmt.Printf("\tjump %d if %s %s 0x%x\n", vy, reg(regA), cond, vx)
	}
	if taken {
		fmt.Printf("*** jumped to %x\n", vy)
	}
}

func boolToUint(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// dumpTwoRegs prints a two-register operation trace.
// op is the opcode name (e.g. "COUNT_SET_BITS_64"), dest/src are register indices,
// valueA is the source register value, and result is the computed result.
func dumpTwoRegs(op string, destReg, srcReg int, valueA, result uint64) {
	if !PvmTrace {
		return
	}

	// map opcode to the short function name used in the dump
	var fnName string
	switch op {
	case "COUNT_SET_BITS_64":
		fnName = "popcnt64"
	case "COUNT_SET_BITS_32":
		fnName = "popcnt32"
	case "LEADING_ZERO_BITS_64":
		fnName = "lzcnt64"
	case "LEADING_ZERO_BITS_32":
		fnName = "lzcnt32"
	case "TRAILING_ZERO_BITS_64":
		fnName = "tzcnt64"
	case "TRAILING_ZERO_BITS_32":
		fnName = "tzcnt32"
	case "SIGN_EXTEND_8":
		fnName = "sext.b"
	case "SIGN_EXTEND_16":
		fnName = "sext.h"
	case "ZERO_EXTEND_16":
		fnName = "zext.h"
	case "REVERSE_BYTES":
		fnName = "revbytes"
	default:
		fnName = op
	}
	if PvmInterpretation {
		fmt.Printf("\t%s  %s = %s(%s = %x) = %x\n",
			op,
			reg(destReg),
			fnName,
			reg(srcReg),
			valueA,
			result,
		)
	}
	fmt.Printf("%s %s = 0x%x\n", prefixTrace, reg(destReg), result)

}

// Termination Instructions
var T = map[int]struct{}{
	TRAP:            {},
	FALLTHROUGH:     {},
	JUMP:            {},
	JUMP_IND:        {},
	LOAD_IMM_JUMP:   {},
	BRANCH_EQ_IMM:   {},
	BRANCH_NE_IMM:   {},
	BRANCH_LT_U_IMM: {},
	BRANCH_LE_U_IMM: {},
	BRANCH_GE_U_IMM: {},
	BRANCH_GT_U_IMM: {},
	BRANCH_LT_S_IMM: {},
	BRANCH_LE_S_IMM: {},
	BRANCH_GE_S_IMM: {},
	BRANCH_GT_S_IMM: {},
	BRANCH_EQ:       {},
	BRANCH_NE:       {},
	BRANCH_LT_U:     {},
	BRANCH_LT_S:     {},
	BRANCH_GE_U:     {},
	BRANCH_GE_S:     {},
}

func opcode_str(opcode byte) string {
	opcodeMap := map[byte]string{
		0:   "TRAP",
		1:   "FALLTHROUGH",
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
		255: "EXTERNAL_WRITE",
	}

	if name, exists := opcodeMap[opcode]; exists {
		return name
	}
	return fmt.Sprintf("OPCODE %d", opcode)
}

func (vm *VMGo) Str(logStr string) string {
	return fmt.Sprintf("%s_%s: %s", vm.ServiceMetadata, vm.Mode, logStr)
}

// Instruction represents a decoded PVM instruction with source/dest register tracking
type Instruction struct {
	Opcode byte
	Args   []byte
	Step   int
	Pc     uint64

	SourceRegs []int
	DestRegs   []int
	Offset1    int64
	Offset2    int64
	Imm1       uint64
	Imm2       uint64
	ImmS       int64

	GasUsage int64
}

func NewInstruction(opcode byte, args []byte, pc uint64) *Instruction {
	inst := &Instruction{
		Opcode: opcode,
		Args:   args,
		Pc:     pc,
	}
	inst.Decode()
	return inst
}

func (i *Instruction) String() string {
	return fmt.Sprintf("PC=%d %s src=%v dst=%v imm1=%d imm2=%d off1=%d",
		i.Pc, opcode_str(i.Opcode), i.SourceRegs, i.DestRegs, i.Imm1, i.Imm2, i.Offset1)
}

// ToSimpleInstruction converts the Instruction to a SimpleInstruction for tracing
func (i *Instruction) ToSimpleInstruction() trace.SimpleInstruction {
	si := trace.SimpleInstruction{
		Opcode: i.Opcode,
	}
	if len(i.SourceRegs) > 0 {
		si.SrcRegs = make([]int, len(i.SourceRegs))
		copy(si.SrcRegs, i.SourceRegs)
	}
	if len(i.DestRegs) > 0 {
		si.DstRegs = make([]int, len(i.DestRegs))
		copy(si.DstRegs, i.DestRegs)
	}
	// Collect all immediates (including offsets as uint64)
	var imms []uint64
	if i.Imm1 != 0 {
		imms = append(imms, i.Imm1)
	}
	if i.Imm2 != 0 {
		imms = append(imms, i.Imm2)
	}
	if i.Offset1 != 0 {
		imms = append(imms, uint64(i.Offset1))
	}
	if i.ImmS != 0 {
		imms = append(imms, uint64(i.ImmS))
	}
	if len(imms) > 0 {
		si.Imm = imms
	}
	return si
}

// Decode extracts source/dest registers and immediates based on opcode type
func (i *Instruction) Decode() {
	opcode := i.Opcode
	switch {
	case opcode <= 1: // A.5.1 No arguments
		i.decodeNoArgs()
	case opcode == ECALLI: // A.5.2 One immediate
		i.decodeOneImm()
	case opcode == LOAD_IMM_64: // A.5.3 One Register and One Extended Width Immediate
		i.decodeOneRegOneEWImm()
	case 30 <= opcode && opcode <= 33: // A.5.4 Two Immediates
		i.decodeTwoImms()
	case opcode == JUMP: // A.5.5 One offset
		i.decodeOneOffset()
	case 50 <= opcode && opcode <= 62: // A.5.6 One Register and One Immediate
		i.decodeOneRegOneImm()
	case 70 <= opcode && opcode <= 73: // A.5.7 One Register and Two Immediates
		i.decodeOneRegTwoImm()
	case 80 <= opcode && opcode <= 90: // A.5.8 One Register, One Immediate and One Offset
		i.decodeOneRegOneImmOneOffset()
	case 100 <= opcode && opcode <= 111: // A.5.9 Two Registers
		i.decodeTwoRegs()
	case 120 <= opcode && opcode <= 161: // A.5.10 Two Registers and One Immediate
		i.decodeTwoRegsOneImm()
	case 170 <= opcode && opcode <= 175: // A.5.11 Two Registers and One Offset
		i.decodeTwoRegsOneOffset()
	case opcode == LOAD_IMM_JUMP_IND: // A.5.12 Two Registers and Two Immediates
		i.decodeTwoRegsTwoImms()
	case 190 <= opcode && opcode <= 230: // A.5.13 Three Registers
		i.decodeThreeRegs()
	default:
		i.decodeNoArgs()
	}
}

func (i *Instruction) decodeNoArgs() {
	// No arguments
}

func (i *Instruction) decodeOneImm() {
	if len(i.Args) > 0 {
		i.Imm1 = decodeE_l(i.Args)
	}
}

func (i *Instruction) decodeOneRegOneEWImm() {
	if len(i.Args) == 0 {
		return
	}
	reg := min(12, int(i.Args[0])%16)
	lx := min(8, max(0, len(i.Args)-1))
	i.DestRegs = []int{reg}
	if lx > 0 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}
}

func (i *Instruction) decodeTwoImms() {
	if len(i.Args) == 0 {
		return
	}
	lx := min(4, int(i.Args[0])%8)
	ly := min(4, max(0, len(i.Args)-lx-1))
	if lx > 0 && len(i.Args) > 1 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}
	if ly > 0 && len(i.Args) > 1+lx {
		i.Imm2 = decodeE_l(i.Args[1+lx : 1+lx+ly])
	}
}

func (i *Instruction) decodeOneOffset() {
	if len(i.Args) > 0 {
		i.Offset1 = decodeSignedOffset(i.Args)
	}
}

func (i *Instruction) decodeOneRegOneImm() {
	if len(i.Args) == 0 {
		return
	}
	reg := min(12, int(i.Args[0])%16)
	lx := min(4, (int(i.Args[0])/16)%8)
	if lx > 0 && len(i.Args) > 1 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}

	// Determine source/dest based on opcode
	switch i.Opcode {
	case JUMP_IND:
		i.SourceRegs = []int{reg}
	case LOAD_IMM, LOAD_U8, LOAD_I8, LOAD_U16, LOAD_I16, LOAD_U32, LOAD_I32, LOAD_U64:
		i.DestRegs = []int{reg}
	case STORE_U8, STORE_U16, STORE_U32, STORE_U64:
		i.SourceRegs = []int{reg}
	default:
		i.DestRegs = []int{reg}
	}
}

func (i *Instruction) decodeOneRegTwoImm() {
	if len(i.Args) == 0 {
		return
	}
	reg := min(12, int(i.Args[0])%16)
	lx := min(4, (int(i.Args[0])/16)%8)
	ly := min(4, max(0, len(i.Args)-lx-1))
	if lx > 0 && len(i.Args) > 1 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}
	if ly > 0 && len(i.Args) > 1+lx {
		i.Imm2 = decodeE_l(i.Args[1+lx : 1+lx+ly])
	}
	i.SourceRegs = []int{reg}
}

func (i *Instruction) decodeOneRegOneImmOneOffset() {
	if len(i.Args) == 0 {
		return
	}
	reg := min(12, int(i.Args[0])%16)
	lx := min(4, (int(i.Args[0])/16)%8)
	ly := min(3, max(0, len(i.Args)-lx-1))
	if lx > 0 && len(i.Args) > 1 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}
	if ly > 0 && len(i.Args) > 1+lx {
		i.Offset1 = decodeSignedOffsetN(i.Args[1+lx:1+lx+ly], ly)
	}

	if i.Opcode == LOAD_IMM_JUMP {
		i.DestRegs = []int{reg}
	} else {
		i.SourceRegs = []int{reg}
	}
}

func (i *Instruction) decodeTwoRegs() {
	if len(i.Args) == 0 {
		return
	}
	regD := min(12, int(i.Args[0])%16)
	regA := min(12, int(i.Args[0])/16)
	i.DestRegs = []int{regD}
	i.SourceRegs = []int{regA}
}

func (i *Instruction) decodeTwoRegsOneImm() {
	if len(i.Args) == 0 {
		return
	}
	reg1 := min(12, int(i.Args[0])%16)
	reg2 := min(12, int(i.Args[0])/16)
	lx := min(4, max(0, len(i.Args)-1))
	if lx > 0 && len(i.Args) > 1 {
		i.Imm1 = decodeE_l(i.Args[1 : 1+lx])
	}

	// Store instructions: reg1 is dest address source, reg2 is value source
	// Load instructions: reg1 is dest, reg2 is address source
	switch i.Opcode {
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32, STORE_IND_U64:
		i.SourceRegs = []int{reg1, reg2}
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32, LOAD_IND_I32, LOAD_IND_U64:
		i.DestRegs = []int{reg1}
		i.SourceRegs = []int{reg2}
	default:
		// ALU operations: reg1 is dest, reg2 is source
		i.DestRegs = []int{reg1}
		i.SourceRegs = []int{reg2}
	}
}

func (i *Instruction) decodeTwoRegsOneOffset() {
	if len(i.Args) == 0 {
		return
	}
	regA := min(12, int(i.Args[0])%16)
	regB := min(12, int(i.Args[0])/16)
	ly := max(0, len(i.Args)-1)
	if ly > 0 {
		i.Offset1 = decodeSignedOffsetN(i.Args[1:], ly)
	}
	i.SourceRegs = []int{regA, regB}
}

func (i *Instruction) decodeTwoRegsTwoImms() {
	if len(i.Args) < 2 {
		return
	}
	regA := min(12, int(i.Args[0])%16)
	regB := min(12, int(i.Args[0])/16)
	lx := min(4, int(i.Args[1])%8)
	ly := min(4, max(0, len(i.Args)-lx-2))
	if lx > 0 && len(i.Args) > 2 {
		i.Imm1 = decodeE_l(i.Args[2 : 2+lx])
	}
	if ly > 0 && len(i.Args) > 2+lx {
		i.Imm2 = decodeE_l(i.Args[2+lx : 2+lx+ly])
	}

	if i.Opcode == LOAD_IMM_JUMP_IND {
		i.DestRegs = []int{regA}
		i.SourceRegs = []int{regB}
	} else {
		i.SourceRegs = []int{regA, regB}
	}
}

func (i *Instruction) decodeThreeRegs() {
	if len(i.Args) < 2 {
		return
	}
	reg1 := min(12, int(i.Args[0])%16)
	reg2 := min(12, int(i.Args[0])/16)
	dst := min(12, int(i.Args[1])%16)
	i.SourceRegs = []int{reg1, reg2}
	i.DestRegs = []int{dst}
}

// Helper functions for decoding
func decodeE_l(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	var result uint64
	for i := 0; i < len(data); i++ {
		result |= uint64(data[i]) << (8 * i)
	}
	return result
}

func decodeSignedOffset(data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	return decodeSignedOffsetN(data, len(data))
}

func decodeSignedOffsetN(data []byte, n int) int64 {
	if n == 0 || len(data) == 0 {
		return 0
	}
	n = min(n, len(data))
	val := decodeE_l(data[:n])
	// Sign extend
	shift := uint(64 - 8*n)
	return int64(val<<shift) >> shift
}
