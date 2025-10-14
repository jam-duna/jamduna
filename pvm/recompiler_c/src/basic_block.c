#include "basic_block.h"
#include "pvm_const.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// Debug macros
#ifdef DEBUG
#define DEBUG_JIT(...) fprintf(stderr, __VA_ARGS__)
#else
#define DEBUG_JIT(...) ((void)0)
#endif

#define INITIAL_INSTRUCTION_CAPACITY 16
#define INITIAL_X86_CODE_CAPACITY 1024

// Internal helper functions
static int resize_instructions(basic_block_t* block, size_t new_capacity);
static int resize_x86_code(basic_block_t* block, size_t new_capacity);
static pvm_pc_map_entry_t* find_pc_mapping(const basic_block_t* block, uint32_t pvm_pc);

// Basic block creation and destruction
basic_block_t* basic_block_create(uint64_t x86_pc) {
    basic_block_t* block = calloc(1, sizeof(basic_block_t));
    if (!block) {
        return NULL;
    }
    
    // Initialize instruction array
    block->instructions = malloc(INITIAL_INSTRUCTION_CAPACITY * sizeof(instruction_t));
    if (!block->instructions) {
        free(block);
        return NULL;
    }
    block->instruction_count = 0;
    block->instruction_capacity = INITIAL_INSTRUCTION_CAPACITY;
    
    // Initialize x86 code buffer
    block->x86_code = malloc(INITIAL_X86_CODE_CAPACITY);
    if (!block->x86_code) {
        free(block->instructions);
        free(block);
        return NULL;
    }
    block->x86_code_size = 0;
    block->x86_code_capacity = INITIAL_X86_CODE_CAPACITY;
    
    // Initialize other fields
    block->gas_usage = 0;
    block->x86_pc = x86_pc;
    block->pvm_pc_to_x86_map = NULL;
    block->jump_type = FALLTHROUGH_JUMP;
    block->indirect_jump_offset = 0;
    block->indirect_source_register = 0;
    block->true_pc = 0;
    block->pvm_next_pc = 0;
    block->last_instruction_offset = 0;
    
    return block;
}

void basic_block_destroy(basic_block_t* block) {
    if (!block) return;
    
    // Free all instructions and their args
    for (size_t i = 0; i < block->instruction_count; i++) {
        instruction_destroy(&block->instructions[i]);
    }
    free(block->instructions);
    
    // Free x86 code
    free(block->x86_code);
    
    // Free PC mapping
    basic_block_clear_pc_mapping(block);
    
    free(block);
}

// Instruction management
int basic_block_add_instruction(basic_block_t* block, uint8_t opcode, 
                               const uint8_t* args, size_t args_size, 
                               int step, uint64_t pc) {
    if (!block) return -1;
    
    // Resize if needed
    if (block->instruction_count >= block->instruction_capacity) {
        if (resize_instructions(block, block->instruction_capacity * 2) != 0) {
            return -1;
        }
    }
    
    // Create instruction
    instruction_t* inst = &block->instructions[block->instruction_count];
    inst->opcode = opcode;
    inst->step = step;
    inst->pc = pc;
    inst->args_size = args_size;
    inst->gas_usage = 0;
    
    // Copy arguments if provided
    if (args_size > 0 && args) {
        inst->args = malloc(args_size);
        if (!inst->args) {
            return -1;
        }
        memcpy(inst->args, args, args_size);
    } else {
        inst->args = NULL;
    }
    
    block->instruction_count++;
    
    return 0;
}

instruction_t* basic_block_get_instruction(const basic_block_t* block, size_t index) {
    if (!block || index >= block->instruction_count) {
        return NULL;
    }
    return &block->instructions[index];
}

size_t basic_block_get_instruction_count(const basic_block_t* block) {
    return block ? block->instruction_count : 0;
}

