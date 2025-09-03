#include "pvm.h"
#include <inttypes.h>

// Global dispatch table, tracing flags
void (*dispatch_table[256])(VM*, uint8_t*, size_t) = {0};
int pvm_interpretation = 0;

void init_dispatch_table(void) {
    // Initialize all opcodes to TRAP handler
    for (int i = 0; i < 256; i++) {
        dispatch_table[i] = handle_TRAP;
    }
    
    // Initialize handlers inline (A.5.1 Basic instructions)
    dispatch_table[TRAP]                = handle_TRAP;
    dispatch_table[FALLTHROUGH]         = handle_FALLTHROUGH;
    dispatch_table[ECALLI]              = handle_ECALLI;
    dispatch_table[LOAD_IMM_64]         = handle_LOAD_IMM_64;
    dispatch_table[JUMP]                = handle_JUMP;
    dispatch_table[LOAD_IMM_JUMP_IND]   = handle_LOAD_IMM_JUMP_IND;
    
    // A.5.4 Two Immediates
    dispatch_table[STORE_IMM_U8]  = handle_STORE_IMM_U8;
    dispatch_table[STORE_IMM_U16] = handle_STORE_IMM_U16;
    dispatch_table[STORE_IMM_U32] = handle_STORE_IMM_U32;
    dispatch_table[STORE_IMM_U64] = handle_STORE_IMM_U64;
    
    // A.5.6 One Register & One Immediate
    dispatch_table[JUMP_IND] = handle_JUMP_IND;
    dispatch_table[LOAD_IMM] = handle_LOAD_IMM;
    dispatch_table[LOAD_U8] = handle_LOAD_U8;
    dispatch_table[LOAD_I8] = handle_LOAD_I8;
    dispatch_table[LOAD_U16] = handle_LOAD_U16;
    dispatch_table[LOAD_I16] = handle_LOAD_I16;
    dispatch_table[LOAD_U32] = handle_LOAD_U32;
    dispatch_table[LOAD_I32] = handle_LOAD_I32;
    dispatch_table[LOAD_U64] = handle_LOAD_U64;
    dispatch_table[STORE_U8] = handle_STORE_U8;
    dispatch_table[STORE_U16] = handle_STORE_U16;
    dispatch_table[STORE_U32] = handle_STORE_U32;
    dispatch_table[STORE_U64] = handle_STORE_U64;
    
    // A.5.7 One Register & Two Immediates
    dispatch_table[STORE_IMM_IND_U8]  = handle_STORE_IMM_IND_U8;
    dispatch_table[STORE_IMM_IND_U16] = handle_STORE_IMM_IND_U16;
    dispatch_table[STORE_IMM_IND_U32] = handle_STORE_IMM_IND_U32;
    dispatch_table[STORE_IMM_IND_U64] = handle_STORE_IMM_IND_U64;
    
    // A.5.8 One Register, One Immediate & One Offset
    dispatch_table[LOAD_IMM_JUMP]   = handle_LOAD_IMM_JUMP;
    dispatch_table[BRANCH_EQ_IMM]   = handle_BRANCH_EQ_IMM;
    dispatch_table[BRANCH_NE_IMM]   = handle_BRANCH_NE_IMM;
    dispatch_table[BRANCH_LT_U_IMM] = handle_BRANCH_LT_U_IMM;
    dispatch_table[BRANCH_LE_U_IMM] = handle_BRANCH_LE_U_IMM;
    dispatch_table[BRANCH_GE_U_IMM] = handle_BRANCH_GE_U_IMM;
    dispatch_table[BRANCH_GT_U_IMM] = handle_BRANCH_GT_U_IMM;
    dispatch_table[BRANCH_LT_S_IMM] = handle_BRANCH_LT_S_IMM;
    dispatch_table[BRANCH_LE_S_IMM] = handle_BRANCH_LE_S_IMM;
    dispatch_table[BRANCH_GE_S_IMM] = handle_BRANCH_GE_S_IMM;
    dispatch_table[BRANCH_GT_S_IMM] = handle_BRANCH_GT_S_IMM;
    
    // A.5.9 Two Registers
    dispatch_table[MOVE_REG]              = handle_MOVE_REG;
    dispatch_table[SBRK]                  = handle_SBRK;
    dispatch_table[COUNT_SET_BITS_64]     = handle_COUNT_SET_BITS_64;
    dispatch_table[COUNT_SET_BITS_32]     = handle_COUNT_SET_BITS_32;
    dispatch_table[LEADING_ZERO_BITS_64]  = handle_LEADING_ZERO_BITS_64;
    dispatch_table[LEADING_ZERO_BITS_32]  = handle_LEADING_ZERO_BITS_32;
    dispatch_table[TRAILING_ZERO_BITS_64] = handle_TRAILING_ZERO_BITS_64;
    dispatch_table[TRAILING_ZERO_BITS_32] = handle_TRAILING_ZERO_BITS_32;
    dispatch_table[SIGN_EXTEND_8]         = handle_SIGN_EXTEND_8;
    dispatch_table[SIGN_EXTEND_16]        = handle_SIGN_EXTEND_16;
    dispatch_table[ZERO_EXTEND_16]        = handle_ZERO_EXTEND_16;
    dispatch_table[REVERSE_BYTES]         = handle_REVERSE_BYTES;
    
    // A.5.10 Two Registers & One Immediate
    dispatch_table[STORE_IND_U8]        = handle_STORE_IND_U8;
    dispatch_table[STORE_IND_U16]       = handle_STORE_IND_U16;
    dispatch_table[STORE_IND_U32]       = handle_STORE_IND_U32;
    dispatch_table[STORE_IND_U64]       = handle_STORE_IND_U64;
    dispatch_table[LOAD_IND_U8]         = handle_LOAD_IND_U8;
    dispatch_table[LOAD_IND_I8]         = handle_LOAD_IND_I8;
    dispatch_table[LOAD_IND_U16]        = handle_LOAD_IND_U16;
    dispatch_table[LOAD_IND_I16]        = handle_LOAD_IND_I16;
    dispatch_table[LOAD_IND_U32]        = handle_LOAD_IND_U32;
    dispatch_table[LOAD_IND_I32]        = handle_LOAD_IND_I32;
    dispatch_table[LOAD_IND_U64]        = handle_LOAD_IND_U64;
    dispatch_table[ADD_IMM_32]          = handle_ADD_IMM_32;
    dispatch_table[ADD_IMM_64]          = handle_ADD_IMM_64;
    dispatch_table[AND_IMM]             = handle_AND_IMM;
    dispatch_table[XOR_IMM]             = handle_XOR_IMM;
    dispatch_table[OR_IMM]              = handle_OR_IMM;
    dispatch_table[MUL_IMM_32]          = handle_MUL_IMM_32;
    dispatch_table[MUL_IMM_64]          = handle_MUL_IMM_64;
    dispatch_table[SET_LT_U_IMM]        = handle_SET_LT_U_IMM;
    dispatch_table[SET_LT_S_IMM]        = handle_SET_LT_S_IMM;
    dispatch_table[SET_GT_U_IMM]        = handle_SET_GT_U_IMM;
    dispatch_table[SET_GT_S_IMM]        = handle_SET_GT_S_IMM;
    dispatch_table[NEG_ADD_IMM_32]      = handle_NEG_ADD_IMM_32;
    dispatch_table[NEG_ADD_IMM_64]      = handle_NEG_ADD_IMM_64;
    dispatch_table[SHLO_L_IMM_32]       = handle_SHLO_L_IMM_32;
    dispatch_table[SHLO_L_IMM_64]       = handle_SHLO_L_IMM_64;
    dispatch_table[SHLO_R_IMM_32]       = handle_SHLO_R_IMM_32;
    dispatch_table[SHLO_R_IMM_64]       = handle_SHLO_R_IMM_64;
    dispatch_table[SHAR_R_IMM_32]       = handle_SHAR_R_IMM_32;
    dispatch_table[SHAR_R_IMM_64]       = handle_SHAR_R_IMM_64;
    dispatch_table[SHLO_L_IMM_ALT_32]   = handle_SHLO_L_IMM_ALT_32;
    dispatch_table[SHLO_L_IMM_ALT_64]   = handle_SHLO_L_IMM_ALT_64;
    dispatch_table[SHLO_R_IMM_ALT_32]   = handle_SHLO_R_IMM_ALT_32;
    dispatch_table[SHLO_R_IMM_ALT_64]   = handle_SHLO_R_IMM_ALT_64;
    dispatch_table[SHAR_R_IMM_ALT_32]   = handle_SHAR_R_IMM_ALT_32;
    dispatch_table[SHAR_R_IMM_ALT_64]   = handle_SHAR_R_IMM_ALT_64;
    dispatch_table[ROT_R_64_IMM]        = handle_ROT_R_64_IMM;
    dispatch_table[ROT_R_64_IMM_ALT]    = handle_ROT_R_64_IMM_ALT;
    dispatch_table[ROT_R_32_IMM]        = handle_ROT_R_32_IMM;
    dispatch_table[ROT_R_32_IMM_ALT]    = handle_ROT_R_32_IMM_ALT;
    dispatch_table[CMOV_IZ_IMM]         = handle_CMOV_IZ_IMM;
    dispatch_table[CMOV_NZ_IMM]         = handle_CMOV_NZ_IMM;
    
    // A.5.11 Two Registers & One Offset
    dispatch_table[BRANCH_EQ]     = handle_BRANCH_EQ;
    dispatch_table[BRANCH_NE]     = handle_BRANCH_NE;
    dispatch_table[BRANCH_LT_U]   = handle_BRANCH_LT_U;
    dispatch_table[BRANCH_LT_S]   = handle_BRANCH_LT_S;
    dispatch_table[BRANCH_GE_U]   = handle_BRANCH_GE_U;
    dispatch_table[BRANCH_GE_S]   = handle_BRANCH_GE_S;
    
    // A.5.13 Three Registers
    dispatch_table[ADD_32]          = handle_ADD_32;
    dispatch_table[SUB_32]          = handle_SUB_32;
    dispatch_table[MUL_32]          = handle_MUL_32;
    dispatch_table[DIV_U_32]        = handle_DIV_U_32;
    dispatch_table[DIV_S_32]        = handle_DIV_S_32;
    dispatch_table[REM_U_32]        = handle_REM_U_32;
    dispatch_table[REM_S_32]        = handle_REM_S_32;
    dispatch_table[SHLO_L_32]       = handle_SHLO_L_32;
    dispatch_table[SHLO_R_32]       = handle_SHLO_R_32;
    dispatch_table[SHAR_R_32]       = handle_SHAR_R_32;
    dispatch_table[ADD_64]          = handle_ADD_64;
    dispatch_table[SUB_64]          = handle_SUB_64;
    dispatch_table[MUL_64]          = handle_MUL_64;
    dispatch_table[DIV_U_64]        = handle_DIV_U_64;
    dispatch_table[DIV_S_64]        = handle_DIV_S_64;
    dispatch_table[REM_U_64]        = handle_REM_U_64;
    dispatch_table[REM_S_64]        = handle_REM_S_64;
    dispatch_table[SHLO_L_64]       = handle_SHLO_L_64;
    dispatch_table[SHLO_R_64]       = handle_SHLO_R_64;
    dispatch_table[SHAR_R_64]       = handle_SHAR_R_64;
    dispatch_table[AND]             = handle_AND;
    dispatch_table[XOR]             = handle_XOR;
    dispatch_table[OR]              = handle_OR;
    dispatch_table[MUL_UPPER_S_S]   = handle_MUL_UPPER_S_S;
    dispatch_table[MUL_UPPER_U_U]   = handle_MUL_UPPER_U_U;
    dispatch_table[MUL_UPPER_S_U]   = handle_MUL_UPPER_S_U;
    dispatch_table[SET_LT_U]        = handle_SET_LT_U;
    dispatch_table[SET_LT_S]        = handle_SET_LT_S;
    dispatch_table[CMOV_IZ]         = handle_CMOV_IZ;
    dispatch_table[CMOV_NZ]         = handle_CMOV_NZ;
    dispatch_table[ROT_L_64]        = handle_ROT_L_64;
    dispatch_table[ROT_L_32]        = handle_ROT_L_32;
    dispatch_table[ROT_R_64]        = handle_ROT_R_64;
    dispatch_table[ROT_R_32]        = handle_ROT_R_32;
    dispatch_table[AND_INV]         = handle_AND_INV;
    dispatch_table[OR_INV]          = handle_OR_INV;
    dispatch_table[XNOR]            = handle_XNOR;
    dispatch_table[MAX_]            = handle_MAX;
    dispatch_table[MAX_U]           = handle_MAX_U;
    dispatch_table[MIN_]            = handle_MIN;
    dispatch_table[MIN_U]           = handle_MIN_U;
}

