#include "pvm.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <inttypes.h>


// ===============================
// VM Lifecycle: pvm_create, pvm_set_memory_bounds, pvm_execute, pvm_destroy
// ===============================

pvm_vm_t* pvm_create(uint32_t service_index, 
                        const uint8_t* code, size_t code_len,
                        const uint64_t* initial_regs,
                        const uint8_t* bitmask, size_t bitmask_len,
                        const uint32_t* jump_table, size_t jump_table_len,
                        uint64_t initial_gas) {
    if (!code || code_len == 0) {
        return NULL;
    }

    // Create VM directly - no wrapper!
    VM* vm = (VM*)calloc(1, sizeof(VM));
    if (!vm) {
        return NULL;
    }

    // Initialize VM fields
    vm->service_index = service_index;
    vm->code = (uint8_t*)code;  // Share pointer, don't copy
    vm->code_len = (uint32_t)code_len;
    vm->pc = 0;
    vm->core_index = 2048;
    vm->gas = 0;
    vm->terminated = 0;
    vm->result_code = WORKDIGEST_OK;
    vm->machine_state = HALT;
    vm->host_call = 0;
    vm->host_func_id = 0;
    vm->is_child = 0;

    // Initialize registers (always 13 registers)
    if (initial_regs) {
        for (size_t i = 0; i < REG_SIZE; i++) {
            vm->registers[i] = initial_regs[i];
        }
    } else {
        // Clear all registers if no initial values provided
        for (size_t i = 0; i < REG_SIZE; i++) {
            vm->registers[i] = 0;
        }
    }

    // Initialize memory pointers to NULL (will be set later)
    vm->rw_data = NULL;
    vm->ro_data = NULL;
    vm->output = NULL;
    vm->stack = NULL;
    vm->bitmask = NULL;
    vm->j = NULL;

    // Initialize CGO integration fields for compatibility
    vm->initializing = 1;
    vm->ext_invoke_host_func = NULL;

    // Initialize FFI host callback fields
    vm->host_callback = NULL;

    // Initialize logging and tracing (disabled by default)
    vm->pvm_logging = 0;
    vm->pvm_tracing = 0;

    // Setup bitmask, jump table, and gas internally
    if (bitmask && bitmask_len > 0) {
        vm->bitmask = (uint8_t*)bitmask;
        vm->bitmask_len = (uint32_t)bitmask_len;
    }
    
    if (jump_table && jump_table_len > 0) {
        vm->j = (uint32_t*)jump_table;
        vm->j_size = (uint32_t)jump_table_len;
    }
    
    vm->gas = initial_gas;

    return vm;  // Return VM directly
}

void pvm_set_memory_bounds(pvm_vm_t* vm,
                          uint32_t rw_addr, uint32_t rw_end,
                          uint32_t ro_addr, uint32_t ro_end,
                          uint32_t output_addr, uint32_t output_end,
                          uint32_t stack_addr, uint32_t stack_end) {
    if (!vm) return;
    
    if (vm->pvm_tracing) {
        printf("pvm_set_memory_bounds: Setting memory bounds and allocating buffers\n");
        printf("  RW: 0x%x - 0x%x\n", rw_addr, rw_end);
        printf("  RO: 0x%x - 0x%x\n", ro_addr, ro_end);
        printf("  Output: 0x%x - 0x%x\n", output_addr, output_end);
        printf("  Stack: 0x%x - 0x%x\n", stack_addr, stack_end);
    }
    

    vm->rw_data_address = rw_addr;
    vm->rw_data_address_end = rw_end;
    vm->ro_data_address = ro_addr;
    vm->ro_data_address_end = ro_end;
    vm->output_address = output_addr;
    vm->output_end = output_end;
    vm->stack_address = stack_addr;
    vm->stack_address_end = stack_end;
    
    if (rw_end > rw_addr) {
        uint32_t rw_size = rw_end - rw_addr;
        if (vm->rw_data) {
            free(vm->rw_data);
        }
        vm->max_heapsize = mb_func(rw_size);
        vm->rw_data = (uint8_t*)calloc(vm->max_heapsize, 1);
        if (!vm->rw_data) {
            printf("ERROR: Failed to allocate RW buffer of size %u\n", rw_size);
            return;
        }
    }
    
    if (ro_end > ro_addr) {
        uint32_t ro_size = ro_end - ro_addr;
        if (vm->ro_data) {
            free(vm->ro_data);
        }
        vm->ro_data = (uint8_t*)calloc(ro_size, 1);
        if ( vm->pvm_tracing ) {
            printf("  Allocated RO buffer: %u bytes at %p\n", ro_size, (void*)vm->ro_data);
        }
        
    }
    
    if (output_end > output_addr) {
        uint32_t output_size = output_end - output_addr;
        if (vm->output) {
            free(vm->output);
        }
        vm->output = (uint8_t*)calloc(output_size, 1);
    }
    
    if (stack_end > stack_addr) {
        uint32_t stack_size = stack_end - stack_addr;
        if (vm->stack) {
            free(vm->stack);
        }
        vm->stack = (uint8_t*)calloc(stack_size, 1);

        if (vm->pvm_tracing) {
            printf("  Allocated Stack buffer: %u bytes\n", stack_size);
        }

    }
}

