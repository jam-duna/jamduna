#include "compiler.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

// Helper: power of two
static inline uint64_t power_of_two(uint32_t exp) {
    return (uint64_t)1 << exp;
}

// Decode little-endian (matches Go's DecodeE_l)
uint64_t decode_e_l(const uint8_t* bytes, int len) {
    uint64_t result = 0;
    for (int i = 0; i < len && i < 8; i++) {
        result |= ((uint64_t)bytes[i]) << (8 * i);
    }
    return result;
}

// Decode encoded integer (matches Go's DecodeE - GP v0.3.6 eq(272))
typedef struct {
    uint64_t value;
    uint32_t bytes_read;
} decode_result_t;

decode_result_t decode_e(const uint8_t* encoded, uint32_t max_len) {
    decode_result_t result = {0, 0};

    if (max_len == 0) {
        return result;
    }

    uint8_t first_byte = encoded[0];

    // Special case: 0 encodes to [0]
    if (first_byte == 0) {
        result.value = 0;
        result.bytes_read = 1;
        return result;
    }

    // Special case: max value uses 0xFF + 8 bytes
    if (first_byte == 255) {
        if (max_len < 9) {
            return result; // Not enough bytes
        }
        result.value = decode_e_l(encoded + 1, 8);
        result.bytes_read = 9;
        return result;
    }

    // General case: find l such that firstByte is in range
    for (uint32_t l = 0; l < 8; l++) {
        uint64_t lower = 256 - power_of_two(8 - l);
        uint64_t upper = 256 - power_of_two(8 - (l + 1));

        if (first_byte >= lower && first_byte < upper) {
            if (max_len < 1 + l) {
                return result; // Not enough bytes
            }

            uint64_t x1 = (uint64_t)first_byte - lower;
            uint64_t x2 = decode_e_l(encoded + 1, (int)l);
            result.value = x1 * power_of_two(8 * l) + x2;
            result.bytes_read = l + 1;
            return result;
        }
    }

    return result; // No match found
}

// Expand bitmask bits (matches Go's expandBits)
uint8_t* expand_bits(const uint8_t* k_bytes, uint32_t k_bytes_len, uint32_t c_size, uint32_t* out_size) {
    uint32_t total_bits = k_bytes_len * 8;
    if (total_bits > c_size) {
        total_bits = c_size;
    }

    uint8_t* k_combined = calloc(total_bits, 1);
    if (!k_combined) return NULL;

    uint32_t bit_index = 0;
    for (uint32_t i = 0; i < k_bytes_len && bit_index < total_bits; i++) {
        uint8_t b = k_bytes[i];
        if (bit_index + 8 <= total_bits) {
            k_combined[bit_index + 0] = b & 1;
            k_combined[bit_index + 1] = (b >> 1) & 1;
            k_combined[bit_index + 2] = (b >> 2) & 1;
            k_combined[bit_index + 3] = (b >> 3) & 1;
            k_combined[bit_index + 4] = (b >> 4) & 1;
            k_combined[bit_index + 5] = (b >> 5) & 1;
            k_combined[bit_index + 6] = (b >> 6) & 1;
            k_combined[bit_index + 7] = (b >> 7) & 1;
            bit_index += 8;
        } else {
            for (int j = 0; bit_index < total_bits; j++) {
                k_combined[bit_index] = (b >> j) & 1;
                bit_index++;
            }
            break;
        }
    }

    *out_size = total_bits;
    return k_combined;
}

// Check if opcode is basic block terminator (matches Go's IsBasicBlockInstruction)
int is_basic_block_instruction(uint8_t opcode) {
    switch (opcode) {
        case 0: case 1: case 40: case 50:
        case 80: case 81: case 82: case 83: case 84: case 85: case 86: case 87: case 88: case 89: case 90:
        case 170: case 171: case 172: case 173: case 174: case 175: case 180:
            return 1;
        default:
            return 0;
    }
}

// Decode program (matches Go's DecodeProgram and DecodeCorePart)
typedef struct {
    uint8_t* code;
    uint32_t code_size;
    uint32_t* jump_table;
    uint32_t jump_table_size;
    uint8_t* bitmask;
    uint32_t bitmask_size;
} decoded_program_t;