// Branch condition symbols
const char* branch_cond_symbol(const char* name) {
    if (strcmp(name, "BRANCH_EQ") == 0 || strcmp(name, "BRANCH_EQ_IMM") == 0) {
        return "==";
    } else if (strcmp(name, "BRANCH_NE") == 0 || strcmp(name, "BRANCH_NE_IMM") == 0) {
        return "!=";
    } else if (strcmp(name, "BRANCH_LT_U") == 0 || strcmp(name, "BRANCH_LT_U_IMM") == 0) {
        return "<u";
    } else if (strcmp(name, "BRANCH_LE_U_IMM") == 0) {
        return "<=u";
    } else if (strcmp(name, "BRANCH_GE_U") == 0 || strcmp(name, "BRANCH_GE_U_IMM") == 0) {
        return ">=u";
    } else if (strcmp(name, "BRANCH_GT_U_IMM") == 0) {
        return ">u";
    } else if (strcmp(name, "BRANCH_LT_S") == 0 || strcmp(name, "BRANCH_LT_S_IMM") == 0) {
        return "<s";
    } else if (strcmp(name, "BRANCH_LE_S_IMM") == 0) {
        return "<=s";
    } else if (strcmp(name, "BRANCH_GE_S") == 0 || strcmp(name, "BRANCH_GE_S_IMM") == 0) {
        return ">=s";
    } else if (strcmp(name, "BRANCH_GT_S_IMM") == 0) {
        return ">s";
    } else {
        return "??";
    }
}

