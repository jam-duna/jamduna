#include "pvm.h"


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
    
    // Charge gas before host call
    vm->gas -= (int64_t)pvm_charge_gas(vm, vm->host_func_id);
    
    if (vm->pvm_tracing) {
 //       printf("Host call: function %d\n", vm->host_func_id);
    }
    
    // Trampoline to Go's InvokeHostFunction with error handling
    if (vm->go_invoke_host_func) {
        // Set sync flag to ensure register state is properly maintained
        vm->sync_registers_to_go = 1;
        
        // Call Go host function - this will handle register sync automatically
        vm->go_invoke_host_func(vm, vm->host_func_id);

        // Check if the host call resulted in termination
        if (vm->terminated) {
            return;
        }
        
        // Reset sync flag after successful call
        vm->sync_registers_to_go = 0;
    } else {
        // No callback registered - this is an error condition
        if (vm->pvm_tracing) {
            printf("Error: No host callback registered for function %d\n", vm->host_func_id);
    }
        pvm_panic(vm, WHAT);
        return;
    }
    
    vm->host_call = 0;
}

void handle_LOAD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a = MIN(12, (int)(operands[0] % 16));
    int lx = 8;
    uint8_t* slice = operands + 1;
    uint64_t vx;
    
    // LOAD_IMM_64 always reads exactly 8 bytes after register, ignoring operand_len
    // This matches how Go handles fixed-length instructions
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

void handle_JUMP(VM* vm, uint8_t* operands, size_t operand_len) {
    int64_t vx;
    int lx = MIN(4, (int)operand_len);
    
    if (lx == 0) {
        vx = 0;
    } else {
        // JUMP instruction reads directly from code like Go implementation
        uint8_t* slice = vm->code + vm->pc + 1;  // Read from code, not operands
        uint64_t decoded = decode_operand_slice(slice, lx);
        
        // Sign extend the decoded value based on lx
        uint32_t shift = 64 - 8 * lx;
        vx = (int64_t)((int64_t)(decoded << shift) >> shift);
    }
    pvm_branch(vm, (uint64_t)((int64_t)vm->pc + vx), 1);
}

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
    
    // Read register B value BEFORE updating register A (like Go implementation)
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

