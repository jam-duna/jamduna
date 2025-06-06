package pvm

import "fmt"

type BasicBlock struct {
	Instructions []Instruction
	EndPoint     Instruction
	InitGas      int64
	GasUsage     int64
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
		EndPoint:     Instruction{},
		InitGas:      initGas,
		GasUsage:     0,
	}
}

func (bb *BasicBlock) AddInstruction(opcode byte, args []byte, step int, pc uint64) {
	if !IsBasicBlockInstruction(opcode) {
		instruction := Instruction{
			Opcode: opcode,
			Args:   args,
			Step:   step,
			Pc:     pc,
		}
		bb.Instructions = append(bb.Instructions, instruction)
	} else {
		// If the instruction is a basic block instruction, we should not add it to the instructions list.
		// Instead, we will set it as the end point of the basic block.
		bb.SetEndPoint(opcode, args, step, pc)
	}
}

func (bb *BasicBlock) SetEndPoint(opcode byte, args []byte, step int, pc uint64) {
	bb.EndPoint = Instruction{
		Opcode: opcode,
		Args:   args,
		Step:   step,
		Pc:     pc,
	}
}

func (i *Instruction) String() string {
	return fmt.Sprintf("%s %d %d\n", opcode_str(i.Opcode), i.Step, i.Pc)
}
func (bb *BasicBlock) String() string {
	result := ""
	for _, inst := range bb.Instructions {
		result += inst.String()
	}
	result += fmt.Sprintf("BasicBlock End: %s", bb.EndPoint.String())
	result += fmt.Sprintf("GasCharge: %d, [%d -> %d]\n", bb.GasUsage, bb.InitGas, bb.InitGas-bb.GasUsage)
	return result
}

// ● Trap and fallthrough: trap , fallthrough
// ● Jumps: jump , jump_ind
// ● Load-and-Jumps: load_imm_jump , load_imm_jump_ind
// ● Branches: branch_eq , branch_ne , branch_ge_u , branch_ge_s , branch_lt_u , branch_lt_s , branch_eq_imm ,
// branch_ne_imm
// ● Immediate branches: branch_lt_u_imm , branch_lt_s_imm , branch_le_u_imm , branch_le_s_imm , branch_ge_u_imm ,
// branch_ge_s_imm , branch_gt_u_imm , branch_gt_s_imm

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
