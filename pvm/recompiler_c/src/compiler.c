/*
 * PVM JIT Compiler - Simplified Implementation (Compiler Only)
 *
 * This is a compiler-only implementation that translates PVM bytecode to x86 machine code.
 * VM management, execution, and memory management are handled by the Go runtime.
 */

#include "compiler.h"
#include "basic_block.h"
#include "x86_codegen.h"
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

// Hash map implementation (from previous working code)
#define JIT_HASH_MAP_SIZE 65536

typedef struct hash_map_entry {
    uint32_t key;
    void* value;
    struct hash_map_entry* next;
} hash_map_entry_t;

typedef struct hash_map {
    hash_map_entry_t* buckets[JIT_HASH_MAP_SIZE];
    uint32_t size;
} hash_map_t;

// Hash function for 32-bit keys (from previous working code)
static uint32_t hash_uint32(uint32_t key) {
    key ^= key >> 16;
    key *= 0x85ebca6b;
    key ^= key >> 13;
    key *= 0xc2b2ae35;
    key ^= key >> 16;
    return key % JIT_HASH_MAP_SIZE;
}

static hash_map_t* hash_map_create(void) {
    return (hash_map_t*)calloc(1, sizeof(hash_map_t));
}

static void* hash_map_lookup(hash_map_t* map, uint32_t key) {
    if (!map) return NULL;
    uint32_t index = hash_uint32(key);
    hash_map_entry_t* entry = map->buckets[index];
    while (entry) {
        if (entry->key == key) {
            return entry->value;
        }
        entry = entry->next;
    }
    return NULL;
}

static int hash_map_insert(hash_map_t* map, uint32_t key, void* value) {
    if (!map) return -1;

    uint32_t index = hash_uint32(key);

    // Check if key already exists
    hash_map_entry_t* entry = map->buckets[index];
    while (entry) {
        if (entry->key == key) {
            entry->value = value; // Update existing
            return 0;
        }
        entry = entry->next;
    }

    // Insert new entry
    hash_map_entry_t* new_entry = (hash_map_entry_t*)malloc(sizeof(hash_map_entry_t));
    if (!new_entry) return -1;

    new_entry->key = key;
    new_entry->value = value;
    new_entry->next = map->buckets[index];
    map->buckets[index] = new_entry;
    map->size++;

    return 0;
}

static void hash_map_destroy(hash_map_t* map) {
    if (!map) return;

    for (uint32_t i = 0; i < JIT_HASH_MAP_SIZE; i++) {
        hash_map_entry_t* entry = map->buckets[i];
        while (entry) {
            hash_map_entry_t* next = entry->next;
            free(entry);
            entry = next;
        }
    }
    free(map);
}

static uint64_t jit_decode_little_endian(const uint8_t* data, size_t len) {
    uint64_t value = 0;
    for (size_t i = 0; i < len; i++) {
        value |= ((uint64_t)data[i]) << (8 * i);
    }
    return value;
}

static uint64_t jit_x_encode(uint64_t x, uint32_t n) {
    if (n == 0 || n > 8) {
        return 0;
    }
    uint32_t shift = 8 * n - 1;
    uint64_t q = x >> shift;
    if (n == 8) {
        return x;
    }
    uint64_t mask = (uint64_t)1 << (8 * n);
    mask -= 1;
    uint64_t factor = ~mask;
    return x + q * factor;
}

static int64_t jit_z_encode(uint64_t a, uint32_t n) {
    if (n == 0 || n > 8) {
        return 0;
    }
    uint32_t shift = 64 - 8 * n;
    return (int64_t)(a << shift) >> shift;
}

static uint64_t jit_min_u64(uint64_t a, uint64_t b) {
    return a < b ? a : b;
}

static int64_t jit_extract_one_offset(const uint8_t* operands, uint32_t operand_len) {
    uint32_t lx = (uint32_t)jit_min_u64(4, operand_len);
    if (lx == 0) {
        lx = 1;
    }
    uint64_t raw = jit_decode_little_endian(operands, lx <= operand_len ? lx : operand_len);
    return jit_z_encode(raw, lx);
}

static int64_t jit_extract_offset_reg_imm_offset(const uint8_t* operands, uint32_t operand_len) {
    if (!operands || operand_len == 0) {
        return 0;
    }
    uint8_t first = operands[0];
    uint32_t lx = ((uint32_t)first / 16U) % 8U;
    if (lx > 4) {
        lx = 4;
    }

    uint32_t remaining = operand_len > 1 ? operand_len - 1 : 0;
    if (remaining < lx) {
        lx = remaining;
    }

    uint32_t ly = 0;
    if (remaining > lx) {
        ly = remaining - lx;
        if (ly > 4) {
            ly = 4;
        }
    }
    if (ly == 0) {
        ly = 1;
    }

    const uint8_t* offset_ptr = operands + 1 + lx;
    if ((uint32_t)(offset_ptr - operands) > operand_len) {
        offset_ptr = NULL;
    }

    uint64_t raw = 0;
    for (uint32_t i = 0; i < ly; i++) {
        uint8_t byte = 0;
        if (offset_ptr && (1 + lx + i) < operand_len) {
            byte = offset_ptr[i];
        }
        raw |= ((uint64_t)byte) << (8 * i);
    }
    return jit_z_encode(raw, ly);
}

static int64_t jit_extract_offset_two_regs(const uint8_t* operands, uint32_t operand_len) {
    uint32_t lx = operand_len > 1 ? operand_len - 1 : 0;
    if (lx > 4) {
        lx = 4;
    }
    if (lx == 0) {
        lx = 1;
    }

    uint64_t raw = 0;
    for (uint32_t i = 0; i < lx; i++) {
        uint8_t byte = 0;
        if (1 + i < operand_len) {
            byte = operands[1 + i];
        }
        raw |= ((uint64_t)byte) << (8 * i);
    }
    return jit_z_encode(raw, lx);
}

static uint64_t jit_add_offset(uint64_t pc, int64_t offset) {
    int64_t signed_pc = (int64_t)pc;
    int64_t target = signed_pc + offset;
    if (target < 0) {
        return 0;
    }
    return (uint64_t)target;
}