pvm_result_t pvm_execute(pvm_vm_t* vm, uint32_t entry_point, uint32_t is_child) {
    if ( vm && vm->pvm_tracing) {
        printf("pvm_execute: entry_point=0x%x is_child=%d initial_gas=%lld\n", entry_point, is_child, (long long)vm->gas);
        fflush(stdout);
    }
    
     // --- IGNORE ---
    if (!vm) return -1;
    vm->terminated = 0;
    vm->pc = (uint64_t)entry_point;
    vm->is_child = is_child;
    vm->start_pc = entry_point;
    vm->initializing = 0;

    // VM initialized successfully
    if (!vm->code || vm->code_len == 0 || !vm->bitmask || vm->bitmask_len == 0) {
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
        vm->terminated = 1;
        return -1;
    }
    
    // Initialize dispatch table if not already done (thread-safe initialization)
    static pthread_once_t dispatch_init_once = PTHREAD_ONCE_INIT;
    pthread_once(&dispatch_init_once, init_dispatch_table);
    int step = 0;
    uint8_t last_opcode = 0; // Track last executed opcode
    uint8_t second_last_opcode = 0; // Track second-to-last executed opcode
    // Main execution loop with optimized gas checking
    while (!vm->terminated && vm->gas > 0) {

        if (vm->pc >= vm->code_len) {
            if (vm->pvm_tracing) {
                printf("TRACE PC=0x%llx gas=%lld OUT_OF_BOUNDS code_len=%u - panicking\n", 
                       (unsigned long long)vm->pc, (long long)vm->gas, vm->code_len);
                fflush(stdout);
            }
            
            pvm_panic(vm, OOB);
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
            if (vm->bitmask[i]) {
                len_operands = i - vm->pc - 1;
                goto found;
            }
        }
        
        len_operands = limit - vm->pc - 1;
        if (len_operands > 24) {
            len_operands = 24;
        }
        
        // Bound operand length by actual code length
        uint64_t max_operands = vm->code_len - vm->pc - 1;
        if (len_operands > max_operands) {
            len_operands = max_operands;
        }
        
    found:
        ; // Empty statement after label
        
        uint64_t prevpc = vm->pc;
        uint8_t* operands = vm->code + vm->pc + 1;
        vm->gas--;  // Decrement gas once per instruction
        // Advance PC to next instruction, dispatch to handler
        if (dispatch_table[opcode]) {
            second_last_opcode = last_opcode;  // Save previous last_opcode
            last_opcode = opcode;
            dispatch_table[opcode](vm, operands, len_operands);
        } else {
            printf("Unknown opcode: %d\n", opcode);
            pvm_panic(vm, WHAT);
            continue;
        }

        // Show every 1K gas, plus every instruction in high-res windows
        int show_this_gas = (vm->gas % 1000 == 0) || (vm->gas > 972100000 && vm->gas <= 972300000);
        if (vm->pvm_logging && show_this_gas) {
            // Read 16 bytes at 0x2d01e0 to track the memory region containing 0x2d01e8
            uint32_t mem_addr = 0x2d01e0;
            unsigned char mem_bytes[16];
            int err;
            pvm_read_ram_bytes(vm, mem_addr, mem_bytes, 16, &err);

            // Extern declaration for mem_op_buffer from opcode_handlers.c
            extern char mem_op_buffer[256];

            printf("%s %d %llu Gas: %lld Registers:[%llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu, %llu] Mem[0x%x..0x%x]:%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%s\n",
                   get_opcode_name(opcode), step+1, (unsigned long long)prevpc,  (long long)vm->gas,
                    (unsigned long long)vm->registers[0], (unsigned long long)vm->registers[1],
                    (unsigned long long)vm->registers[2], (unsigned long long)vm->registers[3],
                    (unsigned long long)vm->registers[4], (unsigned long long)vm->registers[5],
                    (unsigned long long)vm->registers[6], (unsigned long long)vm->registers[7],
                    (unsigned long long)vm->registers[8], (unsigned long long)vm->registers[9],
                    (unsigned long long)vm->registers[10], (unsigned long long)vm->registers[11],
                    (unsigned long long)vm->registers[12],
                    mem_addr, mem_addr + 15,
                    mem_bytes[0], mem_bytes[1], mem_bytes[2], mem_bytes[3],
                    mem_bytes[4], mem_bytes[5], mem_bytes[6], mem_bytes[7],
                    mem_bytes[8], mem_bytes[9], mem_bytes[10], mem_bytes[11],
                    mem_bytes[12], mem_bytes[13], mem_bytes[14], mem_bytes[15],
                    mem_op_buffer);
            mem_op_buffer[0] = '\0'; // Clear for next instruction
            fflush(stdout);
        }
        step++;
    }
    
    // Handle out-of-gas condition
    if (!vm->terminated && vm->gas <= 0) {
        if ( vm->pvm_tracing ){
            printf("OUT_OF_GAS: VM ran out of gas after %d steps, final gas=%lld, pc=0x%llx\n", 
               step, (long long)vm->gas, (unsigned long long)vm->pc);
            fflush(stdout);
        }
        vm->result_code = WORKDIGEST_OOG;
        vm->machine_state = OOG;
        vm->terminated = 1;
    } else if (!vm->terminated) {
        vm->result_code = WORKDIGEST_OK;
    } else {
        // Safe opcode name retrieval - check bounds first
        const char* opcode_name = "UNKNOWN";
        if (vm->pc < vm->code_len) {
            opcode_name = get_opcode_name(vm->code[vm->pc]);
        }
        // Get last executed opcode names
        const char* last_opcode_name = get_opcode_name(last_opcode);
        const char* second_last_opcode_name = get_opcode_name(second_last_opcode);

    }

    if (vm->pvm_tracing) {
        printf("TRACE VM_EXIT result_code=%d terminated=%d gas=%lld\n", 
               vm->result_code, vm->terminated, (long long)vm->gas);
        fflush(stdout);
    }

    return 0;
}


