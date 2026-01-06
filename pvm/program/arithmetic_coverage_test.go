package program

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestArithmeticBenchPVMStats(t *testing.T) {
	pvmPath := filepath.Join("..", "..", "services", "test_services", "arithmetic-bench", "arithmetic-bench.pvm")
	if _, err := os.Stat(pvmPath); err != nil {
		t.Skipf("missing %s (build with `make test-services`)", pvmPath)
	}

	data, err := os.ReadFile(pvmPath)
	if err != nil {
		t.Fatalf("read pvm file: %v", err)
	}

	prog, _, _, _, _, _, _, err := DecodeProgram(data)
	if err != nil {
		t.Fatalf("decode program: %v", err)
	}

	stats := prog.Analyze()
	if stats.InstructionCount == 0 {
		t.Fatalf("expected instruction count > 0")
	}

	totalArithmetic := 0
	for opcode := range opcodeNames {
		if IsArithmeticInstruction(opcode) {
			totalArithmetic++
		}
	}
	if totalArithmetic == 0 {
		t.Fatalf("expected arithmetic opcode set > 0")
	}

	arithmeticCount := 0
	coveredArithmetic := make(map[byte]struct{})
	for opcode, count := range stats.OpcodeDistribution {
		if IsArithmeticInstruction(opcode) {
			arithmeticCount += count
			coveredArithmetic[opcode] = struct{}{}
		}
	}

	coverage := float64(len(coveredArithmetic)) / float64(totalArithmetic)
	arithRatio := float64(arithmeticCount) / float64(stats.InstructionCount)

	t.Logf("Instruction count: %d", stats.InstructionCount)
	t.Logf("Arithmetic instruction count: %d (%.2f%%)", arithmeticCount, arithRatio*100)
	t.Logf("Arithmetic opcode coverage: %d/%d (%.2f%%)", len(coveredArithmetic), totalArithmetic, coverage*100)

	var missing []string
	for opcode := range opcodeNames {
		if !IsArithmeticInstruction(opcode) {
			continue
		}
		if _, ok := coveredArithmetic[opcode]; !ok {
			missing = append(missing, OpcodeToString(opcode))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Logf("Missing arithmetic opcodes: %v", missing)
	}
}