decoded_program_t* decode_program(const uint8_t* raw_bytes, uint32_t raw_size) {
    if (raw_size < 15) { // Minimum header size
        fprintf(stderr, "File too small for valid program\n");
        return NULL;
    }

    // Decode header (A.37)
    uint32_t o_size = (uint32_t)decode_e_l(raw_bytes, 3);
    uint32_t w_size = (uint32_t)decode_e_l(raw_bytes + 3, 3);
    uint32_t z_val = (uint32_t)decode_e_l(raw_bytes + 6, 2);
    uint32_t s_val = (uint32_t)decode_e_l(raw_bytes + 8, 3);

    printf("Decoded header: o_size=%u, w_size=%u, z_val=%u, s_val=%u\n",
           o_size, w_size, z_val, s_val);

    uint64_t offset = 11;
    offset += o_size; // Skip o_byte
    offset += w_size; // Skip w_byte

    if (offset + 4 > raw_size) {
        fprintf(stderr, "Invalid program: offset %lu exceeds size %u\n", offset, raw_size);
        return NULL;
    }

    uint64_t c_size = decode_e_l(raw_bytes + offset, 4);
    offset += 4;

    printf("Code section size: %lu, remaining bytes: %u\n", c_size, raw_size - (uint32_t)offset);

    if (raw_size - offset != c_size) {
        fprintf(stderr, "Warning: c_size mismatch (expected %lu, got %u)\n", c_size, raw_size - (uint32_t)offset);
        // Don't fail - continue with actual remaining bytes
    }

    // Now decode core part (j_size, z, c_size, j_array, code, k_bytes)
    const uint8_t* core = raw_bytes + offset;
    uint32_t core_size = raw_size - (uint32_t)offset;

    // Extract j_size, z, c_size using DecodeE (matches Go exactly)
    const uint8_t* p_remaining = core;

    decode_result_t j_size_result = decode_e(p_remaining, core_size);
    if (j_size_result.bytes_read == 0) {
        fprintf(stderr, "Failed to decode j_size\n");
        return NULL;
    }
    uint64_t j_size = j_size_result.value;
    p_remaining += j_size_result.bytes_read;

    printf("  j_size: first_byte=0x%02x, bytes_read=%u, value=%lu\n",
           core[0], j_size_result.bytes_read, j_size);

    uint32_t remaining = core_size - (uint32_t)(p_remaining - core);
    decode_result_t z_result = decode_e(p_remaining, remaining);
    if (z_result.bytes_read == 0) {
        fprintf(stderr, "Failed to decode z\n");
        return NULL;
    }
    uint64_t z = z_result.value;
    p_remaining += z_result.bytes_read;

    printf("  z: bytes_read=%u, value=%lu\n", z_result.bytes_read, z);

    remaining = core_size - (uint32_t)(p_remaining - core);
    decode_result_t c_size_result = decode_e(p_remaining, remaining);
    if (c_size_result.bytes_read == 0) {
        fprintf(stderr, "Failed to decode c_size\n");
        return NULL;
    }
    uint64_t c_size_core = c_size_result.value;
    p_remaining += c_size_result.bytes_read;

    printf("  c_size: bytes_read=%u, value=%lu\n", c_size_result.bytes_read, c_size_core);

    // Now p_remaining points to start of j_byte array
    // Calculate lengths
    uint64_t j_len = j_size * z;
    uint64_t c_len = c_size_core;

    printf("Core: j_size=%lu, z=%lu, c_size=%lu\n", j_size, z, c_size_core);
    printf("  j_len=%lu, c_len=%lu\n", j_len, c_len);

    // Bounds check
    uint32_t remaining_bytes = (uint32_t)(core_size - (p_remaining - core));
    printf("  remaining_bytes after header=%u\n", remaining_bytes);

    if (j_len + c_len > remaining_bytes) {
        fprintf(stderr, "ERROR: Not enough data (need %lu, have %u)\n", j_len + c_len, remaining_bytes);
        return NULL;
    }

    // Extract arrays by slicing p_remaining (matches Go's slice notation)
    const uint8_t* j_byte = p_remaining;
    const uint8_t* c_byte = p_remaining + j_len;
    const uint8_t* k_bytes = p_remaining + j_len + c_len;

    // Calculate k_bytes length
    uint32_t k_bytes_len = remaining_bytes - (uint32_t)(j_len + c_len);
    printf("  k_bytes_len=%u\n", k_bytes_len);

    // Decode jump table
    uint32_t* j_array = NULL;
    uint32_t j_array_size = 0;
    if (j_len > 0) {
        j_array_size = (uint32_t)(j_len / z);
        j_array = malloc(j_array_size * sizeof(uint32_t));
        if (!j_array) {
            fprintf(stderr, "Failed to allocate jump table\n");
            return NULL;
        }

        for (uint32_t i = 0; i < j_array_size; i++) {
            j_array[i] = (uint32_t)decode_e_l(j_byte + i * z, (int)z);
        }
        printf("Decoded %u jump table entries\n", j_array_size);
    }

    // Expand bitmask
    uint32_t k_combined_size = 0;
    uint8_t* k_combined = expand_bits(k_bytes, k_bytes_len, (uint32_t)c_size_core, &k_combined_size);
    if (!k_combined) {
        fprintf(stderr, "Failed to expand bitmask\n");
        free(j_array);
        return NULL;
    }

    // Mark basic block starts in bitmask (matches Go logic)
    int start_basic_block = 1;
    for (uint32_t i = 0; i < k_combined_size; i++) {
        if (k_combined[i] > 0) {
            if (start_basic_block) {
                k_combined[i] |= 2;
                start_basic_block = 0;
            }
            if (is_basic_block_instruction(c_byte[i])) {
                start_basic_block = 1;
            }
        }
    }

    printf("Bitmask expanded to %u bytes\n", k_combined_size);

    // Create result
    decoded_program_t* result = malloc(sizeof(decoded_program_t));
    if (!result) {
        free(j_array);
        free(k_combined);
        return NULL;
    }

    // Allocate and copy code
    result->code = malloc((uint32_t)c_size_core);
    if (!result->code) {
        free(result);
        free(j_array);
        free(k_combined);
        return NULL;
    }
    memcpy(result->code, c_byte, (uint32_t)c_size_core);
    result->code_size = (uint32_t)c_size_core;

    result->jump_table = j_array;
    result->jump_table_size = j_array_size;
    result->bitmask = k_combined;
    result->bitmask_size = k_combined_size;

    return result;
}