// VM cleanup
void pvm_destroy(pvm_vm_t* vm) {
    if (!vm) return;
    
    // Free allocated memory buffers
    if (vm->rw_data) {
        free(vm->rw_data);
    }
    if (vm->ro_data) {
        free(vm->ro_data);
    }
    if (vm->output) {
        free(vm->output);
    }
    if (vm->stack) {
        free(vm->stack);
    }
    // DO NOT FREE THESE: they are owned by the caller
    // free(vm->code);
    // free(vm->bitmask);
    // free(vm->j);
    
    free(vm);
}


// ===============================
// VM State Access: registers, result code, gas, machine state
// ===============================
void pvm_set_register(pvm_vm_t* vm, int index, uint64_t value) {
    if (!vm  || index < 0 || index >= REG_SIZE) return;
    vm->registers[index] = value;
}

uint64_t pvm_get_register(pvm_vm_t* vm, int index) {
    if (!vm  || index < 0 || index >= REG_SIZE) return 0;
    return vm->registers[index];
}

const uint64_t* pvm_get_registers(pvm_vm_t* vm) {
    if (!vm ) return NULL;
    return vm->registers;
}

// Result codes
int pvm_get_result_code(pvm_vm_t* vm) {
    if (!vm ) return -1;
    return vm->result_code;
}

