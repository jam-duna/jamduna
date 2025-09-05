#include "pvm.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <inttypes.h>

int pvm_logging = 0;

// Get opcode name for logging
const char* get_opcode_name(uint8_t opcode) {
    switch (opcode) {
        case 0: return "TRAP";
        case 1: return "FALLTHROUGH";
        case 10: return "ECALLI";
        case 20: return "LOAD_IMM_64";
        case 30: return "STORE_IMM_U8";
        case 31: return "STORE_IMM_U16";
        case 32: return "STORE_IMM_U32";
        case 33: return "STORE_IMM_U64";
        case 40: return "JUMP";
        case 50: return "JUMP_IND";
        case 51: return "LOAD_IMM";
        case 52: return "LOAD_U8";
        case 53: return "LOAD_I8";
        case 54: return "LOAD_U16";
        case 55: return "LOAD_I16";
        case 56: return "LOAD_U32";
        case 57: return "LOAD_I32";
        case 58: return "LOAD_U64";
        case 59: return "STORE_U8";
        case 60: return "STORE_U16";
        case 61: return "STORE_U32";
        case 62: return "STORE_U64";
        case 70: return "STORE_IMM_IND_U8";
        case 71: return "STORE_IMM_IND_U16";
        case 72: return "STORE_IMM_IND_U32";
        case 73: return "STORE_IMM_IND_U64";
        case 80: return "LOAD_IMM_JUMP";
        case 81: return "BRANCH_EQ_IMM";
        case 82: return "BRANCH_NE_IMM";
        case 83: return "BRANCH_LT_U_IMM";
        case 84: return "BRANCH_LE_U_IMM";
        case 85: return "BRANCH_GE_U_IMM";
        case 86: return "BRANCH_GT_U_IMM";
        case 87: return "BRANCH_LT_S_IMM";
        case 88: return "BRANCH_LE_S_IMM";
        case 89: return "BRANCH_GE_S_IMM";
        case 90: return "BRANCH_GT_S_IMM";
        case 100: return "MOVE_REG";
        case 101: return "SBRK";
        case 102: return "COUNT_SET_BITS_64";
        case 103: return "COUNT_SET_BITS_32";
        case 104: return "LEADING_ZERO_BITS_64";
        case 105: return "LEADING_ZERO_BITS_32";
        case 106: return "TRAILING_ZERO_BITS_64";
        case 107: return "TRAILING_ZERO_BITS_32";
        case 108: return "SIGN_EXTEND_8";
        case 109: return "SIGN_EXTEND_16";
        case 110: return "ZERO_EXTEND_16";
        case 111: return "REVERSE_BYTES";
        case 120: return "STORE_IND_U8";
        case 121: return "STORE_IND_U16";
        case 122: return "STORE_IND_U32";
        case 123: return "STORE_IND_U64";
        case 124: return "LOAD_IND_U8";
        case 125: return "LOAD_IND_I8";
        case 126: return "LOAD_IND_U16";
        case 127: return "LOAD_IND_I16";
        case 128: return "LOAD_IND_U32";
        case 129: return "LOAD_IND_I32";
        case 130: return "LOAD_IND_U64";
        case 131: return "ADD_IMM_32";
        case 132: return "AND_IMM";
        case 133: return "XOR_IMM";
        case 134: return "OR_IMM";
        case 135: return "MUL_IMM_32";
        case 136: return "SET_LT_U_IMM";
        case 137: return "SET_LT_S_IMM";
        case 138: return "SHLO_L_IMM_32";
        case 139: return "SHLO_R_IMM_32";
        case 140: return "SHAR_R_IMM_32";
        case 141: return "NEG_ADD_IMM_32";
        case 142: return "SET_GT_U_IMM";
        case 143: return "SET_GT_S_IMM";
        case 144: return "SHLO_L_IMM_ALT_32";
        case 145: return "SHLO_R_IMM_ALT_32";
        case 146: return "SHAR_R_IMM_ALT_32";
        case 147: return "CMOV_IZ_IMM";
        case 148: return "CMOV_NZ_IMM";
        case 149: return "ADD_IMM_64";
        case 150: return "MUL_IMM_64";
        case 151: return "SHLO_L_IMM_64";
        case 152: return "SHLO_R_IMM_64";
        case 153: return "SHAR_R_IMM_64";
        case 154: return "NEG_ADD_IMM_64";
        case 155: return "SHLO_L_IMM_ALT_64";
        case 156: return "SHLO_R_IMM_ALT_64";
        case 157: return "SHAR_R_IMM_ALT_64";
        case 158: return "ROT_R_64_IMM";
        case 159: return "ROT_R_64_IMM_ALT";
        case 160: return "ROT_R_32_IMM";
        case 161: return "ROT_R_32_IMM_ALT";
        case 170: return "BRANCH_EQ";
        case 171: return "BRANCH_NE";
        case 172: return "BRANCH_LT_U";
        case 173: return "BRANCH_LT_S";
        case 174: return "BRANCH_GE_U";
        case 175: return "BRANCH_GE_S";
        case 180: return "LOAD_IMM_JUMP_IND";
        case 190: return "ADD_32";
        case 191: return "SUB_32";
        case 192: return "MUL_32";
        case 193: return "DIV_U_32";
        case 194: return "DIV_S_32";
        case 195: return "REM_U_32";
        case 196: return "REM_S_32";
        case 197: return "SHLO_L_32";
        case 198: return "SHLO_R_32";
        case 199: return "SHAR_R_32";
        case 200: return "ADD_64";
        case 201: return "SUB_64";
        case 202: return "MUL_64";
        case 203: return "DIV_U_64";
        case 204: return "DIV_S_64";
        case 205: return "REM_U_64";
        case 206: return "REM_S_64";
        case 207: return "SHLO_L_64";
        case 208: return "SHLO_R_64";
        case 209: return "SHAR_R_64";
        case 210: return "AND";
        case 211: return "XOR";
        case 212: return "OR";
        case 213: return "MUL_UPPER_S_S";
        case 214: return "MUL_UPPER_U_U";
        case 215: return "MUL_UPPER_S_U";
        case 216: return "SET_LT_U";
        case 217: return "SET_LT_S";
        case 218: return "CMOV_IZ";
        case 219: return "CMOV_NZ";
        case 220: return "ROT_L_64";
        case 221: return "ROT_L_32";
        case 222: return "ROT_R_64";
        case 223: return "ROT_R_32";
        case 224: return "AND_INV";
        case 225: return "OR_INV";
        case 226: return "XNOR";
        case 227: return "MAX_";
        case 228: return "MAX_U";
        case 229: return "MIN_";
        case 230: return "MIN_U";
        default: return "UNKNOWN";
    }
}

