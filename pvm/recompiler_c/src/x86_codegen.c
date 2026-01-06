#include "x86_codegen.h"
#include "basic_block.h"
#include "pvm_const.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// Debug macros (simplified - compiler only mode)
#ifdef DEBUG
#define DEBUG_JIT(...) fprintf(stderr, __VA_ARGS__)
#define DEBUG_X86(...) fprintf(stderr, __VA_ARGS__)
#else
#define DEBUG_JIT(...) ((void)0)
#define DEBUG_X86(...) ((void)0)
#endif

// Note: pvm_translators_init is now defined in x86_translators_complete.c

// PVM register to x86 register mapping (exactly matches Go recompiler regInfoList)
// Go regInfoList: RAX, RDX, RBX, RSI, RDI, R8, R9, R10, R11, R12, R13, R14, R15, RBP (14 registers)
const x86_reg_t pvm_reg_to_x86[14] = {
    X86_RAX,  // Index 0 → RAX
    X86_RDX,  // Index 1 → RDX
    X86_RBX,  // Index 2 → RBX
    X86_RSI,  // Index 3 → RSI
    X86_RDI,  // Index 4 → RDI
    X86_R8,   // Index 5 → R8
    X86_R9,   // Index 6 → R9
    X86_R10,  // Index 7 → R10
    X86_R11,  // Index 8 → R11
    X86_R12,  // Index 9 → R12
    X86_R13,  // Index 10 → R13
    X86_R14,  // Index 11 → R14
    X86_R15,  // Index 12 → R15
    X86_RBP   // Index 13 → RBP (BaseReg)
};

// Helper function to calculate REX prefix
uint8_t x86_calc_rex_prefix(bool w, bool r, bool x, bool b) {
    uint8_t rex = 0x40; // Base REX prefix
    if (w) rex |= 0x08; // REX.W
    if (r) rex |= 0x04; // REX.R  
    if (x) rex |= 0x02; // REX.X
    if (b) rex |= 0x01; // REX.B
    return rex;
}

// Check if register needs REX prefix
bool x86_reg_needs_rex(x86_reg_t reg) {
    return reg >= X86_R8; // R8-R15 need REX prefix
}

// Get register bits (0-7)
uint8_t x86_reg_bits(x86_reg_t reg) {
    return reg & 0x07;
}

// Core x86 code generator functions
int x86_codegen_init(x86_codegen_t* gen, uint32_t initial_capacity) {
    if (!gen) return -1;
    
    if (initial_capacity == 0) {
        initial_capacity = 4096; // 4KB default
    }
    
    gen->buffer = malloc(initial_capacity);
    if (!gen->buffer) return -1;

    gen->size = 0;
    gen->capacity = initial_capacity;
    gen->offset = 0;
    gen->reg_dump_addr = 0;

    return 0;
}

void x86_codegen_cleanup(x86_codegen_t* gen) {
    if (!gen) return;
    free(gen->buffer);
    gen->buffer = NULL;
    gen->size = 0;
    gen->capacity = 0;
    gen->offset = 0;
}

int x86_codegen_ensure_capacity(x86_codegen_t* gen, uint32_t additional_size) {
    if (!gen) return -1;

    if (gen->offset + additional_size > gen->capacity) {
        uint32_t needed = gen->offset + additional_size;
        uint32_t new_capacity = gen->capacity;

        // Prevent infinite loop and overflow
        while (new_capacity < needed) {
            // Check for overflow before doubling
            if (new_capacity == 0 || new_capacity > UINT32_MAX / 2) {
                DEBUG_X86("x86_codegen_ensure_capacity: capacity would overflow (current=%u, needed=%u)\n",
                         new_capacity, needed);
                fprintf(stderr, "CRITICAL: x86_codegen_ensure_capacity overflow detected (current=%u, needed=%u)\n",
                        new_capacity, needed);
                return -1;
            }
            new_capacity *= 2;
        }

        uint8_t* new_buffer = realloc(gen->buffer, new_capacity);
        if (!new_buffer) {
            DEBUG_X86("x86_codegen_ensure_capacity: realloc FAILED for %u bytes\n", new_capacity);
            fprintf(stderr, "CRITICAL: x86_codegen_ensure_capacity realloc failed for %u bytes (old capacity: %u)\n",
                    new_capacity, gen->capacity);
            return -1;
        }

        DEBUG_X86("x86_codegen_ensure_capacity: SUCCESS, new_capacity=%u (was %u)\n", new_capacity, gen->capacity);
        gen->buffer = new_buffer;
        gen->capacity = new_capacity;
    }

    return 0;
}

uint32_t x86_codegen_get_offset(const x86_codegen_t* gen) {
    return gen ? gen->offset : 0;
}

uint8_t* x86_codegen_get_current_ptr(const x86_codegen_t* gen) {
    return gen ? gen->buffer + gen->offset : NULL;
}

