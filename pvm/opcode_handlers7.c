#include "pvm.h"


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

// Helper function to parse register and two immediate operands
static void parse_reg_two_imm(uint8_t* operands, size_t operand_len, int* reg_a, uint64_t* vx, uint64_t* vy) {
    *reg_a = 0;
    *vx = 0;
    *vy = 0;
    
    if (operand_len == 0) {
        return;
    }
    
    int first_byte = (int)operands[0];
    *reg_a = MIN(12, first_byte % 16);
    int lx = MIN(4, (first_byte / 16) % 8);
    int ly = MIN(4, MAX(1, (int)operand_len - lx - 1));
    
    // Parse first immediate (vx)
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
    
    // Parse second immediate (vy)
    if (1 + lx + ly <= (int)operand_len) {
        uint8_t* slice = operands + 1 + lx;
        uint64_t decoded = decode_operand_slice(slice, ly);
        
        // Sign extend if necessary
        if (ly < 8) {
            uint32_t shift = 64 - 8 * ly;
            *vy = (uint64_t)((int64_t)(decoded << shift) >> shift);
        } else {
            *vy = decoded;
        }
    }
}

// A.5.7. Instructions with Arguments of One Register and Two Immediates

void handle_STORE_IMM_IND_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint8_t value = (uint8_t)vy;
    uint64_t err_code = vm_write_ram_bytes_8(vm, addr, value);
    if (err_code != OK) {
        vm_panic(vm, err_code);
        return;
    }
    
#if PVM_TRACE
        dump_store_generic("STORE_IMM_IND_U8", (uint64_t)addr, "imm", (uint64_t)value, 8);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint16_t value = (uint16_t)vy;
    uint64_t err_code = vm_write_ram_bytes_16(vm, addr, value);
    if (err_code != OK) {
        vm_panic(vm, err_code);
        return;
    }
    
#if PVM_TRACE
        dump_store_generic("STORE_IMM_IND_U16", (uint64_t)addr, "imm", (uint64_t)value, 16);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint32_t value = (uint32_t)vy;
    uint64_t err_code = vm_write_ram_bytes_32(vm, addr, value);
    if (err_code != OK) {
        vm_panic(vm, err_code);
        return;
    }
    
#if PVM_TRACE
        dump_store_generic("STORE_IMM_IND_U32", (uint64_t)addr, "imm", (uint64_t)value, 32);
#endif
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint64_t err_code = vm_write_ram_bytes_64(vm, addr, vy);
    if (err_code != OK) {
        vm_panic(vm, err_code);
        return;
    }
    
#if PVM_TRACE
        dump_store_generic("STORE_IMM_IND_U64", (uint64_t)addr, "imm", vy, 64);
#endif
    
    vm->pc += 1 + operand_len;
}

