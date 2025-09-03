#include "pvm.h"


// Helper function to decode little-endian variable length integers with sign extension
static void parse_two_reg_one_offset(uint8_t* operands, size_t operand_len, int* reg_a, int* reg_b, int64_t* vx0) {
    *reg_a = 0;
    *reg_b = 0;
    *vx0 = 0;
    
    if (operand_len == 0) {
        return;
    }
    
    int first_byte = (int)operands[0];
    *reg_a = MIN(12, first_byte % 16);
    *reg_b = MIN(12, first_byte / 16);
    
    int lx = MIN(4, MAX(0, (int)operand_len - 1));
    if (lx == 0) {
        lx = 1;
    }
    
    if (1 + lx <= (int)operand_len) {
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
            // Handle >= 8 bytes or 5-7 bytes
            if (lx >= 8) {
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
        
        // Sign extend based on lx
        uint32_t shift = 64 - 8 * lx;
        *vx0 = (int64_t)(decoded << shift) >> shift;
    } else {
        uint32_t shift = 64 - 8 * lx;
        *vx0 = (int64_t)(0 << shift) >> shift;
    }
}

// A.5.11. Instructions with Arguments of Two Registers and One Offset (Branch operations)

void handle_BRANCH_EQ(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a == value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_EQ", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

void handle_BRANCH_NE(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a != value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_NE", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

void handle_BRANCH_LT_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a < value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_LT_U", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

void handle_BRANCH_LT_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = ((int64_t)value_a < (int64_t)value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_LT_S", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

void handle_BRANCH_GE_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a >= value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_GE_U", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

void handle_BRANCH_GE_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = ((int64_t)value_a >= (int64_t)value_b);
    
    if (taken) {
        vm_branch(vm, target, 1);
    } else {
        vm->pc += 1 + operand_len;
    }
    
#if PVM_TRACE
        dump_branch("BRANCH_GE_S", register_index_a, register_index_b, value_a, value_b, target, taken);
#endif
}