// Helper function to write bytes to code buffer
static x86_encode_result_t write_bytes(x86_codegen_t* gen, const uint8_t* bytes, uint32_t count) {
    x86_encode_result_t result = {0};
    
    if (!gen || !bytes || count == 0) {
        return result;
    }
    
    if (x86_codegen_ensure_capacity(gen, count) != 0) {
        return result;
    }
    
    result.offset = gen->offset;
    result.code = gen->buffer + gen->offset;
    result.size = count;
    
    memcpy(gen->buffer + gen->offset, bytes, count);
    gen->offset += count;
    gen->size = gen->offset;
    
    return result;
}

// MOV r64, imm64 (10 bytes: REX.W + B8+r + imm64)
x86_encode_result_t x86_encode_mov_reg64_imm64(x86_codegen_t* gen, x86_reg_t dst, uint64_t imm) {
    DEBUG_X86("x86_encode_mov_reg64_imm64: dst=%d, imm=%lu (0x%lx)", dst, imm, imm);
    uint8_t code[10];
    uint32_t pos = 0;
    
    // REX prefix (REX.W + REX.B if needed)
    bool needs_rex_b = x86_reg_needs_rex(dst);
    code[pos++] = x86_calc_rex_prefix(true, false, false, needs_rex_b);
    
    // Opcode + register
    code[pos++] = X86_OP_MOV_REG_IMM64 + x86_reg_bits(dst);
    
    // 64-bit immediate (little-endian)
    for (int i = 0; i < 8; i++) {
        code[pos++] = (imm >> (i * 8)) & 0xFF;
    }
    
    DEBUG_X86("Generated bytes: %.*s", pos * 3, ""); // Simplified debug output
    
    return write_bytes(gen, code, pos);
}

