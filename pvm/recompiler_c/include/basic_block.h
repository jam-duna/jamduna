#ifndef BASIC_BLOCK_H
#define BASIC_BLOCK_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

// Forward declarations
typedef struct instruction instruction_t;
typedef struct basic_block basic_block_t;
typedef struct pvm_pc_map_entry pvm_pc_map_entry_t;

// Instruction structure
typedef struct instruction {
    uint8_t opcode;           // PVM opcode
    uint8_t* args;            // Instruction arguments (dynamically allocated)
    size_t args_size;         // Size of args array
    int step;                 // Step number
    uint64_t pc;              // Program counter
    int64_t gas_usage;        // Gas consumed when executing this instruction
} instruction_t;

// PC mapping entry for PVM PC to x86 offset mapping
typedef struct pvm_pc_map_entry {
    uint32_t pvm_pc;          // PVM program counter
    int x86_offset;           // Offset in x86 code
    struct pvm_pc_map_entry* next; // Next entry (linked list)
} pvm_pc_map_entry_t;

// Basic block structure
typedef struct basic_block {
    // Instructions in this block
    instruction_t* instructions;      // Dynamic array of instructions
    size_t instruction_count;         // Current number of instructions
    size_t instruction_capacity;      // Allocated capacity for instructions
    int64_t gas_usage;               // Gas consumed by this block
    
    // x86 code generation
    uint64_t x86_pc;                 // x86 program counter (offset in generated code)
    uint8_t* x86_code;               // Generated x86 machine code
    size_t x86_code_size;            // Size of generated x86 code
    size_t x86_code_capacity;        // Allocated capacity for x86 code
    
    // PC mapping: PVM PC to x86 code offset
    pvm_pc_map_entry_t* pvm_pc_to_x86_map; // Hash map/linked list for PC mapping
    
    // Jump analysis
    int jump_type;                   // Type of jump (from jump_type_t enum)
    uint64_t indirect_jump_offset;   // Offset for indirect jumps
    int indirect_source_register;    // Source register for indirect jumps
    
    // Control flow
    uint64_t true_pc;                // Target PC for true branch
    uint64_t pvm_next_pc;           // Next PVM PC after this block (false case)
    
    // Code generation metadata
    int last_instruction_offset;     // Offset of last instruction in x86 code
} basic_block_t;

// Basic block creation and destruction
basic_block_t* basic_block_create(uint64_t x86_pc);
void basic_block_destroy(basic_block_t* block);

// Instruction management
int basic_block_add_instruction(basic_block_t* block, uint8_t opcode, 
                               const uint8_t* args, size_t args_size, 
                               int step, uint64_t pc);
instruction_t* basic_block_get_instruction(const basic_block_t* block, size_t index);
size_t basic_block_get_instruction_count(const basic_block_t* block);

// x86 code management
int basic_block_append_x86_code(basic_block_t* block, const uint8_t* code, size_t size);
int basic_block_reserve_x86_space(basic_block_t* block, size_t additional_size);
uint8_t* basic_block_get_x86_code(const basic_block_t* block);
size_t basic_block_get_x86_size(const basic_block_t* block);

// PC mapping
int basic_block_add_pc_mapping(basic_block_t* block, uint32_t pvm_pc, int x86_offset);
int basic_block_get_x86_offset(const basic_block_t* block, uint32_t pvm_pc);
void basic_block_clear_pc_mapping(basic_block_t* block);

// Jump analysis
void basic_block_set_jump_type(basic_block_t* block, int jump_type);
int basic_block_get_jump_type(const basic_block_t* block);
void basic_block_set_indirect_jump(basic_block_t* block, uint64_t offset, int src_register);
void basic_block_set_branch_targets(basic_block_t* block, uint64_t true_pc, uint64_t next_pc);

// Utility functions
char* basic_block_to_string(const basic_block_t* block);
void basic_block_print(const basic_block_t* block);
bool basic_block_is_terminator(const basic_block_t* block);

// Instruction utility functions
instruction_t* instruction_create(uint8_t opcode, const uint8_t* args, 
                                size_t args_size, int step, uint64_t pc);
void instruction_destroy(instruction_t* instruction);
char* instruction_to_string(const instruction_t* instruction);

// Check if opcode terminates a basic block (from Go function IsBasicBlockInstruction)
bool is_basic_block_terminator_opcode(uint8_t opcode);

#ifdef __cplusplus
}
#endif

#endif // BASIC_BLOCK_H