int pvm_is_terminated(pvm_vm_t* vm) {
    if (!vm ) return 1;
    return vm->terminated;
}

uint64_t pvm_get_gas(pvm_vm_t* vm) {
    if (!vm ) return 0;
    return vm->gas;
}

uint64_t pvm_get_pc(pvm_vm_t* vm) {
    if (!vm ) return 0;
    return vm->pc;
}

uint8_t pvm_get_machine_state(pvm_vm_t* vm) {
    if (!vm ) return 0;
    return (uint8_t)vm->machine_state;
}

void pvm_set_result_code(pvm_vm_t* vm, uint64_t result_code) {
    if (!vm ) return;
    vm->result_code = (int)result_code;
}
// Heap Management: we should be able to eliminate this method
void pvm_set_heap_pointer(pvm_vm_t* vm, uint32_t pointer) {
    if (!vm ) return;
    vm->current_heap_pointer = pointer;
}

void pvm_expand_heap(pvm_vm_t* vm, uint32_t new_heap_pointer) {
    if (!vm ) return;
    if (new_heap_pointer <= vm->current_heap_pointer) {
        // No need to shrink!
        return;
    }

    // Calculate new heap size - use p_func for proper alignment
    uint32_t current_size = vm->max_heapsize;
    uint32_t required_size = new_heap_pointer - vm->rw_data_address;
    uint32_t new_size = mb_func(required_size); 
    
    if (new_size <= current_size) {
        // Current heap is already large enough, just update pointer
        vm->current_heap_pointer = new_heap_pointer;
        return;
    }
    
    // 1. Create new heap of the correct size (calloc zeros all bytes)
    uint8_t* new_heap = (uint8_t*)calloc(new_size, 1);
    if (!new_heap) {
        // Out of memory - could set machine state to panic
        pvm_panic(vm, OOB);
        return;
    }

    // 2. Copy the old heap into the new heap (expanded portion remains zeroed)
    if (vm->rw_data && current_size > 0) {
        memcpy(new_heap, vm->rw_data, current_size);
    }
    
    // 3. Free the old heap
    if (vm->rw_data) {
        free(vm->rw_data);
    }
    
    // 4. Update VM state with new heap
    vm->rw_data = new_heap;
    vm->max_heapsize = new_size;
    vm->current_heap_pointer = new_heap_pointer;
}

static inline uint32_t ceiling_divide(uint32_t a, uint32_t b) {
    return (a + b - 1) / b;
}
uint32_t p_func(uint32_t x) {
    return Z_P * ceiling_divide(x, Z_P);
}

uint32_t mb_func(uint32_t x) {
    return 1024*1024 * ceiling_divide(x, 1024*1024);
}


static inline void put_uint16_le(uint8_t* buf, uint16_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
}

static inline void put_uint32_le(uint8_t* buf, uint32_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
    buf[2] = (uint8_t)((val >> 16) & 0xFF);
    buf[3] = (uint8_t)((val >> 24) & 0xFF);
}

static inline void put_uint64_le(uint8_t* buf, uint64_t val) {
    buf[0] = (uint8_t)(val & 0xFF);
    buf[1] = (uint8_t)((val >> 8) & 0xFF);
    buf[2] = (uint8_t)((val >> 16) & 0xFF);
    buf[3] = (uint8_t)((val >> 24) & 0xFF);
    buf[4] = (uint8_t)((val >> 32) & 0xFF);
    buf[5] = (uint8_t)((val >> 40) & 0xFF);
    buf[6] = (uint8_t)((val >> 48) & 0xFF);
    buf[7] = (uint8_t)((val >> 56) & 0xFF);
}