static void jit_set_jump_metadata(basic_block_t* block, uint8_t opcode, const uint8_t* operands, uint32_t operand_len, uint64_t pc) {
    if (!block) {
        return;
    }

    switch (opcode) {
        case JUMP: {
            int64_t offset = jit_extract_one_offset(operands, operand_len);
            basic_block_set_jump_type(block, DIRECT_JUMP);
            block->true_pc = jit_add_offset(pc, offset);
            break;
        }
        case LOAD_IMM_JUMP: {
            int64_t offset = jit_extract_offset_reg_imm_offset(operands, operand_len);
            basic_block_set_jump_type(block, DIRECT_JUMP);
            block->true_pc = jit_add_offset(pc, offset);
            break;
        }
        case JUMP_IND:
        case LOAD_IMM_JUMP_IND:
            basic_block_set_jump_type(block, INDIRECT_JUMP);
            break;
        case TRAP:
            basic_block_set_jump_type(block, TRAP_JUMP);
            break;
        default:
            if (opcode >= BRANCH_EQ_IMM && opcode <= BRANCH_GT_S_IMM) {
                int64_t offset = jit_extract_offset_reg_imm_offset(operands, operand_len);
                basic_block_set_jump_type(block, CONDITIONAL);
                block->true_pc = jit_add_offset(pc, offset);
            } else if (opcode >= BRANCH_EQ && opcode <= BRANCH_GE_S) {
                int64_t offset = jit_extract_offset_two_regs(operands, operand_len);
                basic_block_set_jump_type(block, CONDITIONAL);
                block->true_pc = jit_add_offset(pc, offset);
            } else {
                basic_block_set_jump_type(block, FALLTHROUGH_JUMP);
            }
            break;
    }
}

static int jit_store_basic_block(compiler_t* compiler, basic_block_t* block) {
    if (!compiler || !block) {
        return -1;
    }
    if (compiler->basic_block_count >= compiler->basic_block_capacity) {
        uint32_t new_capacity = compiler->basic_block_capacity ? compiler->basic_block_capacity * 2 : 16;
        basic_block_t** new_blocks = (basic_block_t**)realloc(compiler->basic_blocks, new_capacity * sizeof(basic_block_t*));
        if (!new_blocks) {
            return -1;
        }
        compiler->basic_blocks = new_blocks;
        compiler->basic_block_capacity = new_capacity;
    }
    compiler->basic_blocks[compiler->basic_block_count++] = block;
    return 0;
}

static void jit_free_basic_blocks(compiler_t* compiler) {
    if (!compiler || !compiler->basic_blocks) {
        return;
    }
    for (uint32_t i = 0; i < compiler->basic_block_count; i++) {
        basic_block_destroy(compiler->basic_blocks[i]);
    }
    free(compiler->basic_blocks);
    compiler->basic_blocks = NULL;
    compiler->basic_block_count = 0;
    compiler->basic_block_capacity = 0;
}

static bool jit_lookup_x86_offset(const compiler_t* compiler, uint32_t pvm_pc, uint32_t* out_offset) {
    if (!compiler || !out_offset) {
        return false;
    }
    for (uint32_t i = 0; i < compiler->pvm_to_x86_count; i++) {
        if (compiler->pvm_to_x86_map[i].pvm_pc == pvm_pc) {
            *out_offset = compiler->pvm_to_x86_map[i].x86_offset;
            return true;
        }
    }
    return false;
}

// Helper function to skip instruction and get operand length (matches Go Skip())
uint32_t jit_skip_instruction(const uint8_t* bytecode, const uint8_t* bitmask,
                              uint32_t bitmask_size, uint32_t pc) {
    (void)bytecode; // Unused for now, kept for signature compatibility

    if (!bitmask || pc >= bitmask_size) {
        return 0;
    }

    uint64_t pc64 = (uint64_t)pc;
    uint64_t end = pc64 + 25;
    uint64_t mask_size = (uint64_t)bitmask_size;
    if (end > mask_size) {
        end = mask_size;
    }

    for (uint64_t i = pc64 + 1; i < end; i++) {
        if (bitmask[i] > 0) {
            return (uint32_t)(i - pc64 - 1);
        }
    }

    if (end < pc64 + 25) {
        return (uint32_t)(end - pc64 - 1);
    }

    return 24;
}

// Core compiler functions (stub implementation)
compiler_t* compiler_create(const uint8_t* bytecode, uint32_t bytecode_size) {
    if (!bytecode || bytecode_size == 0) return NULL;

    compiler_t* compiler = (compiler_t*)calloc(1, sizeof(compiler_t));
    if (!compiler) return NULL;

    compiler->bytecode = (uint8_t*)malloc(bytecode_size);
    if (!compiler->bytecode) {
        free(compiler);
        return NULL;
    }
    memcpy(compiler->bytecode, bytecode, bytecode_size);
    compiler->bytecode_size = bytecode_size;

    compiler->basic_blocks_map = hash_map_create();
    compiler->is_charging_gas = true;
    return compiler;
}

void compiler_destroy(compiler_t* compiler) {
    if (!compiler) return;
    jit_free_basic_blocks(compiler);
    free(compiler->bytecode);
    free(compiler->jump_table);
    free(compiler->bitmask);
    free(compiler->x86_code);
    free(compiler->jump_table_map);
    free(compiler->djump_table_code);
    free(compiler->exit_code);
    free(compiler->pvm_to_x86_map);
    free(compiler->x86_to_pvm_map);
    hash_map_destroy((hash_map_t*)compiler->basic_blocks_map);
    free(compiler);
}

int compiler_set_jump_table(compiler_t* compiler, const uint32_t* jump_table, uint32_t size) {
    if (!compiler || !jump_table || size == 0) return -1;
    compiler->jump_table = (uint32_t*)malloc(size * sizeof(uint32_t));
    if (!compiler->jump_table) return -1;
    memcpy(compiler->jump_table, jump_table, size * sizeof(uint32_t));
    compiler->jump_table_size = size;
    return 0;
}

int compiler_set_bitmask(compiler_t* compiler, const uint8_t* bitmask, uint32_t size) {
    if (!compiler || !bitmask || size == 0) return -1;
    compiler->bitmask = (uint8_t*)malloc(size);
    if (!compiler->bitmask) return -1;
    memcpy(compiler->bitmask, bitmask, size);
    compiler->bitmask_size = size;
    return 0;
}

// Forward declarations (from .old implementation)
static basic_block_t* jit_parse_basic_block(compiler_t* compiler, uint32_t start_pc);
static int jit_translate_basic_block_impl(compiler_t* compiler, basic_block_t* block);
static int jit_append_block(compiler_t* compiler, basic_block_t* block);

// Decode function (from .old implementation)
static uint32_t decode_e_l(const uint8_t* operands, uint32_t operand_len) {
    if (!operands || operand_len == 0) {
        return 0;
    }
    uint32_t value = 0;
    uint32_t limit = operand_len < 4 ? operand_len : 4;
    for (uint32_t i = 0; i < limit; i++) {
        value |= (uint32_t)operands[i] << (8 * i);
    }
    return value;
}

// Mirror Go's chargeGas helper for host calls
static uint64_t compiler_charge_gas(int host_fn) {
    switch (host_fn) {
        case HOST_LOG:
            return 0;
        default:
            return 10;
    }
}

