// Enable deprecated ucontext routines on macOS
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

// Forward declaration of the Go-exported host call
extern void Ecalli(void* rvmPtr, int32_t opcode);
void* get_ecalli_address(void) {
    return (void*)Ecalli;
}

extern void Sbrk(void* rvmPtr);
void* get_sbrk_address(void) {
    return (void*)Sbrk;
}

static sigjmp_buf jump_env;
static void* global_reg_ptr = NULL;

static void signal_handler(int sig, siginfo_t *si, void *arg) {
    ucontext_t *ctx     = (ucontext_t*)arg;
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

    fprintf(stderr, "[x86_execute] Caught %s at address %p\n", signame, fault_addr);
    fprintf(stderr, "[x86_execute] RIP=0x%016llx RSP=0x%016llx RBP=0x%016llx\n",
            (unsigned long long)rip,
            (unsigned long long)rsp,
            (unsigned long long)rbp);

    // reg_ptr was saved into global_reg_ptr by execute_x86()
    uint64_t* regPtr = (uint64_t*)global_reg_ptr;

    // Store registers into regPtr in the exact order of the MOV [R12+...] instructions:
    regPtr[0]  = ctx->uc_mcontext.gregs[REG_RAX];  // [R12+0x00] = RAX
    regPtr[1]  = ctx->uc_mcontext.gregs[REG_RCX];  // [R12+0x08] = RCX
    regPtr[2]  = ctx->uc_mcontext.gregs[REG_RDX];  // [R12+0x10] = RDX
    regPtr[3]  = ctx->uc_mcontext.gregs[REG_RBX];  // [R12+0x18] = RBX
    regPtr[4]  = ctx->uc_mcontext.gregs[REG_RSI];  // [R12+0x20] = RSI
    regPtr[5]  = ctx->uc_mcontext.gregs[REG_RDI];  // [R12+0x28] = RDI
    regPtr[6]  = ctx->uc_mcontext.gregs[REG_R8];   // [R12+0x30] = R8
    regPtr[7]  = ctx->uc_mcontext.gregs[REG_R9];   // [R12+0x38] = R9
    regPtr[8]  = ctx->uc_mcontext.gregs[REG_R10];  // [R12+0x40] = R10
    regPtr[9]  = ctx->uc_mcontext.gregs[REG_R11];  // [R12+0x48] = R11
    regPtr[10] = ctx->uc_mcontext.gregs[REG_R13];  // [R12+0x50] = R13
    regPtr[11] = ctx->uc_mcontext.gregs[REG_R14];  // [R12+0x58] = R14
    regPtr[12] = ctx->uc_mcontext.gregs[REG_R15];  // [R12+0x60] = R15

    // Optionally dump bytes around RIP for debugging
    unsigned char* ip = (unsigned char*)rip;
    fprintf(stderr, "[x86_execute] Bytes @ RIP: ");
    for (int i = -4; i < 12; i++) {
        fprintf(stderr, "%02x ", *(ip + i) & 0xff);
    }
    fprintf(stderr, "\n");
    uint64_t r12 = ctx->uc_mcontext.gregs[REG_R12];
    fprintf(stderr, "[x86_execute] R12=0x%016llx\n", (unsigned long long)r12);
    // Jump back to recovery point
    siglongjmp(jump_env, 1);
}
static void setup_signal_handlers() {
    struct sigaction sa = {0};
    sa.sa_sigaction = signal_handler;
    sa.sa_flags     = SA_SIGINFO | SA_ONSTACK;

    sigaction(SIGSEGV, &sa, NULL);
    sigaction(SIGILL,  &sa, NULL);
    sigaction(SIGFPE,  &sa, NULL);
    sigaction(SIGBUS,  &sa, NULL);
    sigaction(SIGTRAP, &sa, NULL);
}

int execute_x86(uint8_t* code, size_t length, void* reg_ptr) {
    setup_signal_handlers();
    global_reg_ptr = reg_ptr;

    struct sigaction sa, old_sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_sigaction = signal_handler;
    sa.sa_flags     = SA_SIGINFO;

    sigaction(SIGSEGV, NULL, &old_sa);
    sigaction(SIGSEGV, &sa, NULL);

    if (sigsetjmp(jump_env, 1) != 0) {
        sigaction(SIGSEGV, &old_sa, NULL);
        return -1;
    }

    typedef void (*code_func_t)(void);
    ((code_func_t)code)();

    sigaction(SIGSEGV, &old_sa, NULL);
    return 0;
}

void* alloc_executable(size_t size) {
    void* ptr = mmap(NULL, size, PROT_READ | PROT_WRITE | PROT_EXEC,
                     MAP_ANON | MAP_PRIVATE, -1, 0);
    if (ptr == MAP_FAILED) return NULL;
    return ptr;
}