static inline uint16_t get_uint16_le(const uint8_t* buf) {
    return (uint16_t)buf[0] | ((uint16_t)buf[1] << 8);
}

static inline uint32_t get_uint32_le(const uint8_t* buf) {
    return (uint32_t)buf[0] | ((uint32_t)buf[1] << 8) | 
           ((uint32_t)buf[2] << 16) | ((uint32_t)buf[3] << 24);
}

static inline uint64_t get_uint64_le(const uint8_t* buf) {
    return (uint64_t)buf[0] | ((uint64_t)buf[1] << 8) | 
           ((uint64_t)buf[2] << 16) | ((uint64_t)buf[3] << 24) |
           ((uint64_t)buf[4] << 32) | ((uint64_t)buf[5] << 40) |
           ((uint64_t)buf[6] << 48) | ((uint64_t)buf[7] << 56);
}

uint64_t pvm_write_ram_bytes_8(pvm_vm_t* vm, uint32_t address, uint8_t data) {
    if (!vm ) return OOB;
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address < heap_end) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset >= vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        vm->rw_data[offset] = data;
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address < vm->output_end) {
        uint32_t offset = address - vm->output_address;
        vm->output[offset] = data;
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address < vm->stack_address_end) {
        uint32_t offset = address - vm->stack_address;
        vm->stack[offset] = data;
        return OK;
    }
    return OOB;
}

uint64_t pvm_write_ram_bytes_16(pvm_vm_t* vm, uint32_t address, uint16_t data) {
    if (!vm ) return OOB;
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 2) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset + 2 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint16_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 2) {
        uint32_t offset = address - vm->output_address;
        put_uint16_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 2) {
        uint32_t offset = address - vm->stack_address;
        put_uint16_le(vm->stack + offset, data);
        return OK;
    }

    return OOB;
}
uint64_t pvm_write_ram_bytes_32(pvm_vm_t* vm, uint32_t address, uint32_t data) {
    if (!vm ) return OOB;
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 4) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset + 4 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint32_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 4) {
        uint32_t offset = address - vm->output_address;
        put_uint32_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 4) {
        uint32_t offset = address - vm->stack_address;
        put_uint32_le(vm->stack + offset, data);
        return OK;
    }
    return OOB;
}

uint64_t pvm_write_ram_bytes_64(pvm_vm_t* vm, uint32_t address, uint64_t data) {
    if (!vm ) return OOB;
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address <= heap_end - 8) {
        uint32_t offset = address - vm->rw_data_address;
        // Safety guard for dynamic growth
        if (offset + 8 > vm->rw_data_address_end - vm->rw_data_address) {
            return OOB;
        }
        put_uint64_le(vm->rw_data + offset, data);
        return OK;
    }

    // Output region (writable)
    if (address >= vm->output_address && address <= vm->output_end - 8) {
        uint32_t offset = address - vm->output_address;
        put_uint64_le(vm->output + offset, data);
        return OK;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address <= vm->stack_address_end - 8) {
        uint32_t offset = address - vm->stack_address;
        put_uint64_le(vm->stack + offset, data);
        return OK;
    }
    return OOB;
}