// Emit gas check sequence matching Go generateGasCheck
x86_encode_result_t x86_encode_gas_check(x86_codegen_t* gen, uint32_t gas_charge) {
    x86_encode_result_t result = {0};
    if (!gen) {
        return result;
    }

    const size_t required = 1 + 1 + 1 + 1 + 4 + 4 + 2 + 2;
    if (x86_codegen_ensure_capacity(gen, required) != 0) {
        return result;
    }

    size_t start_offset = gen->offset;
    // Gas slot addressing (matches Go): BaseReg points to realMemAddr, and reg dump is at BaseReg - DUMP_SIZE.
    int32_t displacement;
#ifdef USE_UNICORN_EXECUTION
    displacement = (int32_t)(GAS_SLOT_INDEX * 8 - DUMP_SIZE);  // 14*8 - 0x100000 = -0xFFF90
#else
    displacement = (int32_t)(GAS_SLOT_INDEX * 8 - DUMP_SIZE);  // 14*8 - 0x100000 = -0xFFF90
#endif

    // Match Go's generateSubMem64Imm32 exactly
    const x86_reg_t base_reg = pvm_reg_to_x86[13]; // BaseReg
    const uint8_t base_bits = x86_reg_bits(base_reg);

    // REX Prefix: REX.W = 1 (0x48), plus REX.B if BaseReg is extended.
    uint8_t rex = 0x48 | (x86_reg_needs_rex(base_reg) ? 0x01 : 0x00);
    gen->buffer[gen->offset++] = rex;

    // Opcode: 81 /5 (SUB r/m64, imm32)
    gen->buffer[gen->offset++] = 0x81;

    // ModRM: mod=10 (disp32), reg=5 (SUB), rm=BaseReg.RegBits
    uint8_t modrm = 0x80 | (5 << 3) | (base_bits & 0x07);
    gen->buffer[gen->offset++] = modrm;

    // SIB is required only when rm==100 (RSP/R12).
    if ((base_bits & 0x07) == 4) {
        gen->buffer[gen->offset++] = 0x24; // scale=0, index=none(100), base=100
    }

    // disp32 (displacement) - little endian
    gen->buffer[gen->offset++] = (uint8_t)(displacement & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((displacement >> 8) & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((displacement >> 16) & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((displacement >> 24) & 0xFF);

    // imm32 (gas charge) - little endian
    gen->buffer[gen->offset++] = (uint8_t)(gas_charge & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((gas_charge >> 8) & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((gas_charge >> 16) & 0xFF);
    gen->buffer[gen->offset++] = (uint8_t)((gas_charge >> 24) & 0xFF);

    // JNS skip_trap
    gen->buffer[gen->offset++] = 0x79;             // JNS rel8
    size_t jns_offset = gen->offset;
    gen->buffer[gen->offset++] = 0x00;             // Placeholder for rel8

    // UD2 trap
    gen->buffer[gen->offset++] = 0x0F;
    gen->buffer[gen->offset++] = 0x0B;

    // Patch JNS rel8 to skip UD2
    int rel8 = (int)(gen->offset - (jns_offset + 1));
    gen->buffer[jns_offset] = (uint8_t)rel8;

    gen->size = gen->offset;
    result.code = &gen->buffer[start_offset];
    result.offset = start_offset;
    result.size = gen->offset - start_offset;
    return result;
}

// MOV r64, [base + offset]
x86_encode_result_t x86_encode_mov_reg64_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset) {
    uint8_t code[11];
    uint32_t pos = 0;

    // REX prefix (always emit with W=1 for 64-bit)
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(base);
    code[pos++] = x86_calc_rex_prefix(true, needs_rex_r, false, needs_rex_b);

    // Opcode: MOV r64, r/m64
    code[pos++] = X86_OP_MOV_R_RM;

    uint8_t base_bits = x86_reg_bits(base);
    // RSP/R12 (rm=100) forces a SIB, and RBP/R13 (rm=101) cannot be encoded with mod=00.
    bool force_disp8 = (base_bits == 4 || base_bits == 5);
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM :
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;

    code[pos++] = X86_MODRM(mod, x86_reg_bits(dst), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }

    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }

    return write_bytes(gen, code, pos);
}

// MOV r32, [base + offset] 
x86_encode_result_t x86_encode_mov_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset) {
    uint8_t code[10]; // Max size
    uint32_t pos = 0;

    // REX prefix if needed
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(base);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    // Opcode
    code[pos++] = X86_OP_MOV_R_RM;
    
    // ModRM byte - R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    code[pos++] = X86_MODRM(mod, x86_reg_bits(dst), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }
    
    // Displacement
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}

// MOV [base + offset], r32
x86_encode_result_t x86_encode_mov_mem_reg32(x86_codegen_t* gen, x86_reg_t base, int32_t offset, x86_reg_t src) {
    uint8_t code[10]; // Max size
    uint32_t pos = 0;
    
    // REX prefix if needed
    bool needs_rex_r = x86_reg_needs_rex(src);
    bool needs_rex_b = x86_reg_needs_rex(base);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    // Opcode  
    code[pos++] = X86_OP_MOV_RM_R;
    
    // ModRM byte - R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    code[pos++] = X86_MODRM(mod, x86_reg_bits(src), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }
    
    // Displacement
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}


// ADD r32, [base + offset]
x86_encode_result_t x86_encode_add_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset) {
    // Similar to MOV r32, [base + offset] but with ADD opcode
    uint8_t code[10];
    uint32_t pos = 0;
    
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(base);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    code[pos++] = 0x03; // ADD r32, r/m32
    
    // R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    code[pos++] = X86_MODRM(mod, x86_reg_bits(dst), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }
    
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}

// SUB r32, r32
x86_encode_result_t x86_encode_sub_reg32_reg32(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src) {
    uint8_t code[4];
    uint32_t pos = 0;
    
    bool needs_rex_r = x86_reg_needs_rex(src);
    bool needs_rex_b = x86_reg_needs_rex(dst);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    code[pos++] = X86_OP_SUB_RM_R;
    code[pos++] = X86_MODRM(X86_MOD_REG, x86_reg_bits(src), x86_reg_bits(dst));
    
    return write_bytes(gen, code, pos);
}

// SUB r32, [base + offset]  
x86_encode_result_t x86_encode_sub_reg32_mem(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t base, int32_t offset) {
    uint8_t code[10];
    uint32_t pos = 0;
    
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(base);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    code[pos++] = 0x2B; // SUB r32, r/m32
    
    // R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    code[pos++] = X86_MODRM(mod, x86_reg_bits(dst), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }
    
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}

// CMP r32, r32
x86_encode_result_t x86_encode_cmp_reg32_reg32(x86_codegen_t* gen, x86_reg_t reg1, x86_reg_t reg2) {
    uint8_t code[4];
    uint32_t pos = 0;
    
    bool needs_rex_r = x86_reg_needs_rex(reg2);
    bool needs_rex_b = x86_reg_needs_rex(reg1);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    code[pos++] = X86_OP_CMP_RM_R;
    code[pos++] = X86_MODRM(X86_MOD_REG, x86_reg_bits(reg2), x86_reg_bits(reg1));
    
    return write_bytes(gen, code, pos);
}

// CMP r32, [base + offset]
x86_encode_result_t x86_encode_cmp_reg32_mem(x86_codegen_t* gen, x86_reg_t reg, x86_reg_t base, int32_t offset) {
    uint8_t code[10];
    uint32_t pos = 0;
    
    bool needs_rex_r = x86_reg_needs_rex(reg);
    bool needs_rex_b = x86_reg_needs_rex(base);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    code[pos++] = 0x3B; // CMP r32, r/m32
    
    // R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    code[pos++] = X86_MODRM(mod, x86_reg_bits(reg), base_bits);

    if (base_bits == 4) {
        code[pos++] = 0x24; // SIB: scale=0, index=4 (none), base=4 (R12/RSP)
    }
    
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}

// JMP rel32
x86_encode_result_t x86_encode_jmp_rel32(x86_codegen_t* gen, int32_t offset) {
    uint8_t code[5];
    code[0] = X86_OP_JMP_REL32;
    
    // 32-bit offset (little-endian)
    for (int i = 0; i < 4; i++) {
        code[1 + i] = (offset >> (i * 8)) & 0xFF;
    }
    
    return write_bytes(gen, code, 5);
}

// JE rel32 (conditional jump if equal)
x86_encode_result_t x86_encode_je_rel32(x86_codegen_t* gen, int32_t offset) {
    uint8_t code[6];
    code[0] = 0x0F; // Two-byte opcode prefix
    code[1] = X86_OP_JE_REL32;
    
    // 32-bit offset (little-endian)
    for (int i = 0; i < 4; i++) {
        code[2 + i] = (offset >> (i * 8)) & 0xFF;
    }
    
    return write_bytes(gen, code, 6);
}

// JNE rel32 (conditional jump if not equal)
x86_encode_result_t x86_encode_jne_rel32(x86_codegen_t* gen, int32_t offset) {
    uint8_t code[6];
    code[0] = 0x0F; // Two-byte opcode prefix
    code[1] = X86_OP_JNE_REL32;
    
    // 32-bit offset (little-endian) 
    for (int i = 0; i < 4; i++) {
        code[2 + i] = (offset >> (i * 8)) & 0xFF;
    }
    
    return write_bytes(gen, code, 6);
}

// PUSH r64
x86_encode_result_t x86_encode_push_reg64(x86_codegen_t* gen, x86_reg_t reg) {
    uint8_t code[2];
    uint32_t pos = 0;
    
    // REX prefix if needed
    if (x86_reg_needs_rex(reg)) {
        code[pos++] = x86_calc_rex_prefix(false, false, false, true);
    }
    
    // Opcode + register
    code[pos++] = X86_OP_PUSH_R + x86_reg_bits(reg);
    
    return write_bytes(gen, code, pos);
}

// POP r64
x86_encode_result_t x86_encode_pop_reg64(x86_codegen_t* gen, x86_reg_t reg) {
    uint8_t code[2];
    uint32_t pos = 0;
    
    // REX prefix if needed
    if (x86_reg_needs_rex(reg)) {
        code[pos++] = x86_calc_rex_prefix(false, false, false, true);
    }
    
    // Opcode + register
    code[pos++] = X86_OP_POP_R + x86_reg_bits(reg);
    
    return write_bytes(gen, code, pos);
}

// NOP
x86_encode_result_t x86_encode_nop(x86_codegen_t* gen) {
    uint8_t code = X86_OP_NOP;
    return write_bytes(gen, &code, 1);
}

// RET
x86_encode_result_t x86_encode_ret(x86_codegen_t* gen) {
    uint8_t code = X86_OP_RET;
    return write_bytes(gen, &code, 1);
}

// TRAP (UD2 - undefined instruction)
x86_encode_result_t x86_encode_trap(x86_codegen_t* gen) {
    uint8_t code[2] = {0x0F, X86_OP_TRAP};
    return write_bytes(gen, code, 2);
}


// MOVSXD r64, r32 (sign extension) - matches Go's emitMovsxd64
x86_encode_result_t x86_encode_movsxd64(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src) {
    uint8_t code[3];
    uint32_t pos = 0;
    
    // REX.W prefix is required for MOVSXD r64, r/m32.
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(src);
    code[pos++] = x86_calc_rex_prefix(true, needs_rex_r, false, needs_rex_b); // REX.W = 1
    
    // Opcode: MOVSXD
    code[pos++] = 0x63;
    
    // ModRM byte: mod=11 (register), reg=dst, r/m=src
    code[pos++] = X86_MODRM(3, x86_reg_bits(dst), x86_reg_bits(src));
    
    return write_bytes(gen, code, pos);
}

// MOV [base + offset], r64 (for exitCode) - matches Go's encodeMovRegToMem
x86_encode_result_t x86_encode_mov_mem_reg64(x86_codegen_t* gen, x86_reg_t base, int32_t offset, x86_reg_t src) {
    uint8_t code[10];
    uint32_t pos = 0;
    
    // REX.W is required for MOV r/m64, r64.
    bool needs_rex_r = x86_reg_needs_rex(src);
    bool needs_rex_b = x86_reg_needs_rex(base);
    code[pos++] = x86_calc_rex_prefix(true, needs_rex_r, false, needs_rex_b); // REX.W = 1
    
    // Opcode: MOV r/m64, r64
    code[pos++] = 0x89;
    
    // ModRM byte - R12 and RSP always require displacement even when offset=0
    uint8_t base_bits = x86_reg_bits(base);
    bool force_disp8 = (base_bits == 4 || base_bits == 5); // RSP/R12 need SIB, RBP/R13 cannot be mod=00
    uint8_t mod = (offset == 0 && !force_disp8) ? X86_MOD_MEM : 
                  (offset >= -128 && offset <= 127) ? X86_MOD_MEM_DISP8 : X86_MOD_MEM_DISP32;
    
    code[pos++] = X86_MODRM(mod, x86_reg_bits(src), base_bits);
    
    // SIB byte if base register requires it (R12, RSP)
    if (base_bits == 4) {  // R12 or RSP
        code[pos++] = 0x24;  // SIB: scale=0, index=4 (none), base=4
    }
    
    // Displacement
    if (mod == X86_MOD_MEM_DISP8) {
        code[pos++] = (uint8_t)offset;
    } else if (mod == X86_MOD_MEM_DISP32) {
        for (int i = 0; i < 4; i++) {
            code[pos++] = (offset >> (i * 8)) & 0xFF;
        }
    }
    
    return write_bytes(gen, code, pos);
}

// Register-to-register operations (for direct PVM register mapping)

// MOV r32, r32
x86_encode_result_t x86_encode_mov_reg32_reg32(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src) {
    uint8_t code[3];
    uint32_t pos = 0;
    
    // REX prefix if needed
    bool needs_rex_r = x86_reg_needs_rex(src);
    bool needs_rex_b = x86_reg_needs_rex(dst);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    // Opcode: MOV r/m32, r32
    code[pos++] = 0x89;  // MOV r/m32, r32
    
    // ModRM byte: mod=11 (register), reg=src, r/m=dst  
    code[pos++] = X86_MODRM(3, x86_reg_bits(src), x86_reg_bits(dst));
    
    return write_bytes(gen, code, pos);
}

// ADD r32, r32
x86_encode_result_t x86_encode_add_reg32_reg32(x86_codegen_t* gen, x86_reg_t dst, x86_reg_t src) {
    uint8_t code[3];
    uint32_t pos = 0;
    
    // REX prefix if needed
    bool needs_rex_r = x86_reg_needs_rex(dst);
    bool needs_rex_b = x86_reg_needs_rex(src);
    if (needs_rex_r || needs_rex_b) {
        code[pos++] = x86_calc_rex_prefix(false, needs_rex_r, false, needs_rex_b);
    }
    
    // Opcode: ADD r32, r/m32
    code[pos++] = 0x01;  // ADD r/m32, r32
    
    // ModRM byte: mod=11 (register), reg=src, r/m=dst
    code[pos++] = X86_MODRM(3, x86_reg_bits(src), x86_reg_bits(dst));
    
    return write_bytes(gen, code, pos);
}

// Utility functions for PVM operand extraction
void extract_two_registers(const uint8_t* operands, uint8_t* reg1, uint8_t* reg2) {
    if (!operands || !reg1 || !reg2) return;
    *reg1 = operands[0];
    *reg2 = operands[1];
}

void extract_three_registers(const uint8_t* operands, uint8_t* dst, uint8_t* src1, uint8_t* src2) {
    if (!operands || !dst || !src1 || !src2) return;
    // Go implementation: extractThreeRegs(args []byte) (reg1, reg2, dst int)
    // reg1 = min(12, int(args[0]&0x0F))    // Lower 4 bits of args[0] = src1
    // reg2 = min(12, int(args[0]>>4))      // Upper 4 bits of args[0] = src2
    // dst = min(12, int(args[1]))          // Full args[1] = dst
    
    // Go: r1, r2, rd := extractThreeRegs(inst.Args)
    // Go: src1 := regInfoList[r1], src2 := regInfoList[r2], dst := regInfoList[rd]
    
    // Extract registers in Go format - match the exact Go semantics:
    *src1 = (operands[0] & 0x0F) > 12 ? 12 : (operands[0] & 0x0F);        // r1 = reg1 (lower 4 bits)
    *src2 = (operands[0] >> 4) > 12 ? 12 : (operands[0] >> 4);            // r2 = reg2 (upper 4 bits) 
    *dst = operands[1] > 12 ? 12 : operands[1];                           // rd = dst (second byte)
}

int32_t extract_offset(const uint8_t* operands) {
    if (!operands) return 0;
    // Assume 32-bit little-endian offset
    return *(int32_t*)operands;
}

void extract_reg_and_offset(const uint8_t* operands, uint8_t* reg, int32_t* offset) {
    if (!operands || !reg || !offset) return;
    *reg = operands[0];
    *offset = *(int32_t*)(operands + 1);
}

// PVM instruction translators
x86_encode_result_t translate_add_32_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    if (!gen || !inst || !inst->args) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // Extract operands: ADD_32 dst, src1, src2 -> dst = src1 + src2
    uint8_t dst_reg, src1_reg, src2_reg;
    extract_three_registers(inst->args, &dst_reg, &src1_reg, &src2_reg);
    
    // Validate register indices
    if (dst_reg >= 13 || src1_reg >= 13 || src2_reg >= 13) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // Map PVM registers to x86 registers
    x86_reg_t x86_dst = pvm_reg_to_x86[dst_reg];
    x86_reg_t x86_src1 = pvm_reg_to_x86[src1_reg];  
    x86_reg_t x86_src2 = pvm_reg_to_x86[src2_reg];
    
    uint32_t start_offset = gen->offset;
    
    // Generate optimized x86 code based on register allocation:
    if (dst_reg == src1_reg) {
        // dst = dst + src2  →  ADD dst, src2
        x86_encode_add_reg32_reg32(gen, x86_dst, x86_src2);
    } else if (dst_reg == src2_reg) {
        // dst = src1 + dst  →  ADD dst, src1  
        x86_encode_add_reg32_reg32(gen, x86_dst, x86_src1);
    } else {
        // dst = src1 + src2  →  MOV dst, src1; ADD dst, src2
        x86_encode_mov_reg32_reg32(gen, x86_dst, x86_src1);
        x86_encode_add_reg32_reg32(gen, x86_dst, x86_src2);
    }
    
    // Sign extend 32-bit result to 64-bit (matches Go's emitMovsxd64 pattern)
    // This is critical for maintaining proper 64-bit values in registers
    x86_encode_movsxd64(gen, x86_dst, x86_dst);
    
    x86_encode_result_t result;
    result.code = gen->buffer + start_offset;
    result.size = gen->offset - start_offset;
    result.offset = start_offset;
    
    return result;
}

