#include "pvm.h"


// Bit manipulation functions for rotation
static inline uint64_t rotate_left_64(uint64_t value, int shift) {
    shift &= 63;
    return (value << shift) | (value >> (64 - shift));
}

static inline uint32_t rotate_left_32(uint32_t value, int shift) {
    shift &= 31;
    return (value << shift) | (value >> (32 - shift));
}

// Helper function to decode little-endian variable length integers with sign extension
static void parse_two_reg_one_imm(uint8_t* operands, size_t operand_len, int* reg_a, int* reg_b, uint64_t* vx) {
    *reg_a = 0;
    *reg_b = 0;
    *vx = 0;
    
    if (operand_len == 0) {
        return;
    }
    
    uint8_t first_byte = operands[0];
    *reg_a = MIN(12, (int)(first_byte & 0x0F));
    *reg_b = MIN(12, (int)(first_byte >> 4));
    
    int lx = MIN(4, MAX(0, (int)operand_len - 1));
    if (lx > 0 && 1 + lx <= (int)operand_len) {
        uint8_t* slice = operands + 1;
        uint64_t decoded = 0;
        
        // Inline little-endian decode for common cases
        switch (lx) {
        case 1:
            decoded = (uint64_t)slice[0];
            break;
        case 2:
            decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8);
            break;
        case 3:
            decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16);
            break;
        case 4:
            decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | 
                     ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24);
            break;
        default:
            // Should not happen due to lx limit, but handle gracefully
            for (int i = 0; i < lx; i++) {
                decoded |= (uint64_t)slice[i] << (8 * i);
            }
            break;
        }
        
        // Sign extend based on lx
        uint32_t shift = 64 - 8 * lx;
        *vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
    }
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate

// Store operations

void handle_STORE_IND_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    // Match Go logic exactly - read from code memory instead of using bitmask-derived operands
    int register_index_a = 0, register_index_b = 0;
    uint64_t vx = 0;
    
    if (vm->pc + 1 < vm->code_len) {
        uint8_t first_byte = vm->code[vm->pc + 1];
        register_index_a = MIN(12, (int)(first_byte & 0x0F));
        register_index_b = MIN(12, (int)(first_byte >> 4));
        
        int lx = MIN(4, MAX(0, (int)operand_len - 1));
        if (lx > 0 && vm->pc + 2 + lx <= vm->code_len) {
            uint8_t* slice = vm->code + vm->pc + 2;
            uint64_t decoded = 0;
            
            // Inline little-endian decode
            switch (lx) {
            case 1:
                decoded = (uint64_t)slice[0];
                break;
            case 2:
                decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8);
                break;
            case 3:
                decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16);
                break;
            case 4:
                decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | 
                         ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24);
                break;
            default:
                for (int i = 0; i < lx && i < 4; i++) {
                    decoded |= (uint64_t)slice[i] << (8 * i);
                }
                break;
            }
            
            // Sign extend based on lx
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        }
    }
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    uint8_t value = (uint8_t)value_a;
    
    int err_code = pvm_write_ram_bytes_8(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IND_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    // Match Go logic exactly - read from code memory instead of using bitmask-derived operands
    int register_index_a = 0, register_index_b = 0;
    uint64_t vx = 0;
    
    if (vm->pc + 1 < vm->code_len) {
        uint8_t first_byte = vm->code[vm->pc + 1];
        register_index_a = MIN(12, (int)(first_byte & 0x0F));
        register_index_b = MIN(12, (int)(first_byte >> 4));
        
        int lx = MIN(4, MAX(0, (int)operand_len - 1));
        if (lx > 0 && vm->pc + 2 + lx <= vm->code_len) {
            uint8_t* slice = vm->code + vm->pc + 2;
            uint64_t decoded = 0;
            
            switch (lx) {
            case 1: decoded = (uint64_t)slice[0]; break;
            case 2: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8); break;
            case 3: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16); break;
            case 4: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24); break;
            default: for (int i = 0; i < lx && i < 4; i++) decoded |= (uint64_t)slice[i] << (8 * i); break;
            }
            
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        }
    }
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    uint16_t value = (uint16_t)value_a;
    
    int err_code = pvm_write_ram_bytes_16(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IND_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    // Match Go logic exactly - read from code memory instead of using bitmask-derived operands
    int register_index_a = 0, register_index_b = 0;
    uint64_t vx = 0;
    
    if (vm->pc + 1 < vm->code_len) {
        uint8_t first_byte = vm->code[vm->pc + 1];
        register_index_a = MIN(12, (int)(first_byte & 0x0F));
        register_index_b = MIN(12, (int)(first_byte >> 4));
        
        int lx = MIN(4, MAX(0, (int)operand_len - 1));
        if (lx > 0 && vm->pc + 2 + lx <= vm->code_len) {
            uint8_t* slice = vm->code + vm->pc + 2;
            uint64_t decoded = 0;
            
            switch (lx) {
            case 1: decoded = (uint64_t)slice[0]; break;
            case 2: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8); break;
            case 3: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16); break;
            case 4: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24); break;
            default: for (int i = 0; i < lx && i < 4; i++) decoded |= (uint64_t)slice[i] << (8 * i); break;
            }
            
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        }
    }
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    uint32_t value = (uint32_t)value_a;
    
    int err_code = pvm_write_ram_bytes_32(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IND_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Match Go logic exactly - read from code memory instead of using bitmask-derived operands
    int register_index_a = 0, register_index_b = 0;
    uint64_t vx = 0;
    
    if (vm->pc + 1 < vm->code_len) {
        uint8_t first_byte = vm->code[vm->pc + 1];
        register_index_a = MIN(12, (int)(first_byte & 0x0F));
        register_index_b = MIN(12, (int)(first_byte >> 4));
        
        int lx = MIN(4, MAX(0, (int)operand_len - 1));
        if (lx > 0 && vm->pc + 2 + lx <= vm->code_len) {
            uint8_t* slice = vm->code + vm->pc + 2;
            uint64_t decoded = 0;
            
            switch (lx) {
            case 1: decoded = (uint64_t)slice[0]; break;
            case 2: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8); break;
            case 3: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16); break;
            case 4: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24); break;
            default: for (int i = 0; i < lx && i < 4; i++) decoded |= (uint64_t)slice[i] << (8 * i); break;
            }
            
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        }
    }
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)(vm->registers[register_index_b] + vx);
    
    int err_code = pvm_write_ram_bytes_64(vm, addr, value_a);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}

