// x86_execute.h
#ifndef X86_EXECUTE_H
#define X86_EXECUTE_H

#include <stdint.h>
#include <stddef.h>

int execute_x86(uint8_t* code, size_t length);

#endif