x86_encode_result_t translate_sub_32_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    if (!gen || !inst || !inst->args) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // Extract operands: SUB_32 dst, src1, src2 -> dst = src1 - src2
    uint8_t dst_reg, src1_reg, src2_reg;
    extract_three_registers(inst->args, &dst_reg, &src1_reg, &src2_reg);
    
    if (dst_reg >= 13 || src1_reg >= 13 || src2_reg >= 13) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // Map PVM registers to x86 registers
    x86_reg_t x86_dst = pvm_reg_to_x86[dst_reg];
    x86_reg_t x86_src1 = pvm_reg_to_x86[src1_reg];  
    x86_reg_t x86_src2 = pvm_reg_to_x86[src2_reg];
    
    uint32_t start_offset = gen->offset;
    
    // Generate register-to-register SUB operations (matches Go pattern)
    // Note: SUB is not commutative, so order matters
    if (dst_reg == src1_reg) {
        // dst = dst - src2  →  SUB dst, src2
        x86_encode_sub_reg32_reg32(gen, x86_dst, x86_src2);
    } else if (dst_reg == src2_reg) {
        // dst = src1 - dst  →  MOV tmp, src1; SUB tmp, dst; MOV dst, tmp
        // Since we can't directly do dst = src1 - dst, we need extra steps
        x86_encode_mov_reg32_reg32(gen, X86_RAX, x86_src1);  // tmp = src1
        x86_encode_sub_reg32_reg32(gen, X86_RAX, x86_dst);   // tmp = src1 - dst
        x86_encode_mov_reg32_reg32(gen, x86_dst, X86_RAX);   // dst = tmp
    } else {
        // dst = src1 - src2  →  MOV dst, src1; SUB dst, src2
        x86_encode_mov_reg32_reg32(gen, x86_dst, x86_src1);
        x86_encode_sub_reg32_reg32(gen, x86_dst, x86_src2);
    }
    
    // Sign extend 32-bit result to 64-bit (matches Go's emitMovsxd64 pattern)
    x86_encode_movsxd64(gen, x86_dst, x86_dst);
    
    x86_encode_result_t result;
    result.code = gen->buffer + start_offset;
    result.size = gen->offset - start_offset;
    result.offset = start_offset;
    
    return result;
}

