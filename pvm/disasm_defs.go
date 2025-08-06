package pvm

import (
	"fmt"
	"strconv"
	"strings"
)

// ArgType represents the type of an instruction argument
type ArgType int

const (
	ArgTypeReg ArgType = iota
	ArgTypeU32
	ArgTypeU64
	ArgTypeI32
	ArgTypeImm // For immediate values that should be formatted as decimal
)

// Argument represents a single instruction argument
type Argument struct {
	Name string
	Type ArgType
}

// InstructionSpec represents a complete instruction specification
type InstructionSpec struct {
	Opcode byte
	Name   string
	Args   []Argument
	Format string
}

// Global instruction specifications map
var InstrSpecs = make(map[byte]*InstructionSpec)

// DSL helper functions
func Reg(name string) Argument {
	return Argument{Name: name, Type: ArgTypeReg}
}

func U32(name string) Argument {
	return Argument{Name: name, Type: ArgTypeU32}
}

func U64(name string) Argument {
	return Argument{Name: name, Type: ArgTypeU64}
}

func I32(name string) Argument {
	return Argument{Name: name, Type: ArgTypeI32}
}

func Imm(name string) Argument {
	return Argument{Name: name, Type: ArgTypeImm}
}

// InstructionBuilder provides a fluent interface for defining instructions
type InstructionBuilder struct {
	spec *InstructionSpec
}

// RegisterInstr creates a new instruction specification with the given opcode and name
func RegisterInstr(opcode byte, name string) *InstructionBuilder {
	spec := &InstructionSpec{
		Opcode: opcode,
		Name:   name,
	}
	InstrSpecs[opcode] = spec
	return &InstructionBuilder{spec: spec}
}

// Args sets the arguments for the instruction
func (b *InstructionBuilder) Args(args ...Argument) *InstructionBuilder {
	b.spec.Args = args
	return b
}

// Format sets the format template for the instruction
func (b *InstructionBuilder) Format(format string) *InstructionBuilder {
	b.spec.Format = format
	return b
}

// formatRegister formats a register index as a register name
func formatRegister(regIndex interface{}) string {
	switch v := regIndex.(type) {
	case int:
		// Special register aliases
		switch v {
		case 0:
			return "a0"
		case 1:
			return "a1"
		case 2:
			return "a2"
		case 3:
			return "a3"
		case 4:
			return "a4"
		case 5:
			return "a5"
		default:
			return fmt.Sprintf("r%d", v)
		}
	case uint32:
		return formatRegister(int(v))
	case uint64:
		return formatRegister(int(v))
	default:
		return fmt.Sprintf("r%v", regIndex)
	}
}

