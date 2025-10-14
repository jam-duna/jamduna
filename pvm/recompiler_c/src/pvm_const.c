#include "pvm_const.h"
#include <string.h>

// Global debugging and logging flags
bool pvm_logging = false;
bool pvm_trace = false;
bool pvm_trace2 = false;
bool show_disassembly = true;
bool use_ecalli500 = false;
bool debug_recompiler = false;
bool use_tally = false;

// Initialize debugging flags (can be called to reset to defaults)
void pvm_const_init(void) {
    pvm_logging = false;
    pvm_trace = false;
    pvm_trace2 = false;
    show_disassembly = true;
    use_ecalli500 = false;
    debug_recompiler = false;
    use_tally = false;
}


// Convert opcode to human-readable string
const char* opcode_to_string(uint8_t opcode) {
    switch (opcode) {
        // A.5.1. Instructions without Arguments
        case TRAP: return "TRAP";
        case FALLTHROUGH: return "FALLTHROUGH";
        
        // A.5.2. Instructions with Arguments of One Immediate
        case ECALLI: return "ECALLI";
        
        // A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
        case LOAD_IMM_64: return "LOAD_IMM_64";
        
        // A.5.4. Instructions with Arguments of Two Immediates
        case STORE_IMM_U8: return "STORE_IMM_U8";
        case STORE_IMM_U16: return "STORE_IMM_U16";
        case STORE_IMM_U32: return "STORE_IMM_U32";
        case STORE_IMM_U64: return "STORE_IMM_U64";
        
        // A.5.5. Instructions with Arguments of One Offset
        case JUMP: return "JUMP";
        
        // A.5.6. Instructions with Arguments of One Register & Two Immediates
        case JUMP_IND: return "JUMP_IND";
        case LOAD_IMM: return "LOAD_IMM";
        case LOAD_U8: return "LOAD_U8";
        case LOAD_I8: return "LOAD_I8";
        case LOAD_U16: return "LOAD_U16";
        case LOAD_I16: return "LOAD_I16";
        case LOAD_U32: return "LOAD_U32";
        case LOAD_I32: return "LOAD_I32";
        case LOAD_U64: return "LOAD_U64";
        case STORE_U8: return "STORE_U8";
        case STORE_U16: return "STORE_U16";
        case STORE_U32: return "STORE_U32";
        case STORE_U64: return "STORE_U64";
        
        // A.5.7. Instructions with Arguments of One Register & Two Immediates
        case STORE_IMM_IND_U8: return "STORE_IMM_IND_U8";
        case STORE_IMM_IND_U16: return "STORE_IMM_IND_U16";
        case STORE_IMM_IND_U32: return "STORE_IMM_IND_U32";
        case STORE_IMM_IND_U64: return "STORE_IMM_IND_U64";
        
        // A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
        case LOAD_IMM_JUMP: return "LOAD_IMM_JUMP";
        case BRANCH_EQ_IMM: return "BRANCH_EQ_IMM";
        case BRANCH_NE_IMM: return "BRANCH_NE_IMM";
        case BRANCH_LT_U_IMM: return "BRANCH_LT_U_IMM";
        case BRANCH_LE_U_IMM: return "BRANCH_LE_U_IMM";
        case BRANCH_GE_U_IMM: return "BRANCH_GE_U_IMM";
        case BRANCH_GT_U_IMM: return "BRANCH_GT_U_IMM";
        case BRANCH_LT_S_IMM: return "BRANCH_LT_S_IMM";
        case BRANCH_LE_S_IMM: return "BRANCH_LE_S_IMM";
        case BRANCH_GE_S_IMM: return "BRANCH_GE_S_IMM";
        case BRANCH_GT_S_IMM: return "BRANCH_GT_S_IMM";
        
        // A.5.9. Instructions with Arguments of Two Registers
        case MOVE_REG: return "MOVE_REG";
        case SBRK: return "SBRK";
        case COUNT_SET_BITS_64: return "COUNT_SET_BITS_64";
        case COUNT_SET_BITS_32: return "COUNT_SET_BITS_32";
        case LEADING_ZERO_BITS_64: return "LEADING_ZERO_BITS_64";
        case LEADING_ZERO_BITS_32: return "LEADING_ZERO_BITS_32";
        case TRAILING_ZERO_BITS_64: return "TRAILING_ZERO_BITS_64";
        case TRAILING_ZERO_BITS_32: return "TRAILING_ZERO_BITS_32";
        case SIGN_EXTEND_8: return "SIGN_EXTEND_8";
        case SIGN_EXTEND_16: return "SIGN_EXTEND_16";
        case ZERO_EXTEND_16: return "ZERO_EXTEND_16";
        case REVERSE_BYTES: return "REVERSE_BYTES";
        
        // A.5.10. Instructions with Arguments of Two Registers & One Immediate
        case STORE_IND_U8: return "STORE_IND_U8";
        case STORE_IND_U16: return "STORE_IND_U16";
        case STORE_IND_U32: return "STORE_IND_U32";
        case STORE_IND_U64: return "STORE_IND_U64";
        case LOAD_IND_U8: return "LOAD_IND_U8";
        case LOAD_IND_I8: return "LOAD_IND_I8";
        case LOAD_IND_U16: return "LOAD_IND_U16";
        case LOAD_IND_I16: return "LOAD_IND_I16";
        case LOAD_IND_U32: return "LOAD_IND_U32";
        case LOAD_IND_I32: return "LOAD_IND_I32";
        case LOAD_IND_U64: return "LOAD_IND_U64";
        case ADD_IMM_32: return "ADD_IMM_32";
        case AND_IMM: return "AND_IMM";
        case XOR_IMM: return "XOR_IMM";
        case OR_IMM: return "OR_IMM";
        case MUL_IMM_32: return "MUL_IMM_32";
        case SET_LT_U_IMM: return "SET_LT_U_IMM";
        case SET_LT_S_IMM: return "SET_LT_S_IMM";
        case SHLO_L_IMM_32: return "SHLO_L_IMM_32";
        case SHLO_R_IMM_32: return "SHLO_R_IMM_32";
        case SHAR_R_IMM_32: return "SHAR_R_IMM_32";
        case NEG_ADD_IMM_32: return "NEG_ADD_IMM_32";
        case SET_GT_U_IMM: return "SET_GT_U_IMM";
        case SET_GT_S_IMM: return "SET_GT_S_IMM";
        case SHLO_L_IMM_ALT_32: return "SHLO_L_IMM_ALT_32";
        case SHLO_R_IMM_ALT_32: return "SHLO_R_IMM_ALT_32";
        case SHAR_R_IMM_ALT_32: return "SHAR_R_IMM_ALT_32";
        case CMOV_IZ_IMM: return "CMOV_IZ_IMM";
        case CMOV_NZ_IMM: return "CMOV_NZ_IMM";
        case ADD_IMM_64: return "ADD_IMM_64";
        case MUL_IMM_64: return "MUL_IMM_64";
        case SHLO_L_IMM_64: return "SHLO_L_IMM_64";
        case SHLO_R_IMM_64: return "SHLO_R_IMM_64";
        case SHAR_R_IMM_64: return "SHAR_R_IMM_64";
        case NEG_ADD_IMM_64: return "NEG_ADD_IMM_64";
        case SHLO_L_IMM_ALT_64: return "SHLO_L_IMM_ALT_64";
        case SHLO_R_IMM_ALT_64: return "SHLO_R_IMM_ALT_64";
        case SHAR_R_IMM_ALT_64: return "SHAR_R_IMM_ALT_64";
        case ROT_R_64_IMM: return "ROT_R_64_IMM";
        case ROT_R_64_IMM_ALT: return "ROT_R_64_IMM_ALT";
        case ROT_R_32_IMM: return "ROT_R_32_IMM";
        case ROT_R_32_IMM_ALT: return "ROT_R_32_IMM_ALT";
        
        // A.5.11. Instructions with Arguments of Two Registers & One Offset
        case BRANCH_EQ: return "BRANCH_EQ";
        case BRANCH_NE: return "BRANCH_NE";
        case BRANCH_LT_U: return "BRANCH_LT_U";
        case BRANCH_LT_S: return "BRANCH_LT_S";
        case BRANCH_GE_U: return "BRANCH_GE_U";
        case BRANCH_GE_S: return "BRANCH_GE_S";
        
        // A.5.12. Instruction with Arguments of Two Registers and Two Immediates
        case LOAD_IMM_JUMP_IND: return "LOAD_IMM_JUMP_IND";
        
        // A.5.13. Instructions with Arguments of Three Registers
        case ADD_32: return "ADD_32";
        case SUB_32: return "SUB_32";
        case MUL_32: return "MUL_32";
        case DIV_U_32: return "DIV_U_32";
        case DIV_S_32: return "DIV_S_32";
        case REM_U_32: return "REM_U_32";
        case REM_S_32: return "REM_S_32";
        case SHLO_L_32: return "SHLO_L_32";
        case SHLO_R_32: return "SHLO_R_32";
        case SHAR_R_32: return "SHAR_R_32";
        case ADD_64: return "ADD_64";
        case SUB_64: return "SUB_64";
        case MUL_64: return "MUL_64";
        case DIV_U_64: return "DIV_U_64";
        case DIV_S_64: return "DIV_S_64";
        case REM_U_64: return "REM_U_64";
        case REM_S_64: return "REM_S_64";
        case SHLO_L_64: return "SHLO_L_64";
        case SHLO_R_64: return "SHLO_R_64";
        case SHAR_R_64: return "SHAR_R_64";
        case AND: return "AND";
        case XOR: return "XOR";
        case OR: return "OR";
        case MUL_UPPER_S_S: return "MUL_UPPER_S_S";
        case MUL_UPPER_U_U: return "MUL_UPPER_U_U";
        case MUL_UPPER_S_U: return "MUL_UPPER_S_U";
        case SET_LT_U: return "SET_LT_U";
        case SET_LT_S: return "SET_LT_S";
        case CMOV_IZ: return "CMOV_IZ";
        case CMOV_NZ: return "CMOV_NZ";
        case ROT_L_64: return "ROT_L_64";
        case ROT_L_32: return "ROT_L_32";
        case ROT_R_64: return "ROT_R_64";
        case ROT_R_32: return "ROT_R_32";
        case AND_INV: return "AND_INV";
        case OR_INV: return "OR_INV";
        case XNOR: return "XNOR";
        case MAX: return "MAX";
        case MAX_U: return "MAX_U";
        case MIN: return "MIN";
        case MIN_U: return "MIN_U";
        
        default: return "UNKNOWN";
    }
}

