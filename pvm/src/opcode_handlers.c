#include "pvm.h"

// Global dispatch table, tracing flags
void (*dispatch_table[256])(VM*, uint8_t*, size_t) = {0};


// Opcode Constants

// A.5.1. Instructions without Arguments
#define TRAP                0
#define FALLTHROUGH         1

// A.5.2. Instructions with Arguments of One Immediate  
#define ECALLI              10  // 0x0a

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
#define LOAD_IMM_64         20  // 0x14

// A.5.4. Instructions with Arguments of Two Immediates
#define STORE_IMM_U8        30
#define STORE_IMM_U16       31
#define STORE_IMM_U32       32
#define STORE_IMM_U64       33

// A.5.5. Instructions with Arguments of One Offset
#define JUMP                40  // 0x28

// A.5.6. Instructions with Arguments of One Register & One Immediate
#define JUMP_IND            50
#define LOAD_IMM            51
#define LOAD_U8             52
#define LOAD_I8             53
#define LOAD_U16            54
#define LOAD_I16            55
#define LOAD_U32            56
#define LOAD_I32            57
#define LOAD_U64            58
#define STORE_U8            59
#define STORE_U16           60
#define STORE_U32           61
#define STORE_U64           62

// A.5.7. Instructions with Arguments of One Register & Two Immediates
#define STORE_IMM_IND_U8    70  // 0x46
#define STORE_IMM_IND_U16   71  // 0x47
#define STORE_IMM_IND_U32   72  // 0x48
#define STORE_IMM_IND_U64   73  // 0x49

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
#define LOAD_IMM_JUMP       80
#define BRANCH_EQ_IMM       81  // 0x51
#define BRANCH_NE_IMM       82
#define BRANCH_LT_U_IMM     83
#define BRANCH_LE_U_IMM     84
#define BRANCH_GE_U_IMM     85
#define BRANCH_GT_U_IMM     86
#define BRANCH_LT_S_IMM     87
#define BRANCH_LE_S_IMM     88
#define BRANCH_GE_S_IMM     89
#define BRANCH_GT_S_IMM     90

// A.5.9. Instructions with Arguments of Two Registers
#define MOVE_REG              100
#define SBRK                  101
#define COUNT_SET_BITS_64     102
#define COUNT_SET_BITS_32     103
#define LEADING_ZERO_BITS_64  104
#define LEADING_ZERO_BITS_32  105
#define TRAILING_ZERO_BITS_64 106
#define TRAILING_ZERO_BITS_32 107
#define SIGN_EXTEND_8         108
#define SIGN_EXTEND_16        109
#define ZERO_EXTEND_16        110
#define REVERSE_BYTES         111

// A.5.10. Instructions with Arguments of Two Registers and One Immediate
#define STORE_IND_U8          120
#define STORE_IND_U16         121
#define STORE_IND_U32         122
#define STORE_IND_U64         123
#define LOAD_IND_U8           124
#define LOAD_IND_I8           125
#define LOAD_IND_U16          126
#define LOAD_IND_I16          127
#define LOAD_IND_U32          128
#define LOAD_IND_I32          129
#define LOAD_IND_U64          130
#define ADD_IMM_32            131
#define AND_IMM               132
#define XOR_IMM               133
#define OR_IMM                134
#define MUL_IMM_32            135
#define SET_LT_U_IMM          136
#define SET_LT_S_IMM          137
#define SHLO_L_IMM_32         138
#define SHLO_R_IMM_32         139
#define SHAR_R_IMM_32         140
#define NEG_ADD_IMM_32        141
#define SET_GT_U_IMM          142
#define SET_GT_S_IMM          143
#define SHLO_L_IMM_ALT_32     144
#define SHLO_R_IMM_ALT_32     145
#define SHAR_R_IMM_ALT_32     146
#define CMOV_IZ_IMM           147
#define CMOV_NZ_IMM           148
#define ADD_IMM_64            149
#define MUL_IMM_64            150
#define SHLO_L_IMM_64         151
#define SHLO_R_IMM_64         152
#define SHAR_R_IMM_64         153
#define NEG_ADD_IMM_64        154
#define SHLO_L_IMM_ALT_64     155
#define SHLO_R_IMM_ALT_64     156
#define SHAR_R_IMM_ALT_64     157
#define ROT_R_64_IMM          158
#define ROT_R_64_IMM_ALT      159
#define ROT_R_32_IMM          160
#define ROT_R_32_IMM_ALT      161

