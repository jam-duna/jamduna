// x86_execute.h
#ifndef X86_EXECUTE_H
#define X86_EXECUTE_H

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

// Execute a block of x86_64 machine code and on SIGSEGV dump registers.
// @code: pointer to the machine code buffer
// @length: size of the buffer in bytes
// @reg_ptr: pointer to a 14-element uint64_t array for register dump
int execute_x86(uint8_t* code, size_t length, void* reg_ptr);
void* alloc_executable(size_t size);
void free_executable(void* ptr, size_t size);

// Get the function pointer to the debug print instruction
void* getDebugPrintInstructionPtr();

#ifdef __cplusplus
}
#endif

#endif // X86_EXECUTE_H