// Utility functions
static inline uint32_t ceiling_divide(uint32_t a, uint32_t b) {
    return (a + b - 1) / b;
}

uint32_t p_func(uint32_t x) {
    return Z_P * ceiling_divide(x, Z_P);
}

static inline uint32_t z_func(uint32_t x) {
    return Z_Z * ceiling_divide(x, Z_Z);
}

// VM panic function
void vm_panic(VM* vm, uint64_t err_code) {
    vm->result_code = WORKDIGEST_PANIC;
    vm->machine_state = PANIC;
    vm->terminated = 1;
    vm->fault_address = (uint32_t)err_code;
}

// Dynamic jump function
void vm_djump(VM* vm, uint64_t a) {
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

// Branch function
void vm_branch(VM* vm, uint64_t vx, int condition) {
    if (condition) {
        vm->pc = vx;
    } else {
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
        vm->terminated = 1;
    }
}

// Standard program initialization
void vm_standard_program_initialization(VM* vm, uint8_t* argument_data, size_t arg_len) {
    if (arg_len == 0) {
        argument_data = (uint8_t*)"\0";
        arg_len = 1;
    }

    uint32_t z_w = z_func(vm->w_size + vm->z_val * Z_P);

    // Copy o_byte to ro_data
    if (vm->o_byte && vm->o_size > 0) {
        memcpy(vm->ro_data, vm->o_byte, MIN(vm->o_size, vm->ro_data_address_end - vm->ro_data_address));
    }

    // Copy w_byte to rw_data
    if (vm->w_byte && vm->w_size > 0) {
        memcpy(vm->rw_data, vm->w_byte, MIN(vm->w_size, vm->rw_data_address_end - vm->rw_data_address));
    }

    // Copy argument to output region
    uint32_t arg_addr = 0xFFFFFFFF - Z_Z - Z_I + 1;
    if (arg_addr >= vm->output_address && arg_addr + arg_len <= vm->output_end) {
        uint32_t offset = arg_addr - vm->output_address;
        memcpy(vm->output + offset, argument_data, arg_len);
    }

    // Initialize registers
    vm->registers[0] = (uint64_t)(0xFFFFFFFF - (1 << 16) + 1);
    vm->registers[1] = (uint64_t)(0xFFFFFFFF - 2 * Z_Z - Z_I + 1);
    vm->registers[7] = (uint64_t)arg_addr;
    vm->registers[8] = (uint64_t)arg_len;
}


// VM initialization
VM* vm_new(uint32_t service_index, uint8_t* code, size_t code_len, 
           uint64_t* initial_regs, size_t num_regs, uint64_t initial_pc) {
    if (!code || code_len == 0) {
        return NULL;
    }

    VM* vm = (VM*)calloc(1, sizeof(VM));
    if (!vm) return NULL;
    vm->service_index = service_index;
    vm->code = (uint8_t*)malloc(code_len);
    if (!vm->code) {
        free(vm);
        return NULL;
    }
    
    memcpy(vm->code, code, code_len);
    vm->code_len = code_len;
    vm->pc = initial_pc;
    vm->core_index = 2048;
    vm->gas = 10000;

    // Initialize registers
    for (size_t i = 0; i < MIN(num_regs, REG_SIZE); i++) {
        vm->registers[i] = initial_regs[i];
    }
        // printf("VM created with service index: %u\n", service_index);


    return vm;
}

int vm_charge_gas(VM* vm, int host_func_id) {
    
    const int LOG_HOST_FN = 100;
    if (host_func_id == LOG_HOST_FN) {
        return 0;
    }
    return 10;
}

// VM cleanup
void vm_free(VM* vm) {
    if (!vm) return;
    
    free(vm->code);
    free(vm->bitmask);
    free(vm->j);
    free(vm->stack);
    free(vm->rw_data);
    free(vm->ro_data);
    free(vm->output);
    free(vm->o_byte);
    free(vm->w_byte);
    free(vm);
}

// ===============================
// CGO Integration Functions
// ===============================

VM* pvm_create(uint32_t service_index, 
               uint8_t* code, size_t code_len,
               uint64_t* initial_regs, size_t num_regs,
               uint64_t initial_pc) {
    VM* vm = (VM*)calloc(1, sizeof(VM));
    if (!vm) return NULL;

    vm->service_index = service_index;
    vm->code = code;  // Share pointer, don't copy
    vm->code_len = code_len;
    vm->pc = initial_pc;
    vm->core_index = 2048;
    vm->gas = 0;
    vm->terminated = 0;
    vm->result_code = WORKDIGEST_OK;
    vm->machine_state = HALT;

    // Initialize registers
    for (size_t i = 0; i < MIN(num_regs, REG_SIZE); i++) {
        vm->registers[i] = initial_regs[i];
    }
    
    // Initialize CGO integration fields
    vm->go_rw_data = NULL;
    vm->go_ro_data = NULL;
    vm->go_output = NULL;
    vm->go_stack = NULL;
    vm->go_invoke_host_func = NULL;
    vm->sync_registers_to_go = 0;

    return vm;
}

void pvm_destroy(VM* vm) {
    if (!vm) return;
    free(vm);
}

void pvm_set_bitmask(VM* vm, uint8_t* bitmask, size_t bitmask_len) {
    if (!vm) return;
    vm->bitmask = bitmask;  // Share pointer, don't copy
    vm->bitmask_len = (uint32_t)bitmask_len;
}

void pvm_set_jump_table(VM* vm, uint32_t* j_table, size_t j_size) {
    if (!vm) return;
    vm->j = j_table;  // Share pointer, don't copy
    vm->j_size = (uint32_t)j_size;
}

void pvm_set_memory_regions(VM* vm,
                           uint8_t* rw_data, uint32_t rw_size,
                           uint8_t* ro_data, uint32_t ro_size,
                           uint8_t* output, uint32_t output_size,
                           uint8_t* stack, uint32_t stack_size) {
    if (!vm) return;
    
    // Set Go memory pointers (zero-copy sharing)
    vm->go_rw_data = rw_data;
    vm->go_ro_data = ro_data;
    vm->go_output = output;
    vm->go_stack = stack;
    
    // Also update legacy pointers for compatibility
    vm->rw_data = rw_data;
    vm->ro_data = ro_data;
    vm->output = output;
    vm->stack = stack;
}

void pvm_set_memory_bounds(VM* vm,
                          uint32_t rw_addr, uint32_t rw_end,
                          uint32_t ro_addr, uint32_t ro_end,
                          uint32_t output_addr, uint32_t output_end,
                          uint32_t stack_addr, uint32_t stack_end) {
    if (!vm) return;
    
    vm->rw_data_address = rw_addr;
    vm->rw_data_address_end = rw_end;
    vm->ro_data_address = ro_addr;
    vm->ro_data_address_end = ro_end;
    vm->output_address = output_addr;
    vm->output_end = output_end;
    vm->stack_address = stack_addr;
    vm->stack_address_end = stack_end;
}

uint64_t* pvm_get_registers_ptr(VM* vm) {
    if (!vm) return NULL;
    return vm->registers;
}

void pvm_set_register(VM* vm, int reg_idx, uint64_t value) {
    if (!vm || reg_idx < 0 || reg_idx >= REG_SIZE) return;
    vm->registers[reg_idx] = value;
}

uint64_t pvm_get_register(VM* vm, int reg_idx) {
    if (!vm || reg_idx < 0 || reg_idx >= REG_SIZE) return 0;
    return vm->registers[reg_idx];
}

int pvm_execute(VM* vm, int entry_point, int is_child) {
    if (!vm) return -1;
    
    vm->terminated = 0;
    vm->pc = (uint64_t)entry_point;
    vm->is_child = is_child;
    
    
    // VM initialized successfully
    
    if (!vm->code || vm->code_len == 0 || !vm->bitmask || vm->bitmask_len == 0) {
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
        vm->terminated = 1;
        return -1;
    }
    
    // Initialize dispatch table if not already done (static initialization)
    static int dispatch_initialized = 0;
    if (!dispatch_initialized) {
        init_dispatch_table();
        dispatch_initialized = 1;
    }
    int step = 0;
    // Main execution loop with optimized gas checking
    while (!vm->terminated && vm->gas > 0) {
        if (vm->pc >= vm->code_len) {
                // printf("TRACE PC=0x%llx gas=%lld OUT_OF_BOUNDS code_len=%u - panicking\n", 
                //        (unsigned long long)vm->pc, (long long)vm->gas, vm->code_len);
                // fflush(stdout);
            vm_panic(vm, OOB);
            continue;
        }
        
        uint8_t opcode = vm->code[vm->pc];
        
        
        // Find operand length using bitmask
        uint64_t len_operands = 0;
        uint64_t limit = vm->pc + 25;
        if (limit > vm->bitmask_len) {
            limit = vm->bitmask_len;
        }
        
        for (uint64_t i = vm->pc + 1; i < limit; i++) {
            if (vm->bitmask[i] == 1) {
                len_operands = i - vm->pc - 1;
                goto found;
            }
        }
        
        len_operands = limit - vm->pc - 1;
        if (len_operands > 24) {
            len_operands = 24;
        }
        
    found:
        ; // Empty statement after label
        uint8_t* operands = vm->code + vm->pc + 1;
        vm->gas--;  // Decrement gas once per instruction
        step++;
        // Dispatch to handler
        if (dispatch_table[opcode]) {
            dispatch_table[opcode](vm, operands, len_operands);
        } else {
            // printf("Unknown opcode: %d\n", opcode);
            vm_panic(vm, WHAT);
            continue;
        }
       // Performance: comment out per-instruction logging (most critical bottleneck)
        if (pvm_logging) {
            printf("CGo: %s %d %llu Gas: %lld Registers: [%llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu]\n", 
                   get_opcode_name(opcode), step, (unsigned long long)vm->pc, (long long)vm->gas,
                   (unsigned long long)vm->registers[0], (unsigned long long)vm->registers[1], 
                   (unsigned long long)vm->registers[2], (unsigned long long)vm->registers[3],
                   (unsigned long long)vm->registers[4], (unsigned long long)vm->registers[5], 
                   (unsigned long long)vm->registers[6], (unsigned long long)vm->registers[7],
                   (unsigned long long)vm->registers[8], (unsigned long long)vm->registers[9], 
                   (unsigned long long)vm->registers[10], (unsigned long long)vm->registers[11],
                   (unsigned long long)vm->registers[12]);
            fflush(stdout);
        }
      

        // Gas check is now handled in loop condition - no need for redundant check
    }
    
    // Handle out-of-gas condition
    if (!vm->terminated && vm->gas <= 0) {
        vm->result_code = WORKDIGEST_OOG;
        vm->machine_state = OOG;
        vm->terminated = 1;
    } else if (!vm->terminated) {
        vm->result_code = WORKDIGEST_OK;
    }
    
    // Performance: comment out VM_EXIT trace
    // printf("TRACE VM_EXIT result_code=%d terminated=%d gas=%lld\n", 
    //        vm->result_code, vm->terminated, (long long)vm->gas);
    // fflush(stdout);
    
    return 0;
}

void pvm_set_host_callback(VM* vm, 
                          void (*callback)(VM* vm, int host_func_id)) {
    if (!vm) return;
    vm->go_invoke_host_func = callback;
}