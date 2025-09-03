#include "pvm.h"


// 128-bit multiplication support (equivalent of Go's bits.Mul64)
static void mul64_full(uint64_t a, uint64_t b, uint64_t* hi, uint64_t* lo) {
    // Use compiler intrinsics if available, otherwise implement manually
    #if defined(__GNUC__) && defined(__x86_64__)
    __uint128_t result = (__uint128_t)a * b;
    *lo = (uint64_t)result;
    *hi = (uint64_t)(result >> 64);
    #else
    // Manual implementation for portability
    uint64_t a_lo = a & 0xFFFFFFFF;
    uint64_t a_hi = a >> 32;
    uint64_t b_lo = b & 0xFFFFFFFF;
    uint64_t b_hi = b >> 32;
    
    uint64_t p0 = a_lo * b_lo;
    uint64_t p1 = a_lo * b_hi;
    uint64_t p2 = a_hi * b_lo;
    uint64_t p3 = a_hi * b_hi;
    
    uint64_t carry = ((p0 >> 32) + (p1 & 0xFFFFFFFF) + (p2 & 0xFFFFFFFF)) >> 32;
    
    *lo = p0 + ((p1 + p2) << 32);
    *hi = p3 + (p1 >> 32) + (p2 >> 32) + carry;
    #endif
}

// Bit rotation functions
static inline uint64_t rotate_left_64(uint64_t value, int shift) {
    shift &= 63;
    return (value << shift) | (value >> (64 - shift));
}

static inline uint32_t rotate_left_32(uint32_t value, int shift) {
    shift &= 31;
    return (value << shift) | (value >> (32 - shift));
}

// Signed modulus function (equivalent to Go's smod)
static int64_t smod(int64_t a, int64_t b) {
    if (b == 0) {
        return a;
    }
    
    int64_t abs_a = (a < 0) ? -a : a;
    int64_t abs_b = (b < 0) ? -b : b;
    int64_t mod_val = abs_a % abs_b;
    
    return (a < 0) ? -mod_val : mod_val;
}

// Helper function to parse three register operands
static void parse_three_registers(uint8_t* operands, int* reg_a, int* reg_b, int* reg_d) {
    *reg_a = MIN(12, (int)(operands[0] & 0x0F));
    *reg_b = MIN(12, (int)(operands[0] >> 4));
    *reg_d = MIN(12, (int)operands[1]);
}

// A.5.13. Instructions with Arguments of Three Registers

// 32-bit arithmetic operations

