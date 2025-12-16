package recompiler

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

const (
	TRAP_JUMP        = 0
	DIRECT_JUMP      = 1
	INDIRECT_JUMP    = 2
	CONDITIONAL      = 3
	FALLTHROUGH_JUMP = 4
	TERMINATED       = 5
)

type BasicBlock struct {
	Instructions []Instruction

	X86PC             uint64
	X86Code           []byte
	pvmPC_TO_x86Index map[uint32]int // maps PVM PC to x86 code offset, where this pc inside this block code

	JumpType               int
	IndirectJumpOffset     uint64
	IndirectSourceRegister int

	TruePC    uint64
	PVMNextPC uint64 // the next PC in PVM after this block, which is the "FALSE" case

	LastInstructionOffset int
	needPatchNextx86Pc    bool

	// Dynamic offsets for patching nextx86 slot (replaces hardcoded EcalliCodeIdx/SbrkCodeIdx)
	ecalliNextx86Offset int
	sbrkNextx86Offset   int
	ecalliTotalLen      int // Total length of ECALLI code block
	sbrkTotalLen        int // Total length of SBRK code block

	GasModel *GasModel
	GasUsage uint32
}

type Instruction struct {
	Opcode byte
	Args   []byte
	Step   int
	Pc     uint64

	SourceRegs []int
	DestRegs   []int
	Offset1    int64
	Offset2    int64
	Imm1       uint64
	Imm2       uint64
	ImmS       int64

	BranchTarget1 byte
	BranchTarget2 byte

	GasUsage int64
}

func NewBasicBlock(x86pc uint64) *BasicBlock {
	return &BasicBlock{
		Instructions:      make([]Instruction, 0, 8), // Pre-allocate with typical capacity
		X86PC:             x86pc,
		pvmPC_TO_x86Index: make(map[uint32]int, 8), // Pre-allocate with typical capacity
		GasModel:          NewGasModel(),
	}
}

func (i *Instruction) String() string {
	return fmt.Sprintf("%s", opcode_str(i.Opcode))
}