x86_encode_result_t translate_jump_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    if (!gen || !inst) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // JMP rel32 - offset will be patched later during jump resolution
    return x86_encode_jmp_rel32(gen, 0xDEADBEEF); // Placeholder offset
}

x86_encode_result_t translate_branch_eq_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    if (!gen || !inst || !inst->args) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    // Extract operands: BRANCH_EQ reg1, reg2, offset
    uint8_t reg1, reg2;
    extract_two_registers(inst->args, &reg1, &reg2);
    
    if (reg1 >= 13 || reg2 >= 13) {
        x86_encode_result_t result = {0};
        return result;
    }
    
    uint32_t start_offset = gen->offset;
    
    // mov eax, [r12 + reg1*8]   ; Load first register
    // cmp eax, [r12 + reg2*8]   ; Compare with second register  
    // je rel32                  ; Jump if equal (offset patched later)
    
    x86_encode_mov_reg32_mem(gen, X86_RAX, X86_R12, PVM_REG_OFFSET(reg1));
    x86_encode_cmp_reg32_mem(gen, X86_RAX, X86_R12, PVM_REG_OFFSET(reg2));
    x86_encode_je_rel32(gen, 0xDEADBEEF); // Placeholder offset
    
    x86_encode_result_t result;
    result.code = gen->buffer + start_offset;
    result.size = gen->offset - start_offset;
    result.offset = start_offset;
    
    return result;
}

