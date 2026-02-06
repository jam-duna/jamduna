package recompiler

import "fmt"

type GasModel struct {
	Index                       int64 // instruction index
	CycleCounter                int64 // c
	NextReorderBufferEntryIndex int64 // n
	DecodingSlots               int64 // d
	ExecutionSlots              int64 // e

	StatusVector            map[int]int              // s
	CycleCounterVector      []int                    // c
	PredecessorVector       map[int][]int            // p
	RegisterVector          map[int]map[int]struct{} // r
	ExecutionUnitCostVector []int                    // e

	ExecuteionUnits      []ExecuteionUnit
	CompareExecutionUnit ExecuteionUnit

	DebugInsts map[int]Instruction
	DebugSlots map[int]string
}

func NewGasModel() *GasModel {
	return &GasModel{
		CycleCounter:                0,
		NextReorderBufferEntryIndex: 0,
		DecodingSlots:               4,
		ExecutionSlots:              5,
		StatusVector:                make(map[int]int),
		CycleCounterVector:          make([]int, 0),
		PredecessorVector:           make(map[int][]int),
		RegisterVector:              make(map[int]map[int]struct{}),
		ExecutionUnitCostVector:     make([]int, 0),
		ExecuteionUnits:             make([]ExecuteionUnit, 0),
		CompareExecutionUnit: ExecuteionUnit{
			ALU:   InitALUUnit,
			Load:  InitLoadUnit,
			Store: InitStoreUnit,
			Mul:   InitMulUnit,
			Div:   InitDivUnit,
		},

		DebugInsts: make(map[int]Instruction),
		DebugSlots: make(map[int]string),
	}
}

func (g *GasModel) PrintStatus() {
	fmt.Printf("n=%d, c=%d, d=%d, e=%d\ns_len=%d, c_len=%d, p_len=%d, r_len=%d, e_len=%d, exec_u_len=%d\n",
		g.NextReorderBufferEntryIndex,
		g.CycleCounter,
		g.DecodingSlots,
		g.ExecutionSlots,
		len(g.StatusVector),
		len(g.CycleCounterVector),
		len(g.PredecessorVector),
		len(g.RegisterVector),
		len(g.ExecutionUnitCostVector),
		len(g.ExecuteionUnits),
	)
}

type ExecuteionUnit struct {
	ALU   int
	Load  int
	Store int
	Mul   int
	Div   int
}

const (
	InitALUUnit   = 4
	InitLoadUnit  = 4
	InitStoreUnit = 4
	InitMulUnit   = 1
	InitDivUnit   = 1
)

type GasCost struct {
	CheckSameReg bool
	CycleCost    int
	DecodeGas1   int
	DecodeGas2   int
	ALUGas       int
	LoadGas      int
	StoreGas     int
	MulGas       int
	DivGas       int
}

func (g *GasCost) GetDecodeGas(i Instruction) int {
	if i.Opcode == SHLO_L_64 ||
		i.Opcode == SHLO_R_64 ||
		i.Opcode == SHAR_R_64 ||
		i.Opcode == ROT_L_64 ||
		i.Opcode == ROT_R_64 ||
		i.Opcode == SHLO_L_32 ||
		i.Opcode == SHLO_R_32 ||
		i.Opcode == SHAR_R_32 ||
		i.Opcode == ROT_L_32 ||
		i.Opcode == ROT_R_32 {
		// special case for shift/rotate instructions
		srcReg := i.SourceRegs[0]
		dstReg := i.DestRegs[0]
		if srcReg == dstReg {
			return g.DecodeGas1
		}
		return g.DecodeGas2
	} else if g.CheckSameReg {
		for _, srcReg := range i.SourceRegs {
			for _, dstReg := range i.DestRegs {
				if srcReg == dstReg {
					return g.DecodeGas1
				}
			}
		}
		return g.DecodeGas2
	}
	return g.DecodeGas1
}

func (g *GasCost) GetBranchCycleCost(i Instruction, dstOp1, dstOp2 byte) int {
	if dstOp1 == UNLIKELY || dstOp2 == UNLIKELY || dstOp1 == TRAP || dstOp2 == TRAP {
		return 1
	}
	return 20
}

