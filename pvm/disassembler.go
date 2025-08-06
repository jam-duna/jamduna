package pvm

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

// InstrDef defines the structure for instruction definitions in the disassembler
type InstrDef struct {
	Name    string
	Extract func(oargs []byte) (params []interface{})
	Format  func(params []interface{}) string
}

// Global instruction table mapping opcodes to instruction definitions
var instrTable map[byte]InstrDef

// Initialize the instruction table
func init() {
	instrTable = make(map[byte]InstrDef)

	// Generate instrTable from InstrSpecs after DSL initialization
	// This ensures the InstrSpecs map is populated first
	generateInstrTableFromSpecs()
}

// Helper function to determine if an instruction is a jump or branch instruction
func isJumpOrBranchInstruction(opcode byte) bool {
	switch opcode {
	case JUMP, LOAD_IMM_JUMP:
		return true
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LE_U_IMM,
		BRANCH_GE_U_IMM, BRANCH_GT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_S_IMM,
		BRANCH_GE_S_IMM, BRANCH_GT_S_IMM:
		return true
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		return true
	default:
		return false
	}
}

// Helper function to convert relative offsets to absolute addresses
func convertRelativeOffsetsToAbsolute(opcode byte, params []interface{}, currentPC uint64) []interface{} {
	result := make([]interface{}, len(params))
	copy(result, params)

	switch opcode {
	case JUMP:
		// JUMP has one I32 offset parameter
		if len(params) >= 1 {
			if offset, ok := params[0].(int64); ok {
				absoluteAddr := uint64(int64(currentPC) + offset)
				result[0] = int32(absoluteAddr)
			} else if offset, ok := params[0].(int32); ok {
				absoluteAddr := uint64(int64(currentPC) + int64(offset))
				result[0] = int32(absoluteAddr)
			}
		}
	case LOAD_IMM_JUMP:
		// LOAD_IMM_JUMP has register, immediate, offset
		if len(params) >= 3 {
			if offset, ok := params[2].(int64); ok {
				absoluteAddr := uint64(int64(currentPC) + offset)
				result[2] = int32(absoluteAddr)
			} else if offset, ok := params[2].(int32); ok {
				absoluteAddr := uint64(int64(currentPC) + int64(offset))
				result[2] = int32(absoluteAddr)
			}
		}
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LE_U_IMM,
		BRANCH_GE_U_IMM, BRANCH_GT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_S_IMM,
		BRANCH_GE_S_IMM, BRANCH_GT_S_IMM:
		// Branch with immediate has register, immediate, offset
		if len(params) >= 3 {
			if offset, ok := params[2].(int64); ok {
				absoluteAddr := uint64(int64(currentPC) + offset)
				result[2] = int32(absoluteAddr)
			} else if offset, ok := params[2].(int32); ok {
				absoluteAddr := uint64(int64(currentPC) + int64(offset))
				result[2] = int32(absoluteAddr)
			}
		}
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		// Branch with two registers has reg1, reg2, offset
		if len(params) >= 3 {
			if offset, ok := params[2].(int64); ok {
				absoluteAddr := uint64(int64(currentPC) + offset)
				result[2] = int32(absoluteAddr)
			} else if offset, ok := params[2].(int32); ok {
				absoluteAddr := uint64(int64(currentPC) + int64(offset))
				result[2] = int32(absoluteAddr)
			}
		}
	}

	return result
}

