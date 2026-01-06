#ifndef JIT_COMPILER_H
#define JIT_COMPILER_H

#include <stdint.h>
#include <stdbool.h>
#include "basic_block.h"
#include "x86_codegen.h"

#ifdef __cplusplus
extern "C" {
#endif

// Forward declarations
typedef struct compiler compiler_t;

// PC mapping entry (matches Go's InstMapPVMToX86 / InstMapX86ToPVM)
typedef struct {
    uint32_t pvm_pc;
    uint32_t x86_offset;
} pc_map_entry_t;

// Main JIT compiler structure (simplified - matches Go's X86Compiler)
struct compiler {
    // Input bytecode
    uint8_t* bytecode;
    uint32_t bytecode_size;

    // Jump table and bitmask (matches Go's J and bitmask)
    uint32_t* jump_table;
    uint32_t jump_table_size;
    uint8_t* bitmask;
    uint32_t bitmask_size;

    // Generated x86 code (matches Go's x86Code)
    uint8_t* x86_code;
    uint32_t x86_code_size;
    uint32_t x86_code_capacity;

    // Dynamic jump table (matches Go's djumpAddr and JumpTableMap)
    uintptr_t djump_addr;
    uint64_t* jump_table_map;      // Maps PVM PC to x86 offset
    uint32_t jump_table_map_size;

    uint8_t* djump_table_code;     // Generated dynamic jump table blob
    uint32_t djump_table_size;

    uint8_t* exit_code;            // Cached exit sequence emitted inside djump table
    uint32_t exit_code_size;

    // PC mappings (matches Go's InstMapPVMToX86 and InstMapX86ToPVM)
    pc_map_entry_t* pvm_to_x86_map;
    uint32_t pvm_to_x86_count;
    uint32_t pvm_to_x86_capacity;

    pc_map_entry_t* x86_to_pvm_map;
    uint32_t x86_to_pvm_count;
    uint32_t x86_to_pvm_capacity;

    // Internal compilation state
    void* basic_blocks_map;  // Hash map of basic blocks (opaque)
    x86_codegen_t codegen;   // Code generation context

    // Configuration flags (matches Go defaults)
    bool is_charging_gas;
    bool is_pc_counting;
    bool is_block_counting;
    bool is_child;

    // Compiled basic blocks (for later jump patching)
    basic_block_t** basic_blocks;
    uint32_t basic_block_count;
    uint32_t basic_block_capacity;

    uint32_t entry_patch_offset;
    uint32_t entry_start_pc;
};

// ============================================================================
// Core Compiler Interface (matches Go's Compiler interface)
// ============================================================================

/**
 * Create a new JIT compiler instance
 * @param bytecode PVM bytecode to compile
 * @param bytecode_size Size of bytecode in bytes
 * @return New compiler instance or NULL on failure
 */
compiler_t* compiler_create(const uint8_t* bytecode, uint32_t bytecode_size);

/**
 * Destroy compiler instance and free all resources
 * @param compiler Compiler instance to destroy
 */
void compiler_destroy(compiler_t* compiler);

/**
 * Set jump table (matches Go's SetJumpTable)
 * @param compiler Compiler instance
 * @param jump_table Array of PVM jump targets
 * @param size Number of entries in jump table
 * @return 0 on success, negative on error
 */
int compiler_set_jump_table(compiler_t* compiler,
                                const uint32_t* jump_table,
                                uint32_t size);

/**
 * Set bitmask (matches Go's SetBitMask)
 * @param compiler Compiler instance
 * @param bitmask Instruction bitmask array
 * @param size Size of bitmask in bytes
 * @return 0 on success, negative on error
 */
int compiler_set_bitmask(compiler_t* compiler,
                             const uint8_t* bitmask,
                             uint32_t size);

/**
 * Set whether this compiler instance is used for a child VM.
 * Matches Go's Compiler.SetIsChild; currently used for parity knobs.
 * @param compiler Compiler instance
 * @param is_child Non-zero to enable child mode
 * @return 0 on success, negative on error
 */
int compiler_set_is_child(compiler_t* compiler, int is_child);

/**
 * Compile PVM bytecode to x86 machine code (matches Go's CompileX86Code)
 *
 * This is the main compilation function that:
 * 1. Parses PVM bytecode into basic blocks
 * 2. Translates each instruction to x86 code
 * 3. Generates dynamic jump table
 * 4. Patches all jump targets
 * 5. Returns compiled code and PC mappings
 *
 * @param compiler Compiler instance
 * @param start_pc Starting PVM program counter
 * @param out_x86_code Pointer to receive generated x86 code (caller must free)
 * @param out_code_size Pointer to receive size of generated code
 * @param out_djump_addr Pointer to receive djump table address offset
 * @return 0 on success, negative on error
 */
int compiler_compile(compiler_t* compiler,
                        uint64_t start_pc,
                        uint8_t** out_x86_code,
                        uint32_t* out_code_size,
                        uintptr_t* out_djump_addr);

/**
 * Get PVM to x86 PC mapping (for InstMapPVMToX86)
 * @param compiler Compiler instance
 * @param out_count Pointer to receive number of entries
 * @return Array of pc_map_entry_t (do not free - owned by compiler)
 */
const pc_map_entry_t* compiler_get_pvm_to_x86_map(compiler_t* compiler,
                                                       uint32_t* out_count);

/**
 * Get x86 to PVM PC mapping (for InstMapX86ToPVM)
 * @param compiler Compiler instance
 * @param out_count Pointer to receive number of entries
 * @return Array of pc_map_entry_t (do not free - owned by compiler)
 */
const pc_map_entry_t* compiler_get_x86_to_pvm_map(compiler_t* compiler,
                                                       uint32_t* out_count);

// ============================================================================
// Internal Functions (used by compiler.c implementation)
// ============================================================================

// Basic block management
int jit_compile_basic_blocks(compiler_t* compiler, uint32_t start_pc);
int jit_translate_basic_block(compiler_t* compiler, basic_block_t* block);
int jit_append_block_code(compiler_t* compiler, basic_block_t* block);

// Jump handling
int jit_generate_djump_table(compiler_t* compiler);
int jit_finalize_jumps(compiler_t* compiler);

// PC mapping management
int jit_add_pc_mapping(compiler_t* compiler, uint32_t pvm_pc, uint32_t x86_offset);

// Utility
uint32_t jit_skip_instruction(const uint8_t* bytecode,
                              const uint8_t* bitmask,
                              uint32_t bitmask_size,
                              uint32_t pc);

// Debug
void compiler_print_state(const compiler_t* compiler);

#ifdef __cplusplus
}
#endif

#endif // JIT_COMPILER_H