// Check if instruction terminates a basic block
bool is_basic_block_terminator(uint8_t opcode) {
    switch (opcode) {
        case TRAP:
        case FALLTHROUGH:
        case JUMP:
        case JUMP_IND:
        case LOAD_IMM_JUMP:
        case LOAD_IMM_JUMP_IND:
        case BRANCH_EQ:
        case BRANCH_NE:
        case BRANCH_LT_U:
        case BRANCH_LT_S:
        case BRANCH_GE_U:
        case BRANCH_GE_S:
        case BRANCH_EQ_IMM:
        case BRANCH_NE_IMM:
        case BRANCH_LT_U_IMM:
        case BRANCH_LE_U_IMM:
        case BRANCH_GE_U_IMM:
        case BRANCH_GT_U_IMM:
        case BRANCH_LT_S_IMM:
        case BRANCH_LE_S_IMM:
        case BRANCH_GE_S_IMM:
        case BRANCH_GT_S_IMM:
            return true;
        default:
            return false;
    }
}

// Check if instruction is a branch/jump instruction
bool is_branch_instruction(uint8_t opcode) {
    switch (opcode) {
        case JUMP:
        case JUMP_IND:
        case LOAD_IMM_JUMP:
        case LOAD_IMM_JUMP_IND:
        case BRANCH_EQ:
        case BRANCH_NE:
        case BRANCH_LT_U:
        case BRANCH_LT_S:
        case BRANCH_GE_U:
        case BRANCH_GE_S:
        case BRANCH_EQ_IMM:
        case BRANCH_NE_IMM:
        case BRANCH_LT_U_IMM:
        case BRANCH_LE_U_IMM:
        case BRANCH_GE_U_IMM:
        case BRANCH_GT_U_IMM:
        case BRANCH_LT_S_IMM:
        case BRANCH_LE_S_IMM:
        case BRANCH_GE_S_IMM:
        case BRANCH_GT_S_IMM:
            return true;
        default:
            return false;
    }
}

