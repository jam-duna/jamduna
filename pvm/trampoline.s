#include "textflag.h"

// func trampoline(entry uintptr)
TEXT ·trampoline(SB), NOSPLIT, $0-8
    MOVQ entry+0(FP), AX
    JMP  AX