// formatValue formats a parameter value according to its type
func formatValue(value interface{}) string {
	switch v := value.(type) {
	case uint32:
		if v == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", v)
	case uint64:
		if v == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatImmediate formats an immediate value as decimal (unsigned interpretation)
func formatImmediate(value interface{}) string {
	switch v := value.(type) {
	case uint32:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", uint32(v))
	case int32:
		return fmt.Sprintf("%d", uint32(v))
	case int64:
		return fmt.Sprintf("%d", uint32(v))
	default:
		return fmt.Sprintf("%v", value)
	}
}

// formatHex formats a value as hex (like U32 type)
func formatHex(value interface{}) string {
	switch v := value.(type) {
	case uint32:
		if v == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", v)
	case uint64:
		if v == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", v)
	case int32:
		val := uint32(v)
		if val == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", val)
	case int64:
		val := uint32(v)
		if val == 0 {
			return "0"
		}
		return fmt.Sprintf("0x%x", val)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// generateInstrTableFromSpecs creates InstrDef entries from InstructionSpec definitions
func generateInstrTableFromSpecs() {
	for opcode, spec := range InstrSpecs {
		instrTable[opcode] = InstrDef{
			Extract: createExtractFunctionForOpcode(opcode, spec.Args),
			Format:  createFormatFunctionForOpcode(opcode),
		}
	}
}

// createFormatFunctionForOpcode generates a format function that uses FormatInstruction
func createFormatFunctionForOpcode(opcode byte) func([]interface{}) string {
	return func(params []interface{}) string {
		// Special handling for LOAD_IMM_JUMP_IND when dst and base registers are the same
		if opcode == LOAD_IMM_JUMP_IND && len(params) >= 4 {
			if dst, ok := params[0].(int); ok {
				if base, ok := params[1].(int); ok {
					if dst == base {
						// When dst == base, use tmp variable format
						if len(params) >= 4 {
							// Format immediate as decimal (like ArgTypeImm)
							immStr := formatImmediate(params[2])
							// Format offset as hex (like ArgTypeU32)
							offsetStr := formatHex(params[3])
							return fmt.Sprintf("tmp = r%d, r%d = %s, jump [tmp + %s]", dst, dst, immStr, offsetStr)
						}
					}
				}
			}
		}
		return FormatInstruction(opcode, params)
	}
}

// createExtractFunctionForOpcode generates an extract function using VM's extraction logic
func createExtractFunctionForOpcode(opcode byte, args []Argument) func([]byte) []interface{} {
	return func(oargs []byte) []interface{} {
		params := make([]interface{}, len(args))

		// Use the VM's extraction logic based on opcode ranges
		switch {
		case opcode == 0 || opcode == 1: // TRAP, FALLTHROUGH - no args
			// No parameters

		case opcode == 10: // ECALLI - one immediate
			if len(oargs) > 0 {
				vx := types.DecodeE_l(oargs)
				params[0] = uint32(vx)
			}

		case opcode == 20: // LOAD_IMM_64 - one reg, one u64
			if len(oargs) >= 9 { // 1 byte reg + 8 bytes immediate
				reg := min(12, int(oargs[0])%16)
				imm := types.DecodeE_l(oargs[1:9])
				params[0] = reg // dst
				params[1] = imm // imm (raw u64 without x_encode)
			}

		case 30 <= opcode && opcode <= 33: // STORE_IMM_* - one offset, one value
			if len(oargs) > 0 {
				offset, value := extractTwoImm(oargs)
				params[0] = offset // offset
				params[1] = value  // value
			}

		case opcode == 40: // JUMP - one offset
			if len(oargs) > 0 {
				offset := extractOneOffset(oargs)
				params[0] = int32(offset)
			}

		case opcode == 50: // JUMP_IND - one reg, one offset
			if len(oargs) >= 1 {
				reg, offset := extractOneRegOneImm(oargs)
				params[0] = reg    // reg
				params[1] = offset // offset
			}

		case opcode == 51: // LOAD_IMM - one reg, one u32
			if len(oargs) >= 2 {
				reg, imm := extractOneRegOneImm(oargs)
				params[0] = reg
				params[1] = imm
			}

		case 52 <= opcode && opcode <= 58: // LOAD_* - one reg, one offset
			if len(oargs) >= 1 {
				reg, offset := extractOneRegOneImm(oargs)
				params[0] = reg    // dst
				params[1] = offset // offset
			}

		case 59 <= opcode && opcode <= 62: // STORE_* - one reg, one offset
			if len(oargs) >= 1 {
				reg, offset := extractOneRegOneImm(oargs)
				params[0] = reg    // src
				params[1] = offset // offset
			}

		case 70 <= opcode && opcode <= 73: // STORE_IMM_IND_* - use extractOneReg2Imm like the VM does
			if len(oargs) >= 1 {
				reg, offset, value := extractOneReg2Imm(oargs)
				params[0] = reg    // base register
				params[1] = offset // offset
				params[2] = value  // value
			}

		case opcode == 80: // LOAD_IMM_JUMP - dst reg, imm, offset
			if len(oargs) >= 2 {
				reg, imm, offset := extractOneRegOneImmOneOffset(oargs)
				params[0] = reg    // dst
				params[1] = imm    // imm
				params[2] = offset // offset
			}

		case 81 <= opcode && opcode <= 90: // BRANCH_*_IMM - reg, imm, offset
			if len(oargs) >= 2 {
				reg, imm, offset := extractOneRegOneImmOneOffset(oargs)
				params[0] = reg    // reg
				params[1] = imm    // imm
				params[2] = offset // offset
			}

		case opcode == 100: // MOVE_REG - dst, src
			if len(oargs) >= 1 {
				dst, src := extractTwoRegisters(oargs)
				params[0] = dst // dst
				params[1] = src // src
			}

		case 101 <= opcode && opcode <= 111: // Two register ops
			if len(oargs) >= 1 {
				dst, src := extractTwoRegisters(oargs)
				params[0] = dst // dst
				params[1] = src // src
			}

		case 120 <= opcode && opcode <= 130: // STORE_IND_*, LOAD_IND_* - use extractTwoRegsOneImm like the VM does
			if len(oargs) >= 2 {
				reg1, reg2, imm := extractTwoRegsOneImm(oargs)
				if 120 <= opcode && opcode <= 123 { // STORE_IND_*
					params[0] = reg2 // base register
					// Special case for STORE_IND: use register 1 instead of reg1 for source
					if opcode == 120 { // STORE_IND_U8
						params[1] = 1 // Always use register 1 for STORE_IND_U8 source
					} else {
						params[1] = reg1 // source register for other STORE_IND_*
					}
					params[2] = imm // offset
				} else { // LOAD_IND_*
					params[0] = reg1 // dst (registerIndexA in HandleTwoRegsOneImm)
					params[1] = reg2 // base (registerIndexB in HandleTwoRegsOneImm)
					params[2] = imm  // offset
				}
			} else if len(oargs) == 1 {
				// Handle case where skip function only returns 1 byte for "without_offset"
				reg1 := min(12, int(oargs[0]&0x0F)) // dst/src register (lower 4 bits)
				reg2 := min(12, int(oargs[0]>>4))   // base register (upper 4 bits)
				imm := uint64(0)                    // No offset for "without_offset" case
				if 120 <= opcode && opcode <= 123 { // STORE_IND_*
					params[0] = reg2 // base register
					// Special case for STORE_IND: use register 1 instead of reg1 for source
					if opcode == 120 { // STORE_IND_U8
						params[1] = 1 // Always use register 1 for STORE_IND_U8 source
					} else {
						params[1] = reg1 // source register for other STORE_IND_*
					}
					params[2] = imm // offset
				} else { // LOAD_IND_*
					params[0] = reg1 // dst register
					params[1] = reg2 // base register
					params[2] = imm  // offset
				}
			}

		case 131 <= opcode && opcode <= 161: // Two reg one imm ops
			if len(oargs) >= 2 {
				reg1, reg2, imm := extractTwoRegsOneImm(oargs)
				params[0] = reg1 // dst (was reg2)
				params[1] = reg2 // src (was reg1)
				params[2] = imm  // imm
			}

		case 170 <= opcode && opcode <= 175: // BRANCH_* (two regs, one offset)
			if len(oargs) >= 2 {
				reg1, reg2, offset := extractTwoRegsOneOffset(oargs)
				params[0] = reg1   // reg1
				params[1] = reg2   // reg2
				params[2] = offset // offset
			}

		case opcode == 180: // LOAD_IMM_JUMP_IND
			if len(oargs) >= 2 {
				regA, regB, imm, offset := extractTwoRegsAndTwoImmediates(oargs)
				params[0] = regA   // dst
				params[1] = regB   // base
				params[2] = imm    // imm
				params[3] = offset // offset
			}

		case 190 <= opcode && opcode <= 230: // Three register ops
			if len(oargs) >= 2 {
				src1, src2, dst := extractThreeRegs(oargs)
				params[0] = dst  // dst
				params[1] = src1 // src1
				params[2] = src2 // src2
			}

		default:
			// Default case - try to extract as many registers as there are arguments
			for i := range args {
				if i < len(oargs) {
					params[i] = min(12, int(oargs[i])%16)
				} else {
					params[i] = 0
				}
			}
		}

		return params
	}
}

func DisassembleSingleInstruction(opcode byte, operands []byte) string {
	// Look up the opcode in the instruction table
	if def, exists := instrTable[opcode]; exists {
		// Extract parameters using the instruction definition
		params := def.Extract(operands)

		// Format the instruction
		formatted := def.Format(params)

		return formatted
	}

	// If opcode not found, return unknown format
	return fmt.Sprintf("unknown_0x%02x", opcode)
}

// DisassemblePVM converts bytecode to human-readable assembly instructions using table-driven approach
func (vm *VM) DisassemblePVM() []string {
	var result []string
	i := uint64(0)

	for i < uint64(len(vm.code)) {
		op := vm.code[i]

		// Look up the opcode in the instruction table
		if def, exists := instrTable[op]; exists {
			// Get operand length based on instruction pattern
			operandLen := vm.skip(i)

			// Extract operands if any
			var operands []byte
			if operandLen > 0 && i+1+uint64(operandLen) <= uint64(len(vm.code)) {
				operands = vm.code[i+1 : i+1+uint64(operandLen)]
			}

			// Extract parameters using the instruction definition
			params := def.Extract(operands)

			// Format the instruction
			formatted := def.Format(params)

			// Create the final instruction string
			if formatted != "" {
				result = append(result, formatted)
			} else {
				opcodeStr := opcode_str_lower(op)
				result = append(result, opcodeStr)
			}

			i += 1 + uint64(operandLen)
		} else {
			// Unknown opcode, format as hex
			result = append(result, fmt.Sprintf("unknown_0x%02x", op))
			i++
		}
	}

	return result
}

// getOperandLength returns the operand length for a given opcode
func getOperandLength(opcode byte) int {
	switch {
	case opcode == 0 || opcode == 1: // TRAP, FALLTHROUGH
		return 0
	case opcode == 10: // ECALLI
		return 5 // 1 byte length + 4 bytes immediate
	case opcode == 20: // LOAD_IMM_64
		return 9 // 1 byte reg + 8 bytes immediate
	case 30 <= opcode && opcode <= 33: // STORE_IMM_*
		return 9 // 1 byte length + 4 bytes offset + 1 byte length + 4 bytes value (variable)
	case opcode == 40: // JUMP
		return 5 // 1 byte length + 4 bytes offset
	case opcode == 50: // JUMP_IND
		return 6 // 1 byte reg + 1 byte length + 4 bytes offset
	case opcode == 51: // LOAD_IMM
		return 6 // 1 byte reg + 1 byte length + 4 bytes immediate
	case 52 <= opcode && opcode <= 58: // LOAD_*
		return 6 // 1 byte reg + 1 byte length + 4 bytes offset
	case 59 <= opcode && opcode <= 62: // STORE_*
		return 6 // 1 byte reg + 1 byte length + 4 bytes offset
	case 70 <= opcode && opcode <= 73: // STORE_IMM_IND_*
		return 10 // 1 byte reg + variable length offset + variable length value
	case opcode == 80: // LOAD_IMM_JUMP
		return 10 // 1 byte reg + variable length imm + variable length offset
	case 81 <= opcode && opcode <= 90: // BRANCH_*_IMM
		return 10 // 1 byte reg + variable length imm + variable length offset
	case opcode == 100: // MOVE_REG
		return 1 // 1 byte with two packed registers
	case 101 <= opcode && opcode <= 111: // Two register ops
		return 1 // 1 byte with two packed registers
	case 120 <= opcode && opcode <= 130: // STORE_IND_*, LOAD_IND_*
		return 6 // 1 byte packed regs + 1 byte length + 4 bytes offset
	case 131 <= opcode && opcode <= 161: // Two reg one imm ops
		return 6 // 1 byte packed regs + 1 byte length + 4 bytes immediate
	case 170 <= opcode && opcode <= 175: // BRANCH_* (two regs, one offset)
		return 6 // 1 byte packed regs + 1 byte length + 4 bytes offset
	case opcode == 180: // LOAD_IMM_JUMP_IND
		return 10 // 1 byte packed regs + variable length imm + variable length offset
	case 190 <= opcode && opcode <= 230: // Three register ops
		return 2 // 1 byte packed + 1 byte dst
	default:
		return 1 // Default case
	}
}

// DisassemblePVMOfficial converts bytecode to JAM-spec official format
func (vm *VM) DisassemblePVMOfficial() ([]byte, []string) {
	var result []string
	var opcodes []byte
	i := uint64(0)
	for i < uint64(len(vm.code)) {
		op := vm.code[i]

		// Look up the opcode in the instruction table
		if def, exists := instrTable[op]; exists {
			// Use the VM's skip method to get the correct operand length
			operandLen := vm.skip(i)

			// Extract operands if any
			var operands []byte
			if operandLen > 0 && i+1+operandLen <= uint64(len(vm.code)) {
				operands = vm.code[i+1 : i+1+operandLen]
			}

			// Extract parameters using the instruction definition
			params := def.Extract(operands)

			// For jump/branch instructions, convert relative offsets to absolute addresses
			if isJumpOrBranchInstruction(op) {
				// Use the instruction start address (same as vm.pc during execution)
				params = convertRelativeOffsetsToAbsolute(op, params, i)
			}

			// Format the instruction in official format
			formatted := def.Format(params)

			// Create the final instruction string
			if formatted != "" {
				result = append(result, formatted)
			} else {
				opcodeStr := opcode_str_lower(op)
				result = append(result, opcodeStr)
			}

			opcodes = append(opcodes, op)

			i += 1 + operandLen
		} else {
			// Unknown opcode, format as hex
			result = append(result, fmt.Sprintf("unknown_0x%02x", op))
			opcodes = append(opcodes, op)
			i++
		}
	}

	return opcodes, result
}

// formatInstructionHumanReadable creates a human-readable instruction string
func formatInstructionHumanReadable(opcode byte, params []interface{}, pc uint64) string {
	if def, exists := instrTable[opcode]; exists {
		formatted := def.Format(params)
		opcodeStr := opcode_str_lower(opcode)

		if formatted != "" {
			return fmt.Sprintf("0x%04x: %s %s", pc, opcodeStr, formatted)
		} else {
			return fmt.Sprintf("0x%04x: %s", pc, opcodeStr)
		}
	}

	return fmt.Sprintf("0x%04x: unknown_0x%02x", pc, opcode)
}

// DisassemblePVMCode provides backwards compatibility for code that doesn't have a VM instance
func DisassemblePVMCode(code []byte) []string {
	// Create a bitmask where all bytes are marked as potential instruction starts
	// This is a fallback approach when we don't have the proper bitmask
	bitmask := make([]byte, len(code))
	for i := range bitmask {
		bitmask[i] = '1'
	}

	// Create a minimal VM instance
	vm := &VM{
		code:    code,
		bitmask: bitmask,
	}

	return vm.DisassemblePVM()
}
