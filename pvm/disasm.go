// PVM Disassembler - Disassembles JAM-format PVM blobs into human-readable assembly

package pvm

import (
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/types"
)

// OpcodeNames maps opcode values to their string names
var OpcodeNames = map[byte]string{
	// A.5.1. Instructions without Arguments
	0: "trap",
	1: "fallthrough",

	// A.5.2. Instructions with Arguments of One Immediate
	10: "ecalli",

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
	20: "load_imm_64",

	// A.5.4. Instructions with Arguments of Two Immediates
	30: "store_imm_u8",
	31: "store_imm_u16",
	32: "store_imm_u32",
	33: "store_imm_u64",

	// A.5.5. Instructions with Arguments of One Offset
	40: "jump",

	// A.5.6. Instructions with Arguments of One Register & Two Immediates
	50: "jump_ind",
	51: "load_imm",
	52: "load_u8",
	53: "load_i8",
	54: "load_u16",
	55: "load_i16",
	56: "load_u32",
	57: "load_i32",
	58: "load_u64",
	59: "store_u8",
	60: "store_u16",
	61: "store_u32",
	62: "store_u64",

	// A.5.7. Instructions with Arguments of One Register & Two Immediates
	70: "store_imm_ind_u8",
	71: "store_imm_ind_u16",
	72: "store_imm_ind_u32",
	73: "store_imm_ind_u64",

	// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
	80: "load_imm_jump",
	81: "branch_eq_imm",
	82: "branch_ne_imm",
	83: "branch_lt_u_imm",
	84: "branch_le_u_imm",
	85: "branch_ge_u_imm",
	86: "branch_gt_u_imm",
	87: "branch_lt_s_imm",
	88: "branch_le_s_imm",
	89: "branch_ge_s_imm",
	90: "branch_gt_s_imm",

	// A.5.9. Instructions with Arguments of Two Registers
	100: "move_reg",
	101: "sbrk",
	102: "count_set_bits_64",
	103: "count_set_bits_32",
	104: "leading_zero_bits_64",
	105: "leading_zero_bits_32",
	106: "trailing_zero_bits_64",
	107: "trailing_zero_bits_32",
	108: "sign_extend_8",
	109: "sign_extend_16",
	110: "zero_extend_16",
	111: "reverse_bytes",

	// A.5.10. Instructions with Arguments of Two Registers & One Immediate
	120: "store_ind_u8",
	121: "store_ind_u16",
	122: "store_ind_u32",
	123: "store_ind_u64",
	124: "load_ind_u8",
	125: "load_ind_i8",
	126: "load_ind_u16",
	127: "load_ind_i16",
	128: "load_ind_u32",
	129: "load_ind_i32",
	130: "load_ind_u64",
	131: "add_imm_32",
	132: "and_imm",
	133: "xor_imm",
	134: "or_imm",
	135: "mul_imm_32",
	136: "set_lt_u_imm",
	137: "set_lt_s_imm",
	138: "shlo_l_imm_32",
	139: "shlo_r_imm_32",
	140: "shar_r_imm_32",
	141: "neg_add_imm_32",
	142: "set_gt_u_imm",
	143: "set_gt_s_imm",
	144: "shlo_l_imm_alt_32",
	145: "shlo_r_imm_alt_32",
	146: "shar_r_imm_alt_32",
	147: "cmov_iz_imm",
	148: "cmov_nz_imm",
	149: "add_imm_64",
	150: "mul_imm_64",
	151: "shlo_l_imm_64",
	152: "shlo_r_imm_64",
	153: "shar_r_imm_64",
	154: "neg_add_imm_64",
	155: "shlo_l_imm_alt_64",
	156: "shlo_r_imm_alt_64",
	157: "shar_r_imm_alt_64",
	158: "rot_r_64_imm",
	159: "rot_r_64_imm_alt",
	160: "rot_r_32_imm",
	161: "rot_r_32_imm_alt",

	// A.5.11. Instructions with Arguments of Two Registers & One Offset
	170: "branch_eq",
	171: "branch_ne",
	172: "branch_lt_u",
	173: "branch_lt_s",
	174: "branch_ge_u",
	175: "branch_ge_s",
	176: "branch_le_u",
	177: "branch_le_s",
	178: "branch_gt_u",
	179: "branch_gt_s",

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
	180: "load_imm_jump_ind",

	// A.5.13. Instructions with Arguments of Three Registers
	190: "add_32",
	191: "sub_32",
	192: "mul_32",
	193: "div_u_32",
	194: "div_s_32",
	195: "rem_u_32",
	196: "rem_s_32",
	197: "shlo_l_32",
	198: "shlo_r_32",
	199: "shar_r_32",
	200: "add_64",
	201: "sub_64",
	202: "mul_64",
	203: "div_u_64",
	204: "div_s_64",
	205: "rem_u_64",
	206: "rem_s_64",
	207: "shlo_l_64",
	208: "shlo_r_64",
	209: "shar_r_64",
	210: "and",
	211: "xor",
	212: "or",
	213: "mul_upper_s_s",
	214: "mul_upper_u_u",
	215: "mul_upper_s_u",
	216: "set_lt_u",
	217: "set_lt_s",
	218: "cmov_iz",
	219: "cmov_nz",
	220: "rot_l_64",
	221: "rot_l_32",
	222: "rot_r_64",
	223: "rot_r_32",
	224: "and_inv",
	225: "or_inv",
	226: "xnor",
	227: "max",
	228: "max_u",
	229: "min",
	230: "min_u",
}