// A.5.11. Instructions with Arguments of Two Registers and One Offset (Branch operations)
#define BRANCH_EQ             170
#define BRANCH_NE             171
#define BRANCH_LT_U           172
#define BRANCH_LT_S           173
#define BRANCH_GE_U           174
#define BRANCH_GE_S           175

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
#define LOAD_IMM_JUMP_IND     180

static uint64_t decode_operand_slice(uint8_t* slice, size_t len) {
    if (len == 0) return 0;
    
    uint64_t decoded = 0;
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
    return decoded;
}

// Opcode Constants

// A.5.1. Instructions without Arguments


// Basic VM instruction handlers
void handle_TRAP(VM* vm, uint8_t* operands, size_t operand_len) {
    (void)operands; (void)operand_len; // Suppress unused parameter warnings
    // Set panic state and terminate execution
    vm->result_code = WORKDIGEST_PANIC;
    vm->machine_state = PANIC;
    vm->terminated = 1;  // Terminate execution
    vm->fault_address = 0xFF;
    // Don't advance PC - execution stops here
}

void handle_FALLTHROUGH(VM* vm, uint8_t* operands, size_t operand_len) {
    (void)operands; (void)operand_len; // Suppress unused parameter warnings
    vm->pc += 1;
}



// A.5.2. Instructions with Arguments of One Immediate  

void handle_ECALLI(VM* vm, uint8_t* operands, size_t operand_len) {
    uint32_t lx;
    if (operand_len >= 8) {
        lx = (uint32_t)decode_operand_slice(operands, 8);
    } else {
        uint64_t decoded = decode_operand_slice(operands, operand_len);
        lx = (uint32_t)decoded;
    }
    
    vm->host_call = 1;
    vm->host_func_id = (int)lx;
    vm->pc += 1 + operand_len;
    
    if (vm->is_child) {
        return;
    }
    
    // Charge gas before host call - inlined logic
    const int LOG_HOST_FN = 100;
    const int TRANSFER_HOST_FN = 20; // NOTE: this could change in future!!
    int gas_charge;
    if (vm->host_func_id == LOG_HOST_FN) {
        gas_charge = 0;
    } else if (vm->host_func_id == TRANSFER_HOST_FN) {
        gas_charge = 10 + vm->registers[9];
    } else {
        gas_charge = 10;
    }
    vm->gas -= (int64_t)gas_charge;
    
    if (vm->pvm_tracing) {
    // printf("Host call: function %d\n", vm->host_func_id);
    }
    
    // Trampoline to ext_invoke_host_func with error handling
    if (vm->ext_invoke_host_func) {
        vm->ext_invoke_host_func(vm, vm->host_func_id);
        if (vm->terminated) {
            return;
        }
    } else {
        // No callback registered - panic for the ext 
        if (vm->pvm_tracing) {
            printf("Error: No host callback registered for function %d\n", vm->host_func_id);
        }
        pvm_panic(vm, WHAT);
        return;
    }    
    vm->host_call = 0;
}

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
void handle_LOAD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a = MIN(12, (int)(operands[0] % 16));
    uint8_t* slice = operands + 1;
    uint64_t vx;
    
    if (vm->pc + 1 + 8 < vm->code_len) {
        vx = decode_operand_slice(slice, 8);
    } else {
        vx = 0;
        size_t available_bytes = (operand_len > 0) ? operand_len - 1 : 0;
        for (size_t i = 0; i < available_bytes && i < 8; i++) {
            vx |= (uint64_t)slice[i] << (8 * i);
        }
    }
    
    vm->registers[register_index_a] = vx;
    vm->pc += 1 + operand_len;
}
// A.5.4. Instructions with Arguments of Two Immediates
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