void handle_ADD_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t sum32 = (uint32_t)value_a + (uint32_t)value_b;
    uint64_t result = (uint64_t)sum32;
    if (sum32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("ADD_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SUB_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t diff32 = (uint32_t)value_a - (uint32_t)value_b;
    uint64_t result = (uint64_t)diff32;
    if (diff32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("SUB_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t prod32 = (uint32_t)value_a * (uint32_t)value_b;
    uint64_t result = (uint64_t)prod32;
    if (prod32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("MUL_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_DIV_U_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if ((value_b & 0xFFFFFFFF) == 0) {
        result = UINT64_MAX;
    } else {
        uint32_t quot32 = (uint32_t)value_a / (uint32_t)value_b;
        result = (uint64_t)quot32;
        if (quot32 & 0x80000000) {
            result |= 0xFFFFFFFF00000000ULL;
        }
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("DIV_U_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_DIV_S_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int32_t a = (int32_t)value_a;
    int32_t b = (int32_t)value_b;
    uint64_t result;
    
    if (b == 0) {
        result = UINT64_MAX;
    } else if (a == INT32_MIN && b == -1) {
        result = (uint64_t)a;
    } else {
        result = (uint64_t)(int64_t)(a / b);
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("DIV_S_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_REM_U_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if ((value_b & 0xFFFFFFFF) == 0) {
        uint32_t val32 = (uint32_t)value_a;
        result = (uint64_t)val32;
        if (val32 & 0x80000000) {
            result |= 0xFFFFFFFF00000000ULL;
        }
    } else {
        uint32_t r = (uint32_t)value_a % (uint32_t)value_b;
        result = (uint64_t)r;
        if (r & 0x80000000) {
            result |= 0xFFFFFFFF00000000ULL;
        }
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("REM_U_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_REM_S_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int32_t a = (int32_t)value_a;
    int32_t b = (int32_t)value_b;
    uint64_t result;
    
    if (b == 0) {
        result = (uint64_t)a;
    } else if (a == INT32_MIN && b == -1) {
        result = 0;
    } else {
        result = (uint64_t)(int64_t)(a % b);
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("REM_S_32", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t shift32 = (uint32_t)value_a << (value_b & 31);
    uint64_t result = (uint64_t)shift32;
    if (shift32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op("<<", register_index_d, register_index_a, value_b & 31, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t shift32 = (uint32_t)value_a >> (value_b & 31);
    uint64_t result = (uint64_t)shift32;
    if (shift32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op(">>", register_index_d, register_index_a, value_b & 31, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int32_t)value_a >> (value_b & 31));
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op(">>", register_index_d, register_index_a, value_b & 31, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// 64-bit arithmetic operations

void handle_ADD_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a + value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("+", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SUB_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a - value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("-", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a * value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("*", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_DIV_U_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if (value_b == 0) {
        result = UINT64_MAX;
    } else {
        result = value_a / value_b;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("/", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_DIV_S_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if (value_b == 0) {
        result = UINT64_MAX;
    } else if ((int64_t)value_a == INT64_MIN && (int64_t)value_b == -1) {
        result = value_a;
    } else {
        result = (uint64_t)((int64_t)value_a / (int64_t)value_b);
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("/", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_REM_U_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if (value_b == 0) {
        result = value_a;
    } else {
        result = value_a % value_b;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("%", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_REM_S_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    
    if ((int64_t)value_a == INT64_MIN && (int64_t)value_b == -1) {
        result = 0;
    } else {
        result = (uint64_t)smod((int64_t)value_a, (int64_t)value_b);
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("%", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a << (value_b & 63);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op("<<", register_index_d, register_index_a, value_b & 63, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a >> (value_b & 63);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op(">>", register_index_d, register_index_a, value_b & 63, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int64_t)value_a >> (value_b & 63));
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_shift_op("SHAR_R_64", register_index_d, register_index_a, value_b & 63, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// Logical operations

void handle_AND(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a & value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("&", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_XOR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a ^ value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("^", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_OR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a | value_b;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("|", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// High multiplication operations

void handle_MUL_UPPER_S_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t hi, lo;
    
    mul64_full(value_a, value_b, &hi, &lo);
    
    // Adjust for signed multiplication
    if (value_a >> 63) {
        hi -= value_b;
    }
    if (value_b >> 63) {
        hi -= value_a;
    }
    
    vm->registers[register_index_d] = hi;
    
#if PVM_TRACE
        dump_three_reg_op("*s", register_index_d, register_index_a, register_index_b, value_a, value_b, hi);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_UPPER_U_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result, lo;
    
    mul64_full(value_a, value_b, &result, &lo);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("*u", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_UPPER_S_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t hi, lo;
    
    mul64_full(value_a, value_b, &hi, &lo);
    
    // Adjust for mixed signed/unsigned multiplication
    if (value_a >> 63) {
        hi -= value_b;
    }
    
    vm->registers[register_index_d] = hi;
    
#if PVM_TRACE
        dump_three_reg_op("*s", register_index_d, register_index_a, register_index_b, value_a, value_b, hi);
#endif
    
    vm->pc += 1 + operand_len;
}

// Comparison operations

void handle_SET_LT_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (value_a < value_b) ? 1 : 0;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_cmp_op("<u", register_index_d, register_index_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_SET_LT_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((int64_t)value_a < (int64_t)value_b) ? 1 : 0;
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_cmp_op("<s", register_index_d, register_index_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// Conditional move operations

void handle_CMOV_IZ(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    
    if (value_b == 0) {
        vm->registers[register_index_d] = value_a;
    #if PVM_TRACE
            dump_cmov_op("CMOV_IZ", register_index_d, register_index_b, value_a, value_a, value_a, 1);
#endif
    }
    
    vm->pc += 1 + operand_len;
}

void handle_CMOV_NZ(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    
    if (value_b != 0) {
        vm->registers[register_index_d] = value_a;
    #if PVM_TRACE
            dump_cmov_op("CMOV_NZ", register_index_d, register_index_b, value_a, value_a, value_a, 0);
#endif
    }
    
    vm->pc += 1 + operand_len;
}

// Rotation operations

void handle_ROT_L_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = rotate_left_64(value_a, (int)(value_b & 63));
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_rot_op("<<", reg_name(register_index_d), reg_name(register_index_a), value_b & 63, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_L_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t rot32 = rotate_left_32((uint32_t)value_a, (int)(value_b & 31));
    uint64_t result = (uint64_t)rot32;
    if (rot32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_rot_op("<<", reg_name(register_index_d), reg_name(register_index_a), value_b & 31, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = rotate_left_64(value_a, -(int)(value_b & 63));
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_rot_op(">>", reg_name(register_index_d), reg_name(register_index_a), value_b & 63, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t rot32 = rotate_left_32((uint32_t)value_a, -(int)(value_b & 31));
    uint64_t result = (uint64_t)rot32;
    if (rot32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_rot_op(">>", reg_name(register_index_d), reg_name(register_index_a), value_b & 31, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// Extended logical operations

void handle_AND_INV(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a & (~value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("&!", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_OR_INV(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a | (~value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("|!", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_XNOR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ~(value_a ^ value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("^!", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

// Min/Max operations

void handle_MAX(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)MAX((int64_t)value_a, (int64_t)value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("max", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MAX_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = MAX(value_a, value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("max", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MIN(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)MIN((int64_t)value_a, (int64_t)value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("min", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_MIN_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = MIN(value_a, value_b);
    
    vm->registers[register_index_d] = result;
    
#if PVM_TRACE
        dump_three_reg_op("minu", register_index_d, register_index_a, register_index_b, value_a, value_b, result);
#endif
    
    vm->pc += 1 + operand_len;
}

