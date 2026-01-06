package program

import (
	"slices"

	"github.com/colorfulnotion/jam/types"
)

// A.5.4. Instructions with Arguments of Two Immediates.
func ExtractTwoImm(oargs []byte) (vx uint64, vy uint64) {
	if len(oargs) < 1 {
		return 0, 0
	}
	args := slices.Clone(oargs)
	lx := min(4, int(args[0])%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}
	vx = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
	vy = XEncode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value

	return
}

// A.5.5. Instructions with Arguments of One Offset. (JUMP)
func ExtractOneOffset(oargs []byte) (vx int64) {
	args := slices.Clone(oargs)
	lx := min(4, len(args))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = ZEncode(types.DecodeE_l(args[0:lx]), uint32(lx))
	return vx
}

// A.5.6. Instructions with Arguments of One Register & One Immediate. (JUMP_IND)
func ExtractOneRegOneImm(oargs []byte) (reg1 int, vx uint64) {
	if len(oargs) < 1 {
		return 0, 0
	}
	args := slices.Clone(oargs)
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, max(0, len(args))-1)
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return int(registerIndexA), vx
}

// A.5.7. Instructions with Arguments of One Register and Two Immediates.
func ExtractOneReg2Imm(oargs []byte) (reg1 int, vx uint64, vy uint64) {
	if len(oargs) < 1 {
		return 0, 0, 0
	}
	args := slices.Clone(oargs)

	reg1 = min(12, int(args[0])%16)
	lx := min(4, (int(args[0])/16)%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = XEncode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return int(reg1), vx, vy
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset. (LOAD_IMM_JUMP, BRANCH_{EQ/NE/...}_IMM)
func ExtractOneRegOneImmOneOffset(oargs []byte) (registerIndexA int, vx uint64, vy int64) {
	if len(oargs) < 1 {
		return 0, 0, 0
	}
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	lx := min(4, (int(args[0]) / 16 % 8))
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = ZEncode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return registerIndexA, vx, vy
}

// A.5.9. Instructions with Arguments of Two Registers.
func ExtractTwoRegisters(args []byte) (regD, regA int) {
	if len(args) < 1 {
		return 0, 0
	}
	regD = min(12, int(args[0]&0x0F))
	regA = min(12, int(args[0]>>4))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func ExtractTwoRegsOneImm(args []byte) (reg1, reg2 int, imm uint64) {
	if len(args) < 1 {
		return 0, 0, 0
	}
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func ExtractTwoRegsOneImm64(args []byte) (reg1, reg2 int, imm int64, uint64imm uint64) {
	if len(args) < 1 {
		return 0, 0, 0, 0
	}
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = int64(XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	uint64imm = XEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.11. Instructions with Arguments of Two Registers and One Offset. (BRANCH_{EQ/NE/...})
func ExtractTwoRegsOneOffset(oargs []byte) (registerIndexA, registerIndexB int, vx int64) {
	if len(oargs) < 1 {
		return 0, 0, 0
	}
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = int64(ZEncode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	return registerIndexA, registerIndexB, vx
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
// This instruction is used to load an immediate value into a register and then jump to an address.
func ExtractTwoRegsAndTwoImmediates(oargs []byte) (registerIndexA, registerIndexB int, vx, vy uint64) {
	if len(oargs) < 2 {
		// Pad to at least 2 bytes
		padded := make([]byte, 2)
		copy(padded, oargs)
		oargs = padded
	}
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, (int(args[1]) % 8))
	ly := min(4, max(0, len(args)-lx-2))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = XEncode(types.DecodeE_l(args[2:2+lx]), uint32(lx))
	vy = XEncode(types.DecodeE_l(args[2+lx:2+lx+ly]), uint32(ly))
	return registerIndexA, registerIndexB, vx, vy
}

// A.5.13. Instructions with Arguments of Three Registers.
func ExtractThreeRegs(args []byte) (reg1, reg2, dst int) {
	if len(args) < 2 {
		// Pad args to at least 2 bytes
		padded := make([]byte, 2)
		copy(padded, args)
		args = padded
	}
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	dst = min(12, int(args[1]))
	return
}

func XEncode(x uint64, n uint32) uint64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 8*n - 1
	q := x >> shift
	if n == 8 {
		return x
	}
	mask := (uint64(1) << (8 * n)) - 1
	factor := ^mask
	return x + q*factor
}

func ZEncode(a uint64, n uint32) int64 {
	if n == 0 || n > 8 {
		return 0
	}
	shift := 64 - 8*n
	return int64(a<<shift) >> shift
}