// FormatInstruction formats an instruction using its specification and provided values
func FormatInstruction(opcode byte, values []interface{}) string {
	spec, exists := InstrSpecs[opcode]
	if !exists {
		return fmt.Sprintf("unknown_opcode_%02x", opcode)
	}

	if len(values) != len(spec.Args) {
		return fmt.Sprintf("%s <invalid_args>", spec.Name)
	}

	result := spec.Format
	for i, arg := range spec.Args {
		placeholder := "{" + arg.Name + "}"
		var formatted string

		switch arg.Type {
		case ArgTypeReg:
			if regVal, ok := values[i].(int); ok {
				// Special case: register 1 is formatted as "a1"
				if regVal == 1 {
					formatted = "a1"
				} else {
					formatted = fmt.Sprintf("r%d", regVal)
				}
			} else {
				formatted = "r?"
			}
		case ArgTypeU32:
			if opcode == ECALLI {
				val := values[i].(uint32)
				formatted = fmt.Sprintf("%d", val)
			} else if val, ok := values[i].(uint32); ok {
				if val == 0 {
					formatted = "0"
				} else {
					formatted = fmt.Sprintf("0x%x", val)
				}
			} else if val, ok := values[i].(uint64); ok {
				if val == 0 {
					formatted = "0"
				} else {
					formatted = fmt.Sprintf("0x%x", val)
				}
			} else {
				formatted = "0x?"
			}
		case ArgTypeU64:
			if val, ok := values[i].(uint64); ok {
				if val == 0 {
					formatted = "0"
				} else {
					formatted = fmt.Sprintf("0x%x", val)
				}
			} else if val, ok := values[i].(uint32); ok {
				if val == 0 {
					formatted = "0"
				} else {
					formatted = fmt.Sprintf("0x%x", val)
				}
			} else {
				formatted = "0x?"
			}
		case ArgTypeI32:
			if val, ok := values[i].(int32); ok {
				formatted = strconv.Itoa(int(val))
			} else if val, ok := values[i].(int64); ok {
				formatted = strconv.Itoa(int(val))
			} else {
				formatted = fmt.Sprintf("%v", values[i])
			}
		case ArgTypeImm:
			// Format immediate values as decimal (unsigned interpretation)
			if val, ok := values[i].(uint32); ok {
				formatted = strconv.FormatUint(uint64(val), 10)
			} else if val, ok := values[i].(uint64); ok {
				// Convert to uint32 to match the expected format
				formatted = strconv.FormatUint(uint64(uint32(val)), 10)
			} else if val, ok := values[i].(int32); ok {
				// Convert signed to unsigned interpretation
				formatted = strconv.FormatUint(uint64(uint32(val)), 10)
			} else if val, ok := values[i].(int64); ok {
				// Convert signed to unsigned interpretation
				formatted = strconv.FormatUint(uint64(uint32(val)), 10)
			} else {
				formatted = fmt.Sprintf("%v", values[i])
			}
		default:
			formatted = fmt.Sprintf("%v", values[i])
		}

		result = strings.Replace(result, placeholder, formatted, -1)
	}

	return result
}