x86_encode_result_t translate_trap_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    (void)inst; // Unused parameter
    return x86_encode_trap(gen);
}

x86_encode_result_t translate_nop_to_x86(x86_codegen_t* gen, const instruction_t* inst) {
    (void)inst; // Unused parameter
    return x86_encode_nop(gen);
}

// Note: pvm_translators_init is now implemented in x86_translators_complete.c

// Simple interface for generating x86 code for single instruction (for testing/comparison)
int generate_single_instruction_x86(uint8_t opcode, const uint8_t* args, size_t args_size, 
                                    uint8_t* output_buffer, size_t buffer_size) {
    if (!output_buffer || buffer_size == 0) {
        return 0;
    }
    
    // Initialize code generator
    x86_codegen_t gen;
    if (x86_codegen_init(&gen, buffer_size) != 0) {
        return 0;
    }
    
    // Create instruction structure
    instruction_t inst = {
        .opcode = opcode,
        .args = (uint8_t*)args,
        .args_size = args_size,
        .step = 0,
        .pc = 0
    };
    
    // Initialize translators if not already done
    pvm_translators_init();
    
    // Get translator function for this opcode
    pvm_to_x86_translator_t translator = pvm_instruction_translators[opcode];
    if (!translator) {
        x86_codegen_cleanup(&gen);
        return 0; // No translator found
    }
    
    // Generate x86 code
    x86_encode_result_t result = translator(&gen, &inst);
    
    // Copy result to output buffer
    int code_size = 0;
    if (result.size > 0 && result.size <= buffer_size && result.code != NULL) {
        memcpy(output_buffer, result.code, result.size);
        code_size = (int)result.size;
    }
    
    x86_codegen_cleanup(&gen);
    return code_size;
}