uint64_t pvm_write_ram_bytes(pvm_vm_t* vm, uint32_t address, const uint8_t* data, uint32_t length) {
    if (!vm  || !data) return OOB;
    if (length == 0) {
        return OK;
    }

    // Check for address + length overflow using 64-bit arithmetic
    uint64_t address_end = (uint64_t)address + (uint64_t)length;
    if (address_end > 0xFFFFFFFFULL) {
        if (vm->pvm_tracing) {
            printf("pvm_write_ram_bytes: OVERFLOW - address=0x%x + length=%d exceeds 32-bit space\n", address, length);
        }
        return OOB;
    }

    uint32_t heap_end = p_func(vm->current_heap_pointer);
    if (vm->pvm_tracing) {
        printf("pvm_write_ram_bytes: address=0x%x, length=%u, address_end=0x%llx\n",
               address, length, (unsigned long long)address_end);
        printf("  RW region: 0x%x - 0x%x (heap_end=0x%x)\n", vm->rw_data_address, vm->rw_data_address_end, heap_end);
        printf("  Output region: 0x%x - 0x%x\n", vm->output_address, vm->output_end);
        printf("  Stack region: 0x%x - 0x%x\n", vm->stack_address, vm->stack_address_end);
        printf("  RO region: 0x%x - 0x%x\n", vm->ro_data_address, vm->ro_data_address_end);
    }

    // RW data / heap region (writable up to heap_end)
    if (address >= vm->rw_data_address && address < heap_end) {
        // Check if the entire write fits within heap_end (no underflow)
        if (address_end <= heap_end) {
            uint32_t offset = address - vm->rw_data_address;
            uint32_t available = vm->rw_data_address_end - vm->rw_data_address;
            // Promote to 64-bit to prevent offset+length overflow
            uint64_t write_extent = (uint64_t)offset + (uint64_t)length;
            if (write_extent <= (uint64_t)available) {
                memcpy(vm->rw_data + offset, data, length);
                return OK;
            }
            if (vm->pvm_tracing) {
                printf("  RW region overflow: offset=%u, length=%u, write_extent=%llu, available=%u -> OOB\n",
                       offset, length, (unsigned long long)write_extent, available);
            }
        } else if (vm->pvm_tracing) {
            printf("  RW region exceeds heap_end: address_end=0x%llx > heap_end=0x%x -> OOB\n",
                   (unsigned long long)address_end, heap_end);
        }
        return OOB;
    }

    // Output region (writable)
    if (address >= vm->output_address && address < vm->output_end) {
        // Check if entire write fits (no underflow)
        if (address_end <= vm->output_end) {
            uint32_t offset = address - vm->output_address;
            memcpy(vm->output + offset, data, length);
            return OK;
        }
        if (vm->pvm_tracing) {
            printf("  Output region exceeds boundary: address_end=0x%llx > end=0x%x -> OOB\n",
                   (unsigned long long)address_end, vm->output_end);
        }
        return OOB;
    }

    // Stack region (writable)
    if (address >= vm->stack_address && address < vm->stack_address_end) {
        // Check if entire write fits (no underflow)
        if (address_end <= vm->stack_address_end) {
            uint32_t offset = address - vm->stack_address;
            memcpy(vm->stack + offset, data, length);
            return OK;
        }
        if (vm->pvm_tracing) {
            printf("  Stack region exceeds boundary: address_end=0x%llx > end=0x%x -> OOB\n",
                   (unsigned long long)address_end, vm->stack_address_end);
        }
        return OOB;
    }

    // RO data region (writes are OOB except during initialization)
    if (address >= vm->ro_data_address && address < vm->ro_data_address_end) {
        if (vm->initializing && address_end <= vm->ro_data_address_end) {
            uint32_t offset = address - vm->ro_data_address;
            memcpy(vm->ro_data + offset, data, length);
            return OK;
        }
        if (vm->pvm_tracing) {
            printf("  RO region write denied: initializing=%d, address_end=0x%llx, end=0x%x -> OOB\n",
                   vm->initializing, (unsigned long long)address_end, vm->ro_data_address_end);
        }
        return OOB;
    }

    if (vm->pvm_tracing) {
        printf("Write failed: address 0x%x not in any memory region\n", address);
    }
    return OOB;
}
uint8_t pvm_read_ram_bytes_8(pvm_vm_t* vm, uint32_t address, int* error_code) {
    if (!vm ) {
        if (error_code) *error_code = (int)OOB;
        return 0;
    }
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (address >= vm->rw_data_address && address < heap_end) {
        uint32_t offset = address - vm->rw_data_address;
        if (offset >= vm->rw_data_address_end - vm->rw_data_address) {
            *error_code = OOB;
            return 0;
        }
        *error_code = OK;
        return vm->rw_data[offset];
    }

    // RO data
    if (address >= vm->ro_data_address && address < vm->ro_data_address_end) {
        uint32_t offset = address - vm->ro_data_address;
        *error_code = OK;
        return vm->ro_data[offset];
    }

    // Output region
    if (address >= vm->output_address && address < vm->output_end) {
        uint32_t offset = address - vm->output_address;
        *error_code = OK;
        return vm->output[offset];
    }

    // Stack
    if (address >= vm->stack_address && address < vm->stack_address_end) {
        uint32_t offset = address - vm->stack_address;
        *error_code = OK;
        return vm->stack[offset];
    }

    *error_code = OOB;
    return 0;
}
uint16_t pvm_read_ram_bytes_16(pvm_vm_t* vm, uint32_t address, int* error_code) {
    if (!vm ) {
        if (error_code) *error_code = (int)OOB;
        return 0;
    }
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (address >= vm->rw_data_address && address <= heap_end - 2) {
        uint32_t offset = address - vm->rw_data_address;
        *error_code = OK;
        return get_uint16_le(vm->rw_data + offset);
    }

    // RO data
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 2) {
        uint32_t offset = address - vm->ro_data_address;
        *error_code = OK;
        return get_uint16_le(vm->ro_data + offset);
    }

    // Output region
    if (address >= vm->output_address && address <= vm->output_end - 2) {
        uint32_t offset = address - vm->output_address;
        *error_code = OK;
        return get_uint16_le(vm->output + offset);
    }

    // Stack
    if (address >= vm->stack_address && address <= vm->stack_address_end - 2) {
        uint32_t offset = address - vm->stack_address;
        *error_code = OK;
        return get_uint16_le(vm->stack + offset);
    }

    *error_code = OOB;
    return 0;
}

