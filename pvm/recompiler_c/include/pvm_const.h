#ifndef PVM_CONST_H
#define PVM_CONST_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

// Register and architecture constants
#define REG_SIZE 13

#define W_X 3072  // MaxExports - GP 0.7.2
#define M   128
#define V   1023
#define Z_A 2

#define Z_P (1 << 12)
#define Z_Q (1 << 16)
#define Z_I (1 << 24)
#define Z_Z (1 << 16)

// Memory constants
#define PageSize 4096  // 4KB page size
#define PAGE_INACCESSIBLE 0
#define PAGE_MUTABLE      1
#define PAGE_IMMUTABLE    2

// Helper functions matching Go utils
uint32_t ceiling_divide(uint32_t a, uint32_t b);
uint32_t P_func(uint32_t x);
uint32_t Z_func(uint32_t x);

// Debug and logging flags (can be modified at runtime)
extern bool pvm_logging;
extern bool pvm_trace;
extern bool pvm_trace2;
extern bool show_disassembly;
extern bool use_ecalli500;
extern bool debug_recompiler;
extern bool use_tally;

// Error codes and special values
#define NONE ((uint64_t)-1)  // 0xFFFFFFFFFFFFFFFF - Error: None/Invalid
#define WHAT ((uint64_t)-2)  // 0xFFFFFFFFFFFFFFFE - Error: What
#define OOB  ((uint64_t)-3)  // 0xFFFFFFFFFFFFFFFD - Error: Out of bounds
#define WHO  ((uint64_t)-4)  // 0xFFFFFFFFFFFFFFFC - Error: Who
#define FULL ((uint64_t)-5)  // 0xFFFFFFFFFFFFFFFB - Error: Full
#define CORE ((uint64_t)-6)  // 0xFFFFFFFFFFFFFFFA - Error: Core
#define CASH ((uint64_t)-7)  // 0xFFFFFFFFFFFFFFF9 - Error: Cash
#define LOW  ((uint64_t)-8)  // 0xFFFFFFFFFFFFFFF8 - Error: Low
#define HUH  ((uint64_t)-9)  // 0xFFFFFFFFFFFFFFF7 - Error: Huh
#define OK   0               // 0x0000000000000000 - Success

// Jump types for basic blocks
typedef enum {
    TRAP_JUMP        = 0,
    DIRECT_JUMP      = 1,
    INDIRECT_JUMP    = 2,
    CONDITIONAL      = 3,
    FALLTHROUGH_JUMP = 4,
    TERMINATED       = 5
} jump_type_t;

// Appendix A - Instructions

// A.5.1. Instructions without Arguments
#define TRAP        0
#define FALLTHROUGH 1

// A.5.2. Instructions with Arguments of One Immediate
#define ECALLI 10  // 0x0a

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
#define LOAD_IMM_64 20  // 0x14

// A.5.4. Instructions with Arguments of Two Immediates
#define STORE_IMM_U8  30
#define STORE_IMM_U16 31
#define STORE_IMM_U32 32
#define STORE_IMM_U64 33  // NEW, 32-bit twin = store_imm_u32

// A.5.5. Instructions with Arguments of One Offset
#define JUMP 40  // 0x28

// A.5.6. Instructions with Arguments of One Register & Two Immediates
#define JUMP_IND  50
#define LOAD_IMM  51
#define LOAD_U8   52
#define LOAD_I8   53
#define LOAD_U16  54
#define LOAD_I16  55
#define LOAD_U32  56
#define LOAD_I32  57
#define LOAD_U64  58
#define STORE_U8  59
#define STORE_U16 60
#define STORE_U32 61
#define STORE_U64 62