// x86 code management
int basic_block_append_x86_code(basic_block_t* block, const uint8_t* code, size_t size) {
    if (!block || !code || size == 0) return -1;
    
    // Resize if needed
    if (block->x86_code_size + size > block->x86_code_capacity) {
        size_t new_capacity = block->x86_code_capacity;
        while (new_capacity < block->x86_code_size + size) {
            new_capacity *= 2;
        }
        if (resize_x86_code(block, new_capacity) != 0) {
            return -1;
        }
    }
    
    // Copy code
    memcpy(block->x86_code + block->x86_code_size, code, size);
    block->x86_code_size += size;
    
    return 0;
}

int basic_block_reserve_x86_space(basic_block_t* block, size_t additional_size) {
    if (!block) return -1;
    
    size_t needed_capacity = block->x86_code_size + additional_size;
    if (needed_capacity > block->x86_code_capacity) {
        size_t new_capacity = block->x86_code_capacity;
        while (new_capacity < needed_capacity) {
            new_capacity *= 2;
        }
        return resize_x86_code(block, new_capacity);
    }
    
    return 0;
}

uint8_t* basic_block_get_x86_code(const basic_block_t* block) {
    return block ? block->x86_code : NULL;
}

size_t basic_block_get_x86_size(const basic_block_t* block) {
    return block ? block->x86_code_size : 0;
}

// PC mapping
int basic_block_add_pc_mapping(basic_block_t* block, uint32_t pvm_pc, int x86_offset) {
    if (!block) return -1;
    
    pvm_pc_map_entry_t* entry = malloc(sizeof(pvm_pc_map_entry_t));
    if (!entry) return -1;
    
    entry->pvm_pc = pvm_pc;
    entry->x86_offset = x86_offset;
    entry->next = block->pvm_pc_to_x86_map;
    block->pvm_pc_to_x86_map = entry;
    
    return 0;
}

int basic_block_get_x86_offset(const basic_block_t* block, uint32_t pvm_pc) {
    if (!block) return -1;
    
    pvm_pc_map_entry_t* entry = find_pc_mapping(block, pvm_pc);
    return entry ? entry->x86_offset : -1;
}

void basic_block_clear_pc_mapping(basic_block_t* block) {
    if (!block) return;
    
    pvm_pc_map_entry_t* current = block->pvm_pc_to_x86_map;
    while (current) {
        pvm_pc_map_entry_t* next = current->next;
        free(current);
        current = next;
    }
    block->pvm_pc_to_x86_map = NULL;
}

// Jump analysis
void basic_block_set_jump_type(basic_block_t* block, int jump_type) {
    if (block) {
        block->jump_type = jump_type;
    }
}

int basic_block_get_jump_type(const basic_block_t* block) {
    return block ? block->jump_type : TERMINATED;
}

void basic_block_set_indirect_jump(basic_block_t* block, uint64_t offset, int src_register) {
    if (block) {
        block->indirect_jump_offset = offset;
        block->indirect_source_register = src_register;
        block->jump_type = INDIRECT_JUMP;
    }
}

void basic_block_set_branch_targets(basic_block_t* block, uint64_t true_pc, uint64_t next_pc) {
    if (block) {
        block->true_pc = true_pc;
        block->pvm_next_pc = next_pc;
    }
}

// Utility functions
char* basic_block_to_string(const basic_block_t* block) {
    if (!block) return NULL;
    
    // Calculate needed buffer size (rough estimate)
    size_t buffer_size = 1024 + block->instruction_count * 64;
    char* buffer = malloc(buffer_size);
    if (!buffer) return NULL;
    
    char* ptr = buffer;
    size_t remaining = buffer_size;
    
    // Add basic block info
    int written = snprintf(ptr, remaining, "BasicBlock[x86_pc=0x%lx, instructions=%zu, gas=%ld]\n", 
                          block->x86_pc, block->instruction_count, block->gas_usage);
    if (written < 0 || (size_t)written >= remaining) {
        free(buffer);
        return NULL;
    }
    ptr += written;
    remaining -= written;
    
    // Add instructions
    for (size_t i = 0; i < block->instruction_count; i++) {
        const instruction_t* inst = &block->instructions[i];
        written = snprintf(ptr, remaining, "  [%zu] %s (step=%d, pc=0x%lx)\n", 
                          i, opcode_to_string(inst->opcode), inst->step, inst->pc);
        if (written < 0 || (size_t)written >= remaining) {
            free(buffer);
            return NULL;
        }
        ptr += written;
        remaining -= written;
    }
    
    // Add jump info
    const char* jump_type_str = "UNKNOWN";
    switch (block->jump_type) {
        case TRAP_JUMP: jump_type_str = "TRAP_JUMP"; break;
        case DIRECT_JUMP: jump_type_str = "DIRECT_JUMP"; break;
        case INDIRECT_JUMP: jump_type_str = "INDIRECT_JUMP"; break;
        case CONDITIONAL: jump_type_str = "CONDITIONAL"; break;
        case FALLTHROUGH_JUMP: jump_type_str = "FALLTHROUGH_JUMP"; break;
        case TERMINATED: jump_type_str = "TERMINATED"; break;
    }
    
    written = snprintf(ptr, remaining, "Jump: %s, true_pc=0x%lx, next_pc=0x%lx\n", 
                      jump_type_str, block->true_pc, block->pvm_next_pc);
    if (written < 0 || (size_t)written >= remaining) {
        free(buffer);
        return NULL;
    }
    
    return buffer;
}

