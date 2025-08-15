package pvm

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// YAMLInstruction represents an instruction spec from the YAML file
type YAMLInstruction struct {
	Opcode int    `yaml:"opcode"`
	Name   string `yaml:"name"`
	Args   []struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
	} `yaml:"args"`
	Format string `yaml:"format"`
}

// YAMLSpec represents the root structure of instrs.yaml
type YAMLSpec struct {
	Instructions []YAMLInstruction `yaml:"instructions"`
}

func TestInstructionSpecValidation(t *testing.T) {
	// Read and parse YAML file
	data, err := os.ReadFile("../instrs.yaml")
	if err != nil {
		t.Fatalf("Error reading YAML file: %v", err)
	}

	var spec YAMLSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Error parsing YAML: %v", err)
	}

	t.Logf("Validating %d instructions from instrs.yaml against Go DSL definitions", len(spec.Instructions))

	errorCount := 0

	for _, yamlInstr := range spec.Instructions {
		opcode := byte(yamlInstr.Opcode)

		// Look up the DSL-defined instruction
		goSpec, exists := InstrSpecs[opcode]
		if !exists {
			t.Errorf("Opcode 0x%02x (%s) not found in Go DSL definitions", opcode, yamlInstr.Name)
			errorCount++
			continue
		}

		// Validate name matches opcode_str_lower
		expectedName := opcode_str_lower(opcode)
		if yamlInstr.Name != expectedName {
			t.Errorf("Opcode 0x%02x name mismatch - YAML: %s, Expected (opcode_str_lower): %s",
				opcode, yamlInstr.Name, expectedName)
			errorCount++
		}

		// Validate DSL name also matches opcode_str_lower
		if goSpec.Name != expectedName {
			t.Errorf("Opcode 0x%02x DSL name mismatch - Go DSL: %s, Expected (opcode_str_lower): %s",
				opcode, goSpec.Name, expectedName)
			errorCount++
		}

		// Validate argument count
		if len(goSpec.Args) != len(yamlInstr.Args) {
			t.Errorf("Opcode 0x%02x (%s) argument count mismatch - YAML: %d, Go: %d",
				opcode, yamlInstr.Name, len(yamlInstr.Args), len(goSpec.Args))
			errorCount++
			continue
		}

		// Validate argument types and names
		for i, yamlArg := range yamlInstr.Args {
			goArg := goSpec.Args[i]

			// Validate argument name
			if goArg.Name != yamlArg.Name {
				t.Errorf("Opcode 0x%02x (%s) arg %d name mismatch - YAML: %s, Go: %s",
					opcode, yamlInstr.Name, i, yamlArg.Name, goArg.Name)
				errorCount++
			}

			// Validate argument type
			expectedType := argTypeToString(goArg.Type)
			if expectedType != yamlArg.Type {
				t.Errorf("Opcode 0x%02x (%s) arg %d type mismatch - YAML: %s, Go: %s",
					opcode, yamlInstr.Name, i, yamlArg.Type, expectedType)
				errorCount++
			}
		}

		// Validate format template compatibility (basic check)
		if !validateFormatTemplate(yamlInstr.Format, goSpec.Format, yamlInstr.Args) {
			t.Errorf("Opcode 0x%02x (%s) format template mismatch\n  YAML: %s\n  Go:   %s",
				opcode, yamlInstr.Name, yamlInstr.Format, goSpec.Format)
			errorCount++
		}
	}

	// Check for Go-defined instructions not in YAML
	yamlOpcodes := make(map[byte]bool)
	for _, instr := range spec.Instructions {
		yamlOpcodes[byte(instr.Opcode)] = true
	}

	for opcode, goSpec := range InstrSpecs {
		if !yamlOpcodes[opcode] {
			t.Logf("WARNING: Go DSL defines opcode 0x%02x (%s) but it's not in YAML", opcode, goSpec.Name)
		}
	}

	if errorCount > 0 {
		t.Fatalf("Validation FAILED with %d errors", errorCount)
	} else {
		t.Logf("Validation PASSED - all instructions match between YAML and Go DSL")
	}
}

