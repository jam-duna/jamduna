package recompiler

import "fmt"

// GetX86Bytes returns the x86-64 machine code bytes for a single PVM instruction.
// This is intended for external tooling (e.g., Python verification) that needs
// direct access to the recompiler's per-opcode translation.
func GetX86Bytes(opcode byte, operands []byte) ([]byte, error) {
	translate, ok := pvmByteCodeToX86Code[opcode]
	if !ok {
		return nil, fmt.Errorf("unsupported opcode: %d", opcode)
	}

	inst := Instruction{
		Opcode: opcode,
		Args:   operands,
	}
	return translate(inst), nil
}