// Debug and introspection functions
void x86_codegen_print_state(const x86_codegen_t* gen) {
    if (!gen) {
        DEBUG_JIT("x86 CodeGen: NULL\n");
        return;
    }
    
    DEBUG_JIT("x86 CodeGen State:\n");
    DEBUG_JIT("  Buffer: %p\n", (void*)gen->buffer);
    DEBUG_JIT("  Size: %u bytes\n", gen->size);
    DEBUG_JIT("  Capacity: %u bytes\n", gen->capacity);
    DEBUG_JIT("  Offset: %u\n", gen->offset);
}

void x86_codegen_print_buffer(const x86_codegen_t* gen, uint32_t start, uint32_t length) {
    if (!gen || !gen->buffer || start >= gen->size) {
        DEBUG_JIT("Invalid buffer or range\n");
        return;
    }
    
    uint32_t end = (start + length > gen->size) ? gen->size : start + length;
    
    DEBUG_JIT("x86 Code [%u:%u]: ", start, end);
    for (uint32_t i = start; i < end; i++) {
        DEBUG_JIT("%02X ", gen->buffer[i]);
    }
    DEBUG_JIT("\n");
}

int x86_codegen_validate_state(const x86_codegen_t* gen) {
    if (!gen) return -1;
    if (!gen->buffer && gen->capacity > 0) return -2;
    if (gen->size > gen->capacity) return -3;
    if (gen->offset > gen->capacity) return -4;
    if (gen->offset < gen->size) return -5; // offset should be >= size
    
    return 0;
}

// Generate JMP rel32 with placeholder (matches Go's entryPatch pattern)
x86_encode_result_t x86_encode_jmp_rel32_placeholder(x86_codegen_t* gen) {
    x86_encode_result_t result = {0};
    if (!gen) return result;

    // Match Go's pattern: X86_OP_JMP_REL32 + 0x99999999 placeholder
    const uint32_t ENTRY_PATCH_PLACEHOLDER = 0x99999999; // Go's entryPatch constant

    if (gen->size + 5 > gen->capacity) {
        DEBUG_JIT("x86_encode_jmp_rel32_placeholder: buffer overflow\n");
        return result;
    }

    // JMP rel32 opcode: 0xE9
    gen->buffer[gen->size++] = 0xE9;

    // Add placeholder bytes (little endian)
    gen->buffer[gen->size++] = (ENTRY_PATCH_PLACEHOLDER >> 0) & 0xFF;
    gen->buffer[gen->size++] = (ENTRY_PATCH_PLACEHOLDER >> 8) & 0xFF;
    gen->buffer[gen->size++] = (ENTRY_PATCH_PLACEHOLDER >> 16) & 0xFF;
    gen->buffer[gen->size++] = (ENTRY_PATCH_PLACEHOLDER >> 24) & 0xFF;

    result.size = 5; // 1 byte opcode + 4 byte offset

    DEBUG_JIT("x86_encode_jmp_rel32_placeholder: generated JMP with placeholder 0x%X\n",
           ENTRY_PATCH_PLACEHOLDER);

    return result;
}

// Note: x86_generate_start_code and x86_generate_exit_code have been removed
// as they are part of VM execution, not compilation. In the simplified architecture,
// these are handled by the Go execution layer.