int compiler_compile(compiler_t* compiler, uint64_t start_pc,
                        uint8_t** out_x86_code, uint32_t* out_code_size,
                        uintptr_t* out_djump_addr) {
    if (!compiler || !out_x86_code || !out_code_size || !out_djump_addr) {
        DEBUG_JIT("compiler_compile: NULL parameter\n");
        return -1;
    }

    // Check if start_pc is within bytecode bounds
    if (start_pc >= compiler->bytecode_size) {
        DEBUG_JIT("ERROR - Start PC %lu out of bounds (bytecode_size=%u)\n", start_pc, compiler->bytecode_size);
        return -2;
    }

    DEBUG_JIT("Starting compilation at PVM PC %lu (matches Go Compile())\n", start_pc);

    // Initialize PVM instruction translators
    pvm_translators_init();
    DEBUG_JIT("PVM instruction translators initialized\n");

    // Initialize compiler state
    compiler->pvm_to_x86_count = 0;
    compiler->x86_to_pvm_count = 0;
    compiler->x86_code_size = 0;
    compiler->entry_start_pc = (uint32_t)start_pc;
    
    // Initialize dynamic PC mapping arrays
    // Scale capacity based on bytecode size to avoid reallocation issues
    uint32_t mapping_capacity = compiler->bytecode_size > 100000 ? 32768 : 16442;
    compiler->pvm_to_x86_capacity = mapping_capacity;
    compiler->x86_to_pvm_capacity = mapping_capacity;

    compiler->pvm_to_x86_map = (pc_map_entry_t*)calloc(mapping_capacity, sizeof(pc_map_entry_t));
    compiler->x86_to_pvm_map = (pc_map_entry_t*)calloc(mapping_capacity, sizeof(pc_map_entry_t));

    if (compiler->djump_table_code) {
        free(compiler->djump_table_code);
        compiler->djump_table_code = NULL;
    }
    compiler->djump_table_size = 0;
    compiler->djump_addr = 0;

    if (compiler->exit_code) {
        free(compiler->exit_code);
        compiler->exit_code = NULL;
    }
    compiler->exit_code_size = 0;
    
    if (!compiler->pvm_to_x86_map || !compiler->x86_to_pvm_map) {
        DEBUG_JIT("Failed to allocate PC mapping arrays\n");
        return -1;  
    }

    // 1. Create start block (matches Go: NewBasicBlock(vm.x86PC))
    DEBUG_JIT("Creating start block...\n");
    x86_codegen_t start_gen;
    if (x86_codegen_init(&start_gen, 1024) != 0) {
        DEBUG_JIT("Failed to initialize start code generator\n");
        return -1;
    }

    // Generate start code matching Go's initStartCode()
    // Placeholder addresses (will be patched at execution time)
    const uint64_t REG_DUMP_MEM_PATCH = 0x8888888888888888ULL;
    const uint32_t ENTRY_PATCH = 0x99999999;
    const uint32_t REG_MEM_SIZE = 0x100000; // dumpSize = 1MB
    const int BASE_REG_INDEX = 14; // R12
    const int NUM_PVM_REGS = 13; // Number of PVM registers

    // 1. MOV R12, 0x8888888888888888 (placeholder for regDumpMem address)
    x86_encode_mov_reg64_imm64(&start_gen, X86_R12, REG_DUMP_MEM_PATCH);

    // 2. Load all 13 registers from regDumpMem: MOV regN, [R12 + N*8]
    // Use pvm_reg_to_x86 mapping to skip RSP/RBP like Go does
    extern const x86_reg_t pvm_reg_to_x86[14];
    for (int i = 0; i < NUM_PVM_REGS; i++) {
        int32_t offset = i * 8;
        x86_reg_t x86_reg = pvm_reg_to_x86[i];
        x86_encode_mov_reg64_mem(&start_gen, x86_reg, X86_R12, offset);
    }

    // 3. Adjust R12 from regDumpAddr to realMemAddr: ADD R12, 0x100000
    // Manual encoding: 49 81 C4 00 00 10 00 = ADD R12, 0x100000
    uint8_t add_r12[] = {
        0x49, 0x81, 0xC4,  // ADD R12, imm32
        (uint8_t)(REG_MEM_SIZE & 0xFF),
        (uint8_t)((REG_MEM_SIZE >> 8) & 0xFF),
        (uint8_t)((REG_MEM_SIZE >> 16) & 0xFF),
        (uint8_t)((REG_MEM_SIZE >> 24) & 0xFF)
    };
    x86_codegen_ensure_capacity(&start_gen, 7);
    memcpy(start_gen.buffer + start_gen.offset, add_r12, 7);
    start_gen.offset += 7;

    // 4. JMP rel32 with placeholder 0x99999999 (to entry point)
    // Manual encoding: E9 99 99 99 99 = JMP rel32
    x86_codegen_ensure_capacity(&start_gen, 5);
    start_gen.buffer[start_gen.offset++] = 0xE9; // JMP rel32
    start_gen.buffer[start_gen.offset++] = (uint8_t)(ENTRY_PATCH & 0xFF);
    start_gen.buffer[start_gen.offset++] = (uint8_t)((ENTRY_PATCH >> 8) & 0xFF);
    start_gen.buffer[start_gen.offset++] = (uint8_t)((ENTRY_PATCH >> 16) & 0xFF);
    start_gen.buffer[start_gen.offset++] = (uint8_t)((ENTRY_PATCH >> 24) & 0xFF);
    compiler->entry_patch_offset = start_gen.offset - 4;

    DEBUG_JIT("Generated %u bytes of start code\n", start_gen.offset);
    
    // Initialize main x86 code buffer
    uint32_t initial_capacity = 500000; // Large enough for complex programs
    compiler->x86_code = (uint8_t*)malloc(initial_capacity);
    if (!compiler->x86_code) {
        DEBUG_JIT("Failed to allocate x86 code buffer\n");
        x86_codegen_cleanup(&start_gen);
        return -1;
    }
    
    // Copy start code to main buffer
    memcpy(compiler->x86_code, start_gen.buffer, start_gen.offset);
    compiler->x86_code_size = start_gen.offset;
    compiler->x86_code_capacity = initial_capacity;
    
    x86_codegen_cleanup(&start_gen);
    
    // 2. Parse and translate basic blocks (matches Go's Compile() main loop)
    DEBUG_JIT("Starting basic block parsing loop - start_pc=%lu, bytecode_size=%u\n", start_pc, compiler->bytecode_size);
    
    uint32_t pc = (uint32_t)start_pc;
    int blocks_compiled = 1; // Count start block
    uint32_t total_instructions = 0;

    while (pc < compiler->bytecode_size) {
        // Parse basic block (matches Go: block := vm.translateBasicBlock(pc))
        basic_block_t* block = jit_parse_basic_block(compiler, pc);
        if (!block) {
            DEBUG_JIT("Failed to parse basic block at PC %u\n", pc);
            break;
        }
        
        // Translate PVM instructions to x86
        if (jit_translate_basic_block_impl(compiler, block) != 0) {
            DEBUG_JIT("Failed to translate basic block at PC %u\n", pc);
            basic_block_destroy(block);
            break;
        }
        
        // Append block to main code buffer (matches Go: vm.appendBlock(block))
        if (jit_append_block(compiler, block) != 0) {
            DEBUG_JIT("Failed to append block at PC %u\n", pc);
            basic_block_destroy(block);
            break;
        }

        if (jit_store_basic_block(compiler, block) != 0) {
            DEBUG_JIT("Failed to store basic block metadata at PC %u\n", pc);
            basic_block_destroy(block);
            break;
        }
        
        total_instructions += basic_block_get_instruction_count(block);
        blocks_compiled++;

        // Move to next block (using pvm_next_pc from basic_block structure)
        pc = (uint32_t)block->pvm_next_pc;

        // NOTE: Unlike old implementation, we do NOT break on TRAP
        // Go's Compile() continues through entire bytecode to build complete code map
        // TRAP instructions are just normal terminators, not compilation stoppers
    }

    DEBUG_JIT("Generated %u bytes of x86 code for %u instructions in %d blocks\n",
              compiler->x86_code_size, total_instructions, blocks_compiled);

    // Append fallthrough trap exactly like Go's emitTrap to guard against direct fallthrough.
    if (compiler->x86_code_size + 2 > compiler->x86_code_capacity) {
        uint32_t new_capacity = (compiler->x86_code_size + 2) * 2;
        uint8_t* new_buffer = (uint8_t*)realloc(compiler->x86_code, new_capacity);
        if (!new_buffer) {
            DEBUG_JIT("Failed to resize x86_code buffer for fallthrough trap\n");
            return -1;
        }
        compiler->x86_code = new_buffer;
        compiler->x86_code_capacity = new_capacity;
    }
    compiler->x86_code[compiler->x86_code_size++] = 0x0F;
    compiler->x86_code[compiler->x86_code_size++] = X86_OP_TRAP;

    // 3. Generate exit code (matches Go's exitCode in initStartCode)
    DEBUG_JIT("Generating exit code...\n");
    x86_codegen_t exit_gen;
    if (x86_codegen_init(&exit_gen, 256) != 0) {
        DEBUG_JIT("Failed to initialize exit code generator\n");
        return -1;
    }

    // 1. SUB R12, 0x100000 (point R12 back to regDumpMem)
    uint8_t sub_r12[] = {
        0x49, 0x81, 0xEC,  // SUB R12, imm32
        0x00, 0x00, 0x10, 0x00  // 0x100000
    };
    x86_codegen_ensure_capacity(&exit_gen, 7);
    memcpy(exit_gen.buffer + exit_gen.offset, sub_r12, 7);
    exit_gen.offset += 7;

    // 2. Save all registers (except R12) back to regDumpMem: MOV [R12 + N*8], regN
    // Use pvm_reg_to_x86 mapping to match Go's register order
    for (int i = 0; i < NUM_PVM_REGS; i++) {
        if (i == BASE_REG_INDEX) continue; // Skip R12 itself (it's the base pointer)
        int32_t offset = i * 8;
        x86_reg_t x86_reg = pvm_reg_to_x86[i];
        x86_encode_mov_mem_reg64(&exit_gen, X86_R12, offset, x86_reg);
    }

    // 3. RET
    x86_encode_ret(&exit_gen);

    // Cache exit code for djump table (matches Go's vm.exitCode usage)
    compiler->exit_code = (uint8_t*)malloc(exit_gen.offset);
    if (!compiler->exit_code) {
        x86_codegen_cleanup(&exit_gen);
        return -1;
    }
    memcpy(compiler->exit_code, exit_gen.buffer, exit_gen.offset);
    compiler->exit_code_size = exit_gen.offset;
    x86_codegen_cleanup(&exit_gen);

    DEBUG_JIT("Generated %u bytes of exit code (cached)\n", compiler->exit_code_size);

    // Build djump table exactly like Go: record address before appending table bytes
    compiler->djump_addr = compiler->x86_code_size;
    if (jit_generate_djump_table(compiler) != 0) {
        DEBUG_JIT("Failed to generate djump table\n");
        jit_free_basic_blocks(compiler);
        return -1;
    }

    if (jit_finalize_jumps(compiler) != 0) {
        DEBUG_JIT("Failed to finalize jump targets\n");
        jit_free_basic_blocks(compiler);
        return -1;
    }

    // Append djump table function to main x86 buffer
    if (compiler->djump_table_size > 0) {
        if (compiler->x86_code_size + compiler->djump_table_size > compiler->x86_code_capacity) {
            uint32_t new_capacity = (compiler->x86_code_size + compiler->djump_table_size) * 2;
            uint8_t* new_buffer = (uint8_t*)realloc(compiler->x86_code, new_capacity);
            if (!new_buffer) {
                jit_free_basic_blocks(compiler);
                return -1;
            }
            compiler->x86_code = new_buffer;
            compiler->x86_code_capacity = new_capacity;
        }
        memcpy(compiler->x86_code + compiler->x86_code_size,
               compiler->djump_table_code,
               compiler->djump_table_size);
        compiler->x86_code_size += compiler->djump_table_size;
    }

    jit_free_basic_blocks(compiler);

    // Set output parameters
    *out_x86_code = compiler->x86_code;
    *out_code_size = compiler->x86_code_size;
    *out_djump_addr = compiler->djump_addr;

    DEBUG_JIT("=== JIT Compilation Completed Successfully ===\n");
    return 0;
}