// A.5.5. Instructions with Arguments of One Offset
void handle_JUMP(VM* vm, uint8_t* operands, size_t operand_len) {
    (void)operands; // Suppress unused parameter warning
    int64_t vx;
    int lx = MIN(4, (int)operand_len);
    
    if (lx == 0) {
        vx = 0;
    } else {
        uint8_t* slice = vm->code + vm->pc + 1;  // Read from code
        uint64_t decoded = decode_operand_slice(slice, lx);
        
        // Sign extend the decoded value based on lx
        uint32_t shift = 64 - 8 * lx;
        vx = (int64_t)((int64_t)(decoded << shift) >> shift);
    }
    //printf("----- handle_JUMP %llx\n", (unsigned long long)(vm->pc + vx));
    pvm_branch(vm, (uint64_t)((int64_t)vm->pc + vx), 1, operand_len);
}  


// VM control functions 
void pvm_djump(VM* vm, uint64_t a) {
    if (a == (uint64_t)((1ULL << 32) - (1ULL << 16))) {
        vm->terminated = 1;
        vm->result_code = WORKDIGEST_OK;
    } else if (a == 0 || a > (uint64_t)(vm->j_size * Z_A) || a % Z_A != 0) {
        vm->terminated = 1;
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
    } else {
        vm->pc = (uint64_t)vm->j[(a / Z_A) - 1];
    }
}

void pvm_branch(VM* vm, uint64_t vx, int taken, size_t operand_len) {

    if (taken == 1 && vx < vm->bitmask_len && (vm->bitmask[vx] & 2)) {
       // printf("----- pvm_branch vx=%x/%x bitmask[vx]=%d\n", vx, vm->bitmask_len, vm->bitmask[vx]);
        vm->pc = vx;
    } else if (!(vm->bitmask[vx] & 2)) {
        // Branch marked taken but invalid target
        if (vx >= vm->bitmask_len) {
            //printf("Error: vx out of bounds (vx=%x, bitmask_len=%x)\n", vx, vm->bitmask_len);
        } else if (!(vm->bitmask[vx] & 2)) {
            //printf("Error: invalid branch target (vx=%x not marked executable)\n", vx);
        }
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
        vm->terminated = 1;
    } else {
        // Not taken; increment pc for fall-through
        vm->pc += 1 + operand_len;
    }
}

// A.5.6. Instructions with Arguments of One Register & One Immediate
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

