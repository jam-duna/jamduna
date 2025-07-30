// Package pvm provides x86-64 register definitions and utilities.
package pvm

// X86Reg represents an x86-64 register with encoding information
type X86Reg struct {
	Name    string
	RegBits byte // 3-bit code for ModRM/SIB
	REXBit  byte // 1 if register index >= 8
}

// Standard x86-64 register definitions
var (
	RAX = X86Reg{"rax", 0, 0} // Commonly used as return value register
	RCX = X86Reg{"rcx", 1, 0} // Used for loop counters or intermediates
	RDX = X86Reg{"rdx", 2, 0} // Often paired with rax for mul/div
	RBX = X86Reg{"rbx", 3, 0}
	RSI = X86Reg{"rsi", 6, 0} // Often used as function argument
	RDI = X86Reg{"rdi", 7, 0} // Often used as function argument
	R8  = X86Reg{"r8", 0, 1}  // Typically function argument #5
	R9  = X86Reg{"r9", 1, 1}
	R10 = X86Reg{"r10", 2, 1}
	R11 = X86Reg{"r11", 3, 1}
	R13 = X86Reg{"r13", 5, 1}
	R14 = X86Reg{"r14", 6, 1}
	R15 = X86Reg{"r15", 7, 1}
	R12 = X86Reg{"r12", 4, 1}
	EAX = X86Reg{"eax", 0, 0}
	ECX = X86Reg{"ecx", 1, 0}
	EDX = X86Reg{"edx", 2, 0}
	EBX = X86Reg{"ebx", 3, 0}
	ESI = X86Reg{"esi", 6, 0}
	EDI = X86Reg{"edi", 7, 0}
)

// regInfoList contains all available registers in allocation order
var regInfoList = []X86Reg{
	RAX, RCX, RDX, RBX, RSI, RDI, R8, R9, R10, R11, R13, R14, R15, R12,
}

// BaseRegIndex is the index for the base register used in memory operations
const BaseRegIndex = 13

var BaseReg = regInfoList[BaseRegIndex]