const pc_map_entry_t* compiler_get_pvm_to_x86_map(compiler_t* compiler, uint32_t* out_count) {
    if (!compiler || !out_count) return NULL;
    *out_count = compiler->pvm_to_x86_count;
    return compiler->pvm_to_x86_map;
}

const pc_map_entry_t* compiler_get_x86_to_pvm_map(compiler_t* compiler, uint32_t* out_count) {
    if (!compiler || !out_count) return NULL;
    *out_count = compiler->x86_to_pvm_count;
    return compiler->x86_to_pvm_map;
}

// Core implementation functions (extracted from .old)
static basic_block_t* jit_parse_basic_block(compiler_t* compiler, uint32_t start_pc) {
    if (!compiler || !compiler->bytecode) return NULL;

    uint32_t pc = start_pc;

    // Check bounds
    if (pc >= compiler->bytecode_size) return NULL;

    basic_block_t* block = basic_block_create(0); // x86_pc = 0 initially
    if (!block) return NULL;

    // Match Go's pattern: chargeGasInstrunctionIndex tracks which instruction to charge gas to
    size_t charge_gas_instruction_index = 0;

    // Parse instructions until basic block boundary (matches Go translateBasicBlock)
    while (pc < compiler->bytecode_size) {
        uint32_t current_pc = pc;
        uint8_t opcode = compiler->bytecode[pc];

        // Calculate operand length using skip function (matches Go vm.skip())
        uint32_t operand_len = jit_skip_instruction(compiler->bytecode,
                                                    compiler->bitmask,
                                                    compiler->bitmask_size, pc);

        const uint8_t* operand_view = NULL;
        uint32_t operand_start = pc + 1;
        if (operand_len > 0 && operand_start <= compiler->bytecode_size &&
            operand_start + operand_len <= compiler->bytecode_size) {
            operand_view = compiler->bytecode + operand_start;
        } else {
            operand_len = 0;
        }

        // Add instruction to block (matches Go: block.AddInstruction)
        if (basic_block_add_instruction(block, opcode, operand_view, operand_len, 0, pc) != 0) {
            basic_block_destroy(block);
            return NULL;
        }

        // Charge gas to the instruction at charge_gas_instruction_index (matches Go logic)
        if (charge_gas_instruction_index < block->instruction_count) {
            instruction_t* charge_inst = basic_block_get_instruction(block, charge_gas_instruction_index);
            if (charge_inst) {
                // If current opcode is ECALLI, add extra gas (matches Go: if op == ECALLI)
                if (opcode == ECALLI) {
                    uint32_t host_func = 0;
                    if (operand_view && operand_len > 0) {
                        host_func = decode_e_l(operand_view, operand_len);
                    } else if (operand_start < compiler->bytecode_size) {
                        host_func = compiler->bytecode[operand_start];
                    }
                    uint64_t extra = compiler_charge_gas((int)host_func);
                    charge_inst->gas_usage += (int64_t)extra;
                }

                // Always add 1 gas (matches Go: block.Instructions[chargeGasInstrunctionIndex].GasUsage += 1)
                charge_inst->gas_usage += 1;
            }
        }

        pc += operand_len + 1; // Move to next instruction

        // Check if this instruction terminates the basic block
        if (is_basic_block_terminator_opcode(opcode)) {
            jit_set_jump_metadata(block, opcode, operand_view, operand_len, current_pc);
            break;
        }

        // Update charge_gas_instruction_index for next iteration (matches Go: chargeGasInstrunctionIndex = len(block.Instructions))
        charge_gas_instruction_index = block->instruction_count;
    }

    // Set next PC
    block->pvm_next_pc = pc;

    // Validate that block has at least one instruction
    if (basic_block_get_instruction_count(block) == 0) {
        basic_block_destroy(block);
        return NULL;
    }

    return block;
}