uint32_t pvm_read_ram_bytes_32(pvm_vm_t* vm, uint32_t address, int* error_code) {
    if (!vm ) {
        if (error_code) *error_code = (int)OOB;
        return 0;
    }
    uint32_t heap_end = p_func(vm->current_heap_pointer);


    // RW data / heap
    if (address >= vm->rw_data_address && address <= heap_end - 4) {
        uint32_t offset = address - vm->rw_data_address;
        *error_code = OK;
        return get_uint32_le(vm->rw_data + offset);
    }

    // RO data
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 4) {
        uint32_t offset = address - vm->ro_data_address;
        *error_code = OK;
        return get_uint32_le(vm->ro_data + offset);
    }

    // Output region
    if (address >= vm->output_address && address <= vm->output_end - 4) {
        uint32_t offset = address - vm->output_address;
        *error_code = OK;
        return get_uint32_le(vm->output + offset);
    }

    // Stack
    if (address >= vm->stack_address && address <= vm->stack_address_end - 4) {
        uint32_t offset = address - vm->stack_address;
        *error_code = OK;
        return get_uint32_le(vm->stack + offset);
    }

    *error_code = OOB;
    return 0;
}
uint64_t pvm_read_ram_bytes_64(pvm_vm_t* vm, uint32_t address, int* error_code) {
    if (!vm ) {
        if (error_code) *error_code = (int)OOB;
        return 0;
    }
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (address >= vm->rw_data_address && address <= heap_end - 8) {
        uint32_t offset = address - vm->rw_data_address;
        *error_code = OK;
        return get_uint64_le(vm->rw_data + offset);
    }

    // RO data
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - 8) {
        uint32_t offset = address - vm->ro_data_address;
        *error_code = OK;
        return get_uint64_le(vm->ro_data + offset);
    }

    // Output region
    if (address >= vm->output_address && address <= vm->output_end - 8) {
        uint32_t offset = address - vm->output_address;
        *error_code = OK;
//        printf("pvm_read_ram_bytes_64: address=0x%x, data=0x%llx\n", address, (unsigned long long)get_uint64_le(vm->output + offset));
        return get_uint64_le(vm->output + offset);
    }

    // Stack
    if (address >= vm->stack_address && address <= vm->stack_address_end - 8) {
        uint32_t offset = address - vm->stack_address;
        *error_code = OK;
        return get_uint64_le(vm->stack + offset);
    }

    *error_code = OOB;
    return 0;
}

