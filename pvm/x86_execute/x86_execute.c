// x86_execute.c
#include <stdint.h>
#include <signal.h>
#include <setjmp.h>
#include <string.h>
#include <sys/mman.h>
#include <stdio.h>

static sigjmp_buf jump_env;

static void sigsegv_handler(int sig) {
    siglongjmp(jump_env, 1);
}

int execute_x86(uint8_t* code, size_t length) {
    signal(SIGSEGV, sigsegv_handler);

    void* mem = mmap(NULL, length, PROT_READ | PROT_WRITE,
                     MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (mem == MAP_FAILED) {
        return -2; // mmap failed
    }

    memcpy(mem, code, length);
    if (mprotect(mem, length, PROT_READ | PROT_EXEC) != 0) {
        return -3; // mprotect failed
    }

    if (sigsetjmp(jump_env, 1) != 0) {
        return -1; // SIGSEGV caught
    }

    ((void(*)())mem)(); // run the code

    munmap(mem, length);
    return 0; // success
}
