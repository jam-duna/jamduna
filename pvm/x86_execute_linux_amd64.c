#define _XOPEN_SOURCE 700
#define _GNU_SOURCE

#include <signal.h>
#include <string.h>
#include <setjmp.h>
#include <sys/ucontext.h>
#include <sys/mman.h>
#include <ucontext.h>
#include <stdint.h>
#include <stdio.h>
#include "x86_execute_linux_amd64.h"

// Forward declaration of the Go-exported host calls
extern void Ecalli(void* rvmPtr, int32_t opcode);
void* get_ecalli_address(void) {
    return (void*)Ecalli;
}

extern void Sbrk(void* rvmPtr);
void* get_sbrk_address(void) {
    return (void*)Sbrk;
}

// Thread-local jump buffer and register pointer
static __thread sigjmp_buf tls_jump_env;
static __thread void*      tls_global_reg_ptr;
static __thread int        tls_signal_installed = 0;
static __thread int        tls_signal_jmp_times = 0;
// Signal handler for trapping faults in JIT code
static void signal_handler(int sig, siginfo_t *si, void *arg) {
    ucontext_t *ctx        = (ucontext_t*)arg;
    void*      fault_addr  = si->si_addr;
    uint64_t   rip         = ctx->uc_mcontext.gregs[REG_RIP];
    uint64_t   rsp         = ctx->uc_mcontext.gregs[REG_RSP];
    uint64_t   rbp         = ctx->uc_mcontext.gregs[REG_RBP];

    // Determine signal name
    const char* signame = "UNKNOWN";
    switch (sig) {
        case SIGSEGV: signame = "SIGSEGV"; break;
        case SIGILL:  signame = "SIGILL";  break;
        case SIGFPE:  signame = "SIGFPE";  break;
        case SIGBUS:  signame = "SIGBUS";  break;
        case SIGTRAP: signame = "SIGTRAP"; break;
    }
    tls_signal_jmp_times++;
    if (tls_signal_jmp_times > 5) {
        fprintf(stderr, "[x86_execute] Signal %s occurred multiple times, exiting.\n", signame);
        exit(1);
    }

    fprintf(stderr, "[x86_execute] Caught %s at address %p\n", signame, fault_addr);
    fprintf(stderr, "[x86_execute] RIP=0x%016llx RSP=0x%016llx RBP=0x%016llx\n",
            (unsigned long long)rip,
            (unsigned long long)rsp,
            (unsigned long long)rbp);

    // Save registers into the provided buffer in R12+offset order
    uint64_t* regPtr = (uint64_t*)tls_global_reg_ptr;
    regPtr[0]  = ctx->uc_mcontext.gregs[REG_RAX];
    regPtr[1]  = ctx->uc_mcontext.gregs[REG_RCX];
    regPtr[2]  = ctx->uc_mcontext.gregs[REG_RDX];
    regPtr[3]  = ctx->uc_mcontext.gregs[REG_RBX];
    regPtr[4]  = ctx->uc_mcontext.gregs[REG_RSI];
    regPtr[5]  = ctx->uc_mcontext.gregs[REG_RDI];
    regPtr[6]  = ctx->uc_mcontext.gregs[REG_R8];
    regPtr[7]  = ctx->uc_mcontext.gregs[REG_R9];
    regPtr[8]  = ctx->uc_mcontext.gregs[REG_R10];
    regPtr[9]  = ctx->uc_mcontext.gregs[REG_R11];
    regPtr[10] = ctx->uc_mcontext.gregs[REG_R13];
    regPtr[11] = ctx->uc_mcontext.gregs[REG_R14];
    regPtr[12] = ctx->uc_mcontext.gregs[REG_R15];
    
    // Jump back to the recovery point
    siglongjmp(tls_jump_env, 1);
}

// Install the signal handler once per thread
static void ensure_signal_handlers() {
    if (tls_signal_installed) return;
    struct sigaction sa = {0};
    sa.sa_sigaction = signal_handler;
    sa.sa_flags     = SA_SIGINFO | SA_ONSTACK;

    sigaction(SIGSEGV, &sa, NULL);
    sigaction(SIGILL,  &sa, NULL);
    sigaction(SIGFPE,  &sa, NULL);
    sigaction(SIGBUS,  &sa, NULL);
    sigaction(SIGTRAP, &sa, NULL);

    tls_signal_installed = 1;
}

// Thread-local execute function with fault recovery
int execute_x86(uint8_t* code, size_t length, void* reg_ptr) {
    ensure_signal_handlers();
    tls_global_reg_ptr = reg_ptr;

    if (sigsetjmp(tls_jump_env, 1) != 0) {
        // Fault occurred, recovered here
        return -1;
    }

    // Execute JIT code
    ((void(*)(void))code)();
    return 0;
}

// Allocate RWX memory for code
void* alloc_executable(size_t size) {
    void* ptr = mmap(NULL, size, PROT_READ | PROT_WRITE | PROT_EXEC,
                     MAP_ANON | MAP_PRIVATE, -1, 0);
    return (ptr == MAP_FAILED) ? NULL : ptr;
}

// Expose C.allocate_executable to Go
void free_executable(void* ptr, size_t size) {
    munmap(ptr, size);
}