// A.5.7. Instructions with Arguments of One Register & Two Immediates
#define STORE_IMM_IND_U8  70  // 0x46
#define STORE_IMM_IND_U16 71  // 0x47
#define STORE_IMM_IND_U32 72  // 0x48
#define STORE_IMM_IND_U64 73  // 0x49 NEW, 32-bit twin = store_imm_ind_u32

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
#define LOAD_IMM_JUMP   80
#define BRANCH_EQ_IMM   81  // 0x51
#define BRANCH_NE_IMM   82
#define BRANCH_LT_U_IMM 83
#define BRANCH_LE_U_IMM 84
#define BRANCH_GE_U_IMM 85
#define BRANCH_GT_U_IMM 86
#define BRANCH_LT_S_IMM 87
#define BRANCH_LE_S_IMM 88
#define BRANCH_GE_S_IMM 89
#define BRANCH_GT_S_IMM 90

// A.5.9. Instructions with Arguments of Two Registers
#define MOVE_REG              100
#define SBRK                  101
#define COUNT_SET_BITS_64     102
#define COUNT_SET_BITS_32     103
#define LEADING_ZERO_BITS_64  104
#define LEADING_ZERO_BITS_32  105
#define TRAILING_ZERO_BITS_64 106
#define TRAILING_ZERO_BITS_32 107
#define SIGN_EXTEND_8         108
#define SIGN_EXTEND_16        109
#define ZERO_EXTEND_16        110
#define REVERSE_BYTES         111

// A.5.10. Instructions with Arguments of Two Registers & One Immediate
#define STORE_IND_U8      120
#define STORE_IND_U16     121
#define STORE_IND_U32     122
#define STORE_IND_U64     123
#define LOAD_IND_U8       124
#define LOAD_IND_I8       125
#define LOAD_IND_U16      126
#define LOAD_IND_I16      127
#define LOAD_IND_U32      128
#define LOAD_IND_I32      129
#define LOAD_IND_U64      130
#define ADD_IMM_32        131
#define AND_IMM           132
#define XOR_IMM           133
#define OR_IMM            134
#define MUL_IMM_32        135
#define SET_LT_U_IMM      136
#define SET_LT_S_IMM      137
#define SHLO_L_IMM_32     138
#define SHLO_R_IMM_32     139
#define SHAR_R_IMM_32     140
#define NEG_ADD_IMM_32    141
#define SET_GT_U_IMM      142
#define SET_GT_S_IMM      143
#define SHLO_L_IMM_ALT_32 144
#define SHLO_R_IMM_ALT_32 145
#define SHAR_R_IMM_ALT_32 146
#define CMOV_IZ_IMM       147
#define CMOV_NZ_IMM       148
#define ADD_IMM_64        149
#define MUL_IMM_64        150
#define SHLO_L_IMM_64     151
#define SHLO_R_IMM_64     152
#define SHAR_R_IMM_64     153
#define NEG_ADD_IMM_64    154
#define SHLO_L_IMM_ALT_64 155
#define SHLO_R_IMM_ALT_64 156
#define SHAR_R_IMM_ALT_64 157
#define ROT_R_64_IMM      158
#define ROT_R_64_IMM_ALT  159
#define ROT_R_32_IMM      160
#define ROT_R_32_IMM_ALT  161

// A.5.11. Instructions with Arguments of Two Registers & One Offset
#define BRANCH_EQ   170
#define BRANCH_NE   171
#define BRANCH_LT_U 172
#define BRANCH_LT_S 173
#define BRANCH_GE_U 174
#define BRANCH_GE_S 175

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
#define LOAD_IMM_JUMP_IND 180