// Debug/trace functions
void dump_store_generic(const char* op, uint64_t addr, const char* reg_or_src, uint64_t value, int bits) {
    char value_str[32];
    snprintf(value_str, sizeof(value_str), "0x%" PRIx64, value);
    
    if (strcmp(value_str, reg_or_src) == 0) {
        printf("%s u%d [0x%" PRIx64 "] = 0x%" PRIx64 "\n", PREFIX_TRACE, bits, addr, value);
    } else {
        printf("%s u%d [0x%" PRIx64 "] = %s = 0x%" PRIx64 "\n", PREFIX_TRACE, bits, addr, reg_or_src, value);
    }
}

void dump_load_imm_jump(const char* op, int register_index_a, uint64_t vx) {
    printf("%s %s = 0x%" PRIx64 "\n", PREFIX_TRACE, reg_name(register_index_a), vx);
}

void dump_load_generic(const char* op, int reg_a, uint64_t addr_or_vx, uint64_t value, int bits, int is_signed) {
    const char* prefix = is_signed ? "i" : "u";
    
    if (pvm_interpretation) {
        printf("\t%s = %s%d[0x%" PRIx64 "] = 0x%" PRIx64 "\n", reg_name(reg_a), prefix, bits, addr_or_vx, value);
    }
    printf("%s %s = %s%d [0x%" PRIx64 "] = 0x%" PRIx64 "\n", PREFIX_TRACE, reg_name(reg_a), prefix, bits, addr_or_vx, value);
}