// RegisterNames maps register indices to their names
var RegisterNames = []string{
	"ra", "sp", "t0", "t1", "t2", "s0", "s1",
	"a0", "a1", "a2", "a3", "a4", "a5",
}

func regName(idx int) string {
	if idx >= 0 && idx < len(RegisterNames) {
		return RegisterNames[idx]
	}
	return fmt.Sprintf("r%d", idx)
}

// InstructionCategory defines what arguments an instruction takes
type InstructionCategory int

const (
	CatNoArgs InstructionCategory = iota
	CatOneImm
	CatOneRegExtImm
	CatTwoImm
	CatOneOffset
	CatOneRegTwoImm
	CatOneRegImmImm
	CatOneRegImmOffset
	CatTwoReg
	CatTwoRegImm
	CatTwoRegOffset
	CatTwoRegTwoImm
	CatThreeReg
)

// GetInstructionCategory returns the category for a given opcode
func GetInstructionCategory(opcode byte) InstructionCategory {
	switch {
	case opcode <= 1:
		return CatNoArgs
	case opcode == 10:
		return CatOneImm
	case opcode == 20:
		return CatOneRegExtImm
	case opcode >= 30 && opcode <= 33:
		return CatTwoImm
	case opcode == 40:
		return CatOneOffset
	case opcode >= 50 && opcode <= 62:
		return CatOneRegTwoImm
	case opcode >= 70 && opcode <= 73:
		return CatOneRegImmImm
	case opcode >= 80 && opcode <= 90:
		return CatOneRegImmOffset
	case opcode >= 100 && opcode <= 111:
		return CatTwoReg
	case opcode >= 120 && opcode <= 161:
		return CatTwoRegImm
	case opcode >= 170 && opcode <= 179:
		return CatTwoRegOffset
	case opcode == 180:
		return CatTwoRegTwoImm
	case opcode >= 190 && opcode <= 230:
		return CatThreeReg
	default:
		return CatNoArgs
	}
}

// DisassembledInstruction represents a decoded instruction
type DisassembledInstruction struct {
	Offset  int
	Opcode  byte
	Name    string
	Args    string
	RawHex  string
	Length  int
}

// DisassembleJAMBlob disassembles a JAM-format program blob
func DisassembleJAMBlob(blob []byte) ([]DisassembledInstruction, error) {
	if len(blob) < 3 {
		return nil, fmt.Errorf("blob too short: %d bytes", len(blob))
	}

	// Parse header
	pos := 0

	// E(j_size)
	jSizeBytes, remaining := extractBytesForDisasm(blob[pos:])
	jSize, _ := types.DecodeE(jSizeBytes)
	pos += len(jSizeBytes)

	// E(z)
	zBytes, remaining := extractBytesForDisasm(remaining)
	z, _ := types.DecodeE(zBytes)
	pos += len(zBytes)

	// E(c_size)
	cSizeBytes, remaining := extractBytesForDisasm(remaining)
	cSize, _ := types.DecodeE(cSizeBytes)
	pos += len(cSizeBytes)

	// Skip jump table (j_size * z bytes)
	jTableLen := int(jSize * z)
	pos += jTableLen

	// Code section
	if pos+int(cSize) > len(blob) {
		return nil, fmt.Errorf("code size %d exceeds blob length", cSize)
	}
	code := blob[pos : pos+int(cSize)]
	pos += int(cSize)

	// Bitmask
	bitmask := blob[pos:]

	return DisassembleCode(code, bitmask)
}