func (bb *BasicBlock) String() string {
	result := ""
	for _, inst := range bb.Instructions {
		result += inst.String()
	}
	result += fmt.Sprintf("GasCharge: %d", len(bb.Instructions))
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

// func extractTwoImm(oargs []byte) (vx uint64, vy uint64) {

func (i *Instruction) GetTwoImm() {
	vx, vy := extractTwoImm(i.Args)
	i.Imm1 = vx
	i.Imm2 = vy
}

// func extractOneOffset(oargs []byte) (vx int64) {

func (i *Instruction) GetOneOffset() {
	vx := extractOneOffset(i.Args)
	i.Offset1 = vx
}

// func extractOneRegOneImm(oargs []byte) (reg1 int, vx uint64) {
func (i *Instruction) GetOneRegOneImm() {
	reg1, vx := extractOneRegOneImm(i.Args)
	if i.Opcode == JUMP_IND {
		i.SourceRegs = []int{reg1}
	} else if i.Opcode > 50 && i.Opcode <= 58 {
		i.DestRegs = []int{reg1}
	} else if i.Opcode > 58 && i.Opcode <= 62 {
		i.SourceRegs = []int{reg1}
	} else {
		i.DestRegs = []int{reg1}
	}
	i.Imm1 = vx
}

// func extractOneReg2Imm(oargs []byte) (reg1 int, vx uint64, vy uint64) {
func (i *Instruction) GetOneRegTwoImm() {
	reg1, vx, vy := extractOneReg2Imm(i.Args)
	i.SourceRegs = []int{reg1}
	i.Imm1 = vx
	i.Imm2 = vy
}

// func extractOneRegOneImmOneOffset(oargs []byte) (registerIndexA int, vx uint64, vy int64) {
func (i *Instruction) GetOneRegOneImmOneOffset() {
	registerIndexA, vx, vy := extractOneRegOneImmOneOffset(i.Args)
	if i.Opcode == LOAD_IMM_JUMP {
		i.DestRegs = []int{registerIndexA}
	} else {
		i.SourceRegs = []int{registerIndexA}
	}
	i.Imm1 = vx
	i.Offset1 = vy
}

// func extractTwoRegisters(args []byte) (regD, regA int) {
func (i *Instruction) GetTwoRegisters() {
	regD, regA := extractTwoRegisters(i.Args)
	i.DestRegs = []int{regD}
	i.SourceRegs = []int{regA}
}

// func extractTwoRegsOneImm(args []byte) (reg1, reg2 int, imm uint64) {
func (i *Instruction) GetTwoRegsOneImm() {
	reg1, reg2, imm := extractTwoRegsOneImm(i.Args)
	if i.Opcode <= 123 {
		i.SourceRegs = []int{reg1, reg2}
	} else {
		i.DestRegs = []int{reg1}
		i.SourceRegs = []int{reg2}
	}
	i.Imm1 = imm
}

// func extractTwoRegsOneImm64(args []byte) (reg1, reg2 int, imm int64, uint64imm uint64) {
func (i *Instruction) GetTwoRegsOneImm64() {
	reg1, reg2, imm, uint64imm := extractTwoRegsOneImm64(i.Args)
	i.SourceRegs = []int{reg1, reg2}
	i.ImmS = imm
	i.Imm1 = uint64imm
}

// func extractTwoRegsOneOffset(oargs []byte) (registerIndexA, registerIndexB int, vx int64) {
func (i *Instruction) GetTwoRegsOneOffset() {
	registerIndexA, registerIndexB, vx := extractTwoRegsOneOffset(i.Args)
	i.SourceRegs = []int{registerIndexA, registerIndexB}
	i.Offset1 = vx
}

// func extractTwoRegsAndTwoImmediates(oargs []byte) (registerIndexA, registerIndexB int, vx, vy uint64) {
func (i *Instruction) GetTwoRegsAndTwoImmediates() {
	registerIndexA, registerIndexB, vx, vy := extractTwoRegsAndTwoImmediates(i.Args)
	if i.Opcode == LOAD_IMM_JUMP_IND {
		i.DestRegs = []int{registerIndexA}
		i.SourceRegs = []int{registerIndexB}
	} else {
		i.SourceRegs = []int{registerIndexA, registerIndexB}
	}
	i.Imm1 = vx
	i.Imm2 = vy
}

// func extractThreeRegs(args []byte) (reg1, reg2, dst int) {
func (i *Instruction) GetThreeRegs() {
	reg1, reg2, dst := extractThreeRegs(i.Args)
	i.SourceRegs = []int{reg1, reg2}
	i.DestRegs = []int{dst}
}

func (i *Instruction) GetNoArgs() {
	// No arguments to extract
}

func (i *Instruction) GetOneImm() {
	lx := types.DecodeE_l(i.Args)
	i.Imm1 = lx
}

func (i *Instruction) GetArgs() {

	opcode := i.Opcode
	switch {
	case opcode <= 3: // A.5.1 No arguments
		i.GetNoArgs()
	case opcode == ECALLI: // A.5.2 One immediate
		i.GetOneImm()
	case opcode == LOAD_IMM_64: // A.5.3 One Register and One Extended Width Immediate
		i.GetOneRegOneImm()
	case 30 <= opcode && opcode <= 33: // A.5.4 Two Immediates
		i.GetTwoImm()
	case opcode == JUMP: // A.5.5 One offset
		i.GetOneOffset()
	case 50 <= opcode && opcode <= 62: // A.5.6 One Register and One Immediate
		i.GetOneRegOneImm()
	case 70 <= opcode && opcode <= 73: // A.5.7 One Register and Two Immediate
		i.GetOneRegTwoImm()
	case 80 <= opcode && opcode <= 90: // A.5.8 One Register, One Immediate and One Offset
		i.GetOneRegOneImmOneOffset()
	case 100 <= opcode && opcode <= 111: // A.5.9 Two Registers
		i.GetTwoRegisters()
	case 120 <= opcode && opcode <= 161: // A.5.10 Two Registers and One Immediate
		i.GetTwoRegsOneImm()
	case 170 <= opcode && opcode <= 175: // A.5.11 Two Registers and One Offset
		i.GetTwoRegsOneOffset()
	case opcode == LOAD_IMM_JUMP_IND: // A.5.12 Two Register and Two Immediate
		i.GetTwoRegsAndTwoImmediates()
	case 190 <= opcode && opcode <= 230: // A.5.13 Three Registers
		i.GetThreeRegs()
	default:

	}
}
