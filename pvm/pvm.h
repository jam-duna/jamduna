#ifndef PVM_H
#define PVM_H

#include <stdint.h>
#include <limits.h>
#include <stdio.h>
#include <string.h>
#include <stddef.h>

// ===============================
// VM Structure and Constants
// ===============================

#define REG_SIZE 13
#define Z_P (1 << 12)  // Page size constant
#define Z_A 2          // Alignment constant
#define OK 0           // Success code
#define OOB ((uint64_t)((1ULL << 32) - 3))  // Out of bounds error

// PVM-specific constants not in pvm.h
#define W_X 1024
#define M 128
#define V 1023
#define Z_Q (1 << 16)
#define Z_I (1 << 24)
#define Z_Z (1 << 16)

// Result codes
#define NONE ((uint64_t)((1ULL << 32) - 1))
#define WHAT ((uint64_t)((1ULL << 32) - 2))
#define OOB  ((uint64_t)((1ULL << 32) - 3))
#define WHO  ((uint64_t)((1ULL << 32) - 4))
#define FULL ((uint64_t)((1ULL << 32) - 5))
#define CORE ((uint64_t)((1ULL << 32) - 6))
#define CASH ((uint64_t)((1ULL << 32) - 7))
#define LOW  ((uint64_t)((1ULL << 32) - 8))
#define HUH  ((uint64_t)((1ULL << 32) - 9))

// Machine states
#define HALT  0
#define PANIC 1
#define FAULT 2
#define HOST  3
#define OOG   4

// Work digest codes (must match types/workresult.go)
#define WORKDIGEST_OK         0
#define WORKDIGEST_OOG        1
#define WORKDIGEST_PANIC      2
#define WORKDIGEST_BAD_EXPORT 3
#define WORKDIGEST_OVERSIZE   4
#define WORKDIGEST_BAD        5
#define WORKDIGEST_BIG        6


// VM structure forward declaration
typedef struct VM VM;

struct VM {
    uint64_t registers[REG_SIZE];
    uint64_t pc;
    uint8_t* code;
    uint8_t* bitmask;
    uint32_t* ram_read_write;
    uint32_t* ram_read_only;
    uint64_t gas;
    int host_call;
    int host_func_id;
    int is_child;
    uint32_t heap_pointer;
    
    // Memory regions
    uint8_t* rw_data;
    uint8_t* ro_data;
    uint8_t* output;
    uint8_t* stack;
    uint32_t rw_data_address;
    uint32_t rw_data_address_end;
    uint32_t ro_data_address;
    uint32_t ro_data_address_end;
    uint32_t output_address;
    uint32_t output_end;
    uint32_t stack_address;
    uint32_t stack_address_end;
    uint32_t current_heap_pointer;
    
    // Code and data segments
    uint32_t code_len;
    uint32_t bitmask_len;
    uint8_t* o_byte;
    uint32_t o_size;
    uint8_t* w_byte;
    uint32_t w_size;
    uint32_t z_val;
    
    // VM state
    int result_code;
    int machine_state;
    int terminated;
    uint32_t fault_address;
    uint32_t j_size;
    uint32_t* j;
    uint32_t service_index;
    uint32_t core_index;
    
    // CGO Integration - Go memory pointers (shared)
    uint8_t* go_rw_data;            // Pointer to Go's rw_data slice
    uint8_t* go_ro_data;            // Pointer to Go's ro_data slice  
    uint8_t* go_output;             // Pointer to Go's output slice
    uint8_t* go_stack;              // Pointer to Go's stack slice
    
    // Go callback function pointer for host calls
    void (*go_invoke_host_func)(struct VM* vm, int host_func_id);
    
    // State synchronization
    int sync_registers_to_go;       // Flag to sync registers back to Go
};

// ===============================
// Utility Macros
// ===============================

#define MIN(a, b) ((a) < (b) ? (a) : (b))
#define MAX(a, b) ((a) > (b) ? (a) : (b))

// ===============================
// Debug and Tracing
// ===============================

#define PREFIX_TRACE "TRACE polkavm::interpreter"

// Global tracing flags
#define PVM_TRACE 0  // Disable tracing for maximum performance
extern int pvm_trace;
extern int pvm_interpretation;