uint64_t pvm_read_ram_bytes(pvm_vm_t* vm, uint32_t address, uint8_t* buffer, uint32_t length, int* error_code) {
    if (!vm  || !buffer) {
        if (error_code) *error_code = (int)OOB;
        return OOB;
    }
    uint32_t heap_end = p_func(vm->current_heap_pointer);

    // RW data / heap
    if (address >= vm->rw_data_address && address <= heap_end - length) {
        uint32_t offset = address - vm->rw_data_address;
        memcpy(buffer, vm->rw_data + offset, length);
        return OK;
    }

    // RO data
    if (address >= vm->ro_data_address && address <= vm->ro_data_address_end - length) {
        uint32_t offset = address - vm->ro_data_address;
        memcpy(buffer, vm->ro_data + offset, length);
        return OK;
    }

    // Output region
    if (address >= vm->output_address && address <= vm->output_end - length) {
        uint32_t offset = address - vm->output_address;
        memcpy(buffer, vm->output + offset, length);
        return OK;
    }

    // Stack
    if (address >= vm->stack_address && address <= vm->stack_address_end - length) {
        uint32_t offset = address - vm->stack_address;
        memcpy(buffer, vm->stack + offset, length);
        return OK;
    }

    return OOB;
}

// ===============================
// Debug and Tracing
// ===============================

void pvm_set_logging(pvm_vm_t* vm, int enable) {
    if (!vm ) return;
    vm->pvm_logging = enable ? 1 : 0;
}

void pvm_set_tracing(pvm_vm_t* vm, int enable) {
    if (!vm ) return;
    vm->pvm_tracing = enable ? 1 : 0;
}

// VM panic function
void pvm_panic(pvm_vm_t* vm, uint64_t err_code) {
    vm->result_code = WORKDIGEST_PANIC;
    vm->machine_state = PANIC;
    vm->terminated = 1;
    vm->fault_address = (uint32_t)err_code;
}

// ===============================
// Host Function Callbacks
// ===============================

// Maps internal VM callback to FFI callback
void internal_host_callback_wrapper(VM* internal_vm, int host_func_id) {
    if (!internal_vm || !internal_vm->ext_invoke_host_func) return;
    
    // Find the FFI VM that owns this internal VM
    // Note: This is a simplified approach. In practice, you'd want a proper registry.
    pvm_vm_t* ext_vm = (pvm_vm_t*)internal_vm->ext_vm; // Hack: using ext_vm to store ffi_vm pointer

    if (ext_vm && ext_vm->host_callback) {
        pvm_host_callback_t callback = (pvm_host_callback_t)ext_vm->host_callback;
        pvm_host_result_t result = callback(ext_vm, host_func_id);

        // Handle callback result
        switch (result) {
            case PVM_HOST_TERMINATE:
                internal_vm->terminated = 1;
                internal_vm->result_code = WORKDIGEST_OK;
                break;
            case PVM_HOST_ERROR:
                internal_vm->machine_state = PANIC;
                internal_vm->result_code = WORKDIGEST_PANIC;
                internal_vm->terminated = 1;
                break;
            case PVM_HOST_CONTINUE:
            default:
                // Continue execution
                break;
        }
    }
}

void pvm_set_host_callback(pvm_vm_t* vm, pvm_host_callback_t callback) {
    if (!vm ) return;
    vm->host_callback = (void*)callback;
    if (callback) {
        // Set up the internal callback wrapper
        vm->ext_invoke_host_func = internal_host_callback_wrapper;
        // Store FFI VM pointer in ext_vm for callback lookup
        vm->ext_vm = (uint8_t*)vm;
    } else {
        vm->ext_invoke_host_func = NULL;
        vm->ext_vm = NULL;
    }
}