// A.5.13. Instructions with Arguments of Three Registers
#define ADD_32        190
#define SUB_32        191
#define MUL_32        192
#define DIV_U_32      193
#define DIV_S_32      194
#define REM_U_32      195
#define REM_S_32      196
#define SHLO_L_32     197
#define SHLO_R_32     198
#define SHAR_R_32     199
#define ADD_64        200
#define SUB_64        201
#define MUL_64        202
#define DIV_U_64      203
#define DIV_S_64      204
#define REM_U_64      205
#define REM_S_64      206
#define SHLO_L_64     207
#define SHLO_R_64     208
#define SHAR_R_64     209
#define AND           210
#define XOR           211
#define OR            212
#define MUL_UPPER_S_S 213
#define MUL_UPPER_U_U 214
#define MUL_UPPER_S_U 215
#define SET_LT_U      216
#define SET_LT_S      217
#define CMOV_IZ       218
#define CMOV_NZ       219
#define ROT_L_64      220
#define ROT_L_32      221
#define ROT_R_64      222
#define ROT_R_32      223
#define AND_INV       224
#define OR_INV        225
#define XNOR          226
#define MAX           227
#define MAX_U         228
#define MIN           229
#define MIN_U         230

// Appendix B - Host Functions
// Host function indexes for ECALLI instruction
#define HOST_GAS               0
#define HOST_FETCH             1   // NEW
#define HOST_LOOKUP            2
#define HOST_READ              3
#define HOST_WRITE             4
#define HOST_INFO              5   // ADJ
#define HOST_HISTORICAL_LOOKUP 6
#define HOST_EXPORT            7
#define HOST_MACHINE           8
#define HOST_PEEK              9
#define HOST_POKE              10
#define HOST_PAGES             11  // NEW
#define HOST_INVOKE            12
#define HOST_EXPUNGE           13
#define HOST_BLESS             14
#define HOST_ASSIGN            15
#define HOST_DESIGNATE         16
#define HOST_CHECKPOINT        17
#define HOST_NEW               18
#define HOST_UPGRADE           19
#define HOST_TRANSFER          20
#define HOST_EJECT             21
#define HOST_QUERY             22
#define HOST_SOLICIT           23
#define HOST_FORGET            24
#define HOST_YIELD             25
#define HOST_PROVIDE           26

#define HOST_MANIFEST          64  // Not in 0.6.7
#define HOST_LOG               100

// Memory management constants (from recompiler.go)
#define PAGE_INACCESSIBLE 0
#define PAGE_MUTABLE      1
#define PAGE_IMMUTABLE    2

#define DUMP_SIZE   0x100000                   // 1MB
#define PAGE_SIZE   4096                       // 4 KiB
#define TOTAL_MEM   (4ULL * 1024 * 1024 * 1024) // 4 GiB
#define TOTAL_PAGES (TOTAL_MEM / PAGE_SIZE)    // 1,048,576 pages

// VM execution constants
#define HOST_CALL 4
#define PANIC_CALL 2

// VM context slot indices (matches Go's recompiler.go)
#define GAS_SLOT_INDEX           14  // Gas counter is at index 14
#define PC_SLOT_INDEX            15  // PC is at index 15
#define BLOCK_COUNTER_SLOT_INDEX 16  // Block counter is at index 16
#define INDIRECT_JUMP_POINT_SLOT 20  // Indirect jump point is at index 20
#define NEXT_X86_SLOT_INDEX      21  // Next x86 instruction address is at index 21 : for sbrk and ecalli
#define SBRK_A_INDEX             22  // Sbrk A is at index 22
#define SBRK_D_INDEX             23  // Sbrk D is at index 23
#define VM_STATE_SLOT_INDEX      30  // VM state is at index 30
#define HOST_FUNC_ID_INDEX       31  // Host function ID is at index 31
#define RIP_SLOT_INDEX           32  // RIP is at index 32

// Utility functions for opcode information
const char* opcode_to_string(uint8_t opcode);
bool is_basic_block_terminator(uint8_t opcode);
bool is_branch_instruction(uint8_t opcode);
bool is_memory_instruction(uint8_t opcode);
int get_instruction_arg_count(uint8_t opcode);

// Memory utility functions
uint32_t ceiling_divide(uint32_t a, uint32_t b);
uint32_t P_func(uint32_t x);
uint32_t Z_func(uint32_t x);

// Initialize debugging flags
void pvm_const_init(void);

#ifdef __cplusplus
}
#endif

#endif // PVM_CONST_H