static int jit_translate_basic_block_impl(compiler_t* compiler, basic_block_t* block) {
    if (!compiler || !block) return -1;

    // Initialize x86 code generator for this block
    x86_codegen_t gen;
    if (x86_codegen_init(&gen, 1024) != 0) {
        return -1;
    }

    // Set reg_dump_addr placeholder for ECALLI/SBRK translators (matches Go pattern)
    // This is a placeholder that will be patched at runtime
    gen.reg_dump_addr = 0x8888888888888888ULL;

    // Translate each PVM instruction to x86 using real translators
    size_t instruction_count = basic_block_get_instruction_count(block);
    
    for (size_t i = 0; i < instruction_count; i++) {
        instruction_t* inst = basic_block_get_instruction(block, i);
        if (!inst) continue;
        
        uint32_t inst_offset = gen.offset;

        // Generate gas check (matches Go: always emit even if gas_usage is 0)
        if (compiler->is_charging_gas) {
            uint32_t gas_charge = inst->gas_usage > 0 ? (uint32_t)inst->gas_usage : 0;
            // IMPORTANT: Generate gas check even when gas_charge is 0 (matches Go behavior)
            x86_encode_result_t gas_result = x86_encode_gas_check(&gen, gas_charge);
            if (gas_result.size == 0) {
                DEBUG_JIT("Failed to emit gas check for opcode=%u pc=%lu", inst->opcode, inst->pc);
            }
        }
        
        // Use real PVM instruction translator if available
        if (inst->opcode < 256 && pvm_instruction_translators[inst->opcode] != NULL) {
            DEBUG_JIT("[JIT] Translating opcode=%u pc=%lu args_size=%zu\n", inst->opcode, inst->pc, inst->args_size);

            // Reserve capacity for instruction translation (most instructions need < 256 bytes)
            // This prevents buffer overflow in emit_* functions
            if (x86_codegen_ensure_capacity(&gen, 256) != 0) {
                DEBUG_JIT("[JIT] Failed to ensure capacity for opcode=%u\n", inst->opcode);
                basic_block_destroy(block);
                return -1;
            }

            x86_encode_result_t result = pvm_instruction_translators[inst->opcode](&gen, inst);
            if (result.size == 0) {
                DEBUG_JIT("[JIT] Translator for opcode=%u produced no code\n", inst->opcode);
                x86_encode_nop(&gen);
                x86_encode_nop(&gen);
            } else {
                DEBUG_JIT("[JIT] Translator for opcode=%u generated %u bytes\n", inst->opcode, result.size);
            }
        } else {
            DEBUG_JIT("[JIT] No translator for opcode=%u, emitting NOPs\n", inst->opcode);
            x86_encode_nop(&gen);
            x86_encode_nop(&gen);
        }

        if (basic_block_add_pc_mapping(block, (uint32_t)inst->pc, (int)inst_offset) != 0) {
            x86_codegen_cleanup(&gen);
            return -1;
        }
    }
    
    // Copy generated code to block
    if (gen.offset > 0) {
        if (basic_block_append_x86_code(block, gen.buffer, gen.offset) != 0) {
            x86_codegen_cleanup(&gen);
            return -1;
        }
    }
    
    x86_codegen_cleanup(&gen);
    return 0;
}

