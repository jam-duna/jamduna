#include "pvm.h"
#include <inttypes.h>


// Helper function to decode little-endian variable length integers
static uint64_t decode_operand_slice(uint8_t* slice, size_t len) {
    if (len == 0) return 0;
    
    uint64_t decoded = 0;
    switch (len) {
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
        if (len >= 8) {
            // Use first 8 bytes for 64-bit value
            for (int i = 0; i < 8; i++) {
                decoded |= (uint64_t)slice[i] << (8 * i);
            }
        } else {
            // Variable length, less than 8 bytes
            for (size_t i = 0; i < len; i++) {
                decoded |= (uint64_t)slice[i] << (8 * i);
            }
        }
        break;
    }
    return decoded;
}

// Helper function to parse register and one immediate operand
static void parse_reg_imm(uint8_t* operands, size_t operand_len, int* reg_a, uint64_t* vx) {
    *reg_a = 0;
    *vx = 0;
    
    if (operand_len == 0) {
        return;
    }
    
    *reg_a = MIN(12, (int)(operands[0]) % 16);
    int lx = MIN(4, MAX(1, (int)operand_len - 1));
    
    if (1 + lx <= (int)operand_len) {
        uint8_t* slice = operands + 1;
        uint64_t decoded = decode_operand_slice(slice, lx);
        
        // Sign extend if necessary
        if (lx < 8) {
            uint32_t shift = 64 - 8 * lx;
            *vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        } else {
            *vx = decoded;
        }
    }
}


// A.5.6. Instructions with Arguments of One Register & One Immediate

void handle_JUMP_IND(VM* vm, uint8_t* operands, size_t operand_len) {
    
    int register_index_a;
    uint64_t vx;
    
    // Match Go logic exactly - read from operands but ensure we have enough data
    if (operand_len == 0) {
        register_index_a = 0;
        vx = 0;
    } else {
        register_index_a = MIN(12, (int)(operands[0] % 16));
        int lx = MIN(4, MAX(1, (int)operand_len - 1));
        if (1 + lx <= (int)operand_len) {
            uint8_t* slice = operands + 1;
            uint64_t decoded = 0;
            // Match Go's exact switch logic
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
                if (lx >= 8) {
                    // Use first 8 bytes
                    for (int i = 0; i < 8; i++) {
                        decoded |= (uint64_t)slice[i] << (8 * i);
                    }
                } else {
                    for (int i = 0; i < lx; i++) {
                        decoded |= (uint64_t)slice[i] << (8 * i);
                    }
                }
                break;
            }
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
        } else {
            uint32_t shift = 64 - 8 * lx;
            vx = (uint64_t)((int64_t)(0 << shift) >> shift);
        }
    }
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (value_a + vx) & 0xffffffff;

    
    pvm_djump(vm, target);
}

void handle_LOAD_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    parse_reg_imm(operands, operand_len, &register_index_a, &vx);
    
    vm->registers[register_index_a] = vx;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
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

void handle_LOAD_I8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    int err_code;
    uint8_t value = pvm_read_ram_bytes_8(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    if (value & 0x80) {
        result |= 0xFFFFFFFFFFFFFF00ULL;
    }
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
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

void handle_LOAD_I16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    int err_code;
    uint16_t value = pvm_read_ram_bytes_16(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    if (value & 0x8000) {
        result |= 0xFFFFFFFFFFFF0000ULL;
    }
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    if (vm->pvm_tracing) {
    printf("LOAD_U32 DEBUG: register_index=%d, addr_64=0x%llx, operands=[", 
           register_index_a, (unsigned long long)addr_64);
    for (size_t i = 0; i < operand_len && i < 8; i++) {
        printf("%02x", operands[i]);
        if (i < operand_len - 1 && i < 7) printf(" ");
    }
    printf("]\n");
    }
    
    uint32_t addr = (uint32_t)addr_64;
    int err_code;
    uint32_t value = pvm_read_ram_bytes_32(vm, addr, &err_code);
    if (err_code != OK) {
    if (vm->pvm_tracing) {
        printf("LOAD_U32 ERROR: Failed to read from address 0x%x, error=%d\n", addr, err_code);
    }
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    vm->registers[register_index_a] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_I32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    int err_code;
    uint32_t value = pvm_read_ram_bytes_32(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    uint64_t result = (uint64_t)value;
    if (value & 0x80000000) {
        result |= 0xFFFFFFFF00000000ULL;
    }
    vm->registers[register_index_a] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_LOAD_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    int err_code;
    uint64_t value = pvm_read_ram_bytes_64(vm, addr, &err_code);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->registers[register_index_a] = value;
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t err_code = pvm_write_ram_bytes_8(vm, addr, (uint8_t)value_a);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    uint64_t value_a = vm->registers[register_index_a];
    uint16_t value = (uint16_t)value_a;
    uint64_t err_code = pvm_write_ram_bytes_16(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t value = (uint32_t)value_a;
    uint64_t err_code = pvm_write_ram_bytes_32(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t addr_64;
    parse_reg_imm(operands, operand_len, &register_index_a, &addr_64);
    
    uint32_t addr = (uint32_t)addr_64;
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t err_code = pvm_write_ram_bytes_64(vm, addr, value_a);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    vm->pc += 1 + operand_len;
}


const char* reg_name(int index) {
    static const char* names[] = {
        "ra", "sp", "t0", "t1", "t2", "s0", "s1", 
        "a0", "a1", "a2", "a3", "a4", "a5"
    };
    if (index >= 0 && index <= 12) {
        return names[index];
    }
    static char buf[8];
    snprintf(buf, sizeof(buf), "R%d", index);
    return buf;
}

