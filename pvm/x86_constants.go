// Package pvm provides x86-64 machine code generation constants and utilities.
package pvm

// ================================================================================================
// X86 Instruction Constants
// ================================================================================================

// REX Prefix Constants
const (
	X86_REX_W = 0x08 // REX.W - 64-bit operand size
	X86_REX_R = 0x04 // REX.R - Extension of ModRM reg field
	X86_REX_X = 0x02 // REX.X - Extension of SIB index field
	X86_REX_B = 0x01 // REX.B - Extension of ModRM r/m, SIB base, or opcode reg field
)

// ModRM Mode Constants
const (
	X86_MOD_INDIRECT        = 0x00 // [reg] or [disp32]
	X86_MOD_INDIRECT_DISP8  = 0x01 // [reg + disp8]
	X86_MOD_INDIRECT_DISP32 = 0x02 // [reg + disp32]
	X86_MOD_REGISTER        = 0x03 // reg
)

// Primary Opcodes
const (
	X86_OP_ADD_RM_R        = 0x01 // ADD r/m, r
	X86_OP_ADD_R_RM        = 0x03 // ADD r, r/m
	X86_OP_ADD_AL_IMM8     = 0x04 // ADD AL, imm8
	X86_OP_ADD_AX_IMM      = 0x05 // ADD rAX, imm32
	X86_OP_OR_RM_R         = 0x09 // OR r/m, r
	X86_OP_OR_R_RM         = 0x0B // OR r, r/m
	X86_OP_OR_AL_IMM8      = 0x0C // OR AL, imm8
	X86_OP_OR_AX_IMM       = 0x0D // OR rAX, imm32
	X86_OP_AND_RM_R        = 0x21 // AND r/m, r
	X86_OP_AND_R_RM        = 0x23 // AND r, r/m
	X86_OP_SUB_RM_R        = 0x29 // SUB r/m, r
	X86_OP_SUB_R_RM        = 0x2B // SUB r, r/m
	X86_OP_XOR_RM_R        = 0x31 // XOR r/m, r
	X86_OP_XOR_R_RM        = 0x33 // XOR r, r/m
	X86_OP_CMP_RM_R        = 0x39 // CMP r/m, r
	X86_OP_CMP_R_RM        = 0x3B // CMP r, r/m
	X86_OP_REX             = 0x40 // REX prefix base
	X86_OP_PUSH_R          = 0x50 // PUSH r64 (+ reg)
	X86_OP_POP_R           = 0x58 // POP r64 (+ reg)
	X86_OP_MOVSXD          = 0x63 // MOVSXD r64, r/m32
	X86_OP_PUSH_IMM8       = 0x6A // PUSH imm8
	X86_OP_PUSH_IMM32      = 0x68 // PUSH imm32
	X86_OP_IMUL_R_RM       = 0x69 // IMUL r, r/m, imm32
	X86_OP_GROUP1_RM_IMM32 = 0x81 // Group 1 operations with imm32
	X86_OP_GROUP1_RM_IMM8  = 0x83 // Group 1 operations with imm8
	X86_OP_TEST_RM_R       = 0x85 // TEST r/m, r
	X86_OP_XCHG_RM_R       = 0x87 // XCHG r/m, r
	X86_OP_MOV_RM8_R8      = 0x88 // MOV r/m8, r8
	X86_OP_MOV_RM_R        = 0x89 // MOV r/m, r
	X86_OP_MOV_R_RM        = 0x8B // MOV r, r/m
	X86_OP_LEA             = 0x8D // LEA r, m
	X86_OP_MOV_R_IMM       = 0xB8 // MOV r, imm64 (+ reg)
	X86_OP_GROUP2_RM_IMM8  = 0xC1 // Group 2 shift operations with imm8
	X86_OP_MOV_RM_IMM8     = 0xC6 // MOV r/m8, imm8
	X86_OP_MOV_RM_IMM      = 0xC7 // MOV r/m, imm32
	X86_OP_GROUP2_RM_1     = 0xD1 // Group 2 shift operations by 1
	X86_OP_GROUP2_RM_CL    = 0xD3 // Group 2 shift operations by CL
	X86_OP_JMP_REL8        = 0xEB // JMP rel8
	X86_OP_CALL_REL32      = 0xE8 // CALL rel32
	X86_OP_JMP_REL32       = 0xE9 // JMP rel32
	X86_OP_RET             = 0xC3 // RET
	X86_OP_GROUP3_RM       = 0xF7 // Group 3 unary operations
	X86_OP_GROUP5_RM       = 0xFF // Group 5 operations (INC, DEC, CALL, JMP, PUSH)
)

// Additional opcodes not in the main sequence
const (
	X86_OP_UNARY_RM = 0xF7 // Unary operations (TEST, NOT, NEG, MUL, IMUL, DIV, IDIV) on r/m
)

// Additional MOV r64, imm64 opcodes (0xB8 + reg)
const (
	X86_OP_MOV_RAX_IMM64 = 0xB8 // MOV RAX, imm64
	X86_OP_MOV_RDX_IMM64 = 0xBA // MOV RDX, imm64
)

