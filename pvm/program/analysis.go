package program

// ProgramStats contains statistics about a PVM program
type ProgramStats struct {
	InstructionCount int            // Total number of PVM instructions
	BasicBlockCount  int            // Total number of basic blocks
	OpcodeDistribution map[byte]int // Distribution of opcodes
}

// Analyze analyzes the program and returns statistics including
// instruction count and basic block count
func (p *Program) Analyze() *ProgramStats {
	stats := &ProgramStats{
		OpcodeDistribution: make(map[byte]int),
	}

	if len(p.Code) == 0 || len(p.K) == 0 {
		return stats
	}

	// Iterate through the bitmask to find instruction starts
	// K[i] & 1 == 1 means there's an instruction at position i
	// K[i] & 2 == 2 means it's the start of a basic block
	for i := 0; i < len(p.K); i++ {
		if p.K[i]&1 == 1 {
			// This is a valid instruction position
			stats.InstructionCount++

			// Count opcode if within code bounds
			if i < len(p.Code) {
				opcode := p.Code[i]
				stats.OpcodeDistribution[opcode]++
			}

			// Check if this is a basic block start
			if p.K[i]&2 == 2 {
				stats.BasicBlockCount++
			}
		}
	}

	return stats
}

// CountInstructions returns the total number of PVM instructions in the program
func (p *Program) CountInstructions() int {
	count := 0
	for i := 0; i < len(p.K); i++ {
		if p.K[i]&1 == 1 {
			count++
		}
	}
	return count
}

// CountBasicBlocks returns the total number of basic blocks in the program
func (p *Program) CountBasicBlocks() int {
	count := 0
	for i := 0; i < len(p.K); i++ {
		// K[i] & 2 == 2 means it's the start of a basic block
		if p.K[i]&2 == 2 {
			count++
		}
	}
	return count
}

// GetInstructionDetails returns detailed information about each instruction
type InstructionInfo struct {
	PC     int    // Program counter (position in code)
	Opcode byte   // Instruction opcode
	IsBasicBlockStart bool // Whether this instruction starts a basic block
}

// GetInstructions returns a list of all instructions with their details
func (p *Program) GetInstructions() []InstructionInfo {
	var instructions []InstructionInfo

	for i := 0; i < len(p.K); i++ {
		if p.K[i]&1 == 1 {
			info := InstructionInfo{
				PC:                i,
				IsBasicBlockStart: (p.K[i]&2 == 2),
			}
			if i < len(p.Code) {
				info.Opcode = p.Code[i]
			}
			instructions = append(instructions, info)
		}
	}

	return instructions
}

// GetBasicBlockBoundaries returns the PC positions where each basic block starts
func (p *Program) GetBasicBlockBoundaries() []int {
	var boundaries []int

	for i := 0; i < len(p.K); i++ {
		if p.K[i]&2 == 2 {
			boundaries = append(boundaries, i)
		}
	}

	return boundaries
}
