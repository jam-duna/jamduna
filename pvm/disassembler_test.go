package pvm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestDisassemble(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm/programs"

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	for _, file := range files {

		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var testCase TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		t.Run(file.Name(), func(t *testing.T) {
			testSingleCase(t, testCase)
		})
	}
}

func testSingleCase(t *testing.T, testCase TestCase) {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	pvm := NewVM(serviceAcct, testCase.Code, testCase.InitialRegs, uint64(testCase.InitialPC), hostENV, false, []byte{}, BackendInterpreter)

	opcodes, codeLines := pvm.DisassemblePVMOfficial()
	name := testCase.Name
	instrFilePath := "../jamtestvectors/pvm/pure_disassembly_tests/" + name + ".txt"
	instructions, err := ParseInstructionLines(instrFilePath)
	if err != nil {
		t.Fatalf("Failed to parse instruction lines from file %s: %v", instrFilePath, err)
	}
	if len(opcodes) != len(instructions) {
		t.Fatalf("Number of disassembled instructions (%d) does not match expected (%d) for %s", len(opcodes), len(instructions), name)
	}
	for i, instruction := range instructions {
		// test if instructions match with disassembled codeLines
		if i >= len(codeLines) {
			t.Fatalf("Disassembled codeLines for %s has fewer lines than expected", name)
		}
		if codeLines[i] != instruction {
			t.Errorf("Disassembled opcode %d (%s) for %s does not match expected: got %s, want %s", opcodes[i], opcode_str(opcodes[i]), name, codeLines[i], instruction)
		}
	}

}

func ParseInstructionLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var instructions []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		instructions = append(instructions, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return instructions, nil
}

func ParseInstructions(r io.Reader) (map[int]string, error) {
	re := regexp.MustCompile(`\[\d+\]:\s+(\d+):\s+(.+)`)
	instMap := make(map[int]string)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		pcStr, inst := m[1], m[2]
		if inst == "charge_gas" {
			continue
		}

		pc, err := strconv.Atoi(pcStr)
		if err != nil {
			continue
		}
		instMap[pc] = inst
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return instMap, nil
}
func TestParseInstructions(t *testing.T) {
	sampleLog := `
2025-08-04 11:55:51 [D] ... [81]: 77309: charge_gas
2025-08-04 11:55:51 [D] ... [82]: 77309: a0 = 0
2025-08-04 11:55:51 [D] ... [83]: 77311: ret
2025-08-04 11:55:51 [D] ... [84]: 77313: charge_gas
2025-08-04 11:55:51 [D] ... [85]: 77313: sp = sp + 0xffffffffffffffd8
2025-08-04 11:55:51 [D] ... [86]: 77316: u64 [sp + 0x20] = ra
2025-08-04 11:55:51 [D] ... [87]: 77319: u64 [sp + 0x18] = s0
2025-08-04 11:55:51 [D] ... [88]: 77322: u64 [sp + 0x10] = s1
2025-08-04 11:55:51 [D] ... [89]: 77325: s1 = a1 + 0x1f
2025-08-04 11:55:51 [D] ... [90]: 77328: s0 = 0x30008
2025-08-04 11:55:51 [D] ... [91]: 77333: a2 = s0 + 0x2748
2025-08-04 11:55:51 [D] ... [92]: 77337: a2 = a2 & 0xfffffffffffffffc
2025-08-04 11:55:51 [D] ... [93]: 77340: fallthrough
2025-08-04 11:55:51 [D] ... [94]: 77343: foo = bar
`

	expected := map[int]string{
		77309: "a0 = 0",
		77311: "ret",
		77313: "sp = sp + 0xffffffffffffffd8",
		77316: "u64 [sp + 0x20] = ra",
		77319: "u64 [sp + 0x18] = s0",
		77322: "u64 [sp + 0x10] = s1",
		77325: "s1 = a1 + 0x1f",
		77328: "s0 = 0x30008",
		77333: "a2 = s0 + 0x2748",
		77337: "a2 = a2 & 0xfffffffffffffffc",
		77340: "fallthrough",
		77343: "foo = bar",
	}

	got, err := ParseInstructions(strings.NewReader(sampleLog))
	if err != nil {
		t.Fatalf("ParseInstructions returned error: %v", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(got))
	}
	for pc, inst := range expected {
		if got[pc] != inst {
			t.Errorf("pc %d: expected %q, got %q", pc, inst, got[pc])
		}
	}
}

func TestParseParityTrace(t *testing.T) {
	f, err := os.Open("../statedb/storage_light_000000003/pvm-trace.log")
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	defer f.Close()
	instMap, err := ParseInstructions(f)
	if err != nil {
		log.Fatalf("failed to parse instructions: %v", err)
	}
	for pc, inst := range instMap {
		fmt.Printf("[%d] = %q\n", pc, inst)
	}
}
