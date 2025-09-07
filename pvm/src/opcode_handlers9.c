#include "pvm.h"
#include <inttypes.h>


// Utility functions
extern uint32_t p_func(uint32_t x);

// Bit manipulation functions (C equivalents of Go's math/bits)
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

// A.5.9. Instructions with Arguments of Two Registers

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
        vm->registers[register_index_d] = (uint64_t)pvm_get_current_heap_pointer(vm);
        vm->pc += 1 + operand_len;
        return;
    }

    uint64_t result = (uint64_t)pvm_get_current_heap_pointer(vm);
    uint32_t next_page_boundary = p_func(pvm_get_current_heap_pointer(vm));
    uint64_t new_heap_pointer = (uint64_t)pvm_get_current_heap_pointer(vm) + value_a;

    if (new_heap_pointer > (uint64_t)next_page_boundary) {
        if ( vm->pvm_tracing ) {
            printf("TRACE SBRK: new_heap_pointer=0x%llx next_page_boundary=0x%x\n",
                   (unsigned long long)new_heap_pointer, next_page_boundary);
            fflush(stdout);
        }
        uint32_t final_boundary = p_func((uint32_t)new_heap_pointer);
        uint32_t idx_start = next_page_boundary / Z_P;
        uint32_t idx_end = final_boundary / Z_P;
        uint32_t page_count = idx_end - idx_start;
        pvm_allocate_pages(vm, idx_start, page_count);
    }
    pvm_set_current_heap_pointer(vm, (uint32_t)new_heap_pointer);
    
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