void handle_JUMP_IND(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    if (operand_len == 0) {
        register_index_a = 0;
        vx = 0;
    } else {
        register_index_a = MIN(12, (int)(operands[0] % 16));
        int lx = MIN(4, MAX(1, (int)operand_len - 1));
        if (1 + lx <= (int)operand_len) {
            uint8_t* slice = operands + 1;
            uint64_t decoded = 0;
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

// A.5.7. Instructions with Arguments of One Register and Two Immediates
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
void handle_STORE_IMM_IND_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint8_t value = (uint8_t)vy;
    uint64_t err_code = pvm_write_ram_bytes_8(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U16(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint16_t value = (uint16_t)vy;
    uint64_t err_code = pvm_write_ram_bytes_16(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint32_t value = (uint32_t)vy;
    uint64_t err_code = pvm_write_ram_bytes_32(vm, addr, value);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

void handle_STORE_IMM_IND_U64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx, vy;
    parse_reg_two_imm(operands, operand_len, &register_index_a, &vx, &vy);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint32_t addr = (uint32_t)value_a + (uint32_t)vx;
    uint64_t err_code = pvm_write_ram_bytes_64(vm, addr, vy);
    if (err_code != OK) {
        pvm_panic(vm, err_code);
        return;
    }
    
    
    vm->pc += 1 + operand_len;
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
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

void handle_LOAD_IMM_JUMP(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    vm->registers[register_index_a] = vx;
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    
    pvm_branch(vm, target, 1, operand_len);
}

void handle_BRANCH_EQ_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a == vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_NE_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a != vx);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a < vx);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a <= vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a >= vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = (value_a > vx);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a < (int64_t)vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a <= (int64_t)vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a >= (int64_t)vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a;
    uint64_t vx;
    int64_t vy0;
    parse_reg_imm_offset(operands, operand_len, &register_index_a, &vx, &vy0);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t target = (uint64_t)((int64_t)vm->pc + vy0);
    int taken = ((int64_t)value_a > (int64_t)vx);
    
    
    pvm_branch(vm, target, taken, operand_len);
}

// A.5.9. Instructions with Arguments of Two Registers
extern uint32_t p_func(uint32_t x);

// Bit manipulation functions 
static inline int count_set_bits_64(uint64_t x) {
    return __builtin_popcountll(x);
}

static inline int count_set_bits_32(uint32_t x) {
    return __builtin_popcount(x);
}

static inline int leading_zero_bits_64(uint64_t x) {
    return x ? __builtin_clzll(x) : 64;
}

static inline int leading_zero_bits_32(uint32_t x) {
    return x ? __builtin_clz(x) : 32;
}

static inline int trailing_zero_bits_64(uint64_t x) {
    return x ? __builtin_ctzll(x) : 64;
}

static inline int trailing_zero_bits_32(uint32_t x) {
    return x ? __builtin_ctz(x) : 32;
}

static inline uint64_t reverse_bytes_64(uint64_t x) {
    return __builtin_bswap64(x);
}

void handle_MOVE_REG(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction with minimal bounds checking
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    
    // Use branchless bounds check with bitwise operations
    // Since indices are 4-bit (0-15), we only need to clamp values > 12
    if (register_index_d > 12) {
        register_index_d = 12;
    }
    if (register_index_a > 12) {
        register_index_a = 12;
    }
    
    vm->registers[register_index_d] = vm->registers[register_index_a];
    
    vm->pc += 1 + operand_len;
}

void handle_SBRK(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    uint64_t value_a = vm->registers[register_index_a];
    
    if (value_a == 0) {
        vm->registers[register_index_d] = vm->current_heap_pointer;
        vm->pc += 1 + operand_len;
        return;
    }

    uint64_t result = (uint64_t)vm->current_heap_pointer;
    uint32_t next_page_boundary = p_func(vm->current_heap_pointer);
    uint64_t new_heap_pointer = (uint64_t)vm->current_heap_pointer + value_a;

    if (new_heap_pointer > (uint64_t)next_page_boundary) {
        uint32_t new_heap_pointer_expanded = p_func((uint32_t)new_heap_pointer);
        pvm_expand_heap(vm, new_heap_pointer_expanded);
    }

    vm->current_heap_pointer = (uint32_t)new_heap_pointer;
    vm->rw_data_address_end = vm->current_heap_pointer;

    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_COUNT_SET_BITS_64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)count_set_bits_64(value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_COUNT_SET_BITS_32(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)count_set_bits_32((uint32_t)value_a);
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_LEADING_ZERO_BITS_64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)leading_zero_bits_64(value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_LEADING_ZERO_BITS_32(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)leading_zero_bits_32((uint32_t)value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_TRAILING_ZERO_BITS_64(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)trailing_zero_bits_64(value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_TRAILING_ZERO_BITS_32(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)trailing_zero_bits_32((uint32_t)value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_SIGN_EXTEND_8(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)(int8_t)(value_a & 0xFF);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_SIGN_EXTEND_16(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = (uint64_t)(int16_t)(value_a & 0xFFFF);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_ZERO_EXTEND_16(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = value_a & 0xFFFF;
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}

void handle_REVERSE_BYTES(VM* vm, uint8_t* operands, size_t operand_len) {
    // Inline register extraction
    uint8_t reg_byte = operands[0];
    int register_index_d = (int)(reg_byte & 0x0F);
    int register_index_a = (int)(reg_byte >> 4);
    if (register_index_d > 12) register_index_d = 12;
    if (register_index_a > 12) register_index_a = 12;
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t result = reverse_bytes_64(value_a);
    
    
    vm->registers[register_index_d] = result;
    vm->pc += 1 + operand_len;
}


// A.5.10. Instructions with Arguments of Two Registers and One Immediate
// Bit manipulation functions for rotation
static inline uint64_t rotate_left_64(uint64_t value, int shift) {
    shift &= 63;
    return (value << shift) | (value >> (64 - shift));
}

static inline uint32_t rotate_left_32(uint32_t value, int shift) {
    shift &= 31;
    return (value << shift) | (value >> (32 - shift));
}

// decodes little-endian variable length integers with sign extension
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

// Store operations
void handle_STORE_IND_U8(VM* vm, uint8_t* operands, size_t operand_len) {
    (void)operands; // Suppress unused parameter warning
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
    (void)operands; // Suppress unused parameter warning
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
    (void)operands; // Suppress unused parameter warning
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
    (void)operands; // Suppress unused parameter warning
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
    (void)operands; // Suppress unused parameter warning
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

// A.5.11. Instructions with Arguments of Two Registers and One Offset (Branch operations)

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

void handle_BRANCH_EQ(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a == value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_NE(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a != value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LT_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a < value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_LT_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = ((int64_t)value_a < (int64_t)value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GE_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = (value_a >= value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}

void handle_BRANCH_GE_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b;
    int64_t vx0;
    parse_two_reg_one_offset(operands, operand_len, &register_index_a, &register_index_b, &vx0);
    
    uint64_t target = (uint64_t)((int64_t)vm->pc + vx0);
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    int taken = ((int64_t)value_a >= (int64_t)value_b);
    
    pvm_branch(vm, target, taken, operand_len);
}


// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
void handle_LOAD_IMM_JUMP_IND(VM* vm, uint8_t* operands, size_t operand_len) {
    if (operand_len < 2) {
        return;
    }
    
    int first_byte = (int)operands[0];
    int register_index_a = MIN(12, first_byte % 16);
    int register_index_b = MIN(12, first_byte / 16);
    
    int lx = MIN(4, (int)(operands[1] % 8));
    int ly = MIN(4, MAX(0, (int)operand_len - lx - 2));
    if (ly == 0) {
        ly = 1;
    }
    
    uint64_t value_b = vm->registers[register_index_b];
    
    // Extract vx
    uint64_t vx = 0;
    uint64_t vy = 0;
    
    if (2 + lx <= (int)operand_len) {
        uint8_t* slice = operands + 2;
        uint64_t decoded = decode_operand_slice(slice, lx);
        uint32_t shift = 64 - 8 * lx;
        vx = (uint64_t)((int64_t)(decoded << shift) >> shift);
    } else {
        uint32_t shift = 64 - 8 * lx;
        vx = (uint64_t)((int64_t)(0 << shift) >> shift);
    }
    
    vm->registers[register_index_a] = vx;
    
    // Extract vy
    if (2 + lx + ly <= (int)operand_len) {
        uint8_t* slice = operands + 2 + lx;
        uint64_t decoded = decode_operand_slice(slice, ly);
        uint32_t shift = 64 - 8 * ly;
        vy = (uint64_t)((int64_t)(decoded << shift) >> shift);
    } else {
        uint32_t shift = 64 - 8 * ly;
        vy = (uint64_t)((int64_t)(0 << shift) >> shift);
    }
    
    uint64_t jump_addr = (value_b + vy) & 0xFFFFFFFF;
    vm->pc += 1 + operand_len;
    pvm_djump(vm, jump_addr);
}

// A.5.13. Instructions with Arguments of Three Registers
#define ADD_32                190
#define SUB_32                191
#define MUL_32                192
#define DIV_U_32              193
#define DIV_S_32              194
#define REM_U_32              195
#define REM_S_32              196
#define SHLO_L_32             197
#define SHLO_R_32             198
#define SHAR_R_32             199
#define ADD_64                200
#define SUB_64                201
#define MUL_64                202
#define DIV_U_64              203
#define DIV_S_64              204
#define REM_U_64              205
#define REM_S_64              206
#define SHLO_L_64             207
#define SHLO_R_64             208
#define SHAR_R_64             209
#define AND                   210
#define XOR                   211
#define OR                    212
#define MUL_UPPER_S_S         213
#define MUL_UPPER_U_U         214
#define MUL_UPPER_S_U         215
#define SET_LT_U              216
#define SET_LT_S              217
#define CMOV_IZ               218
#define CMOV_NZ               219
#define ROT_L_64              220
#define ROT_L_32              221
#define ROT_R_64              222
#define ROT_R_32              223
#define AND_INV               224
#define OR_INV                225
#define XNOR                  226
#define MAX_                  227
#define MAX_U                 228
#define MIN_                  229
#define MIN_U                 230

// 128-bit multiplication support 
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

// Signed modulus function 
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_32(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int32_t)value_a >> (value_b & 31));
    
    vm->registers[register_index_d] = result;
    
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_SUB_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a - value_b;
    
    vm->registers[register_index_d] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_MUL_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a * value_b;
    
    vm->registers[register_index_d] = result;
    
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_L_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a << (value_b & 63);
    
    vm->registers[register_index_d] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SHLO_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a >> (value_b & 63);
    
    vm->registers[register_index_d] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_SHAR_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)((int64_t)value_a >> (value_b & 63));
    
    vm->registers[register_index_d] = result;
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_XOR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a ^ value_b;
    
    vm->registers[register_index_d] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_OR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a | value_b;
    
    vm->registers[register_index_d] = result;
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_SET_LT_S(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ((int64_t)value_a < (int64_t)value_b) ? 1 : 0;
    
    vm->registers[register_index_d] = result;
    
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_ROT_R_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = rotate_left_64(value_a, -(int)(value_b & 63));
    
    vm->registers[register_index_d] = result;
    
    
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
    
    vm->pc += 1 + operand_len;
}

void handle_OR_INV(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = value_a | (~value_b);
    
    vm->registers[register_index_d] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_XNOR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = ~(value_a ^ value_b);
    
    vm->registers[register_index_d] = result;
    
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
    
    
    vm->pc += 1 + operand_len;
}

void handle_MAX_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = MAX(value_a, value_b);
    
    vm->registers[register_index_d] = result;
    
    vm->pc += 1 + operand_len;
}

void handle_MIN(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = (uint64_t)MIN((int64_t)value_a, (int64_t)value_b);
    
    vm->registers[register_index_d] = result;
    
    
    vm->pc += 1 + operand_len;
}

void handle_MIN_U(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a, register_index_b, register_index_d;
    parse_three_registers(operands, &register_index_a, &register_index_b, &register_index_d);
    
    uint64_t value_a = vm->registers[register_index_a];
    uint64_t value_b = vm->registers[register_index_b];
    uint64_t result = MIN(value_a, value_b);
    
    vm->registers[register_index_d] = result;
    
    
    vm->pc += 1 + operand_len;
}

// Get opcode name for logging
const char* get_opcode_name(uint8_t opcode) {
    switch (opcode) {
        case TRAP: return "TRAP";
        case FALLTHROUGH: return "FALLTHROUGH";
        case ECALLI: return "ECALLI";
        case LOAD_IMM_64: return "LOAD_IMM_64";
        case STORE_IMM_U8: return "STORE_IMM_U8";
        case STORE_IMM_U16: return "STORE_IMM_U16";
        case STORE_IMM_U32: return "STORE_IMM_U32";
        case STORE_IMM_U64: return "STORE_IMM_U64";
        case JUMP: return "JUMP";
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
        case STORE_IMM_IND_U8: return "STORE_IMM_IND_U8";
        case STORE_IMM_IND_U16: return "STORE_IMM_IND_U16";
        case STORE_IMM_IND_U32: return "STORE_IMM_IND_U32";
        case STORE_IMM_IND_U64: return "STORE_IMM_IND_U64";
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
        case BRANCH_EQ: return "BRANCH_EQ";
        case BRANCH_NE: return "BRANCH_NE";
        case BRANCH_LT_U: return "BRANCH_LT_U";
        case BRANCH_LT_S: return "BRANCH_LT_S";
        case BRANCH_GE_U: return "BRANCH_GE_U";
        case BRANCH_GE_S: return "BRANCH_GE_S";
        case LOAD_IMM_JUMP_IND: return "LOAD_IMM_JUMP_IND";
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
        case MAX_: return "MAX_";
        case MAX_U: return "MAX_U";
        case MIN_: return "MIN_";
        case MIN_U: return "MIN_U";
        default: return "UNKNOWN";
    }
}

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