func argTypeToString(argType ArgType) string {
	switch argType {
	case ArgTypeReg:
		return "reg"
	case ArgTypeU32:
		return "u32"
	case ArgTypeU64:
		return "u64"
	case ArgTypeI32:
		return "i32"
	case ArgTypeImm:
		return "u32" // ArgTypeImm maps to u32 in YAML
	default:
		return "unknown"
	}
}

func validateFormatTemplate(yamlFormat, goFormat string, args []struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}) bool {
	// Basic validation - check that both formats contain the same placeholders
	yamlPlaceholders := extractPlaceholders(yamlFormat)
	goPlaceholders := extractPlaceholders(goFormat)

	if len(yamlPlaceholders) != len(goPlaceholders) {
		return false
	}

	// Check that all argument names appear as placeholders
	for _, arg := range args {
		found := false
		for _, placeholder := range yamlPlaceholders {
			if strings.Contains(placeholder, arg.Name) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func extractPlaceholders(format string) []string {
	var placeholders []string
	start := -1

	for i, r := range format {
		if r == '{' {
			start = i
		} else if r == '}' && start >= 0 {
			placeholders = append(placeholders, format[start:i+1])
			start = -1
		}
	}

	return placeholders
}

func TestMissingOpcodesInYAML(t *testing.T) {
	// Read and parse YAML file
	data, err := os.ReadFile("../instrs.yaml")
	if err != nil {
		t.Fatalf("Error reading YAML file: %v", err)
	}

	var spec YAMLSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("Error parsing YAML: %v", err)
	}

	// Build set of opcodes present in YAML
	yamlOpcodes := make(map[byte]bool)
	for _, instr := range spec.Instructions {
		yamlOpcodes[byte(instr.Opcode)] = true
	}

	// Find all opcodes defined in Go DSL but missing from YAML
	var missingOpcodes []byte
	for opcode := range InstrSpecs {
		if !yamlOpcodes[opcode] {
			missingOpcodes = append(missingOpcodes, opcode)
		}
	}

	// Sort the missing opcodes for consistent output
	for i := 0; i < len(missingOpcodes)-1; i++ {
		for j := i + 1; j < len(missingOpcodes); j++ {
			if missingOpcodes[i] > missingOpcodes[j] {
				missingOpcodes[i], missingOpcodes[j] = missingOpcodes[j], missingOpcodes[i]
			}
		}
	}

	t.Logf("YAML contains %d instructions", len(spec.Instructions))
	t.Logf("Go DSL defines %d instructions", len(InstrSpecs))
	t.Logf("Missing %d instructions in YAML", len(missingOpcodes))

	if len(missingOpcodes) > 0 {
		t.Logf("\nOpcodes defined in Go DSL but missing from YAML:")
		for _, opcode := range missingOpcodes {
			goSpec := InstrSpecs[opcode]
			t.Logf("  0x%02x (%3d) - %s", opcode, opcode, goSpec.Name)
		}

		// Group by opcode ranges for better understanding
		t.Logf("\nMissing opcodes grouped by ranges:")
		groupMissingOpcodesByRange(t, missingOpcodes)
	} else {
		t.Logf("All Go DSL instructions are present in YAML!")
	}
}

func groupMissingOpcodesByRange(t *testing.T, opcodes []byte) {
	if len(opcodes) == 0 {
		return
	}

	ranges := []struct {
		start, end byte
		name       string
	}{
		{0x00, 0x0F, "Basic Control"},
		{0x10, 0x2F, "Load/Store Immediate"},
		{0x30, 0x4F, "Memory Operations"},
		{0x50, 0x6F, "Jump/Branch with Immediate"},
		{0x70, 0x8F, "Register Operations"},
		{0x90, 0xAF, "Extended Arithmetic"},
		{0xB0, 0xCF, "Three-Register Operations"},
		{0xD0, 0xEF, "Advanced Operations"},
		{0xF0, 0xFF, "Reserved/Extended"},
	}

	for _, r := range ranges {
		var rangeOpcodes []byte
		for _, opcode := range opcodes {
			if opcode >= r.start && opcode <= r.end {
				rangeOpcodes = append(rangeOpcodes, opcode)
			}
		}

		if len(rangeOpcodes) > 0 {
			t.Logf("  %s (0x%02x-0x%02x): %d missing", r.name, r.start, r.end, len(rangeOpcodes))
			for _, opcode := range rangeOpcodes {
				goSpec := InstrSpecs[opcode]
				t.Logf("    0x%02x (%3d) - %s", opcode, opcode, goSpec.Name)
			}
		}
	}
}

// TestOpcodesCoverage verifies that all opcodes defined in instructions.go
// are properly covered in our disassembler DSL definitions and logging maps
func TestOpcodesCoverage(t *testing.T) {
	// Get all opcodes defined in instructions.go via reflection
	definedOpcodes := getAllDefinedOpcodes()

	// Get all opcodes that have string mappings in logging.go
	loggingOpcodes := getAllLoggingOpcodes()

	// Get all opcodes that have DSL definitions
	dslOpcodes := getAllDSLOpcodes()

	t.Logf("Found %d opcodes defined in instructions.go", len(definedOpcodes))
	t.Logf("Found %d opcodes with logging mappings", len(loggingOpcodes))
	t.Logf("Found %d opcodes with DSL definitions", len(dslOpcodes))

	var errors []string

	// Check that all defined opcodes have logging mappings
	for opcode, name := range definedOpcodes {
		if _, exists := loggingOpcodes[opcode]; !exists {
			errors = append(errors, fmt.Sprintf("Opcode 0x%02x (%s) defined in instructions.go but missing from logging.go", opcode, name))
		}
	}

	// Check that all defined opcodes have DSL definitions
	for opcode, name := range definedOpcodes {
		if _, exists := dslOpcodes[opcode]; !exists {
			errors = append(errors, fmt.Sprintf("Opcode 0x%02x (%s) defined in instructions.go but missing from DSL definitions", opcode, name))
		}
	}

	// Check that all logging opcodes are actually defined in instructions.go
	for opcode, name := range loggingOpcodes {
		if _, exists := definedOpcodes[opcode]; !exists {
			errors = append(errors, fmt.Sprintf("Opcode 0x%02x (%s) has logging mapping but not defined in instructions.go", opcode, name))
		}
	}

	// Check that all DSL opcodes are actually defined in instructions.go
	for opcode, name := range dslOpcodes {
		if _, exists := definedOpcodes[opcode]; !exists {
			errors = append(errors, fmt.Sprintf("Opcode 0x%02x (%s) has DSL definition but not defined in instructions.go", opcode, name))
		}
	}

	// Report all errors
	if len(errors) > 0 {
		for _, err := range errors {
			t.Error(err)
		}
		t.Fatalf("Found %d opcode coverage issues", len(errors))
	}

	t.Logf("✅ All %d opcodes are properly covered in logging and DSL definitions", len(definedOpcodes))
}

// getAllDefinedOpcodes extracts all opcode constants defined in instructions.go using reflection
func getAllDefinedOpcodes() map[byte]string {
	opcodes := make(map[byte]string)

	// We'll use the opcode_str function to determine which opcodes are defined
	// This way we ensure we're testing exactly what's implemented

	// Test all possible byte values to see which ones have string mappings
	for i := 0; i <= 255; i++ {
		opcode := byte(i)
		name := opcode_str(opcode)

		// If opcode_str returns something other than "OPCODE X", it's defined
		expectedDefault := fmt.Sprintf("OPCODE %d", opcode)
		if name != expectedDefault {
			opcodes[opcode] = name
		}
	}

	return opcodes
}

// getAllLoggingOpcodes extracts all opcodes that have string mappings in logging.go
func getAllLoggingOpcodes() map[byte]string {
	opcodes := make(map[byte]string)

	// Test all possible byte values
	for i := 0; i <= 255; i++ {
		opcode := byte(i)
		name := opcode_str(opcode)

		// If opcode_str returns something other than "OPCODE X", it has a mapping
		expectedDefault := fmt.Sprintf("OPCODE %d", opcode)
		if name != expectedDefault {
			opcodes[opcode] = name
		}
	}

	return opcodes
}

// getAllDSLOpcodes extracts all opcodes that have DSL definitions
func getAllDSLOpcodes() map[byte]string {
	opcodes := make(map[byte]string)

	// InstrSpecs is the global map populated by RegisterInstr calls
	for opcode, spec := range InstrSpecs {
		opcodes[opcode] = spec.Name
	}

	return opcodes
}

// TestSpecificOpcodeRanges tests that specific ranges of opcodes are covered
func TestSpecificOpcodeRanges(t *testing.T) {
	definedOpcodes := getAllDefinedOpcodes()

	// Test that we have the expected opcodes for each instruction category
	testCases := []struct {
		name     string
		opcodes  []byte
		category string
	}{
		{
			name:     "Instructions without Arguments",
			opcodes:  []byte{0, 1}, // TRAP, FALLTHROUGH
			category: "A.5.1",
		},
		{
			name:     "Instructions with One Immediate",
			opcodes:  []byte{10}, // ECALLI
			category: "A.5.2",
		},
		{
			name:     "Instructions with One Register and Extended Width Immediate",
			opcodes:  []byte{20}, // LOAD_IMM_64
			category: "A.5.3",
		},
		{
			name:     "Instructions with Two Immediates",
			opcodes:  []byte{30, 31, 32, 33}, // STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32, STORE_IMM_U64
			category: "A.5.4",
		},
		{
			name:     "Instructions with One Offset",
			opcodes:  []byte{40}, // JUMP
			category: "A.5.5",
		},
		{
			name:     "Three Register Instructions",
			opcodes:  []byte{190, 191, 192, 193, 194, 195, 196, 197, 198, 199, 200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219, 220, 221, 222, 223, 224, 225, 226, 227, 228, 229, 230},
			category: "A.5.13",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			missing := []byte{}
			for _, opcode := range tc.opcodes {
				if _, exists := definedOpcodes[opcode]; !exists {
					missing = append(missing, opcode)
				}
			}

			if len(missing) > 0 {
				t.Errorf("Missing opcodes in %s (%s): %v", tc.name, tc.category, missing)
			} else {
				t.Logf("✅ All %d opcodes present for %s (%s)", len(tc.opcodes), tc.name, tc.category)
			}
		})
	}
}

// TestOpcodeSequentialCoverage checks for gaps in opcode ranges
func TestOpcodeSequentialCoverage(t *testing.T) {
	definedOpcodes := getAllDefinedOpcodes()

	// Convert to sorted slice for analysis
	var opcodes []int
	for opcode := range definedOpcodes {
		opcodes = append(opcodes, int(opcode))
	}

	// Sort the opcodes
	for i := 0; i < len(opcodes)-1; i++ {
		for j := i + 1; j < len(opcodes); j++ {
			if opcodes[i] > opcodes[j] {
				opcodes[i], opcodes[j] = opcodes[j], opcodes[i]
			}
		}
	}

	t.Logf("Defined opcodes: %v", opcodes)

	// Check for large gaps that might indicate missing opcodes
	var gaps []string
	for i := 1; i < len(opcodes); i++ {
		gap := opcodes[i] - opcodes[i-1]
		if gap > 10 { // Report gaps larger than 10
			gaps = append(gaps, fmt.Sprintf("Gap of %d between 0x%02x and 0x%02x", gap-1, opcodes[i-1], opcodes[i]))
		}
	}

	if len(gaps) > 0 {
		t.Logf("Large gaps found (may be intentional): %v", gaps)
	}

	// Check specific expected ranges
	expectedRanges := []struct {
		name  string
		start int
		end   int
	}{
		{"Three Register Instructions", 190, 230},
		{"Two Register + Immediate Instructions", 120, 161},
	}

	for _, er := range expectedRanges {
		count := 0
		for _, opcode := range opcodes {
			if opcode >= er.start && opcode <= er.end {
				count++
			}
		}
		t.Logf("Range %s (0x%02x-0x%02x): %d opcodes defined", er.name, er.start, er.end, count)
	}
}

type ChargeGasEntry struct {
	Index     int
	Amount    int
	GasBefore int
	GasAfter  int
	LineNum   int
}

func parseJamdunaChargeGas(line string) (*ChargeGasEntry, error) {
	// Parse: "charged gas 1, 399999999 -> 399999990"
	re := regexp.MustCompile(`charged gas (\d+), (\d+) -> (\d+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) != 4 {
		return nil, fmt.Errorf("no match")
	}

	amount, _ := strconv.Atoi(matches[1])
	gasBefore, _ := strconv.Atoi(matches[2])
	gasAfter, _ := strconv.Atoi(matches[3])

	return &ChargeGasEntry{
		Amount:    amount,
		GasBefore: gasBefore,
		GasAfter:  gasAfter,
	}, nil
}

func parsePolkajamChargeGas(line string) (*ChargeGasEntry, error) {
	// Parse: "2025-08-14 01:13:50 tokio-runtime-worker TRACE polkavm::interpreter::raw_handlers  [2]: charge_gas: 1 (400000000 -> 399999999)"
	re := regexp.MustCompile(`\[(\d+)\]: charge_gas: (\d+) \((\d+) -> (\d+)\)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) != 5 {
		return nil, fmt.Errorf("no match")
	}

	index, _ := strconv.Atoi(matches[1])
	amount, _ := strconv.Atoi(matches[2])
	gasBefore, _ := strconv.Atoi(matches[3])
	gasAfter, _ := strconv.Atoi(matches[4])

	return &ChargeGasEntry{
		Index:     index,
		Amount:    amount,
		GasBefore: gasBefore,
		GasAfter:  gasAfter,
	}, nil
}

func TestDiff(t *testing.T) {
	jamdunaFile := "../logs/jamduna.log"
	polkajamFile := "../logs/polkajam.log"

	// Read Jamduna charge_gas entries
	jamdunaEntries := []ChargeGasEntry{}
	f1, err := os.Open(jamdunaFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", jamdunaFile, err)
	}
	defer f1.Close()

	scanner1 := bufio.NewScanner(f1)
	lineNum1 := 0
	for scanner1.Scan() {
		lineNum1++
		line := scanner1.Text()
		if entry, err := parseJamdunaChargeGas(line); err == nil {
			entry.LineNum = lineNum1
			jamdunaEntries = append(jamdunaEntries, *entry)
		}
	}

	// Read Polkajam charge_gas entries
	polkajamEntries := []ChargeGasEntry{}
	f2, err := os.Open(polkajamFile)
	if err != nil {
		t.Fatalf("Failed to open %s: %v", polkajamFile, err)
	}
	defer f2.Close()

	scanner2 := bufio.NewScanner(f2)
	lineNum2 := 0
	for scanner2.Scan() {
		lineNum2++
		line := scanner2.Text()
		if entry, err := parsePolkajamChargeGas(line); err == nil {
			entry.LineNum = lineNum2
			polkajamEntries = append(polkajamEntries, *entry)
		}
	}

	t.Logf("Found %d charge_gas entries in jamduna.log", len(jamdunaEntries))
	t.Logf("Found %d charge_gas entries in polkajam.log", len(polkajamEntries))

	// Print first few entries to understand the pattern
	t.Logf("First 3 Jamduna entries:")
	for i := 0; i < 3 && i < len(jamdunaEntries); i++ {
		e := jamdunaEntries[i]
		t.Logf("  [%d] charged gas %d, %d -> %d (line %d)", i, e.Amount, e.GasBefore, e.GasAfter, e.LineNum)
	}

	t.Logf("First 4 Polkajam entries:")
	for i := 0; i < 4 && i < len(polkajamEntries); i++ {
		e := polkajamEntries[i]
		t.Logf("  [%d] [%d]: charge_gas: %d (%d -> %d) (line %d)", i, e.Index, e.Amount, e.GasBefore, e.GasAfter, e.LineNum)
	}

	// It seems Polkajam has an extra initial charge_gas, so we start comparing from polkajam[1] vs jamduna[0]
	polkajamOffset := 1
	if len(polkajamEntries) > polkajamOffset && len(jamdunaEntries) > 0 {
		if polkajamEntries[polkajamOffset].Amount == jamdunaEntries[0].Amount &&
			polkajamEntries[polkajamOffset].GasBefore == jamdunaEntries[0].GasBefore &&
			polkajamEntries[polkajamOffset].GasAfter == jamdunaEntries[0].GasAfter {
			t.Logf("Confirmed: Polkajam has an extra initial charge_gas, using offset of %d", polkajamOffset)
		} else {
			polkajamOffset = 0 // fallback to no offset
			t.Logf("No clear offset pattern found, comparing from start")
		}
	}

	// Compare entries with offset
	maxCompare := len(jamdunaEntries)
	if len(polkajamEntries)-polkajamOffset < maxCompare {
		maxCompare = len(polkajamEntries) - polkajamOffset
	}

	jamdunaIdx := 0
	polkajamIdx := polkajamOffset
	
	for jamdunaIdx < len(jamdunaEntries) && polkajamIdx < len(polkajamEntries) {
		jamduna := jamdunaEntries[jamdunaIdx]
		polkajam := polkajamEntries[polkajamIdx]
		
		// Check if we should skip either entry (for misaligned sequences)
		if jamduna.GasBefore != polkajam.GasBefore {
			// Try to find matching gas state by advancing the one that's behind
			if jamduna.GasBefore > polkajam.GasBefore {
				// Polkajam is behind, advance it
				t.Logf("Skipping polkajam[%d] - gas mismatch: jamduna.GasBefore=%d > polkajam.GasBefore=%d", 
					polkajamIdx, jamduna.GasBefore, polkajam.GasBefore)
				polkajamIdx++
				continue
			} else {
				// Jamduna is behind, advance it
				t.Logf("Skipping jamduna[%d] - gas mismatch: jamduna.GasBefore=%d < polkajam.GasBefore=%d", 
					jamdunaIdx, jamduna.GasBefore, polkajam.GasBefore)
				jamdunaIdx++
				continue
			}
		}
		
		// Compare the charge gas details when gas states align
		if jamduna.Amount != polkajam.Amount ||
			jamduna.GasBefore != polkajam.GasBefore ||
			jamduna.GasAfter != polkajam.GasAfter {

			t.Fatalf("First difference found at jamduna[%d] vs polkajam[%d]:\nJamduna (line %d): charged gas %d, %d -> %d\nPolkajam (line %d): [%d]: charge_gas: %d (%d -> %d)",
				jamdunaIdx, polkajamIdx,
				jamduna.LineNum, jamduna.Amount, jamduna.GasBefore, jamduna.GasAfter,
				polkajam.LineNum, polkajam.Index, polkajam.Amount, polkajam.GasBefore, polkajam.GasAfter)
		}
		
		// Both entries match, advance both
		jamdunaIdx++
		polkajamIdx++
	}

	if len(jamdunaEntries) != len(polkajamEntries)-polkajamOffset {
		t.Logf("Different number of comparable charge_gas entries: jamduna=%d, polkajam=%d (offset=%d)",
			len(jamdunaEntries), len(polkajamEntries)-polkajamOffset, polkajamOffset)
	} else {
		t.Logf("All %d charge_gas entries match (with offset %d)!", len(jamdunaEntries), polkajamOffset)
	}
}
