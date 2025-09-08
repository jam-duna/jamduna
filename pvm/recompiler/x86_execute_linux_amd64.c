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
void* get_ecalli_address(void) { return (void*)Ecalli; }

extern void Sbrk(void* rvmPtr);
void* get_sbrk_address(void) { return (void*)Sbrk; }

// Thread-local jump buffer and register pointer
static __thread sigjmp_buf tls_jump_env;
static __thread void*      tls_global_reg_ptr;

// Thread-local flag to indicate we are inside JIT execution
static __thread int        in_jit = 0;

// Global installation flag for our signal handler
static int handlers_installed = 0;

// List of signals to intercept during JIT
static int signums[] = { SIGSEGV, SIGILL, SIGFPE, SIGBUS, SIGTRAP };
static const int n_signums = sizeof(signums) / sizeof(signums[0]);

// Our unified signal handler: if not in JIT, restore default and re-raise
static void signal_handler(int sig, siginfo_t *si, void *arg) {
    if (!in_jit) {
        // Not in JIT phase: uninstall our handler and forward to default
        struct sigaction act = { .sa_handler = SIG_DFL };
        sigaction(sig, &act, NULL);
        raise(sig);
        return;
    }

    // We are in JIT: handle the fault, save registers, and longjmp back
    ucontext_t *ctx       = (ucontext_t*)arg;
    void*      fault_addr = si->si_addr;
    uint64_t   rip        = ctx->uc_mcontext.gregs[REG_RIP];
    uint64_t   rsp        = ctx->uc_mcontext.gregs[REG_RSP];
    uint64_t   rbp        = ctx->uc_mcontext.gregs[REG_RBP];

    // Determine signal name
    const char* signame = "UNKNOWN";
    switch (sig) {
      case SIGSEGV: signame = "SIGSEGV"; break;
      case SIGILL:  signame = "SIGILL";  break;
      case SIGFPE:  signame = "SIGFPE";  break;
      case SIGBUS:  signame = "SIGBUS";  break;
      case SIGTRAP: signame = "SIGTRAP"; break;
    }

    fprintf(stderr, "[x86_execute] Caught %s at address %p (RIP=0x%llx RSP=0x%llx RBP=0x%llx)\n",
            signame, fault_addr, (unsigned long long)rip,
            (unsigned long long)rsp, (unsigned long long)rbp);

    // Save callee-saved registers into the provided buffer
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

    // Jump back to the recovery point in execute_x86
    siglongjmp(tls_jump_env, 1);
}

// Install our signal handler once for the process
static void ensure_signal_handlers() {
    if (handlers_installed) return;
    struct sigaction sa = {0};
    sa.sa_sigaction = signal_handler;
    sa.sa_flags     = SA_SIGINFO | SA_ONSTACK;

    for (int i = 0; i < n_signums; i++) {
        sigaction(signums[i], &sa, NULL);
    }
    handlers_installed = 1;
}

// Execute JIT code with fault recovery using in_jit flag
int execute_x86(uint8_t* code, size_t length, void* reg_ptr) {
    tls_global_reg_ptr   = reg_ptr;

    ensure_signal_handlers();

    in_jit = 1;  // Enter JIT execution phase

    if (sigsetjmp(tls_jump_env, 1) != 0) {
        // A fault occurred during JIT; we recovered here.
        in_jit = 0;  
        return -1;
    }

    // Run the JIT-compiled code pointer
    ((void(*)(void))code)();

    in_jit = 0;  // Exit JIT execution phase
    return 0;
}

// Allocate RWX memory for code
void* alloc_executable(size_t size) {
    void* ptr = mmap(NULL, size,
                     PROT_READ | PROT_WRITE | PROT_EXEC,
                     MAP_ANON | MAP_PRIVATE, -1, 0);
    return (ptr == MAP_FAILED) ? NULL : ptr;
}

// Free the executable memory
void free_executable(void* ptr, size_t size) {
    munmap(ptr, size);
}
