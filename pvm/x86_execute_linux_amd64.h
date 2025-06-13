// x86_execute.h
#ifndef X86_EXECUTE_H
#define X86_EXECUTE_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// Execute a block of x86_64 machine code and on SIGSEGV dump registers.
// @code: pointer to the machine code buffer
// @length: size of the buffer in bytes
// @reg_ptr: pointer to a 14-element uint64_t array for register dump
int execute_x86(uint8_t* code, size_t length, void* reg_ptr);
// Declaration of the Go-exported host call stub
// Ecalli: handles host call from JIT code
void Ecalli(void* rvmPtr, int32_t opcode);

// Returns the function pointer of the Go-exported Ecalli for JIT stubs
void* get_ecalli_address(void);

#ifdef __cplusplus
}
#endif

#endif // X86_EXECUTE_H