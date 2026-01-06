package program

import (
	"testing"
)

func TestAnalyze(t *testing.T) {
	// Create a simple program for testing
	// K bitmask: bit 0 = instruction start, bit 2 = basic block start
	prog := &Program{
		Code: []byte{
			0x33, // opcode at PC=0 (STORE_IMM_U64)
			0x00, // arg
			0x00, // arg
			0x00, // arg
			0x28, // opcode at PC=4 (JUMP - 40)
			0x00, // arg
		},
		K: []byte{
			0x03, // PC=0: instruction start (1) + basic block start (2) = 3
			0x00, // PC=1: not instruction
			0x00, // PC=2: not instruction
			0x00, // PC=3: not instruction
			0x03, // PC=4: instruction start (1) + basic block start (2) = 3
			0x00, // PC=5: not instruction
		},
	}

	stats := prog.Analyze()

	if stats.InstructionCount != 2 {
		t.Errorf("Expected 2 instructions, got %d", stats.InstructionCount)
	}

	if stats.BasicBlockCount != 2 {
		t.Errorf("Expected 2 basic blocks, got %d", stats.BasicBlockCount)
	}

	// Check opcode distribution
	if stats.OpcodeDistribution[0x33] != 1 {
		t.Errorf("Expected 1 occurrence of opcode 0x33, got %d", stats.OpcodeDistribution[0x33])
	}
	if stats.OpcodeDistribution[0x28] != 1 {
		t.Errorf("Expected 1 occurrence of opcode 0x28, got %d", stats.OpcodeDistribution[0x28])
	}
}

func TestCountInstructions(t *testing.T) {
	prog := &Program{
		Code: []byte{0x00, 0x00, 0x00, 0x01, 0x00},
		K:    []byte{0x01, 0x00, 0x00, 0x01, 0x00}, // 2 instructions
	}

	count := prog.CountInstructions()
	if count != 2 {
		t.Errorf("Expected 2 instructions, got %d", count)
	}
}

func TestCountBasicBlocks(t *testing.T) {
	prog := &Program{
		Code: []byte{0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00},
		K: []byte{
			0x03, // instruction + basic block start
			0x00,
			0x00,
			0x03, // instruction + basic block start
			0x00,
			0x00,
			0x01, // instruction only (not a new block)
		},
	}

	count := prog.CountBasicBlocks()
	if count != 2 {
		t.Errorf("Expected 2 basic blocks, got %d", count)
	}
}

func TestGetInstructions(t *testing.T) {
	prog := &Program{
		Code: []byte{0x33, 0x00, 0x00, 0x28},
		K:    []byte{0x03, 0x00, 0x00, 0x01},
	}

	instructions := prog.GetInstructions()
	if len(instructions) != 2 {
		t.Errorf("Expected 2 instructions, got %d", len(instructions))
	}

	// First instruction
	if instructions[0].PC != 0 {
		t.Errorf("Expected first instruction at PC=0, got %d", instructions[0].PC)
	}
	if instructions[0].Opcode != 0x33 {
		t.Errorf("Expected first opcode 0x33, got 0x%02x", instructions[0].Opcode)
	}
	if !instructions[0].IsBasicBlockStart {
		t.Error("Expected first instruction to be basic block start")
	}

	// Second instruction
	if instructions[1].PC != 3 {
		t.Errorf("Expected second instruction at PC=3, got %d", instructions[1].PC)
	}
	if instructions[1].Opcode != 0x28 {
		t.Errorf("Expected second opcode 0x28, got 0x%02x", instructions[1].Opcode)
	}
	if instructions[1].IsBasicBlockStart {
		t.Error("Expected second instruction to NOT be basic block start")
	}
}

func TestGetBasicBlockBoundaries(t *testing.T) {
	prog := &Program{
		Code: []byte{0x00, 0x00, 0x00, 0x00, 0x00},
		K: []byte{
			0x03, // basic block start at PC=0
			0x00,
			0x03, // basic block start at PC=2
			0x01, // instruction but not block start
			0x03, // basic block start at PC=4
		},
	}

	boundaries := prog.GetBasicBlockBoundaries()
	expected := []int{0, 2, 4}

	if len(boundaries) != len(expected) {
		t.Errorf("Expected %d boundaries, got %d", len(expected), len(boundaries))
	}

	for i, b := range boundaries {
		if b != expected[i] {
			t.Errorf("Expected boundary[%d]=%d, got %d", i, expected[i], b)
		}
	}
}

func TestEmptyProgram(t *testing.T) {
	prog := &Program{
		Code: []byte{},
		K:    []byte{},
	}

	stats := prog.Analyze()
	if stats.InstructionCount != 0 {
		t.Errorf("Expected 0 instructions for empty program, got %d", stats.InstructionCount)
	}
	if stats.BasicBlockCount != 0 {
		t.Errorf("Expected 0 basic blocks for empty program, got %d", stats.BasicBlockCount)
	}
}
