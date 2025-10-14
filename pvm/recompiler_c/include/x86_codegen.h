#ifndef X86_CODEGEN_H
#define X86_CODEGEN_H

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// Forward declarations
typedef struct x86_codegen x86_codegen_t;
typedef struct instruction instruction_t;

// x86-64 registers (matches Go's x86 register mapping)
typedef enum {
    X86_RAX = 0,  X86_RCX = 1,  X86_RDX = 2,  X86_RBX = 3,
    X86_RSP = 4,  X86_RBP = 5,  X86_RSI = 6,  X86_RDI = 7,
    X86_R8 = 8,   X86_R9 = 9,   X86_R10 = 10, X86_R11 = 11,
    X86_R12 = 12, X86_R13 = 13, X86_R14 = 14, X86_R15 = 15
} x86_reg_t;

// PVM register to x86 register mapping
// R12 points to the register dump area in memory (matches Go implementation)
extern const x86_reg_t pvm_reg_to_x86[14];

// x86 instruction prefixes and opcodes
#define X86_REX_W     0x48  // REX.W prefix (64-bit operand)
#define X86_REX_R     0x44  // REX.R prefix (extend ModRM reg field)
#define X86_REX_X     0x42  // REX.X prefix (extend SIB index field)  
#define X86_REX_B     0x41  // REX.B prefix (extend ModRM r/m field)

// Common x86 opcodes
#define X86_OP_MOV_REG_IMM64  0xB8  // MOV r64, imm64
#define X86_OP_MOV_RM_R       0x89  // MOV r/m32, r32
#define X86_OP_MOV_R_RM       0x8B  // MOV r32, r/m32
#define X86_OP_ADD_RM_R       0x01  // ADD r/m32, r32
#define X86_OP_SUB_RM_R       0x29  // SUB r/m32, r32
#define X86_OP_CMP_RM_R       0x39  // CMP r/m32, r32
#define X86_OP_JMP_REL32      0xE9  // JMP rel32
#define X86_OP_JE_REL32       0x84  // JE rel32 (with 0x0F prefix)
#define X86_OP_JNE_REL32      0x85  // JNE rel32 (with 0x0F prefix)
#define X86_OP_TRAP           0x0B  // UD2 (undefined instruction - trap)
#define X86_OP_NOP            0x90  // NOP
#define X86_OP_RET            0xC3  // RET
#define X86_OP_PUSH_R         0x50  // PUSH r64
#define X86_OP_POP_R          0x58  // POP r64

// ModRM byte construction
#define X86_MODRM(mod, reg, rm) (((mod) << 6) | ((reg) << 3) | (rm))
#define X86_MOD_REG           3   // Register addressing
#define X86_MOD_MEM           0   // Memory addressing (no displacement)
#define X86_MOD_MEM_DISP8     1   // Memory addressing (8-bit displacement)
#define X86_MOD_MEM_DISP32    2   // Memory addressing (32-bit displacement)

// SIB byte construction  
#define X86_SIB(scale, index, base) (((scale) << 6) | ((index) << 3) | (base))

// x86 code generator structure (simplified - no VM dependency)
typedef struct x86_codegen {
    uint8_t* buffer;          // Code buffer
    uint32_t size;            // Current size of generated code
    uint32_t capacity;        // Buffer capacity
    uint32_t offset;          // Current write offset
    uintptr_t reg_dump_addr;  // Register dump address for context slots
} x86_codegen_t;

// x86 instruction encoding result
typedef struct {
    uint8_t* code;            // Pointer to encoded instruction
    uint32_t size;            // Size of encoded instruction
    uint32_t offset;          // Offset where instruction was written
} x86_encode_result_t;

// Core x86 code generator functions
int x86_codegen_init(x86_codegen_t* gen, uint32_t initial_capacity);
void x86_codegen_cleanup(x86_codegen_t* gen);
int x86_codegen_ensure_capacity(x86_codegen_t* gen, uint32_t additional_size);
uint32_t x86_codegen_get_offset(const x86_codegen_t* gen);
uint8_t* x86_codegen_get_current_ptr(const x86_codegen_t* gen);

// Low-level x86 instruction encoding
x86_encode_result_t x86_encode_mov_reg64_imm64(x86_codegen_t* gen, x86_reg_t dst, uint64_t imm);
x86_encode_result_t x86_encode_mov_reg64_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset);
x86_encode_result_t x86_encode_mov_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset);
x86_encode_result_t x86_encode_mov_mem_reg32(x86_codegen_t* gen, x86_reg_t base, int32_t offset, x86_reg_t src);
x86_encode_result_t x86_encode_gas_check(x86_codegen_t* gen, uint32_t gas_charge);
x86_encode_result_t x86_encode_add_reg32_reg32(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src);
x86_encode_result_t x86_encode_add_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset);
x86_encode_result_t x86_encode_sub_reg32_reg32(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src);
x86_encode_result_t x86_encode_sub_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset);
x86_encode_result_t x86_encode_cmp_reg32_reg32(x86_codegen_t* gen, x86_reg_t reg1, x86_reg_t reg2);
x86_encode_result_t x86_encode_cmp_reg32_mem(x86_codegen_t* gen, x86_reg_t reg, x86_reg_t base, int32_t offset);
x86_encode_result_t x86_encode_jmp_rel32(x86_codegen_t* gen, int32_t offset);
x86_encode_result_t x86_encode_trap(x86_codegen_t* gen);
x86_encode_result_t x86_encode_je_rel32(x86_codegen_t* gen, int32_t offset);
x86_encode_result_t x86_encode_jne_rel32(x86_codegen_t* gen, int32_t offset);
x86_encode_result_t x86_encode_push_reg64(x86_codegen_t* gen, x86_reg_t reg);
x86_encode_result_t x86_encode_pop_reg64(x86_codegen_t* gen, x86_reg_t reg);
x86_encode_result_t x86_encode_nop(x86_codegen_t* gen);
x86_encode_result_t x86_encode_ret(x86_codegen_t* gen);