static int jit_append_block(compiler_t* compiler, basic_block_t* block) {
    if (!compiler || !block) return -1;
    
    // Get block's x86 code
    uint8_t* block_code = basic_block_get_x86_code(block);
    size_t block_size = basic_block_get_x86_size(block);
    
    if (block_size == 0) return 0; // Nothing to append
    
    uint64_t block_start = compiler->x86_code_size;
    block->x86_pc = block_start;

    // Resize main x86 code buffer if needed
    if (compiler->x86_code_size + block_size > compiler->x86_code_capacity) {
        size_t new_capacity = (compiler->x86_code_size + block_size) * 2;
        // Ensure new_capacity fits in uint32_t
        if (new_capacity > UINT32_MAX) {
            DEBUG_JIT("jit_append_block: x86 code buffer would exceed UINT32_MAX\n");
            return -1;
        }

        uint8_t* new_buffer = (uint8_t*)realloc(compiler->x86_code, new_capacity);
        if (!new_buffer) {
            DEBUG_JIT("jit_append_block: realloc failed (requested %zu bytes)\n", new_capacity);
            return -1;
        }

        compiler->x86_code = new_buffer;
        compiler->x86_code_capacity = (uint32_t)new_capacity;
    }
    
    // Append block code to main buffer
    memcpy(compiler->x86_code + compiler->x86_code_size, block_code, block_size);
    compiler->x86_code_size += block_size;

    // Update PC mappings using block-local offsets and patch ECALLI/SBRK next x86 addresses
    pvm_pc_map_entry_t* entry = block->pvm_pc_to_x86_map;
    while (entry) {
        uint64_t absolute_offset = block_start + (uint64_t)entry->x86_offset;
        if (jit_add_pc_mapping(compiler, entry->pvm_pc, (uint32_t)absolute_offset) != 0) {
            return -1;
        }
        entry = entry->next;
    }

    // Patch nextx86 placeholder for ECALLI/SBRK instructions (matches Go's appendBlock logic)
    // const EcalliCodeIdx = 146, SbrkCodeIdx = 185, nextPcStartOffset = 119
    const uint32_t ECALLI_CODE_IDX = 146;
    const uint32_t SBRK_CODE_IDX = 185;
    const uint32_t NEXT_PC_START_OFFSET = 119;

    for (size_t i = 0; i < block->instruction_count; i++) {
        instruction_t* inst = basic_block_get_instruction(block, i);
        if (!inst) continue;

        if (inst->opcode == ECALLI || inst->opcode == SBRK) {
            // Find this instruction's x86 offset
            pvm_pc_map_entry_t* inst_entry = block->pvm_pc_to_x86_map;
            while (inst_entry) {
                if (inst_entry->pvm_pc == (uint32_t)inst->pc) {
                    uint64_t x86_realpc = block_start + (uint64_t)inst_entry->x86_offset;
                    uint32_t code_idx = (inst->opcode == ECALLI) ? ECALLI_CODE_IDX : SBRK_CODE_IDX;
                    uint64_t patch_location = x86_realpc + code_idx;
                    uint64_t next_x86_addr = x86_realpc + code_idx + NEXT_PC_START_OFFSET;

                    // Patch the 8-byte placeholder with actual next x86 address (little-endian)
                    if (patch_location + 8 <= compiler->x86_code_size) {
                        compiler->x86_code[patch_location + 0] = (uint8_t)(next_x86_addr & 0xFF);
                        compiler->x86_code[patch_location + 1] = (uint8_t)((next_x86_addr >> 8) & 0xFF);
                        compiler->x86_code[patch_location + 2] = (uint8_t)((next_x86_addr >> 16) & 0xFF);
                        compiler->x86_code[patch_location + 3] = (uint8_t)((next_x86_addr >> 24) & 0xFF);
                        compiler->x86_code[patch_location + 4] = (uint8_t)((next_x86_addr >> 32) & 0xFF);
                        compiler->x86_code[patch_location + 5] = (uint8_t)((next_x86_addr >> 40) & 0xFF);
                        compiler->x86_code[patch_location + 6] = (uint8_t)((next_x86_addr >> 48) & 0xFF);
                        compiler->x86_code[patch_location + 7] = (uint8_t)((next_x86_addr >> 56) & 0xFF);
                    }
                    break;
                }
                inst_entry = inst_entry->next;
            }
        }
    }

    return 0;
}

// Stub remaining functions
int jit_compile_basic_blocks(compiler_t* compiler, uint32_t start_pc) {
    (void)compiler; (void)start_pc; return -1;
}

int jit_translate_basic_block(compiler_t* compiler, basic_block_t* block) {
    return jit_translate_basic_block_impl(compiler, block);
}

int jit_append_block_code(compiler_t* compiler, basic_block_t* block) {
    return jit_append_block(compiler, block);
}

