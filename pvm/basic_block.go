package pvm

import "fmt"

const (
	TRAP_JUMP     = 0
	DIRECT_JUMP   = 1
	INDIRECT_JUMP = 2
	CONDITIONAL   = 3
)

type BasicBlock struct {
	Instructions []Instruction
	InitGas      int64
	GasUsage     int64

	JumpType               int
	IndirectJumpOffset     uint64
	IndirectSourceRegister int
	TruePC                 uint64
	FalsePC                uint64
}

type Instruction struct {
	Opcode byte
	Args   []byte
	Step   int
	Pc     uint64
}

func NewBasicBlock(initGas int64) *BasicBlock {
	return &BasicBlock{
		Instructions: make([]Instruction, 0),
		InitGas:      initGas,
		GasUsage:     0,
	}
}

func (bb *BasicBlock) AddInstruction(opcode byte, args []byte, step int, pc uint64) {
	instruction := Instruction{
		Opcode: opcode,
		Args:   args,
		Step:   step,
		Pc:     pc,
	}
	bb.Instructions = append(bb.Instructions, instruction)
}

func (i *Instruction) String() string {
	return fmt.Sprintf("%s | ", opcode_str(i.Opcode))
}
func (bb *BasicBlock) String() string {
	result := ""
	for _, inst := range bb.Instructions {
		result += inst.String()
	}
	result += fmt.Sprintf("GasCharge: %d, [%d -> %d]\n", bb.GasUsage, bb.InitGas, bb.InitGas-bb.GasUsage)
	return result
}

func IsBasicBlockInstruction(opcode byte) bool {
	switch opcode {
	case TRAP, FALLTHROUGH, JUMP, JUMP_IND, LOAD_IMM_JUMP, LOAD_IMM_JUMP_IND,
		BRANCH_EQ, BRANCH_NE, BRANCH_GE_U, BRANCH_GE_S, BRANCH_LT_U, BRANCH_LT_S,
		BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM,
		BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM,
		BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		return true
	default:
		return false
	}
}