// Load operations

void handle_LOAD_IND_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint8_t value = pvm_read_ram_bytes_8(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_I8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint8_t value = pvm_read_ram_bytes_8(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)(int8_t)value;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint16_t value = pvm_read_ram_bytes_16(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_I16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint16_t value = pvm_read_ram_bytes_16(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)(int16_t)value;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint32_t value = pvm_read_ram_bytes_32(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_I32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t addr = (uint32_t)(value_b + vx);
    
    int err_code;
    uint32_t value = pvm_read_ram_bytes_32(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)(int32_t)value;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_IND_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Match Go logic exactly - read from code memory instead of using bitmask-derived operands
    int register_index_a = 0, register_index_b = 0;
    uint64_t offset = 0;
    
    if (vm->pc + 1 < vm->code_len) {
        uint8_t first_byte = vm->code[vm->pc + 1];
        register_index_a = MIN(12, (int)(first_byte & 0x0F));
        register_index_b = MIN(12, (int)(first_byte >> 4));
        
        int lx = MIN(4, MAX(0, (int)operand_len - 1));
        if (lx > 0 && vm->pc + 2 + lx <= vm->code_len) {
            uint8_t* slice = vm->code + vm->pc + 2;
            uint64_t decoded = 0;
            
            switch (lx) {
            case 1: decoded = (uint64_t)slice[0]; break;
            case 2: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8); break;
            case 3: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16); break;
            case 4: decoded = (uint64_t)slice[0] | ((uint64_t)slice[1] << 8) | ((uint64_t)slice[2] << 16) | ((uint64_t)slice[3] << 24); break;
            default: for (int i = 0; i < lx && i < 4; i++) decoded |= (uint64_t)slice[i] << (8 * i); break;
            }
            
            uint32_t shift = 64 - 8 * lx;
            offset = (uint64_t)((int64_t)(decoded << shift) >> shift);
        }
    }
    
    uint32_t memory_address = (uint32_t)(vm->registers[register_index_b] + offset);
    
    int err_code;
    uint64_t loaded_value = pvm_read_ram_bytes_64(vm, memory_address, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->registers[register_index_a] = loaded_value;
    
    vm->pc += 1 + operand_len;
}

// Arithmetic operations

void handle_ADD_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t sum32 = (value_b + vx) & 0xFFFFFFFF;
    uint64_t result = sum32;
    if (sum32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_ADD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Optimized version for ADD_IMM_64
    int register_index_a = (int)(operands[0] & 0x0F);
    int register_index_b = (int)(operands[0] >> 4);
    
    int operand_len_int = (int)operand_len - 1;
    if (operand_len_int > 8) {
        operand_len_int = 8;
    }
    
    uint64_t vx = 0;
    if (operand_len_int == 0) {
        vx = 0;
    } else if (operand_len_int >= 8) {
        // 8-byte operands - direct little-endian read
        for (int i = 0; i < 8; i++) {
            vx |= (uint64_t)operands[1 + i] << (8 * i);
        }
    } else {
        // Shorter operands - decode and sign-extend
        uint64_t raw_value = 0;
        switch (operand_len_int) {
        case 1:
            raw_value = (uint64_t)operands[1];
            break;
        case 2:
            raw_value = (uint64_t)operands[1] | ((uint64_t)operands[2] << 8);
            break;
        case 3:
            raw_value = (uint64_t)operands[1] | ((uint64_t)operands[2] << 8) | ((uint64_t)operands[3] << 16);
            break;
        case 4:
            raw_value = (uint64_t)operands[1] | ((uint64_t)operands[2] << 8) |
                       ((uint64_t)operands[3] << 16) | ((uint64_t)operands[4] << 24);
            break;
        default:
            for (int i = 0; i < operand_len_int; i++) {
                raw_value |= (uint64_t)operands[1 + i] << (8 * i);
            }
            break;
        }
        
        uint32_t shift = 64 - 8 * operand_len_int;
        vx = (uint64_t)((int64_t)(raw_value << shift) >> shift);
    }
    
    uint64_t result = vm->registers[register_index_b] + vx;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_AND_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b & vx;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_XOR_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b ^ vx;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_OR_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b | vx;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t prod32 = (value_b * vx) & 0xFFFFFFFF;
    uint64_t result = prod32;
    if (prod32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b * vx;
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SET_LT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (value_b < vx) ? 1 : 0;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SET_LT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((int64_t)value_b < (int64_t)vx) ? 1 : 0;
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SET_GT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (value_b > vx) ? 1 : 0;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SET_GT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((int64_t)value_b > (int64_t)vx) ? 1 : 0;
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_NEG_ADD_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t diff32 = (vx - value_b) & 0xFFFFFFFF;
    uint64_t result = diff32;
    if (diff32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_NEG_ADD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = vx - value_b;
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

// Shift operations

void handle_SHLO_L_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t shift32 = (value_b << (vx & 63)) & 0xFFFFFFFF;
    uint64_t result = shift32;
    if (shift32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b << (vx & 63);
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((uint32_t)value_b >> (vx & 31)) & 0xFFFFFFFF;
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_b >> (vx & 63);
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_IMM_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int64_t)(int32_t)value_b >> (vx & 31));
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int64_t)value_b >> (vx & 63));
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t shift32 = (vx << (value_b & 63)) & 0xFFFFFFFF;
    uint64_t result = shift32;
    if (shift32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = vx << (value_b & 63);
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((uint32_t)vx >> (value_b & 31)) & 0xFFFFFFFF;
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = vx >> (value_b & 63);
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int32_t)vx >> (value_b & 31));
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int64_t)vx >> (value_b & 63));
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

// Rotation operations

void handle_ROT_R_64_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = rotate_left_64(value_b, -(int)(vx & 63));
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_64_IMM_ALT(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = rotate_left_64(vx, -(int)(value_b & 63));
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_32_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t rot32 = rotate_left_32((uint32_t)value_b, -(int)(vx & 31));
    uint64_t result = (uint64_t)rot32;
    if (rot32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_32_IMM_ALT(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_b = vm->registers[register_index_b];
    uint32_t rot32 = rotate_left_32((uint32_t)vx, -(int)(value_b & 31));
    uint64_t result = (uint64_t)rot32;
    if (rot32 & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

// Conditional move operations

void handle_CMOV_IZ_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = vx;
    if (value_b != 0) {
        result = value_a;
    }
    
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_CMOV_NZ_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    uint64_t vx;
    parse_two_reg_one_imm(operands, operand_len, &register_index_a, &register_index_b, &vx);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result;
    if (value_b != 0) {
        result = vx;
    } else {
        result = value_a;
    }
    
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