int jit_generate_djump_table(compiler_t* compiler) {
    if (!compiler) {
        return -1;
    }

    if (compiler->djump_table_code) {
        free(compiler->djump_table_code);
        compiler->djump_table_code = NULL;
    }
    compiler->djump_table_size = 0;

    if (!compiler->exit_code || compiler->exit_code_size == 0) {
        return 0;
    }

    uint32_t handler_capacity = compiler->jump_table_size;
    uint32_t handler_count = 0;
    uint32_t* handler_offsets = NULL;

    if (handler_capacity > 0) {
        handler_offsets = (uint32_t*)malloc(handler_capacity * sizeof(uint32_t));
        if (!handler_offsets) {
            return -1;
        }
        for (uint32_t i = 0; i < handler_capacity; ++i) {
            uint32_t target_pc = compiler->jump_table[i];
            uint32_t target_offset = 0;
            if (jit_lookup_x86_offset(compiler, target_pc, &target_offset)) {
                handler_offsets[handler_count++] = target_offset;
            }
        }
    }

    typedef struct {
        int je_off;
        uint32_t handler;
    } pending_handler_t;

    pending_handler_t* pendings = NULL;

    uint32_t base_overhead = 256;
    uint32_t handler_bytes = handler_count * 7;
    uint32_t exit_bytes = compiler->exit_code_size;
    uint32_t capacity = base_overhead + handler_bytes + exit_bytes;
    if (capacity < 128) {
        capacity = 128;
    }

    uint8_t* code = (uint8_t*)malloc(capacity);
    if (!code) {
        free(handler_offsets);
        return -1;
    }
    uint32_t offset = 0;
    uint32_t x86_code_len = compiler->x86_code_size;

    uint32_t je_ret_offset = 0;
    uint32_t je_zero_offset = 0;
    uint32_t ja_panic_offset = 0;
    uint32_t jne_panic_offset = 0;
    uint32_t handler_add_offset = 0;
    uint32_t panic_stub_offset = 0;
    uint32_t ret_stub_offset = 0;

#define ENSURE_CAP(required) \
    do { \
        if (offset + (required) > capacity) { \
            uint32_t new_capacity = capacity * 2; \
            if (new_capacity < offset + (required)) { \
                new_capacity = offset + (required) + 128; \
            } \
            /* Check for overflow */ \
            if (new_capacity < capacity || new_capacity > UINT32_MAX - 1024) { \
                DEBUG_JIT("ENSURE_CAP: capacity overflow (old=%u, new=%u, required=%u)\n", \
                         capacity, new_capacity, (uint32_t)(required)); \
                fprintf(stderr, "CRITICAL: ENSURE_CAP overflow in djump table\n"); \
                if (handler_offsets) free(handler_offsets); \
                if (pendings) free(pendings); \
                free(code); \
                return -1; \
            } \
            uint8_t* new_buf = (uint8_t*)realloc(code, new_capacity); \
            if (!new_buf) { \
                DEBUG_JIT("ENSURE_CAP: realloc FAILED for %u bytes\n", new_capacity); \
                fprintf(stderr, "CRITICAL: ENSURE_CAP realloc failed for %u bytes\n", new_capacity); \
                if (handler_offsets) free(handler_offsets); \
                if (pendings) free(pendings); \
                free(code); \
                return -1; \
            } \
            code = new_buf; \
            capacity = new_capacity; \
        } \
    } while (0)

#define WRITE_U32_LE(ptr, value) \
    do { \
        uint32_t __tmp = (uint32_t)(value); \
        (ptr)[0] = (uint8_t)(__tmp & 0xFF); \
        (ptr)[1] = (uint8_t)((__tmp >> 8) & 0xFF); \
        (ptr)[2] = (uint8_t)((__tmp >> 16) & 0xFF); \
        (ptr)[3] = (uint8_t)((__tmp >> 24) & 0xFF); \
    } while (0)

    // (a) CMP ECX, 0xFFFF0000; JE ret_stub
    ENSURE_CAP(7);
    code[offset++] = 0x40;
    code[offset++] = 0x81;
    code[offset++] = 0xF9;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0xFF;
    code[offset++] = 0xFF;

    ENSURE_CAP(6);
    je_ret_offset = offset;
    code[offset++] = 0x0F;
    code[offset++] = 0x84;
    offset += 4; // placeholder

    // (b) CMP RCX, 0; JE panic_stub
    ENSURE_CAP(4);
    code[offset++] = 0x48;
    code[offset++] = 0x83;
    code[offset++] = 0xF9;
    code[offset++] = 0x00;

    ENSURE_CAP(6);
    je_zero_offset = offset;
    code[offset++] = 0x0F;
    code[offset++] = 0x84;
    offset += 4; // placeholder

    // (c) CMP RCX, threshold; JA panic_stub
    uint32_t threshold = handler_count * Z_A;
    ENSURE_CAP(7);
    code[offset++] = 0x48;
    code[offset++] = 0x81;
    code[offset++] = 0xF9;
    WRITE_U32_LE(code + offset, threshold);
    offset += 4;

    ENSURE_CAP(6);
    ja_panic_offset = offset;
    code[offset++] = 0x0F;
    code[offset++] = 0x87;
    offset += 4; // placeholder

    // (d) TEST RCX, 1; JNE panic_stub
    ENSURE_CAP(7);
    code[offset++] = 0x48;
    code[offset++] = 0xF7;
    code[offset++] = 0xC1;
    code[offset++] = 0x01;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;

    ENSURE_CAP(6);
    jne_panic_offset = offset;
    code[offset++] = 0x0F;
    code[offset++] = 0x85;
    offset += 4; // placeholder

    // Compute (RCX / 2 - 1) * 7
    ENSURE_CAP(4);
    code[offset++] = 0x48;
    code[offset++] = 0xC1;
    code[offset++] = 0xF9;
    code[offset++] = 0x01;

    ENSURE_CAP(4);
    code[offset++] = 0x48;
    code[offset++] = 0x83;
    code[offset++] = 0xE9;
    code[offset++] = 0x01;

    ENSURE_CAP(7);
    code[offset++] = 0x48;
    code[offset++] = 0x69;
    code[offset++] = 0xC9;
    code[offset++] = 0x07;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;

    // PUSH RAX
    ENSURE_CAP(1);
    code[offset++] = 0x50;

    // LEA RAX, [RIP+0]
    ENSURE_CAP(7);
    code[offset++] = 0x48;
    code[offset++] = 0x8D;
    code[offset++] = 0x05;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;

    // ADD RCX, RAX
    ENSURE_CAP(3);
    code[offset++] = 0x48;
    code[offset++] = 0x01;
    code[offset++] = 0xC1;

    // POP RAX
    ENSURE_CAP(1);
    code[offset++] = 0x58;

    // ADD RCX, imm32 (patched later)
    ENSURE_CAP(7);
    handler_add_offset = offset + 3;
    code[offset++] = 0x48;
    code[offset++] = 0x81;
    code[offset++] = 0xC1;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;
    code[offset++] = 0x00;

    // JMP RCX
    ENSURE_CAP(3);
    code[offset++] = 0x48;
    code[offset++] = 0xFF;
    code[offset++] = 0xE1;

    // Panic stub: POP RCX; UD2
    ENSURE_CAP(4);
    panic_stub_offset = offset;
    code[offset++] = 0x48;
    code[offset++] = 0x59;
    code[offset++] = 0x0F;
    code[offset++] = 0x0B;

    // Return stub: POP RCX + exit code
    ENSURE_CAP(1);
    ret_stub_offset = offset;
    code[offset++] = 0x59;

    if (compiler->exit_code_size > 0) {
        ENSURE_CAP(compiler->exit_code_size);
        memcpy(code + offset, compiler->exit_code, compiler->exit_code_size);
        offset += compiler->exit_code_size;
    }

    // Patch pre-check jumps
    int32_t ret_rel = (int32_t)(ret_stub_offset - (int32_t)(je_ret_offset + 6));
    WRITE_U32_LE(code + je_ret_offset + 2, (uint32_t)ret_rel);

    int32_t panic_rel1 = (int32_t)(panic_stub_offset - (int32_t)(je_zero_offset + 6));
    WRITE_U32_LE(code + je_zero_offset + 2, (uint32_t)panic_rel1);

    int32_t panic_rel2 = (int32_t)(panic_stub_offset - (int32_t)(ja_panic_offset + 6));
    WRITE_U32_LE(code + ja_panic_offset + 2, (uint32_t)panic_rel2);

    int32_t panic_rel3 = (int32_t)(panic_stub_offset - (int32_t)(jne_panic_offset + 6));
    WRITE_U32_LE(code + jne_panic_offset + 2, (uint32_t)panic_rel3);

    if (handler_count > 0) {
        pendings = (pending_handler_t*)malloc(handler_count * sizeof(pending_handler_t));
        if (!pendings) {
            if (handler_offsets) free(handler_offsets);
            free(code);
            return -1;
        }
        for (uint32_t i = 0; i < handler_count; ++i) {
            pendings[i].handler = handler_offsets[i];
            pendings[i].je_off = 0;
        }
    }

    for (uint32_t i = 0; i < handler_count; ++i) {
        pending_handler_t* p = &pendings[i];
        p->je_off = (int)offset;

        ENSURE_CAP(2);
        code[offset++] = 0x48;
        code[offset++] = 0x59;

        int len_code_after_pop = (int)offset;
        int64_t to_minus64 = (int64_t)x86_code_len + (int64_t)len_code_after_pop - (int64_t)p->handler;
        int32_t rel = (int32_t)(-to_minus64 - 5);

        ENSURE_CAP(5);
        code[offset++] = 0xE9;
        WRITE_U32_LE(code + offset, (uint32_t)rel);
        offset += 4;
    }

    if (handler_count > 0) {
        int32_t to_add = (int32_t)(pendings[0].je_off - (int32_t)handler_add_offset + 7);
        WRITE_U32_LE(code + handler_add_offset, (uint32_t)to_add);
    } else {
        WRITE_U32_LE(code + handler_add_offset, 0);
    }

#undef ENSURE_CAP
#undef WRITE_U32_LE

    if (pendings) {
        free(pendings);
    }
    if (handler_offsets) {
        free(handler_offsets);
    }

    uint8_t* final_buf = (uint8_t*)realloc(code, offset);
    if (final_buf) {
        code = final_buf;
    }

    compiler->djump_table_code = code;
    compiler->djump_table_size = offset;
    return 0;
}