// Check if instruction performs memory operations
bool is_memory_instruction(uint8_t opcode) {
    switch (opcode) {
        case LOAD_IMM_64:
        case STORE_IMM_U8:
        case STORE_IMM_U16:
        case STORE_IMM_U32:
        case STORE_IMM_U64:
        case LOAD_IMM:
        case LOAD_U8:
        case LOAD_I8:
        case LOAD_U16:
        case LOAD_I16:
        case LOAD_U32:
        case LOAD_I32:
        case LOAD_U64:
        case STORE_U8:
        case STORE_U16:
        case STORE_U32:
        case STORE_U64:
        case STORE_IMM_IND_U8:
        case STORE_IMM_IND_U16:
        case STORE_IMM_IND_U32:
        case STORE_IMM_IND_U64:
        case STORE_IND_U8:
        case STORE_IND_U16:
        case STORE_IND_U32:
        case STORE_IND_U64:
        case LOAD_IND_U8:
        case LOAD_IND_I8:
        case LOAD_IND_U16:
        case LOAD_IND_I16:
        case LOAD_IND_U32:
        case LOAD_IND_I32:
        case LOAD_IND_U64:
        case SBRK:
            return true;
        default:
            return false;
    }
}

// Get number of arguments for an instruction (approximate, used for parsing)
int get_instruction_arg_count(uint8_t opcode) {
    // A.5.1. Instructions without Arguments
    if (opcode == TRAP || opcode == FALLTHROUGH) {
        return 0;
    }
    
    // A.5.2. Instructions with Arguments of One Immediate
    if (opcode == ECALLI) {
        return 1;
    }
    
    // A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
    if (opcode == LOAD_IMM_64) {
        return 2;
    }
    
    // A.5.4. Instructions with Arguments of Two Immediates
    if (opcode >= STORE_IMM_U8 && opcode <= STORE_IMM_U64) {
        return 2;
    }
    
    // A.5.5. Instructions with Arguments of One Offset
    if (opcode == JUMP) {
        return 1;
    }
    
    // A.5.6. Instructions with Arguments of One Register & Two Immediates
    if ((opcode >= JUMP_IND && opcode <= STORE_U64)) {
        return 3;
    }
    
    // A.5.7. Instructions with Arguments of One Register & Two Immediates
    if (opcode >= STORE_IMM_IND_U8 && opcode <= STORE_IMM_IND_U64) {
        return 3;
    }
    
    // A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
    if (opcode >= LOAD_IMM_JUMP && opcode <= BRANCH_GT_S_IMM) {
        return 3;
    }
    
    // A.5.9. Instructions with Arguments of Two Registers
    if (opcode >= MOVE_REG && opcode <= REVERSE_BYTES) {
        return 2;
    }
    
    // A.5.10. Instructions with Arguments of Two Registers & One Immediate
    if (opcode >= STORE_IND_U8 && opcode <= ROT_R_32_IMM_ALT) {
        return 3;
    }
    
    // A.5.11. Instructions with Arguments of Two Registers & One Offset
    if (opcode >= BRANCH_EQ && opcode <= BRANCH_GE_S) {
        return 3;
    }
    
    // A.5.12. Instruction with Arguments of Two Registers and Two Immediates
    if (opcode == LOAD_IMM_JUMP_IND) {
        return 4;
    }
    
    // A.5.13. Instructions with Arguments of Three Registers
    if (opcode >= ADD_32 && opcode <= MIN_U) {
        return 3;
    }
    
    // Unknown instruction
    return -1;
}

// Memory utility functions (translated from Go utils.go)
uint32_t ceiling_divide(uint32_t a, uint32_t b) {
    return (a + b - 1) / b;
}

uint32_t P_func(uint32_t x) {
    return Z_P * ceiling_divide(x, Z_P);
}

uint32_t Z_func(uint32_t x) {
    return Z_Z * ceiling_divide(x, Z_Z);
}