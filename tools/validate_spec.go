package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/colorfulnotion/jam/pvm"
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

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <instrs.yaml>\n", os.Args[0])
		os.Exit(1)
	}

	yamlFile := os.Args[1]

	// Read and parse YAML file
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading YAML file: %v\n", err)
		os.Exit(1)
	}

	var spec YAMLSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Validating %d instructions from %s against Go DSL definitions...\n", len(spec.Instructions), yamlFile)

	errorCount := 0

	for _, yamlInstr := range spec.Instructions {
		opcode := byte(yamlInstr.Opcode)

		// Look up the DSL-defined instruction
		goSpec, exists := pvm.InstrSpecs[opcode]
		if !exists {
			fmt.Printf("ERROR: Opcode 0x%02x (%s) not found in Go DSL definitions\n", opcode, yamlInstr.Name)
			errorCount++
			continue
		}

		// Validate name
		if goSpec.Name != yamlInstr.Name {
			fmt.Printf("ERROR: Opcode 0x%02x name mismatch - YAML: %s, Go: %s\n", opcode, yamlInstr.Name, goSpec.Name)
			errorCount++
		}

		// Validate argument count
		if len(goSpec.Args) != len(yamlInstr.Args) {
			fmt.Printf("ERROR: Opcode 0x%02x (%s) argument count mismatch - YAML: %d, Go: %d\n",
				opcode, yamlInstr.Name, len(yamlInstr.Args), len(goSpec.Args))
			errorCount++
			continue
		}

		// Validate argument types and names
		for i, yamlArg := range yamlInstr.Args {
			goArg := goSpec.Args[i]

			// Validate argument name
			if goArg.Name != yamlArg.Name {
				fmt.Printf("ERROR: Opcode 0x%02x (%s) arg %d name mismatch - YAML: %s, Go: %s\n",
					opcode, yamlInstr.Name, i, yamlArg.Name, goArg.Name)
				errorCount++
			}

			// Validate argument type
			expectedType := argTypeToString(goArg.Type)
			if expectedType != yamlArg.Type {
				fmt.Printf("ERROR: Opcode 0x%02x (%s) arg %d type mismatch - YAML: %s, Go: %s\n",
					opcode, yamlInstr.Name, i, yamlArg.Type, expectedType)
				errorCount++
			}
		}

		// Validate format template compatibility (basic check)
		if !validateFormatTemplate(yamlInstr.Format, goSpec.Format, yamlInstr.Args) {
			fmt.Printf("ERROR: Opcode 0x%02x (%s) format template mismatch\n  YAML: %s\n  Go:   %s\n",
				opcode, yamlInstr.Name, yamlInstr.Format, goSpec.Format)
			errorCount++
		}
	}

	// Check for Go-defined instructions not in YAML
	yamlOpcodes := make(map[byte]bool)
	for _, instr := range spec.Instructions {
		yamlOpcodes[byte(instr.Opcode)] = true
	}

	for opcode, goSpec := range pvm.InstrSpecs {
		if !yamlOpcodes[opcode] {
			fmt.Printf("WARNING: Go DSL defines opcode 0x%02x (%s) but it's not in YAML\n", opcode, goSpec.Name)
		}
	}

	if errorCount > 0 {
		fmt.Printf("\nValidation FAILED with %d errors\n", errorCount)
		os.Exit(1)
	} else {
		fmt.Printf("\nValidation PASSED - all instructions match between YAML and Go DSL\n")
	}
}

func argTypeToString(argType pvm.ArgType) string {
	switch argType {
	case 0: // ArgTypeReg
		return "reg"
	case 1: // ArgTypeU32
		return "u32"
	case 2: // ArgTypeU64
		return "u64"
	case 3: // ArgTypeI32
		return "i32"
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
