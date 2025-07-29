package pvm

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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
		t.Logf("Disassembled %s successfully", file.Name())
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