// DisassembleCode disassembles raw PVM code with its bitmask
func DisassembleCode(code []byte, bitmask []byte) ([]DisassembledInstruction, error) {
	var instructions []DisassembledInstruction

	offset := 0
	for offset < len(code) {
		// Check if this is an instruction start (bit in bitmask is set)
		byteIdx := offset / 8
		bitIdx := offset % 8
		isInstructionStart := byteIdx < len(bitmask) && (bitmask[byteIdx]&(1<<bitIdx)) != 0

		if !isInstructionStart && offset > 0 {
			// This shouldn't happen with valid code, but handle gracefully
			offset++
			continue
		}

		opcode := code[offset]
		name, ok := OpcodeNames[opcode]
		if !ok {
			name = fmt.Sprintf("unknown_%d", opcode)
		}

		// Find instruction length by looking for next instruction start
		instLen := 1
		for nextOffset := offset + 1; nextOffset < len(code); nextOffset++ {
			nextByteIdx := nextOffset / 8
			nextBitIdx := nextOffset % 8
			if nextByteIdx < len(bitmask) && (bitmask[nextByteIdx]&(1<<nextBitIdx)) != 0 {
				instLen = nextOffset - offset
				break
			}
			instLen = nextOffset - offset + 1
		}

		// Get raw bytes
		endOffset := offset + instLen
		if endOffset > len(code) {
			endOffset = len(code)
		}
		rawBytes := code[offset:endOffset]

		// Decode arguments
		args := decodeArguments(opcode, rawBytes[1:], offset)

		instructions = append(instructions, DisassembledInstruction{
			Offset: offset,
			Opcode: opcode,
			Name:   name,
			Args:   args,
			RawHex: bytesToHex(rawBytes),
			Length: instLen,
		})

		offset += instLen
	}

	return instructions, nil
}

func decodeArguments(opcode byte, args []byte, pc int) string {
	cat := GetInstructionCategory(opcode)

	switch cat {
	case CatNoArgs:
		return ""

	case CatOneImm:
		if len(args) > 0 {
			imm := decodeVarInt(args)
			return fmt.Sprintf("%d", imm)
		}

	case CatOneRegExtImm:
		if len(args) >= 1 {
			reg := int(args[0] & 0x0F)
			imm := decodeVarInt(args[1:])
			return fmt.Sprintf("%s, 0x%x", regName(reg), imm)
		}

	case CatTwoImm:
		if len(args) >= 1 {
			lx := min(4, int(args[0])%8)
			ly := max(1, len(args)-lx-1)
			if len(args) > lx {
				vx := decodeVarIntN(args[1:], lx)
				vy := uint64(0)
				if len(args) > 1+lx {
					vy = decodeVarIntN(args[1+lx:], ly)
				}
				return fmt.Sprintf("[0x%x], 0x%x", vx, vy)
			}
		}

	case CatOneOffset:
		if len(args) > 0 {
			offset := decodeSignedVarInt(args)
			target := pc + 1 + len(args) + int(offset)
			return fmt.Sprintf("@%d (offset %+d)", target, offset)
		}

	case CatOneRegTwoImm:
		if len(args) >= 1 {
			reg := int(args[0] & 0x0F)
			lx := min(4, int(args[0]>>4)%8)
			if len(args) > 1 {
				vx := decodeVarIntN(args[1:], lx)
				return fmt.Sprintf("%s, 0x%x", regName(reg), vx)
			}
			return fmt.Sprintf("%s", regName(reg))
		}

	case CatOneRegImmOffset:
		if len(args) >= 1 {
			reg := int(args[0] & 0x0F)
			lx := min(4, int(args[0]>>4)%8)
			if len(args) > 1 {
				vx := decodeVarIntN(args[1:], lx)
				ly := len(args) - 1 - lx
				offset := int64(0)
				if ly > 0 && len(args) > 1+lx {
					offset = decodeSignedVarIntN(args[1+lx:], ly)
				}
				target := pc + 1 + len(args) + int(offset)
				return fmt.Sprintf("%s, 0x%x, @%d", regName(reg), vx, target)
			}
			return fmt.Sprintf("%s", regName(reg))
		}

	case CatTwoReg:
		if len(args) >= 1 {
			regA := int(args[0] & 0x0F)
			regB := int((args[0] >> 4) & 0x0F)
			return fmt.Sprintf("%s, %s", regName(regA), regName(regB))
		}

	case CatTwoRegImm:
		if len(args) >= 1 {
			regA := int(args[0] & 0x0F)
			regB := int((args[0] >> 4) & 0x0F)
			imm := decodeVarInt(args[1:])
			return fmt.Sprintf("%s, %s, 0x%x", regName(regA), regName(regB), imm)
		}

	case CatTwoRegOffset:
		if len(args) >= 1 {
			regA := int(args[0] & 0x0F)
			regB := int((args[0] >> 4) & 0x0F)
			offset := decodeSignedVarInt(args[1:])
			target := pc + 1 + len(args) + int(offset)
			return fmt.Sprintf("%s, %s, @%d", regName(regA), regName(regB), target)
		}

	case CatThreeReg:
		if len(args) >= 2 {
			regA := int(args[0] & 0x0F)
			regB := int((args[0] >> 4) & 0x0F)
			regC := int(args[1] & 0x0F)
			return fmt.Sprintf("%s, %s, %s", regName(regA), regName(regB), regName(regC))
		}
	}

	// Fallback: show raw hex
	if len(args) > 0 {
		return bytesToHex(args)
	}
	return ""
}