// Common REX prefix combinations
const (
	X86_REX_W_PREFIX = 0x48 // REX.W prefix byte (REX_BASE + REX_W)
)

// ModRM addressing mode constants
const (
	X86_MOD_REG_MASK = 0x07 // Mask for register bits (3 bits)
)

// Two-byte Opcodes (0x0F prefix)
const (
	X86_OP2_MOVZX_R_RM8  = 0xB6 // MOVZX r, r/m8
	X86_OP2_MOVZX_R_RM16 = 0xB7 // MOVZX r, r/m16
	X86_OP2_MOVSX_R_RM8  = 0xBE // MOVSX r, r/m8
	X86_OP2_MOVSX_R_RM16 = 0xBF // MOVSX r, r/m16
	X86_OP2_POPCNT       = 0xB8 // POPCNT r, r/m
	X86_OP2_BSF          = 0xBC // BSF r, r/m
	X86_OP2_BSR          = 0xBD // BSR r, r/m
	X86_OP2_IMUL_R_RM    = 0xAF // IMUL r, r/m
	X86_OP2_BSWAP        = 0xC8 // BSWAP r32/r64 (+ reg)
	X86_OP2_SHLD         = 0xA4 // SHLD r/m, r, imm8
	X86_OP2_SHLD_CL      = 0xA5 // SHLD r/m, r, CL
	X86_OP2_SHRD         = 0xAC // SHRD r/m, r, imm8
	X86_OP2_SHRD_CL      = 0xAD // SHRD r/m, r, CL
)

// Conditional Jump Opcodes (0x0F prefix)
const (
	X86_OP2_JO  = 0x80 // JO rel32
	X86_OP2_JNO = 0x81 // JNO rel32
	X86_OP2_JB  = 0x82 // JB/JNAE/JC rel32
	X86_OP2_JAE = 0x83 // JAE/JNB/JNC rel32
	X86_OP2_JE  = 0x84 // JE/JZ rel32
	X86_OP2_JNE = 0x85 // JNE/JNZ rel32
	X86_OP2_JBE = 0x86 // JBE/JNA rel32
	X86_OP2_JA  = 0x87 // JA/JNBE rel32
	X86_OP2_JS  = 0x88 // JS rel32
	X86_OP2_JNS = 0x89 // JNS rel32
	X86_OP2_JP  = 0x8A // JP/JPE rel32
	X86_OP2_JNP = 0x8B // JNP/JPO rel32
	X86_OP2_JL  = 0x8C // JL/JNGE rel32
	X86_OP2_JGE = 0x8D // JGE/JNL rel32
	X86_OP2_JLE = 0x8E // JLE/JNG rel32
	X86_OP2_JG  = 0x8F // JG/JNLE rel32
)

// Conditional Move Opcodes (0x0F prefix)
const (
	X86_OP2_CMOVB  = 0x42 // CMOVB r, r/m
	X86_OP2_CMOVAE = 0x43 // CMOVAE r, r/m
	X86_OP2_CMOVE  = 0x44 // CMOVE r, r/m
	X86_OP2_CMOVNE = 0x45 // CMOVNE r, r/m
	X86_OP2_CMOVBE = 0x46 // CMOVBE r, r/m
	X86_OP2_CMOVA  = 0x47 // CMOVA r, r/m
	X86_OP2_CMOVGE = 0x4D // CMOVGE r, r/m (greater or equal signed)
	X86_OP2_CMOVLE = 0x4E // CMOVLE r, r/m (less or equal signed)
)

// Conditional Set Opcodes (0x0F prefix)
const (
	X86_OP2_SETB = 0x92 // SETB r/m8 (below unsigned)
	X86_OP2_SETA = 0x97 // SETA r/m8 (above unsigned)
	X86_OP2_SETL = 0x9C // SETL r/m8 (less signed)
	X86_OP2_SETG = 0x9F // SETG r/m8 (greater signed)
)

// ModRM reg field constants for opcodes with sub-operations
const (
	X86_REG_ADD = 0 // ADD (for 0x83 opcode)
	X86_REG_OR  = 1 // OR  (for 0x83 opcode)
	X86_REG_ADC = 2 // ADC (for 0x83 opcode)
	X86_REG_SBB = 3 // SBB (for 0x83 opcode)
	X86_REG_AND = 4 // AND (for 0x83 opcode)
	X86_REG_SUB = 5 // SUB (for 0x83 opcode)
	X86_REG_XOR = 6 // XOR (for 0x83 opcode)
	X86_REG_CMP = 7 // CMP (for 0x83 opcode)
)

// Unary operation reg field constants (for 0xF7 opcode)
const (
	X86_REG_TEST = 0 // TEST (for 0xF7 opcode)
	X86_REG_NOT  = 2 // NOT  (for 0xF7 opcode)
	X86_REG_NEG  = 3 // NEG  (for 0xF7 opcode)
	X86_REG_MUL  = 4 // MUL  (for 0xF7 opcode)
	X86_REG_IMUL = 5 // IMUL (for 0xF7 opcode)
	X86_REG_DIV  = 6 // DIV  (for 0xF7 opcode)
	X86_REG_IDIV = 7 // IDIV (for 0xF7 opcode)
)