// ===============================
// Opcode Constants
// ===============================

// A.5.1. Instructions without Arguments
#define TRAP                0
#define FALLTHROUGH         1

// A.5.2. Instructions with Arguments of One Immediate  
#define ECALLI              10  // 0x0a

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
#define LOAD_IMM_64         20  // 0x14

// A.5.4. Instructions with Arguments of Two Immediates
#define STORE_IMM_U8        30
#define STORE_IMM_U16       31
#define STORE_IMM_U32       32
#define STORE_IMM_U64       33

// A.5.5. Instructions with Arguments of One Offset
#define JUMP                40  // 0x28

// A.5.6. Instructions with Arguments of One Register & One Immediate
#define JUMP_IND            50
#define LOAD_IMM            51
#define LOAD_U8             52
#define LOAD_I8             53
#define LOAD_U16            54
#define LOAD_I16            55
#define LOAD_U32            56
#define LOAD_I32            57
#define LOAD_U64            58
#define STORE_U8            59
#define STORE_U16           60
#define STORE_U32           61
#define STORE_U64           62

// A.5.7. Instructions with Arguments of One Register & Two Immediates
#define STORE_IMM_IND_U8    70  // 0x46
#define STORE_IMM_IND_U16   71  // 0x47
#define STORE_IMM_IND_U32   72  // 0x48
#define STORE_IMM_IND_U64   73  // 0x49

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
#define LOAD_IMM_JUMP       80
#define BRANCH_EQ_IMM       81  // 0x51
#define BRANCH_NE_IMM       82
#define BRANCH_LT_U_IMM     83
#define BRANCH_LE_U_IMM     84
#define BRANCH_GE_U_IMM     85
#define BRANCH_GT_U_IMM     86
#define BRANCH_LT_S_IMM     87
#define BRANCH_LE_S_IMM     88
#define BRANCH_GE_S_IMM     89
#define BRANCH_GT_S_IMM     90

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

// A.5.10. Instructions with Arguments of Two Registers and One Immediate
#define STORE_IND_U8          120
#define STORE_IND_U16         121
#define STORE_IND_U32         122
#define STORE_IND_U64         123
#define LOAD_IND_U8           124
#define LOAD_IND_I8           125
#define LOAD_IND_U16          126
#define LOAD_IND_I16          127
#define LOAD_IND_U32          128
#define LOAD_IND_I32          129
#define LOAD_IND_U64          130
#define ADD_IMM_32            131
#define AND_IMM               132
#define XOR_IMM               133
#define OR_IMM                134
#define MUL_IMM_32            135
#define SET_LT_U_IMM          136
#define SET_LT_S_IMM          137
#define SHLO_L_IMM_32         138
#define SHLO_R_IMM_32         139
#define SHAR_R_IMM_32         140
#define NEG_ADD_IMM_32        141
#define SET_GT_U_IMM          142
#define SET_GT_S_IMM          143
#define SHLO_L_IMM_ALT_32     144
#define SHLO_R_IMM_ALT_32     145
#define SHAR_R_IMM_ALT_32     146
#define CMOV_IZ_IMM           147
#define CMOV_NZ_IMM           148
#define ADD_IMM_64            149
#define MUL_IMM_64            150
#define SHLO_L_IMM_64         151
#define SHLO_R_IMM_64         152
#define SHAR_R_IMM_64         153
#define NEG_ADD_IMM_64        154
#define SHLO_L_IMM_ALT_64     155
#define SHLO_R_IMM_ALT_64     156
#define SHAR_R_IMM_ALT_64     157
#define ROT_R_64_IMM          158
#define ROT_R_64_IMM_ALT      159
#define ROT_R_32_IMM          160
#define ROT_R_32_IMM_ALT      161

// A.5.11. Instructions with Arguments of Two Registers and One Offset
#define BRANCH_EQ             170
#define BRANCH_NE             171
#define BRANCH_LT_U           172
#define BRANCH_LT_S           173
#define BRANCH_GE_U           174
#define BRANCH_GE_S           175

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
#define LOAD_IMM_JUMP_IND     180

