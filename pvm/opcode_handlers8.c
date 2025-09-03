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

// Helper function to parse register, immediate and offset operands
static void parse_reg_imm_offset(uint8_t* operands, size_t operand_len, int* reg_a, uint64_t* vx, int64_t* vy0) {
    *reg_a = 0;
    *vx = 0;
    *vy0 = 0;
    
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
    
    // Parse second immediate/offset (vy0)
    if (1 + lx + ly <= (int)operand_len) {
        uint8_t* slice = operands + 1 + lx;
        uint64_t decoded = decode_operand_slice(slice, ly);
        
        // Sign extend for offset
        uint32_t shift = 64 - 8 * ly;
        *vy0 = (int64_t)(decoded << shift) >> shift;
    }
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset

void handle_LOAD_IMM_JUMP(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    vm->registers[register_index_a] = vx;
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    
#if PVM_TRACE
        dump_load_imm_jump("LOAD_IMM_JUMP", register_index_a, vx);
#endif
    
    vm_branch(vm, target, 1);
}

void handle_BRANCH_EQ_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a == vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_EQ_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_NE_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a != vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_NE_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_LT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a < vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_LT_U_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_LE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a <= vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_LE_U_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_GE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a >= vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_GE_U_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_GT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a > vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_GT_U_IMM", register_index_a, value_a, vx, target, 0, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_LT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a < (int64_t)vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_LT_S_IMM", register_index_a, value_a, vx, target, 1, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_LE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a <= (int64_t)vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_LE_S_IMM", register_index_a, value_a, vx, target, 1, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_GE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a >= (int64_t)vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_GE_S_IMM", register_index_a, value_a, vx, target, 1, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}

void handle_BRANCH_GT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a > (int64_t)vx);
    
#if PVM_TRACE
        dump_branch_imm("BRANCH_GT_S_IMM", register_index_a, value_a, vx, target, 1, taken);
#endif
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
}