// Shift operation reg field constants (for 0xC1/0xD1/0xD3 opcodes)
const (
	X86_REG_ROL = 0 // ROL
	X86_REG_ROR = 1 // ROR
	X86_REG_RCL = 2 // RCL
	X86_REG_RCR = 3 // RCR
	X86_REG_SHL = 4 // SHL/SAL
	X86_REG_SHR = 5 // SHR
	X86_REG_SAR = 7 // SAR
)

// Jump operation reg field constants (for 0xFF opcode)
const (
	X86_REG_CALL_RM  = 2 // CALL r/m (for 0xFF opcode)
	X86_REG_CALL_FAR = 3 // CALL FAR (for 0xFF opcode)
	X86_REG_JMP_RM   = 4 // JMP r/m (for 0xFF opcode)
	X86_REG_JMP_FAR  = 5 // JMP FAR (for 0xFF opcode)
	X86_REG_PUSH_RM  = 6 // PUSH r/m (for 0xFF opcode)
)

// Prefixes
const (
	X86_PREFIX_LOCK       = 0xF0 // LOCK prefix
	X86_PREFIX_REPNE      = 0xF2 // REPNE/REPNZ prefix
	X86_PREFIX_REP        = 0xF3 // REP/REPE/REPZ prefix
	X86_PREFIX_0F         = 0x0F // Two-byte opcode prefix
	X86_PREFIX_66         = 0x66 // Operand-size override prefix
	X86_PREFIX_67         = 0x67 // Address-size override prefix
	X86_PREFIX_SEGMENT_ES = 0x26 // ES segment override prefix
	X86_PREFIX_SEGMENT_CS = 0x2E // CS segment override prefix
	X86_PREFIX_SEGMENT_SS = 0x36 // SS segment override prefix
	X86_PREFIX_SEGMENT_DS = 0x3E // DS segment override prefix
	X86_PREFIX_SEGMENT_FS = 0x64 // FS segment override prefix
	X86_PREFIX_SEGMENT_GS = 0x65 // GS segment override prefix
)

// Single-byte instructions
const (
	X86_INST_NOP   = 0x90 // NOP
	X86_INST_CDQE  = 0x98 // CDQE (RAX sign extend)
	X86_INST_CQO   = 0x99 // CQO (RDX:RAX sign extend)
	X86_INST_LEAVE = 0xC9 // LEAVE
	X86_INST_RET   = 0xC3 // RET
	X86_INST_UD2   = 0x0B // UD2 (undefined instruction - needs 0x0F prefix)
)

// Special immediate values
const (
	X86_IMM_0 = 0x00
	X86_IMM_1 = 0x01
)
const (
	X86_REX_BASE = 0x40 // Base value for REX prefix
	X86_OP_CDQ   = 0x99 // CDQ/CQO instruction
	X86_2OP_UD2  = 0x0B // UD2 (undefined instruction) with 0x0F prefix
	X86_OP_NOP   = 0x90 // NOP instruction
	X86_2OP_JE   = 0x84 // JE/JZ rel32 (0x0F 0x84)
	X86_2OP_JA   = 0x87 // JA/JNBE rel32 (0x0F 0x87)
	X86_2OP_JNE  = 0x85 // JNE/JNZ rel32 (0x0F 0x85)
)

// SIB (Scale-Index-Base) Constants
const (
	X86_SIB_SCALE_1  = 0x00 // Scale factor 1
	X86_SIB_SCALE_2  = 0x01 // Scale factor 2
	X86_SIB_SCALE_4  = 0x02 // Scale factor 4
	X86_SIB_SCALE_8  = 0x03 // Scale factor 8
	X86_SIB_NO_INDEX = 0x04 // No index register (ESP/RSP encoding)
)

// Common register combinations
const (
	X86_RSP_REGBITS = 0x04 // RSP/ESP register bits
	X86_RBP_REGBITS = 0x05 // RBP/EBP register bits
)

// Additional ModRM and addressing constants
const (
	X86_MODRM_RAX_RDX = 0x10 // ModRM byte for [RAX], RDX addressing
	X86_SIB_INDICATOR = 0x04 // rm=4 indicates SIB byte follows
)

// Additional specific immediate values
const (
	X86_NO_PREFIX     = 0x00               // No prefix byte
	X86_MASK_8BIT     = 0xFF               // 8-bit mask
	X86_MASK_16BIT    = 0xFFFF             // 16-bit mask
	X86_MASK_32BIT    = 0xFFFFFFFF         // 32-bit mask
	X86_SHIFT_MASK_32 = 0x1F               // 32-bit shift mask (& 31)
	X86_SHIFT_MASK_64 = 0x3F               // 64-bit shift mask (& 63)
	X86_MIN_INT32     = 0x80000000         // MinInt32 constant
	X86_MAX_INT32     = 0x7FFFFFFF         // MaxInt32 constant
	X86_MIN_INT64     = 0x8000000000000000 // MinInt64 constant
	X86_NEG_ONE       = 0xFF               // -1 as unsigned byte
)
