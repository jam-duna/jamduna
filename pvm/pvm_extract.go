package pvm

import (
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/slices"
)

// A.5.4. Instructions with Arguments of Two Immediates.
func extractTwoImm(oargs []byte) (vx uint64, vy uint64) {
	args := slices.Clone(oargs)
	lx := min(4, int(args[0])%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value

	return
}

// A.5.5. Instructions with Arguments of One Offset. (JUMP)
func extractOneOffset(oargs []byte) (vx int64) {
	args := slices.Clone(oargs)
	lx := min(4, len(args))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = z_encode(types.DecodeE_l(args[0:lx]), uint32(lx))
	return vx
}

// A.5.6. Instructions with Arguments of One Register & One Immediate. (JUMP_IND)
func extractOneRegOneImm(oargs []byte) (reg1 int, vx uint64) {
	args := slices.Clone(oargs)
	registerIndexA := min(12, int(args[0])%16)
	lx := min(4, max(0, len(args))-1)
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return int(registerIndexA), vx
}

// A.5.7. Instructions with Arguments of One Register and Two Immediates.
func extractOneReg2Imm(oargs []byte) (reg1 int, vx uint64, vy uint64) {
	args := slices.Clone(oargs)

	reg1 = min(12, int(args[0])%16)
	lx := min(4, (int(args[0])/16)%8)
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return int(reg1), vx, vy
}

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset. (LOAD_IMM_JUMP, BRANCH_{EQ/NE/...}_IMM)
func extractOneRegOneImmOneOffset(oargs []byte) (registerIndexA int, vx uint64, vy int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	lx := min(4, (int(args[0]) / 16 % 8))
	ly := min(4, max(0, len(args)-lx-1))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	vy = z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
	return registerIndexA, vx, vy
}

// A.5.9. Instructions with Arguments of Two Registers.
func extractTwoRegisters(args []byte) (regD, regA int) {
	regD = min(12, int(args[0]&0x0F))
	regA = min(12, int(args[0]>>4))
	return
}

// A.5.10. Instructions with Arguments of Two Registers and One Immediate.
func extractTwoRegsOneImm(args []byte) (reg1, reg2 int, imm uint64) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	imm = x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
	return
}

// A.5.11. Instructions with Arguments of Two Registers and One Offset. (BRANCH_{EQ/NE/...})
func extractTwoRegsOneOffset(oargs []byte) (registerIndexA, registerIndexB int, vx int64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, max(0, len(args)-1))
	if lx == 0 {
		lx = 1
		args = append(args, 0)
	}
	vx = int64(z_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx)))
	return registerIndexA, registerIndexB, vx
}

// A.5.12. Instructions with Arguments of Two Registers and Two Immediates. (LOAD_IMM_JUMP_IND)
// This instruction is used to load an immediate value into a register and then jump to an address.
func extractTwoRegsAndTwoImmediates(oargs []byte) (registerIndexA, registerIndexB int, vx, vy uint64) {
	args := slices.Clone(oargs)
	registerIndexA = min(12, int(args[0])%16)
	registerIndexB = min(12, int(args[0])/16)
	lx := min(4, (int(args[1]) % 8))
	ly := min(4, max(0, len(args)-lx-2))
	if ly == 0 {
		ly = 1
		args = append(args, 0)
	}

	vx = x_encode(types.DecodeE_l(args[2:2+lx]), uint32(lx))
	vy = x_encode(types.DecodeE_l(args[2+lx:2+lx+ly]), uint32(ly))
	return registerIndexA, registerIndexB, vx, vy
}

// A.5.13. Instructions with Arguments of Three Registers.
func extractThreeRegs(args []byte) (reg1, reg2, dst int) {
	reg1 = min(12, int(args[0]&0x0F))
	reg2 = min(12, int(args[0]>>4))
	dst = min(12, int(args[1]))
	return
}
