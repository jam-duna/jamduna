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

static sigjmp_buf jump_env;
static void* global_reg_ptr = NULL;

// SIGSEGV handler: dumps register state then longjmp
// SIGSEGV handler: dumps CPU state then longjmp

// SIGSEGV handler: dumps CPU state then longjmp
static void sigsegv_handler(int sig, siginfo_t *si, void *arg) {
    ucontext_t *ctx = (ucontext_t*)arg;
    uint64_t* regs = (uint64_t*)global_reg_ptr;

    // Fault info
    void* fault_addr = si->si_addr;
    uint64_t rip = ctx->uc_mcontext.gregs[REG_RIP];
    uint64_t rsp = ctx->uc_mcontext.gregs[REG_RSP];
    uint64_t rbp = ctx->uc_mcontext.gregs[REG_RBP];

    fprintf(stderr, "[x86_execute] Caught SIGSEGV: sig=%d at address=%p\n", sig, fault_addr);
    fprintf(stderr, "[x86_execute] RIP=0x%016llx RSP=0x%016llx RBP=0x%016llx\n", (unsigned long long)rip, (unsigned long long)rsp, (unsigned long long)rbp);

    // Dump registers to buffer
    regs[0]  = ctx->uc_mcontext.gregs[REG_RAX];
    regs[1]  = ctx->uc_mcontext.gregs[REG_RCX];
    regs[2]  = ctx->uc_mcontext.gregs[REG_RDX];
    regs[3]  = ctx->uc_mcontext.gregs[REG_RBX];
    regs[4]  = ctx->uc_mcontext.gregs[REG_RSI];
    regs[5]  = ctx->uc_mcontext.gregs[REG_RDI];
    regs[6]  = ctx->uc_mcontext.gregs[REG_R8];
    regs[7]  = ctx->uc_mcontext.gregs[REG_R9];
    regs[8]  = ctx->uc_mcontext.gregs[REG_R10];
    regs[9]  = ctx->uc_mcontext.gregs[REG_R11];
    regs[10] = ctx->uc_mcontext.gregs[REG_R13];
    regs[11] = ctx->uc_mcontext.gregs[REG_R14];
    regs[12] = ctx->uc_mcontext.gregs[REG_R15];
    regs[13] = ctx->uc_mcontext.gregs[REG_R12];

    // Print register dump
    const char* names[14] = {"RAX","RCX","RDX","RBX","RSI","RDI","R8","R9","R10","R11","R13","R14","R15","R12"};
    fprintf(stderr, "[x86_execute] Register dump:\n");
    for (int i = 0; i < 14; i++) {
        fprintf(stderr, "  %4s = 0x%016llx\n", names[i], (unsigned long long)regs[i]);
    }

    // Optionally dump bytes around RIP
    unsigned char* ip = (unsigned char*)rip;
    fprintf(stderr, "[x86_execute] Bytes @ RIP: ");
    for (int i = -4; i < 12; i++) {
        fprintf(stderr, "%02x ", *(ip + i) & 0xff);
    }
    fprintf(stderr, "\n");

    // Jump back to recovery point
    siglongjmp(jump_env, 1);
}
/**
 * execute_x86: maps, executes and handles SIGSEGV of JIT code
 * @code: pointer to machine code buffer
 * @length: size of the buffer
 * @reg_ptr: pointer to 14-element uint64_t array to dump registers
 * return: 0 on success,
 *         -1 on segfault (registers dumped),
 *         -2 on mmap failure,
 *         -3 on mprotect failure
 */
 int execute_x86(uint8_t* code, size_t length, void* reg_ptr) {
    global_reg_ptr = reg_ptr;

    struct sigaction sa, old_sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_sigaction = sigsegv_handler;
    sa.sa_flags = SA_SIGINFO;

    sigaction(SIGSEGV, NULL, &old_sa);
    sigaction(SIGSEGV, &sa, NULL);

    void* mem = mmap(NULL, length, PROT_READ | PROT_WRITE | PROT_EXEC,
                     MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (mem == MAP_FAILED) return -2;

    memcpy(mem, code, length);

    if (sigsetjmp(jump_env, 1) != 0) {
        munmap(mem, length);
        sigaction(SIGSEGV, &old_sa, NULL);
        return -1;
    }

    typedef void (*code_func_t)(void);
    ((code_func_t)mem)();

    munmap(mem, length);
    sigaction(SIGSEGV, &old_sa, NULL);
    return 0;
}



void* alloc_executable(size_t size) {
	void* ptr = mmap(NULL, size, PROT_READ | PROT_WRITE | PROT_EXEC,
	                 MAP_ANON | MAP_PRIVATE, -1, 0);
	if (ptr == MAP_FAILED) return NULL;
	return ptr;
}

