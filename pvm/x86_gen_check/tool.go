package x86gencheck

import "github.com/colorfulnotion/jam/pvm/program"

func Skip(bitmask []byte, pc uint64) uint64 {
	n := uint64(len(bitmask))
	end := pc + 25
	if end > n {
		end = n
	}
	for i := pc + 1; i < end; i++ {
		if bitmask[i] == 0x01 {
			return i - pc - 1
		}
	}
	if end < pc+25 {
		return end - pc - 1
	}
	return 24
}

func ParsePVM_Instructions(rawCodeBytes []byte) (instrs []Instruction) {
	p := program.DecodeCorePart(rawCodeBytes)
	instrs = make([]Instruction, 0)
	bitMask := p.K
	code := p.Code

	pc := uint64(0)
	for pc < uint64(len(code)) {
		op := code[pc]
		olen := Skip(bitMask, pc)
		operands := code[pc+1 : pc+1+olen]
		instrs = append(instrs, Instruction{Opcode: op, Args: operands})
		pc += 1 + olen
	}
	return instrs
}

func ParsePvmByteCodeInstructions(raw_code []byte) (instrs []Instruction) {

	instrs = make([]Instruction, 0)
	p, _, _, _, _, _, _ := program.DecodeProgram(raw_code)
	bitMask := p.K
	code := p.Code

	pc := uint64(0)
	for pc < uint64(len(code)) {
		op := code[pc]
		olen := Skip(bitMask, pc)

		operands := code[pc+1 : pc+1+olen]
		instrs = append(instrs, Instruction{Opcode: op, Args: operands})
		pc += 1 + olen
	}
	return instrs
}

func ParsePvmByteCode(raw_code []byte) (code []byte, bitmask []byte, jumpTable []uint32) {
	p, _, _, _, _, _, _ := program.DecodeProgram(raw_code)
	return p.Code, p.K, p.J
}