// Context slot functions (equivalent to Go's BuildWriteContextSlotCode)
x86_encode_result_t x86_encode_write_context_slot(x86_codegen_t* gen,
                                                  uintptr_t reg_dump_addr,
                                                  int slot_index,
                                                  uint64_t value,
                                                  int size) {
    x86_encode_result_t result = {0, 0};

    if (!gen) {
        DEBUG_JIT("ERROR: x86_encode_write_context_slot - null generator\n");
        return result;
    }

    if (size != 4 && size != 8) {
        DEBUG_JIT("ERROR: x86_encode_write_context_slot - unsupported size %d\n", size);
        return result;
    }

    // Calculate target address: reg_dump_addr + slot_index * 8
    uintptr_t target = reg_dump_addr + (slot_index * 8);

    DEBUG_JIT("DEBUG: Writing context slot %d (value=%lu, size=%d) to address 0x%lx\n",
           slot_index, value, size, target);

    uint32_t initial_offset = gen->offset;

    // Step 1: Push RAX to save it (matches Go)
    x86_encode_result_t push_result = x86_encode_push_reg64(gen, X86_RAX);
    if (push_result.size == 0) {
        DEBUG_JIT("ERROR: Failed to push RAX\n");
        return result;
    }

    // Step 2: Load value into RAX (matches Go)
    x86_encode_result_t mov_result;
    if (size == 8) {
        // MOV RAX, imm64 (matches Go: 48 B8 imm64)
        mov_result = x86_encode_mov_reg64_imm64(gen, X86_RAX, value);
    } else {
        // MOV RAX, imm32 (zero-extended to 64-bit, matches Go: B8 imm32)
        mov_result = x86_encode_mov_reg64_imm64(gen, X86_RAX, (uint32_t)value);
    }

    if (mov_result.size == 0) {
        DEBUG_JIT("ERROR: Failed to load value into RAX\n");
        return result;
    }

    // Step 3: Store RAX to absolute address (matches Go)
    // This is the tricky part - we need to generate MOV [absolute], RAX
    // Go uses: 48 A3 moffs64 (MOV [moffs64], rax) or A3 moffs64 (MOV [moffs64], eax)

    if (size == 8) {
        // 48 A3 moffs64 (MOV [moffs64], rax)
        if (gen->offset + 10 > gen->capacity) {
            DEBUG_JIT("ERROR: Buffer overflow during context slot write\n");
            return result;
        }
        gen->buffer[gen->offset++] = 0x48; // REX.W
        gen->buffer[gen->offset++] = 0xA3; // MOV [moffs64], rax
        // Write 64-bit absolute address in little endian
        for (int i = 0; i < 8; i++) {
            gen->buffer[gen->offset++] = (target >> (i * 8)) & 0xFF;
        }
    } else {
        // A3 moffs64 (MOV [moffs64], eax)
        if (gen->offset + 9 > gen->capacity) {
            DEBUG_JIT("ERROR: Buffer overflow during context slot write\n");
            return result;
        }
        gen->buffer[gen->offset++] = 0xA3; // MOV [moffs64], eax
        // Write 64-bit absolute address in little endian (even for 32-bit store)
        for (int i = 0; i < 8; i++) {
            gen->buffer[gen->offset++] = (target >> (i * 8)) & 0xFF;
        }
    }

    // Step 4: Pop RAX to restore it (matches Go)
    x86_encode_result_t pop_result = x86_encode_pop_reg64(gen, X86_RAX);
    if (pop_result.size == 0) {
        DEBUG_JIT("ERROR: Failed to pop RAX\n");
        return result;
    }

    // Calculate total size
    result.size = gen->offset - initial_offset;

    DEBUG_JIT("DEBUG: Generated %u bytes for context slot %d write\n", result.size, slot_index);

    return result;
}

// Increment qword at absolute address (matches Go generateIncMem behaviour)
x86_encode_result_t x86_encode_inc_mem64_abs(x86_codegen_t* gen, uint64_t addr) {
    x86_encode_result_t result = {0};
    if (!gen) {
        return result;
    }

    x86_encode_result_t push_res = x86_encode_push_reg64(gen, X86_RCX);
    if (push_res.size == 0) {
        return result;
    }

    x86_encode_result_t mov_res = x86_encode_mov_reg64_imm64(gen, X86_RCX, addr);
    if (mov_res.size == 0) {
        return result;
    }

    if (x86_codegen_ensure_capacity(gen, 3) != 0) {
        return result;
    }

    gen->buffer[gen->offset++] = 0x48; // REX.W
    gen->buffer[gen->offset++] = 0xFF; // INC/DEC opcode group
    gen->buffer[gen->offset++] = 0x01; // INC qword ptr [RCX]

    x86_encode_result_t pop_res = x86_encode_pop_reg64(gen, X86_RCX);
    if (pop_res.size == 0) {
        return result;
    }

    result.offset = push_res.offset;
    result.code = gen->buffer + result.offset;
    result.size = gen->offset - result.offset;
    return result;
}