int jit_finalize_jumps(compiler_t* compiler) {
    if (!compiler) {
        return -1;
    }

    for (uint32_t i = 0; i < compiler->basic_block_count; i++) {
        basic_block_t* block = compiler->basic_blocks[i];
        if (!block) {
            continue;
        }

        int jump_type = basic_block_get_jump_type(block);
        if (jump_type != DIRECT_JUMP && jump_type != CONDITIONAL) {
            continue;
        }

        size_t block_size = basic_block_get_x86_size(block);
        if (block_size < 4) {
            continue;
        }

        uint32_t target_x86 = 0;
        if (!jit_lookup_x86_offset(compiler, (uint32_t)block->true_pc, &target_x86)) {
            DEBUG_JIT("jit_finalize_jumps: target PC %llu not found in mapping\n",
                      (unsigned long long)block->true_pc);
            continue;
        }

        uint64_t block_end = block->x86_pc + (uint64_t)block_size;
        if (block_end < 4 || block_end > compiler->x86_code_size) {
            DEBUG_JIT("jit_finalize_jumps: invalid block end for block at x86_pc=%llu\n",
                      (unsigned long long)block->x86_pc);
            continue;
        }

        int64_t rel = (int64_t)target_x86 - (int64_t)block_end;
        int32_t rel32 = (int32_t)rel;

        uint32_t patch_offset = (uint32_t)(block_end - 4);
        if (patch_offset + 4 > compiler->x86_code_size) {
            DEBUG_JIT("jit_finalize_jumps: patch_offset out of bounds (%u)\n", patch_offset);
            return -1;
        }

        compiler->x86_code[patch_offset + 0] = (uint8_t)(rel32 & 0xFF);
        compiler->x86_code[patch_offset + 1] = (uint8_t)((rel32 >> 8) & 0xFF);
        compiler->x86_code[patch_offset + 2] = (uint8_t)((rel32 >> 16) & 0xFF);
        compiler->x86_code[patch_offset + 3] = (uint8_t)((rel32 >> 24) & 0xFF);
    }

    return 0;
}

int jit_add_pc_mapping(compiler_t* compiler, uint32_t pvm_pc, uint32_t x86_offset) {
    if (!compiler) {
        DEBUG_JIT("jit_add_pc_mapping: NULL compiler\n");
        return -1;
    }

    // Check for overflow before resizing
    if (compiler->pvm_to_x86_count >= compiler->pvm_to_x86_capacity) {
        uint32_t new_capacity = compiler->pvm_to_x86_capacity ? compiler->pvm_to_x86_capacity * 2 : 1024;
        // Ensure we don't overflow
        if (new_capacity > UINT32_MAX / sizeof(pc_map_entry_t)) {
            DEBUG_JIT("jit_add_pc_mapping: capacity overflow detected\n");
            return -1;
        }

        pc_map_entry_t* new_entries = (pc_map_entry_t*)realloc(
            compiler->pvm_to_x86_map, new_capacity * sizeof(pc_map_entry_t));
        if (!new_entries) {
            DEBUG_JIT("jit_add_pc_mapping: realloc failed for pvm_to_x86_map (requested %u entries)\n", new_capacity);
            return -1;
        }

        // Clear newly allocated memory
        memset(new_entries + compiler->pvm_to_x86_capacity, 0,
               (new_capacity - compiler->pvm_to_x86_capacity) * sizeof(pc_map_entry_t));

        compiler->pvm_to_x86_map = new_entries;
        compiler->pvm_to_x86_capacity = new_capacity;
    }

    compiler->pvm_to_x86_map[compiler->pvm_to_x86_count].pvm_pc = pvm_pc;
    compiler->pvm_to_x86_map[compiler->pvm_to_x86_count].x86_offset = x86_offset;
    compiler->pvm_to_x86_count++;

    // Same for x86_to_pvm
    if (compiler->x86_to_pvm_count >= compiler->x86_to_pvm_capacity) {
        uint32_t new_capacity = compiler->x86_to_pvm_capacity ? compiler->x86_to_pvm_capacity * 2 : 1024;
        // Ensure we don't overflow
        if (new_capacity > UINT32_MAX / sizeof(pc_map_entry_t)) {
            DEBUG_JIT("jit_add_pc_mapping: capacity overflow detected for x86_to_pvm\n");
            return -1;
        }

        pc_map_entry_t* new_entries = (pc_map_entry_t*)realloc(
            compiler->x86_to_pvm_map, new_capacity * sizeof(pc_map_entry_t));
        if (!new_entries) {
            DEBUG_JIT("jit_add_pc_mapping: realloc failed for x86_to_pvm_map (requested %u entries)\n", new_capacity);
            return -1;
        }

        // Clear newly allocated memory
        memset(new_entries + compiler->x86_to_pvm_capacity, 0,
               (new_capacity - compiler->x86_to_pvm_capacity) * sizeof(pc_map_entry_t));

        compiler->x86_to_pvm_map = new_entries;
        compiler->x86_to_pvm_capacity = new_capacity;
    }

    compiler->x86_to_pvm_map[compiler->x86_to_pvm_count].pvm_pc = pvm_pc;
    compiler->x86_to_pvm_map[compiler->x86_to_pvm_count].x86_offset = x86_offset;
    compiler->x86_to_pvm_count++;

    return 0;
}

void compiler_print_state(const compiler_t* compiler) {
    if (!compiler) {
        printf("Compiler: NULL\n");
        return;
    }
    printf("JIT Compiler (stub):\n");
    printf("  Bytecode size: %u bytes\n", compiler->bytecode_size);
    printf("  Jump table: %u entries\n", compiler->jump_table_size);
    printf("  Bitmask: %u bytes\n", compiler->bitmask_size);
}