void free_decoded_program(decoded_program_t* prog) {
    if (!prog) return;
    free(prog->code);
    free(prog->jump_table);
    free(prog->bitmask);
    free(prog);
}

int main() {
    // Read the raw algo.pvm file
    FILE* f = fopen("/root/go/src/github.com/colorfulnotion/jam/services/algo/algo.pvm", "rb");
    if (!f) {
        fprintf(stderr, "Failed to open algo.pvm\n");
        return 1;
    }

    // Get file size
    fseek(f, 0, SEEK_END);
    long size = ftell(f);
    fseek(f, 0, SEEK_SET);

    // Read raw bytes
    uint8_t* raw_bytes = malloc(size);
    if (!raw_bytes) {
        fprintf(stderr, "Failed to allocate memory\n");
        fclose(f);
        return 1;
    }

    fread(raw_bytes, 1, size, f);
    fclose(f);

    printf("Loaded raw program: %ld bytes\n", size);
    printf("===========================================\n");

    // Decode the program (matches Go's DecodeProgram)
    decoded_program_t* prog = decode_program(raw_bytes, (uint32_t)size);
    free(raw_bytes);

    if (!prog) {
        fprintf(stderr, "Failed to decode program\n");
        return 1;
    }

    printf("===========================================\n");
    printf("Program decoded successfully!\n");
    printf("  Code size: %u bytes\n", prog->code_size);
    printf("  Jump table: %u entries\n", prog->jump_table_size);
    printf("  Bitmask: %u bytes\n", prog->bitmask_size);
    printf("===========================================\n");

    // Create compiler with decoded code
    compiler_t* compiler = compiler_create(prog->code, prog->code_size);
    if (!compiler) {
        fprintf(stderr, "Failed to create compiler\n");
        free_decoded_program(prog);
        return 1;
    }

    printf("Compiler created successfully\n");

    // Set jump table (CRITICAL - matches Go's SetJumpTable)
    if (prog->jump_table_size > 0) {
        int result = compiler_set_jump_table(compiler, prog->jump_table, prog->jump_table_size);
        if (result != 0) {
            fprintf(stderr, "Failed to set jump table: %d\n", result);
            compiler_destroy(compiler);
            free_decoded_program(prog);
            return 1;
        }
        printf("Jump table set: %u entries\n", prog->jump_table_size);
    }

    // Set bitmask (CRITICAL - matches Go's SetBitMask)
    if (prog->bitmask_size > 0) {
        int result = compiler_set_bitmask(compiler, prog->bitmask, prog->bitmask_size);
        if (result != 0) {
            fprintf(stderr, "Failed to set bitmask: %d\n", result);
            compiler_destroy(compiler);
            free_decoded_program(prog);
            return 1;
        }
        printf("Bitmask set: %u bytes\n", prog->bitmask_size);
    }

    printf("===========================================\n");
    printf("Starting compilation...\n");

    // Try to compile
    uint8_t* x86_code = NULL;
    uint32_t code_size = 0;
    uintptr_t djump_addr = 0;

    int result = compiler_compile(compiler, 0, &x86_code, &code_size, &djump_addr);

    printf("===========================================\n");
    if (result != 0) {
        fprintf(stderr, "Compilation failed with error code: %d\n", result);
    } else {
        printf("Compilation succeeded! Generated %u bytes of x86 code\n", code_size);
    }

    // Cleanup
    if (x86_code) free(x86_code);
    compiler_destroy(compiler);
    free_decoded_program(prog);

    return result;
}