void basic_block_print(const basic_block_t* block) {
    char* str = basic_block_to_string(block);
    if (str) {
        DEBUG_JIT("%s", str);
        free(str);
    } else {
        DEBUG_JIT("BasicBlock[NULL or error]");
    }
}

bool basic_block_is_terminator(const basic_block_t* block) {
    if (!block || block->instruction_count == 0) {
        return false;
    }
    
    // Check the last instruction
    const instruction_t* last_inst = &block->instructions[block->instruction_count - 1];
    return is_basic_block_terminator_opcode(last_inst->opcode);
}

// Instruction utility functions
instruction_t* instruction_create(uint8_t opcode, const uint8_t* args, 
                                size_t args_size, int step, uint64_t pc) {
    instruction_t* inst = malloc(sizeof(instruction_t));
    if (!inst) return NULL;
    
    inst->opcode = opcode;
    inst->step = step;
    inst->pc = pc;
    inst->args_size = args_size;
    
    if (args_size > 0 && args) {
        inst->args = malloc(args_size);
        if (!inst->args) {
            free(inst);
            return NULL;
        }
        memcpy(inst->args, args, args_size);
    } else {
        inst->args = NULL;
    }
    
    return inst;
}

void instruction_destroy(instruction_t* instruction) {
    if (instruction) {
        free(instruction->args);
        instruction->args = NULL;
        instruction->args_size = 0;
    }
}

char* instruction_to_string(const instruction_t* instruction) {
    if (!instruction) return NULL;
    
    char* buffer = malloc(128);
    if (!buffer) return NULL;
    
    snprintf(buffer, 128, "%s (step=%d, pc=0x%lx, args_size=%zu)", 
             opcode_to_string(instruction->opcode), 
             instruction->step, instruction->pc, instruction->args_size);
    
    return buffer;
}

// Check if opcode terminates a basic block
bool is_basic_block_terminator_opcode(uint8_t opcode) {
    return is_basic_block_terminator(opcode); // Defined in pvm_const.c
}

// Internal helper functions
static int resize_instructions(basic_block_t* block, size_t new_capacity) {
    instruction_t* new_instructions = realloc(block->instructions, 
                                            new_capacity * sizeof(instruction_t));
    if (!new_instructions) {
        return -1;
    }
    
    block->instructions = new_instructions;
    block->instruction_capacity = new_capacity;
    return 0;
}

static int resize_x86_code(basic_block_t* block, size_t new_capacity) {
    uint8_t* new_code = realloc(block->x86_code, new_capacity);
    if (!new_code) {
        return -1;
    }
    
    block->x86_code = new_code;
    block->x86_code_capacity = new_capacity;
    return 0;
}

static pvm_pc_map_entry_t* find_pc_mapping(const basic_block_t* block, uint32_t pvm_pc) {
    pvm_pc_map_entry_t* current = block->pvm_pc_to_x86_map;
    while (current) {
        if (current->pvm_pc == pvm_pc) {
            return current;
        }
        current = current->next;
    }
    return NULL;
}