void dump_load_imm(const char* op, int reg_a, uint64_t addr_or_vx, uint64_t value, int bits, int is_signed) {
    const char* prefix = is_signed ? "i" : "u";
    
    if (pvm_interpretation) {
        printf("\t%s = %s%d[0x%" PRIx64 "] = 0x%" PRIx64 "\n", reg_name(reg_a), prefix, bits, addr_or_vx, value);
    }
    
    printf("%s %s = 0x%" PRIx64 "\n", PREFIX_TRACE, reg_name(reg_a), value);
}

void dump_mov(int reg_d, int reg_a, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_three_reg_op(const char* opname, int reg_d, int reg_a, int reg_b, uint64_t val_a, uint64_t val_b, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_bin_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_cmp_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_shift_op(const char* name, int reg_a, int reg_b, uint64_t shift, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_rot_op(const char* op, const char* reg_dst, const char* src, uint64_t shift, uint64_t result) {
    // No logging - main loop handles all trace output
}

void dump_cmov_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t val_a, uint64_t result, int is_zero) {
    // No logging - main loop handles all trace output
}

void dump_jump_offset(const char* op, int64_t offset, uint64_t pc) {
    printf("*** jump %" PRId64 "\n", (int64_t)((int64_t)pc + offset));
}

void dump_branch(const char* name, int reg_a, int reg_b, uint64_t value_a, uint64_t value_b, uint64_t vx, int taken) {
    const char* cond = branch_cond_symbol(name);
    
    if (pvm_interpretation) {
        printf("\tjump %" PRId64 " if %s (%" PRIx64 ") %s %s (%" PRIx64 ")\n", (int64_t)vx, reg_name(reg_a), value_a, cond, reg_name(reg_b), value_b);
    }
    if (taken) {
        printf("*** jumped to %" PRId64 "\n", (int64_t)vx);
    }
}

void dump_branch_imm(const char* name, int reg_a, uint64_t reg_val, uint64_t vx, uint64_t vy, int is_signed, int taken) {
    const char* cond = branch_cond_symbol(name);
    
    if (pvm_interpretation) {
        printf("\tjump %" PRId64 " if %s %s 0x%" PRIx64 "\n", (int64_t)vy, reg_name(reg_a), cond, vx);
    }
    if (taken) {
        printf("*** jumped to %" PRId64 "\n", (int64_t)vy);
    }
}

void dump_two_regs(const char* op, int dest_reg, int src_reg, uint64_t value_a, uint64_t result) {
    // Map opcode to the short function name used in the dump
    const char* fn_name;
    if (strcmp(op, "COUNT_SET_BITS_64") == 0) {
        fn_name = "popcnt64";
    } else if (strcmp(op, "COUNT_SET_BITS_32") == 0) {
        fn_name = "popcnt32";
    } else if (strcmp(op, "LEADING_ZERO_BITS_64") == 0) {
        fn_name = "lzcnt64";
    } else if (strcmp(op, "LEADING_ZERO_BITS_32") == 0) {
        fn_name = "lzcnt32";
    } else if (strcmp(op, "TRAILING_ZERO_BITS_64") == 0) {
        fn_name = "tzcnt64";
    } else if (strcmp(op, "TRAILING_ZERO_BITS_32") == 0) {
        fn_name = "tzcnt32";
    } else if (strcmp(op, "SIGN_EXTEND_8") == 0) {
        fn_name = "sext.b";
    } else if (strcmp(op, "SIGN_EXTEND_16") == 0) {
        fn_name = "sext.h";
    } else if (strcmp(op, "ZERO_EXTEND_16") == 0) {
        fn_name = "zext.h";
    } else if (strcmp(op, "REVERSE_BYTES") == 0) {
        fn_name = "revbytes";
    } else {
        fn_name = op;
    }
    
    if (pvm_interpretation) {
        printf("\t%s  %s = %s(%s = %" PRIx64 ") = %" PRIx64 "\n",
               op,
               reg_name(dest_reg),
               fn_name,
               reg_name(src_reg),
               value_a,
               result);
    }
    printf("%s %s = 0x%" PRIx64 "\n", PREFIX_TRACE, reg_name(dest_reg), result);
}
