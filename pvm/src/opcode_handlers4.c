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

// Helper function to parse two immediate operands
static void parse_two_immediates(uint8_t* operands, size_t operand_len, uint64_t* vx, uint64_t* vy) {
    *vx = 0;
    *vy = 0;
    
    if (operand_len == 0) {
        return;
    }
    
    int lx = MIN(4, (int)(operands[0]) % 8);
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

// A.5.4. Instructions with Arguments of Two Immediates

void handle_STORE_IMM_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    uint64_t vx, vy;
    parse_two_immediates(operands, operand_len, &vx, &vy);
    
    uint32_t addr = (uint32_t)vx;
    uint64_t err_code = pvm_write_ram_bytes_8(vm, addr, (uint8_t)vy);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    uint64_t vx, vy;
    parse_two_immediates(operands, operand_len, &vx, &vy);
    
    uint32_t addr = (uint32_t)vx;
    uint16_t value = (uint16_t)vy;
    uint64_t err_code = pvm_write_ram_bytes_16(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    uint64_t vx, vy;
    parse_two_immediates(operands, operand_len, &vx, &vy);
    
    uint32_t addr = (uint32_t)vx;
    uint32_t value = (uint32_t)vy;
    uint64_t err_code = pvm_write_ram_bytes_32(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    uint64_t vx, vy;
    parse_two_immediates(operands, operand_len, &vx, &vy);
    
    uint32_t addr = (uint32_t)vx;
    uint64_t err_code = pvm_write_ram_bytes_64(vm, addr, vy);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}