func (i *Instruction) IsBranchInstruction() bool {
	if (i.Opcode >= BRANCH_EQ && i.Opcode <= BRANCH_GE_S) || (i.Opcode >= BRANCH_EQ_IMM && i.Opcode <= BRANCH_GT_S_IMM) {
		return true
	}
	return false
}

func (i *Instruction) GetGasCost() GasCost {
	return InstrGasTable[i.Opcode]
}

type GasTable map[byte]GasCost

var InstrGasTable = GasTable{
	// Format: CheckSameReg, CycleCost, DecodeGas1, DecodeGas2, ALUGas, LoadGas, StoreGas, MulGas, DivGas

	MOVE_REG: {false, 0, 1, 0, 0, 0, 0, 0, 0},

	// Bitwise operations - P(1,2)
	AND: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	XOR: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	OR:  {true, 1, 1, 2, 1, 0, 0, 0, 0},

	// 64-bit arithmetic - P(1,2)
	ADD_64: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	SUB_64: {true, 1, 1, 2, 1, 0, 0, 0, 0},

	// 32-bit arithmetic - P(2,3)
	ADD_32: {true, 2, 2, 3, 1, 0, 0, 0, 0},
	SUB_32: {true, 2, 2, 3, 1, 0, 0, 0, 0},

	// Bitwise operations with immediate - P(1,2)
	AND_IMM: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	XOR_IMM: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	OR_IMM:  {true, 1, 1, 2, 1, 0, 0, 0, 0},

	// 64-bit arithmetic with immediate - P(1,2)
	ADD_IMM_64:    {true, 1, 1, 2, 1, 0, 0, 0, 0},
	SHLO_R_IMM_64: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	SHAR_R_IMM_64: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	SHLO_L_IMM_64: {true, 1, 1, 2, 1, 0, 0, 0, 0},
	ROT_R_64_IMM:  {true, 1, 1, 2, 1, 0, 0, 0, 0},
	REVERSE_BYTES: {true, 1, 1, 2, 1, 0, 0, 0, 0},

	// 32-bit arithmetic with immediate - P(2,3)
	ADD_IMM_32:    {true, 2, 2, 3, 1, 0, 0, 0, 0},
	SHLO_R_IMM_32: {true, 2, 2, 3, 1, 0, 0, 0, 0},
	SHAR_R_IMM_32: {true, 2, 2, 3, 1, 0, 0, 0, 0},
	SHLO_L_IMM_32: {true, 2, 2, 3, 1, 0, 0, 0, 0},
	ROT_R_32_IMM:  {true, 2, 2, 3, 1, 0, 0, 0, 0},

	// Bit counting operations (no P)
	COUNT_SET_BITS_64:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	COUNT_SET_BITS_32:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	LEADING_ZERO_BITS_64:  {false, 1, 1, 0, 1, 0, 0, 0, 0},
	LEADING_ZERO_BITS_32:  {false, 1, 1, 0, 1, 0, 0, 0, 0},
	SIGN_EXTEND_8:         {false, 1, 1, 0, 1, 0, 0, 0, 0},
	SIGN_EXTEND_16:        {false, 1, 1, 0, 1, 0, 0, 0, 0},
	ZERO_EXTEND_16:        {false, 1, 1, 0, 1, 0, 0, 0, 0},
	TRAILING_ZERO_BITS_64: {false, 2, 1, 0, 2, 0, 0, 0, 0},
	TRAILING_ZERO_BITS_32: {false, 2, 1, 0, 2, 0, 0, 0, 0},

	// 64-bit shift operations - P(2,3)
	SHLO_L_64: {true, 1, 2, 3, 1, 0, 0, 0, 0},
	SHLO_R_64: {true, 1, 2, 3, 1, 0, 0, 0, 0},
	SHAR_R_64: {true, 1, 2, 3, 1, 0, 0, 0, 0},
	ROT_L_64:  {true, 1, 2, 3, 1, 0, 0, 0, 0},
	ROT_R_64:  {true, 1, 2, 3, 1, 0, 0, 0, 0},

	// 32-bit shift operations - P(3,4)
	SHLO_L_32: {true, 2, 3, 4, 1, 0, 0, 0, 0},
	SHLO_R_32: {true, 2, 3, 4, 1, 0, 0, 0, 0},
	SHAR_R_32: {true, 2, 3, 4, 1, 0, 0, 0, 0},
	ROT_L_32:  {true, 2, 3, 4, 1, 0, 0, 0, 0},
	ROT_R_32:  {true, 2, 3, 4, 1, 0, 0, 0, 0},

	// Alternate shift operations (no P)
	SHLO_L_IMM_ALT_64: {false, 1, 3, 0, 1, 0, 0, 0, 0},
	SHLO_R_IMM_ALT_64: {false, 1, 3, 0, 1, 0, 0, 0, 0},
	SHAR_R_IMM_ALT_64: {false, 1, 3, 0, 1, 0, 0, 0, 0},
	ROT_R_64_IMM_ALT:  {false, 1, 3, 0, 1, 0, 0, 0, 0},
	SHLO_L_IMM_ALT_32: {false, 2, 4, 0, 1, 0, 0, 0, 0},
	SHLO_R_IMM_ALT_32: {false, 2, 4, 0, 1, 0, 0, 0, 0},
	SHAR_R_IMM_ALT_32: {false, 2, 4, 0, 1, 0, 0, 0, 0},
	ROT_R_32_IMM_ALT:  {false, 2, 4, 0, 1, 0, 0, 0, 0},

	// Comparison and set operations (no P)
	SET_LT_U:     {false, 3, 3, 0, 1, 0, 0, 0, 0},
	SET_LT_S:     {false, 3, 3, 0, 1, 0, 0, 0, 0},
	SET_LT_U_IMM: {false, 3, 3, 0, 1, 0, 0, 0, 0},
	SET_LT_S_IMM: {false, 3, 3, 0, 1, 0, 0, 0, 0},
	SET_GT_U_IMM: {false, 3, 3, 0, 1, 0, 0, 0, 0},
	SET_GT_S_IMM: {false, 3, 3, 0, 1, 0, 0, 0, 0},

	// Conditional move (no P)
	CMOV_IZ:     {false, 2, 2, 0, 1, 0, 0, 0, 0},
	CMOV_NZ:     {false, 2, 2, 0, 1, 0, 0, 0, 0},
	CMOV_IZ_IMM: {false, 2, 3, 0, 1, 0, 0, 0, 0},
	CMOV_NZ_IMM: {false, 2, 3, 0, 1, 0, 0, 0, 0},

	// Min/max operations - P(2,3)
	MAX:   {true, 3, 2, 3, 1, 0, 0, 0, 0},
	MAX_U: {true, 3, 2, 3, 1, 0, 0, 0, 0},
	MIN:   {true, 3, 2, 3, 1, 0, 0, 0, 0},
	MIN_U: {true, 3, 2, 3, 1, 0, 0, 0, 0},

	// Load operations (no P, 'm' for cycle cost means load latency)
	LOAD_IND_U8:  {false, 25, 1, 0, 1, 1, 0, 0, 0}, // 'm' treated as 25
	LOAD_IND_I8:  {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_IND_U16: {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_IND_I16: {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_IND_U32: {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_IND_I32: {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_IND_U64: {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_U8:      {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_I8:      {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_U16:     {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_I16:     {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_U32:     {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_I32:     {false, 25, 1, 0, 1, 1, 0, 0, 0},
	LOAD_U64:     {false, 25, 1, 0, 1, 1, 0, 0, 0},

	// Store operations (no P)
	STORE_IMM_IND_U8:  {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_IND_U16: {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_IND_U32: {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_IND_U64: {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IND_U8:      {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IND_U16:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IND_U32:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IND_U64:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_U8:      {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_U16:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_U32:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_IMM_U64:     {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_U8:          {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_U16:         {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_U32:         {false, 25, 1, 0, 1, 0, 1, 0, 0},
	STORE_U64:         {false, 25, 1, 0, 1, 0, 1, 0, 0},

	// Branch operations (no P, 'b' treated as 1) 170-175 81-90
	BRANCH_EQ:       {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_NE:       {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LT_U:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LT_S:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GE_U:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GE_S:     {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_EQ_IMM:   {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_NE_IMM:   {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LT_U_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LE_U_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GE_U_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GT_U_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LT_S_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_LE_S_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GE_S_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},
	BRANCH_GT_S_IMM: {false, 1, 1, 0, 1, 0, 0, 0, 0},

	// Division and remainder operations (no P)
	DIV_U_32: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	DIV_S_32: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	REM_U_32: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	REM_S_32: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	DIV_U_64: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	DIV_S_64: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	REM_U_64: {false, 60, 4, 0, 1, 0, 0, 0, 1},
	REM_S_64: {false, 60, 4, 0, 1, 0, 0, 0, 1},

	// Bitwise operations (no P for and_inv/or_inv, P for xnor)
	AND_INV: {false, 2, 3, 0, 1, 0, 0, 0, 0},
	OR_INV:  {false, 2, 3, 0, 1, 0, 0, 0, 0},
	XNOR:    {true, 2, 2, 3, 1, 0, 0, 0, 0},

	// Negate and add operations (no P)
	NEG_ADD_IMM_64: {false, 2, 3, 0, 1, 0, 0, 0, 0},
	NEG_ADD_IMM_32: {false, 3, 4, 0, 1, 0, 0, 0, 0},

	// Load immediate (no P)
	LOAD_IMM:    {false, 1, 1, 0, 0, 0, 0, 0, 0},
	LOAD_IMM_64: {false, 1, 2, 0, 0, 0, 0, 0, 0},

	// Multiplication operations - P(1,2) for 64-bit, P(2,3) for 32-bit
	MUL_64:     {true, 3, 1, 2, 1, 0, 0, 1, 0},
	MUL_32:     {true, 4, 2, 3, 1, 0, 0, 1, 0},
	MUL_IMM_64: {true, 3, 1, 2, 1, 0, 0, 1, 0},
	MUL_IMM_32: {true, 4, 2, 3, 1, 0, 0, 1, 0},

	// Upper multiplication operations (no P)
	MUL_UPPER_S_S: {false, 4, 4, 0, 1, 0, 0, 1, 0},
	MUL_UPPER_U_U: {false, 4, 4, 0, 1, 0, 0, 1, 0},
	MUL_UPPER_S_U: {false, 6, 4, 0, 1, 0, 0, 1, 0},

	// Control flow (no P)
	TRAP:        {false, 2, 1, 0, 0, 0, 0, 0, 0},
	FALLTHROUGH: {false, 2, 1, 0, 0, 0, 0, 0, 0},
	UNLIKELY:    {false, 40, 1, 0, 0, 0, 0, 0, 0},

	// Jump operations (no P)
	JUMP:              {false, 15, 1, 0, 0, 0, 0, 0, 0},
	LOAD_IMM_JUMP:     {false, 15, 1, 0, 0, 0, 0, 0, 0},
	JUMP_IND:          {false, 22, 1, 0, 0, 0, 0, 0, 0},
	LOAD_IMM_JUMP_IND: {false, 22, 1, 0, 0, 0, 0, 0, 0},

	// System calls (no P)
	ECALLI: {false, 100, 4, 0, 1, 0, 0, 0, 0},
	SBRK:   {false, 100, 4, 0, 1, 0, 0, 0, 0},
}

/*
Instruction ˇc ˇd ˇxA ˇxL ˇxS ˇxM ˇxD

	and 1 P(1,2) 1 0 0 0 0
	xor 1 P(1,2) 1 0 0 0 0
	or 1 P(1,2) 1 0 0 0 0
	add_64 1 P(1,2) 1 0 0 0 0
	sub_64 1 P(1,2) 1 0 0 0 0
	add_32 2 P(2,3) 1 0 0 0 0
	sub_32 2 P(2,3) 1 0 0 0 0
	and_imm 1 P(1,2) 1 0 0 0 0
	xor_imm 1 P(1,2) 1 0 0 0 0
	or_imm 1 P(1,2) 1 0 0 0 0
	add_imm_64 1 P(1,2) 1 0 0 0 0
	shlo_r_imm_64 1 P(1,2) 1 0 0 0 0
	shar_r_imm_64 1 P(1,2) 1 0 0 0 0
	shlo_l_imm_64 1 P(1,2) 1 0 0 0 0
	rot_r_64_imm 1 P(1,2) 1 0 0 0 0
	reverse_bytes 1 P(1,2) 1 0 0 0 0
	add_imm_32 2 P(2,3) 1 0 0 0 0
	shlo_r_imm_32 2 P(2,3) 1 0 0 0 0
	shar_r_imm_32 2 P(2,3) 1 0 0 0 0
	shlo_l_imm_32 2 P(2,3) 1 0 0 0 0
	rot_r_32_imm 2 P(2,3) 1 0 0 0 0
	count_set_bits_64 1 1 1 0 0 0 0
	count_set_bits_32 1 1 1 0 0 0 0
	leading_zero_bits_64 1 1 1 0 0 0 0
	leading_zero_bits_32 1 1 1 0 0 0 0
	sign_extend_8 1 1 1 0 0 0 0
	sign_extend_16 1 1 1 0 0 0 0
	zero_extend_16 1 1 1 0 0 0 0
	trailing_zero_bits_64 2 1 2 0 0 0 0
	trailing_zero_bits_32 2 1 2 0 0 0 0
	shlo_l_64 1 P(2,3) 1 0 0 0 0
	shlo_r_64 1 P(2,3) 1 0 0 0 0
	shar_r_64 1 P(2,3) 1 0 0 0 0
	rot_l_64 1 P(2,3) 1 0 0 0 0
	rot_r_64 1 P(2,3) 1 0 0 0 0
	shlo_l_32 2 P(3,4) 1 0 0 0 0
	shlo_r_32 2 P(3,4) 1 0 0 0 0
	shar_r_32 2 P(3,4) 1 0 0 0 0
	rot_l_32 2 P(3,4) 1 0 0 0 0
	rot_r_32 2 P(3,4) 1 0 0 0 0
	shlo_l_imm_alt_64 1 3 1 0 0 0 0
	shlo_r_imm_alt_64 1 3 1 0 0 0 0
	shar_r_imm_alt_64 1 3 1 0 0 0 0
	rot_r_64_imm_alt 1 3 1 0 0 0 0
	shlo_l_imm_alt_32 2 4 1 0 0 0 0
	shlo_r_imm_alt_32 2 4 1 0 0 0 0
	shar_r_imm_alt_32 2 4 1 0 0 0 0
	rot_r_32_imm_alt 2 4 1 0 0 0 0
	set_lt_u 3 3 1 0 0 0 0
	set_lt_s 3 3 1 0 0 0 0
	set_lt_u_imm 3 3 1 0 0 0 0
	set_lt_s_imm 3 3 1 0 0 0 0
	set_gt_u_imm 3 3 1 0 0 0 0
	set_gt_s_imm 3 3 1 0 0 0 0
	cmov_iz 2 2 1 0 0 0 0
	cmov_nz 2 2 1 0 0 0 0
	cmov_iz_imm 2 3 1 0 0 0 0
	cmov_nz_imm 2 3 1 0 0 0 0
	max 3 P(2,3) 1 0 0 0 0
	max_u 3 P(2,3) 1 0 0 0 0
	min 3 P(2,3) 1 0 0 0 0
	min_u 3 P(2,3) 1 0 0 0 0
	load_ind_u8 m 1 1 1 0 0 0
	load_ind_i8 m 1 1 1 0 0 0
	load_ind_u16 m 1 1 1 0 0 0
	load_ind_i16 m 1 1 1 0 0 0
	load_ind_u32 m 1 1 1 0 0 0
	load_ind_i32 m 1 1 1 0 0 0
	load_ind_u64 m 1 1 1 0 0 0
	load_u8 m 1 1 1 0 0 0
	load_i8 m 1 1 1 0 0 0
	load_u16 m 1 1 1 0 0 0
	load_i16 m 1 1 1 0 0 0
	load_u32 m 1 1 1 0 0 0
	load_i32 m 1 1 1 0 0 0
	load_u64 m 1 1 1 0 0 0
	store_imm_ind_u8 25 1 1 0 1 0 0
	store_imm_ind_u16 25 1 1 0 1 0 0
	store_imm_ind_u32 25 1 1 0 1 0 0
	store_imm_ind_u64 25 1 1 0 1 0 0
	store_ind_u8 25 1 1 0 1 0 0
	store_ind_u16 25 1 1 0 1 0 0
	store_ind_u32 25 1 1 0 1 0 0
	store_ind_u64 25 1 1 0 1 0 0
	store_imm_u8 25 1 1 0 1 0 0
	store_imm_u16 25 1 1 0 1 0 0
	store_imm_u32 25 1 1 0 1 0 0
	store_imm_u64 25 1 1 0 1 0 0
	store_u8 25 1 1 0 1 0 0
	store_u16 25 1 1 0 1 0 0
	store_u32 25 1 1 0 1 0 0
	store_u64 25 1 1 0 1 0 0
	branch_eq b 1 1 0 0 0 0
	branch_ne b 1 1 0 0 0 0
	branch_lt_u b 1 1 0 0 0 0
	branch_lt_s b 1 1 0 0 0 0
	branch_ge_u b 1 1 0 0 0 0
	branch_ge_s b 1 1 0 0 0 0
	branch_eq_imm b 1 1 0 0 0 0
	branch_ne_imm b 1 1 0 0 0 0
	branch_lt_u_imm b 1 1 0 0 0 0
	branch_le_u_imm b 1 1 0 0 0 0
	branch_ge_u_imm b 1 1 0 0 0 0
	branch_gt_u_imm b 1 1 0 0 0 0
	branch_lt_s_imm b 1 1 0 0 0 0
	branch_le_s_imm b 1 1 0 0 0 0
	branch_ge_s_imm b 1 1 0 0 0 0
	branch_gt_s_imm b 1 1 0 0 0 0
	div_u_32 60 4 1 0 0 0 1
	div_s_32 60 4 1 0 0 0 1
	rem_u_32 60 4 1 0 0 0 1
	rem_s_32 60 4 1 0 0 0 1
	div_u_64 60 4 1 0 0 0 1
	div_s_64 60 4 1 0 0 0 1
	rem_u_64 60 4 1 0 0 0 1
	rem_s_64 60 4 1 0 0 0 1
	and_inv 2 3 1 0 0 0 0
	or_inv 2 3 1 0 0 0 0
	xnor 2 P(2,3) 1 0 0 0 0
	neg_add_imm_64 2 3 1 0 0 0 0
	neg_add_imm_32 3 4 1 0 0 0 0
	load_imm 1 1 0 0 0 0 0
	load_imm_64 1 2 0 0 0 0 0
	mul_64 3 P(1,2) 1 0 0 1 0
	mul_32 4 P(2,3) 1 0 0 1 0
	mul_imm_64 3 P(1,2) 1 0 0 1 0
	mul_imm_32 4 P(2,3) 1 0 0 1 0
	mul_upper_s_s 4 4 1 0 0 1 0
	mul_upper_u_u 4 4 1 0 0 1 0
	mul_upper_s_u 6 4 1 0 0 1 0
	trap 2 1 0 0 0 0 0
	fallthrough 2 1 0 0 0 0 0
	unlikely 40 1 0 0 0 0 0
	jump 15 1 0 0 0 0 0
	load_imm_jump 15 1 0 0 0 0 0
	jump_ind 22 1 0 0 0 0 0
	load_imm_jump_ind 22 1 0 0 0 0 0
	ecalli 100 4 1 0 0 0 0
	sbrk 100 4 1 0 0 0 0
*/