// A.5.13. Instructions with Arguments of Three Registers
#define ADD_32                190
#define SUB_32                191
#define MUL_32                192
#define DIV_U_32              193
#define DIV_S_32              194
#define REM_U_32              195
#define REM_S_32              196
#define SHLO_L_32             197
#define SHLO_R_32             198
#define SHAR_R_32             199
#define ADD_64                200
#define SUB_64                201
#define MUL_64                202
#define DIV_U_64              203
#define DIV_S_64              204
#define REM_U_64              205
#define REM_S_64              206
#define SHLO_L_64             207
#define SHLO_R_64             208
#define SHAR_R_64             209
#define AND                   210
#define XOR                   211
#define OR                    212
#define MUL_UPPER_S_S         213
#define MUL_UPPER_U_U         214
#define MUL_UPPER_S_U         215
#define SET_LT_U              216
#define SET_LT_S              217
#define CMOV_IZ               218
#define CMOV_NZ               219
#define ROT_L_64              220
#define ROT_L_32              221
#define ROT_R_64              222
#define ROT_R_32              223
#define AND_INV               224
#define OR_INV                225
#define XNOR                  226
#define MAX_                  227
#define MAX_U                 228
#define MIN_                  229
#define MIN_U                 230

// VM Core functions
void vm_panic(VM* vm, uint64_t err_code);
void vm_branch(VM* vm, uint64_t target, int condition);
void vm_djump(VM* vm, uint64_t target);
int vm_charge_gas(VM* vm, int host_func_id);

// key trampoline
void vm_invoke_host_call(VM* vm, int host_func_id);

// CGO Integration Functions
VM* pvm_create(uint32_t service_index, uint8_t* code, size_t code_len, uint64_t* initial_regs, size_t num_regs, uint64_t initial_pc);

void pvm_destroy(VM* vm);

// Bitmask setup (called from Go)
void pvm_set_bitmask(VM* vm, uint8_t* bitmask, size_t bitmask_len);

// Jump table setup (called from Go)
void pvm_set_jump_table(VM* vm, uint32_t* j_table, size_t j_size);

// Memory region setup (called from Go)
void pvm_set_memory_regions(VM* vm, uint8_t* rw_data, uint32_t rw_size, uint8_t* ro_data, uint32_t ro_size, uint8_t* output, uint32_t output_size, uint8_t* stack, uint32_t stack_size);

// Set memory addresses for bounds checking
void pvm_set_memory_bounds(VM* vm, uint32_t rw_addr, uint32_t rw_end, uint32_t ro_addr, uint32_t ro_end, uint32_t output_addr, uint32_t output_end, uint32_t stack_addr, uint32_t stack_end);

// Register access
uint64_t* pvm_get_registers_ptr(VM* vm);
void pvm_set_register(VM* vm, int reg_idx, uint64_t value);
uint64_t pvm_get_register(VM* vm, int reg_idx);

// Execution
int pvm_execute(VM* vm, int entry_point, int is_child);

// Host call callback setup  
void pvm_set_host_callback(VM* vm, void (*callback)(VM* vm, int host_func_id));


const char* branch_cond_symbol(const char* name);
const char* reg_name(int index);

// Memory access functions
uint64_t vm_write_ram_bytes_8(VM* vm, uint32_t address, uint8_t data);
uint64_t vm_write_ram_bytes_16(VM* vm, uint32_t address, uint16_t data);
uint64_t vm_write_ram_bytes_32(VM* vm, uint32_t address, uint32_t data);
uint64_t vm_write_ram_bytes_64(VM* vm, uint32_t address, uint64_t data);
uint64_t vm_write_ram_bytes(VM* vm, uint32_t address, uint8_t* data, uint32_t length);
uint64_t vm_read_ram_bytes(VM* vm, uint32_t address, uint32_t length, uint8_t* buffer);
uint8_t vm_read_ram_bytes_8(VM* vm, uint32_t addr, int* err_code);
uint16_t vm_read_ram_bytes_16(VM* vm, uint32_t addr, int* err_code);
uint32_t vm_read_ram_bytes_32(VM* vm, uint32_t addr, int* err_code);
uint64_t vm_read_ram_bytes_64(VM* vm, uint32_t addr, int* err_code);
uint32_t vm_get_current_heap_pointer(VM* vm);
void vm_set_current_heap_pointer(VM* vm, uint32_t pointer);
void vm_allocate_pages(VM* vm, uint32_t start_page, uint32_t page_count);
uint32_t p_func(uint32_t x);