func decodeVarInt(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	result, _ := types.DecodeE(data)
	return result
}

func decodeVarIntN(data []byte, n int) uint64 {
	if len(data) == 0 || n == 0 {
		return 0
	}
	n = min(n, len(data))
	return types.DecodeE_l(data[:n])
}

func decodeSignedVarInt(data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	val := decodeVarInt(data)
	// Sign extend based on the number of bytes
	bits := len(data) * 8
	if bits < 64 && val&(1<<(bits-1)) != 0 {
		val |= ^uint64(0) << bits
	}
	return int64(val)
}

func decodeSignedVarIntN(data []byte, n int) int64 {
	if len(data) == 0 || n == 0 {
		return 0
	}
	n = min(n, len(data))
	val := types.DecodeE_l(data[:n])
	bits := n * 8
	if bits < 64 && val&(1<<(bits-1)) != 0 {
		val |= ^uint64(0) << bits
	}
	return int64(val)
}

func bytesToHex(data []byte) string {
	var parts []string
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, " ")
}

func extractBytesForDisasm(input []byte) ([]byte, []byte) {
	if len(input) == 0 {
		return nil, input
	}

	firstByte := input[0]
	var numBytes int

	switch {
	case firstByte < 128:
		numBytes = 1
	case firstByte >= 128 && firstByte < 192:
		numBytes = 2
	case firstByte >= 192 && firstByte < 224:
		numBytes = 3
	case firstByte >= 224 && firstByte < 240:
		numBytes = 4
	case firstByte >= 240 && firstByte < 248:
		numBytes = 5
	case firstByte >= 248 && firstByte < 252:
		numBytes = 6
	case firstByte >= 252 && firstByte < 254:
		numBytes = 7
	case firstByte >= 254:
		numBytes = 8
	default:
		numBytes = 1
	}

	if len(input) < numBytes {
		return input, nil
	}

	return input[:numBytes], input[numBytes:]
}

// DisassembleToString returns a formatted string representation
func DisassembleToString(blob []byte) string {
	instructions, err := DisassembleJAMBlob(blob)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("; JAM blob: %x\n", blob))
	sb.WriteString(fmt.Sprintf("; %d instructions\n\n", len(instructions)))

	for _, inst := range instructions {
		if inst.Args != "" {
			sb.WriteString(fmt.Sprintf("%4d: %-20s %-30s ; %s\n",
				inst.Offset, inst.Name, inst.Args, inst.RawHex))
		} else {
			sb.WriteString(fmt.Sprintf("%4d: %-20s %-30s ; %s\n",
				inst.Offset, inst.Name, "", inst.RawHex))
		}
	}

	return sb.String()
}

// PrintDisassembly prints disassembly to stdout
func PrintDisassembly(blob []byte) {
	fmt.Print(DisassembleToString(blob))
}