// Initialize all instruction definitions using the DSL
func init() {
	// A.5.1. Instructions without Arguments
	RegisterInstr(TRAP, opcode_str_lower(TRAP)).
		Args().
		Format("trap")

	RegisterInstr(FALLTHROUGH, opcode_str_lower(FALLTHROUGH)).
		Args().
		Format("fallthrough")

	// A.5.2. Instructions with Arguments of One Immediate
	RegisterInstr(ECALLI, opcode_str_lower(ECALLI)).
		Args(U32("imm")).
		Format("ecalli {imm}")

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
	RegisterInstr(LOAD_IMM_64, opcode_str_lower(LOAD_IMM_64)).
		Args(Reg("dst"), U64("imm")).
		Format("{dst} = {imm}")

	// A.5.4. Instructions with Arguments of Two Immediates
	RegisterInstr(STORE_IMM_U8, opcode_str_lower(STORE_IMM_U8)).
		Args(U32("offset"), U32("value")).
		Format("u8 [{offset}] = {value}")

	RegisterInstr(STORE_IMM_U16, opcode_str_lower(STORE_IMM_U16)).
		Args(U32("offset"), U32("value")).
		Format("u16 [{offset}] = {value}")

	RegisterInstr(STORE_IMM_U32, opcode_str_lower(STORE_IMM_U32)).
		Args(U32("offset"), U32("value")).
		Format("u32 [{offset}] = {value}")

	RegisterInstr(STORE_IMM_U64, opcode_str_lower(STORE_IMM_U64)).
		Args(U32("offset"), U32("value")).
		Format("u64 [{offset}] = {value}")

	// A.5.5. Instructions with Arguments of One Offset
	RegisterInstr(JUMP, opcode_str_lower(JUMP)).
		Args(I32("offset")).
		Format("jump {offset}")

	// A.5.6. Instructions with Arguments of One Register & One Immediate
	RegisterInstr(JUMP_IND, opcode_str_lower(JUMP_IND)).
		Args(Reg("reg"), U32("offset")).
		Format("jump [{reg} + {offset}]")

	RegisterInstr(LOAD_IMM, opcode_str_lower(LOAD_IMM)).
		Args(Reg("dst"), U32("imm")).
		Format("{dst} = {imm}")

	RegisterInstr(LOAD_U8, opcode_str_lower(LOAD_U8)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = u8 [{offset}]")

	RegisterInstr(LOAD_I8, opcode_str_lower(LOAD_I8)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = i8 [{offset}]")

	RegisterInstr(LOAD_U16, opcode_str_lower(LOAD_U16)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = u16 [{offset}]")

	RegisterInstr(LOAD_I16, opcode_str_lower(LOAD_I16)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = i16 [{offset}]")

	RegisterInstr(LOAD_U32, opcode_str_lower(LOAD_U32)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = u32 [{offset}]")

	RegisterInstr(LOAD_I32, opcode_str_lower(LOAD_I32)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = i32 [{offset}]")

	RegisterInstr(LOAD_U64, opcode_str_lower(LOAD_U64)).
		Args(Reg("dst"), U32("offset")).
		Format("{dst} = u64 [{offset}]")

	RegisterInstr(STORE_U8, opcode_str_lower(STORE_U8)).
		Args(Reg("src"), U32("offset")).
		Format("u8 [{offset}] = {src}")

	RegisterInstr(STORE_U16, opcode_str_lower(STORE_U16)).
		Args(Reg("src"), U32("offset")).
		Format("u16 [{offset}] = {src}")

	RegisterInstr(STORE_U32, opcode_str_lower(STORE_U32)).
		Args(Reg("src"), U32("offset")).
		Format("u32 [{offset}] = {src}")

	RegisterInstr(STORE_U64, opcode_str_lower(STORE_U64)).
		Args(Reg("src"), U32("offset")).
		Format("u64 [{offset}] = {src}")

	// A.5.7. Instructions with Arguments of One Register & Two Immediates
	RegisterInstr(STORE_IMM_IND_U8, opcode_str_lower(STORE_IMM_IND_U8)).
		Args(Reg("base"), Imm("offset"), U32("value")).
		Format("u8 [{base} + {offset}] = {value}")

	RegisterInstr(STORE_IMM_IND_U16, opcode_str_lower(STORE_IMM_IND_U16)).
		Args(Reg("base"), Imm("offset"), U32("value")).
		Format("u16 [{base} + {offset}] = {value}")

	RegisterInstr(STORE_IMM_IND_U32, opcode_str_lower(STORE_IMM_IND_U32)).
		Args(Reg("base"), Imm("offset"), U32("value")).
		Format("u32 [{base} + {offset}] = {value}")

	RegisterInstr(STORE_IMM_IND_U64, opcode_str_lower(STORE_IMM_IND_U64)).
		Args(Reg("base"), Imm("offset"), U32("value")).
		Format("u64 [{base} + {offset}] = {value}")

	// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset
	RegisterInstr(LOAD_IMM_JUMP, opcode_str_lower(LOAD_IMM_JUMP)).
		Args(Reg("dst"), Imm("imm"), I32("offset")).
		Format("{dst} = {imm}, jump {offset}")

	RegisterInstr(BRANCH_EQ_IMM, opcode_str_lower(BRANCH_EQ_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} == {imm}")

	RegisterInstr(BRANCH_NE_IMM, opcode_str_lower(BRANCH_NE_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} != {imm}")

	RegisterInstr(BRANCH_LT_U_IMM, opcode_str_lower(BRANCH_LT_U_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} <u {imm}")

	RegisterInstr(BRANCH_LE_U_IMM, opcode_str_lower(BRANCH_LE_U_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} <=u {imm}")

	RegisterInstr(BRANCH_GE_U_IMM, opcode_str_lower(BRANCH_GE_U_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} >=u {imm}")

	RegisterInstr(BRANCH_GT_U_IMM, opcode_str_lower(BRANCH_GT_U_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} >u {imm}")

	RegisterInstr(BRANCH_LT_S_IMM, opcode_str_lower(BRANCH_LT_S_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} <s {imm}")

	RegisterInstr(BRANCH_LE_S_IMM, opcode_str_lower(BRANCH_LE_S_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} <=s {imm}")

	RegisterInstr(BRANCH_GE_S_IMM, opcode_str_lower(BRANCH_GE_S_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} >=s {imm}")

	RegisterInstr(BRANCH_GT_S_IMM, opcode_str_lower(BRANCH_GT_S_IMM)).
		Args(Reg("reg"), Imm("imm"), I32("offset")).
		Format("jump {offset} if {reg} >s {imm}")

	// A.5.9. Instructions with Arguments of Two Registers
	RegisterInstr(MOVE_REG, opcode_str_lower(MOVE_REG)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst} = {src}")

	RegisterInstr(SBRK, opcode_str_lower(SBRK)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst} = sbrk {src}")

	RegisterInstr(COUNT_SET_BITS_64, opcode_str_lower(COUNT_SET_BITS_64)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(COUNT_SET_BITS_32, opcode_str_lower(COUNT_SET_BITS_32)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(LEADING_ZERO_BITS_64, opcode_str_lower(LEADING_ZERO_BITS_64)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(LEADING_ZERO_BITS_32, opcode_str_lower(LEADING_ZERO_BITS_32)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(TRAILING_ZERO_BITS_64, opcode_str_lower(TRAILING_ZERO_BITS_64)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(TRAILING_ZERO_BITS_32, opcode_str_lower(TRAILING_ZERO_BITS_32)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(SIGN_EXTEND_8, opcode_str_lower(SIGN_EXTEND_8)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(SIGN_EXTEND_16, opcode_str_lower(SIGN_EXTEND_16)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(ZERO_EXTEND_16, opcode_str_lower(ZERO_EXTEND_16)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	RegisterInstr(REVERSE_BYTES, opcode_str_lower(REVERSE_BYTES)).
		Args(Reg("dst"), Reg("src")).
		Format("{dst}, {src}")

	// A.5.10. Instructions with Arguments of Two Registers & One Immediate
	RegisterInstr(STORE_IND_U8, opcode_str_lower(STORE_IND_U8)).
		Args(Reg("base"), Reg("src"), U32("offset")).
		Format("u8 [{base} + {offset}] = {src}")

	RegisterInstr(STORE_IND_U16, opcode_str_lower(STORE_IND_U16)).
		Args(Reg("base"), Reg("src"), U32("offset")).
		Format("u16 [{base} + {offset}] = {src}")

	RegisterInstr(STORE_IND_U32, opcode_str_lower(STORE_IND_U32)).
		Args(Reg("base"), Reg("src"), U32("offset")).
		Format("u32 [{base} + {offset}] = {src}")

	RegisterInstr(STORE_IND_U64, opcode_str_lower(STORE_IND_U64)).
		Args(Reg("base"), Reg("src"), U32("offset")).
		Format("u64 [{base} + {offset}] = {src}")

	RegisterInstr(LOAD_IND_U8, opcode_str_lower(LOAD_IND_U8)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = u8 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_I8, opcode_str_lower(LOAD_IND_I8)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = i8 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_U16, opcode_str_lower(LOAD_IND_U16)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = u16 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_I16, opcode_str_lower(LOAD_IND_I16)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = i16 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_U32, opcode_str_lower(LOAD_IND_U32)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = u32 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_I32, opcode_str_lower(LOAD_IND_I32)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = i32 [{base} + {offset}]")

	RegisterInstr(LOAD_IND_U64, opcode_str_lower(LOAD_IND_U64)).
		Args(Reg("dst"), Reg("base"), U32("offset")).
		Format("{dst} = u64 [{base} + {offset}]")

	RegisterInstr(ADD_IMM_32, opcode_str_lower(ADD_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} + {imm}")

	RegisterInstr(AND_IMM, opcode_str_lower(AND_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} & {imm}")

	RegisterInstr(XOR_IMM, opcode_str_lower(XOR_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} ^ {imm}")

	RegisterInstr(OR_IMM, opcode_str_lower(OR_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} | {imm}")

	RegisterInstr(MUL_IMM_32, opcode_str_lower(MUL_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} * {imm}")

	RegisterInstr(SET_LT_U_IMM, opcode_str_lower(SET_LT_U_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} <u {imm}")

	RegisterInstr(SET_LT_S_IMM, opcode_str_lower(SET_LT_S_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} <s {imm}")

	RegisterInstr(SHLO_L_IMM_32, opcode_str_lower(SHLO_L_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} << {imm}")

	RegisterInstr(SHLO_R_IMM_32, opcode_str_lower(SHLO_R_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} >> {imm}")

	RegisterInstr(SHAR_R_IMM_32, opcode_str_lower(SHAR_R_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} >>a {imm}")

	RegisterInstr(NEG_ADD_IMM_32, opcode_str_lower(NEG_ADD_IMM_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {imm} - {src}")

	RegisterInstr(SET_GT_U_IMM, opcode_str_lower(SET_GT_U_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} >u {imm}")

	RegisterInstr(SET_GT_S_IMM, opcode_str_lower(SET_GT_S_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} >s {imm}")

	RegisterInstr(SHLO_L_IMM_ALT_32, opcode_str_lower(SHLO_L_IMM_ALT_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {imm} << {src}")

	RegisterInstr(SHLO_R_IMM_ALT_32, opcode_str_lower(SHLO_R_IMM_ALT_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {imm} >> {src}")

	RegisterInstr(SHAR_R_IMM_ALT_32, opcode_str_lower(SHAR_R_IMM_ALT_32)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {imm} >>a {src}")

	RegisterInstr(CMOV_IZ_IMM, opcode_str_lower(CMOV_IZ_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} if {src} == 0")

	RegisterInstr(CMOV_NZ_IMM, opcode_str_lower(CMOV_NZ_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} if {src} != 0")

	RegisterInstr(ADD_IMM_64, opcode_str_lower(ADD_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} + {imm}")

	RegisterInstr(MUL_IMM_64, opcode_str_lower(MUL_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} * {imm}")

	RegisterInstr(SHLO_L_IMM_64, opcode_str_lower(SHLO_L_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} << {imm}")

	RegisterInstr(SHLO_R_IMM_64, opcode_str_lower(SHLO_R_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} >> {imm}")

	RegisterInstr(SHAR_R_IMM_64, opcode_str_lower(SHAR_R_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} >>a {imm}")

	RegisterInstr(NEG_ADD_IMM_64, opcode_str_lower(NEG_ADD_IMM_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} - {src}")

	RegisterInstr(SHLO_L_IMM_ALT_64, opcode_str_lower(SHLO_L_IMM_ALT_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} << {src}")

	RegisterInstr(SHLO_R_IMM_ALT_64, opcode_str_lower(SHLO_R_IMM_ALT_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} >> {src}")

	RegisterInstr(SHAR_R_IMM_ALT_64, opcode_str_lower(SHAR_R_IMM_ALT_64)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} >>a {src}")

	RegisterInstr(ROT_R_64_IMM, opcode_str_lower(ROT_R_64_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {src} >>r {imm}")

	RegisterInstr(ROT_R_64_IMM_ALT, opcode_str_lower(ROT_R_64_IMM_ALT)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("{dst} = {imm} >>r {src}")

	RegisterInstr(ROT_R_32_IMM, opcode_str_lower(ROT_R_32_IMM)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {src} >>r {imm}")

	RegisterInstr(ROT_R_32_IMM_ALT, opcode_str_lower(ROT_R_32_IMM_ALT)).
		Args(Reg("dst"), Reg("src"), U32("imm")).
		Format("i32 {dst} = {imm} >>r {src}")

	// A.5.11. Instructions with Arguments of Two Registers & One Offset
	RegisterInstr(BRANCH_EQ, opcode_str_lower(BRANCH_EQ)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} == {reg2}")

	RegisterInstr(BRANCH_NE, opcode_str_lower(BRANCH_NE)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} != {reg2}")

	RegisterInstr(BRANCH_LT_U, opcode_str_lower(BRANCH_LT_U)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} <u {reg2}")

	RegisterInstr(BRANCH_LT_S, opcode_str_lower(BRANCH_LT_S)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} <s {reg2}")

	RegisterInstr(BRANCH_GE_U, opcode_str_lower(BRANCH_GE_U)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} >=u {reg2}")

	RegisterInstr(BRANCH_GE_S, opcode_str_lower(BRANCH_GE_S)).
		Args(Reg("reg1"), Reg("reg2"), I32("offset")).
		Format("jump {offset} if {reg1} >=s {reg2}")

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates
	RegisterInstr(LOAD_IMM_JUMP_IND, opcode_str_lower(LOAD_IMM_JUMP_IND)).
		Args(Reg("dst"), Reg("base"), Imm("imm"), U32("offset")).
		Format("{dst} = {imm}, jump [{base} + {offset}]")

	// A.5.13. Instructions with Arguments of Three Registers
	RegisterInstr(ADD_32, opcode_str_lower(ADD_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} + {src2}")

	RegisterInstr(SUB_32, opcode_str_lower(SUB_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} - {src2}")

	RegisterInstr(MUL_32, opcode_str_lower(MUL_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} * {src2}")

	RegisterInstr(DIV_U_32, opcode_str_lower(DIV_U_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} /u {src2}")

	RegisterInstr(DIV_S_32, opcode_str_lower(DIV_S_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} /s {src2}")

	RegisterInstr(REM_U_32, opcode_str_lower(REM_U_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} %u {src2}")

	RegisterInstr(REM_S_32, opcode_str_lower(REM_S_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} %s {src2}")

	RegisterInstr(SHLO_L_32, opcode_str_lower(SHLO_L_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} << {src2}")

	RegisterInstr(SHLO_R_32, opcode_str_lower(SHLO_R_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} >> {src2}")

	RegisterInstr(SHAR_R_32, opcode_str_lower(SHAR_R_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("i32 {dst} = {src1} >>a {src2}")

	RegisterInstr(ADD_64, opcode_str_lower(ADD_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} + {src2}")

	RegisterInstr(SUB_64, opcode_str_lower(SUB_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} - {src2}")

	RegisterInstr(MUL_64, opcode_str_lower(MUL_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} * {src2}")

	RegisterInstr(DIV_U_64, opcode_str_lower(DIV_U_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} /u {src2}")

	RegisterInstr(DIV_S_64, opcode_str_lower(DIV_S_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} /s {src2}")

	RegisterInstr(REM_U_64, opcode_str_lower(REM_U_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} %u {src2}")

	RegisterInstr(REM_S_64, opcode_str_lower(REM_S_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} %s {src2}")

	RegisterInstr(SHLO_L_64, opcode_str_lower(SHLO_L_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} << {src2}")

	RegisterInstr(SHLO_R_64, opcode_str_lower(SHLO_R_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} >> {src2}")

	RegisterInstr(SHAR_R_64, opcode_str_lower(SHAR_R_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} >>a {src2}")

	RegisterInstr(AND, opcode_str_lower(AND)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} & {src2}")

	RegisterInstr(XOR, opcode_str_lower(XOR)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} ^ {src2}")

	RegisterInstr(OR, opcode_str_lower(OR)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} | {src2}")

	RegisterInstr(MUL_UPPER_S_S, opcode_str_lower(MUL_UPPER_S_S)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} mulh {src2}")

	RegisterInstr(MUL_UPPER_U_U, opcode_str_lower(MUL_UPPER_U_U)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} mulhu {src2}")

	RegisterInstr(MUL_UPPER_S_U, opcode_str_lower(MUL_UPPER_S_U)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} mulhsu {src2}")

	RegisterInstr(SET_LT_U, opcode_str_lower(SET_LT_U)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} <u {src2}")

	RegisterInstr(SET_LT_S, opcode_str_lower(SET_LT_S)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} <s {src2}")

	RegisterInstr(CMOV_IZ, opcode_str_lower(CMOV_IZ)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} if {src2} == 0")

	RegisterInstr(CMOV_NZ, opcode_str_lower(CMOV_NZ)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst} = {src1} if {src2} != 0")

	// Missing opcodes 220-230
	RegisterInstr(ROT_L_64, opcode_str_lower(ROT_L_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(ROT_L_32, opcode_str_lower(ROT_L_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(ROT_R_64, opcode_str_lower(ROT_R_64)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(ROT_R_32, opcode_str_lower(ROT_R_32)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(AND_INV, opcode_str_lower(AND_INV)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(OR_INV, opcode_str_lower(OR_INV)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(XNOR, opcode_str_lower(XNOR)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(MAX, opcode_str_lower(MAX)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(MAX_U, opcode_str_lower(MAX_U)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(MIN, opcode_str_lower(MIN)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")

	RegisterInstr(MIN_U, opcode_str_lower(MIN_U)).
		Args(Reg("dst"), Reg("src1"), Reg("src2")).
		Format("{dst}, {src1}, {src2}")
}