// Debug/trace functions
void dump_store_generic(const char* op, uint64_t addr, const char* reg_or_src, uint64_t value, int bits);
void dump_load_imm_jump(const char* op, int register_index_a, uint64_t vx);
void dump_load_generic(const char* op, int reg_a, uint64_t addr_or_vx, uint64_t value, int bits, int is_signed);
void dump_load_imm(const char* op, int reg_a, uint64_t addr_or_vx, uint64_t value, int bits, int is_signed);
void dump_mov(int reg_d, int reg_a, uint64_t result);
void dump_three_reg_op(const char* opname, int reg_d, int reg_a, int reg_b, uint64_t val_a, uint64_t val_b, uint64_t result);
void dump_bin_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t result);
void dump_cmp_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t result);
void dump_shift_op(const char* name, int reg_a, int reg_b, uint64_t shift, uint64_t result);
void dump_rot_op(const char* op, const char* reg_dst, const char* src, uint64_t shift, uint64_t result);
void dump_cmov_op(const char* name, int reg_a, int reg_b, uint64_t vx, uint64_t val_a, uint64_t result, int is_zero);
void dump_jump_offset(const char* op, int64_t offset, uint64_t pc);
void dump_branch(const char* name, int reg_a, int reg_b, uint64_t value_a, uint64_t value_b, uint64_t vx, int taken);
void dump_branch_imm(const char* name, int reg_a, uint64_t reg_val, uint64_t vx, uint64_t vy, int is_signed, int taken);
void dump_two_regs(const char* op, int dest_reg, int src_reg, uint64_t value_a, uint64_t result);

// Dispatch table, initializer and handlers
extern void (*dispatch_table[256])(VM*, uint8_t*, size_t);
void init_dispatch_table(void);
extern void handle_TRAP(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_FALLTHROUGH(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ECALLI(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_JUMP(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IMM_JUMP_IND(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_JUMP_IND(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_I8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_I16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_I32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_IND_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_IND_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_IND_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IMM_IND_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IMM_JUMP(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_EQ_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_NE_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GE_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GE_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MOVE_REG(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SBRK(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_COUNT_SET_BITS_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_COUNT_SET_BITS_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LEADING_ZERO_BITS_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LEADING_ZERO_BITS_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_TRAILING_ZERO_BITS_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_TRAILING_ZERO_BITS_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SIGN_EXTEND_8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SIGN_EXTEND_16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ZERO_EXTEND_16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_REVERSE_BYTES(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IND_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IND_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IND_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_STORE_IND_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_U8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_I8(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_U16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_I16(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_U32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_I32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_LOAD_IND_U64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ADD_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ADD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_AND_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_XOR_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_OR_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_LT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_LT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_GT_U_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_GT_S_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_NEG_ADD_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_NEG_ADD_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_IMM_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_IMM_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_IMM_ALT_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_IMM_ALT_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_64_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_64_IMM_ALT(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_32_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_32_IMM_ALT(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_CMOV_IZ_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_CMOV_NZ_IMM(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_EQ(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_NE(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LT_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_LT_S(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GE_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_BRANCH_GE_S(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ADD_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SUB_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_DIV_U_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_DIV_S_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_REM_U_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_REM_S_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ADD_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SUB_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_DIV_U_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_DIV_S_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_REM_U_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_REM_S_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_L_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHLO_R_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SHAR_R_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_AND(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_XOR(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_OR(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_UPPER_S_S(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_UPPER_U_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MUL_UPPER_S_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_LT_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_SET_LT_S(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_CMOV_IZ(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_CMOV_NZ(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_L_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_L_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_64(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_ROT_R_32(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_AND_INV(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_OR_INV(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_XNOR(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MAX(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MAX_U(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MIN(VM* vm, uint8_t* operands, size_t operand_len);
extern void handle_MIN_U(VM* vm, uint8_t* operands, size_t operand_len);

#endif // PVM_H