// Helper functions for REX prefix calculation
uint8_t x86_calc_rex_prefix(bool w, bool r, bool x, bool b);
bool x86_reg_needs_rex(x86_reg_t reg);
uint8_t x86_reg_bits(x86_reg_t reg);

// Helper functions for memory addressing
x86_encode_result_t x86_encode_mem_access(x86_codegen_t* gen, uint8_t opcode,
                                         x86_reg_t reg, x86_reg_t base, int32_t offset);

// Context slot functions (equivalent to Go's BuildWriteContextSlotCode)
x86_encode_result_t x86_encode_write_context_slot(x86_codegen_t* gen,
                                                  uintptr_t reg_dump_addr,
                                                  int slot_index,
                                                  uint64_t value,
                                                  int size);

// Absolute memory helpers (mirrors Go helper utilities)
x86_encode_result_t x86_encode_inc_mem64_abs(x86_codegen_t* gen, uint64_t addr);


// PVM instruction translators (function pointer type)
typedef x86_encode_result_t (*pvm_to_x86_translator_t)(x86_codegen_t* gen, const instruction_t* inst);

// PVM→x86 translator dispatch table
extern pvm_to_x86_translator_t pvm_instruction_translators[256];

// Individual PVM instruction translators
x86_encode_result_t translate_add_32_to_x86(x86_codegen_t* gen, const instruction_t* inst);
x86_encode_result_t translate_sub_32_to_x86(x86_codegen_t* gen, const instruction_t* inst);
x86_encode_result_t translate_jump_to_x86(x86_codegen_t* gen, const instruction_t* inst);
x86_encode_result_t translate_branch_eq_to_x86(x86_codegen_t* gen, const instruction_t* inst);
x86_encode_result_t translate_trap_to_x86(x86_codegen_t* gen, const instruction_t* inst);
x86_encode_result_t translate_nop_to_x86(x86_codegen_t* gen, const instruction_t* inst);

// Utility functions for PVM operand extraction
void extract_two_registers(const uint8_t* operands, uint8_t* reg1, uint8_t* reg2);
void extract_three_registers(const uint8_t* operands, uint8_t* dst, uint8_t* src1, uint8_t* src2);
int32_t extract_offset(const uint8_t* operands);
void extract_reg_and_offset(const uint8_t* operands, uint8_t* reg, int32_t* offset);

// Memory offset calculation for PVM registers (matches Go implementation)
// R12 points to register dump area, registers are at R12 + (reg_index * 8)
#define PVM_REG_OFFSET(reg_index) ((reg_index) * 8)

// Debug and introspection
void x86_codegen_print_state(const x86_codegen_t* gen);
void x86_codegen_print_buffer(const x86_codegen_t* gen, uint32_t start, uint32_t length);
int x86_codegen_validate_state(const x86_codegen_t* gen);

// Initialize the PVM→x86 translator table (implemented in x86_translators_complete.c)
void pvm_translators_init(void);

// Simple interface for generating x86 code for single instruction (for testing/comparison)
// Returns size of generated x86 code, or 0 if no translator exists
int generate_single_instruction_x86(uint8_t opcode, const uint8_t* args, size_t args_size, 
                                    uint8_t* output_buffer, size_t buffer_size);

// Additional instruction encodings for Go compatibility

// Register initialization (for startCode generation) - matches Go's encodeMovImm
x86_encode_result_t x86_encode_mov_reg64_imm64(x86_codegen_t* gen, x86_reg_t dst, uint64_t immediate);

// Sign extension (for 32-bit to 64-bit conversion) - matches Go's emitMovsxd64
x86_encode_result_t x86_encode_movsxd64(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src);

// Memory access for exitCode generation - matches Go's encodeMovRegToMem
x86_encode_result_t x86_encode_mov_mem_reg64(x86_codegen_t* gen, x86_reg_t base, int32_t offset, x86_reg_t src);

// Jump instruction with placeholder (matches Go's entryPatch pattern)
x86_encode_result_t x86_encode_jmp_rel32_placeholder(x86_codegen_t* gen);

// Instruction operand extraction functions (matches Go's utils.go functions)
void extract_one_reg_one_imm_one_offset(const uint8_t* args, size_t args_len, uint8_t* reg, uint64_t* imm, int64_t* offset);

#ifdef __cplusplus
}
#endif

#endif // X86_CODEGEN_H
