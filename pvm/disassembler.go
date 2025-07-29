package pvm

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/slices"
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

	// A.5.1. Instructions without Arguments
	instrTable[TRAP] = InstrDef{
		Name: "TRAP",
		Extract: func(oargs []byte) (params []interface{}) {
			return []interface{}{}
		},
		Format: func(params []interface{}) string {
			return ""
		},
	}

	instrTable[FALLTHROUGH] = InstrDef{
		Name: "FALLTHROUGH",
		Extract: func(oargs []byte) (params []interface{}) {
			return []interface{}{}
		},
		Format: func(params []interface{}) string {
			return ""
		},
	}

	// A.5.2. Instructions with Arguments of One Immediate
	instrTable[ECALLI] = InstrDef{
		Name: "ECALLI",
		Extract: func(oargs []byte) (params []interface{}) {
			lx := len(oargs)
			if lx == 0 {
				return []interface{}{uint64(0)}
			}
			vx := types.DecodeE_l(oargs[:lx])
			return []interface{}{vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("%#x", params[0])
		},
	}

	// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate
	instrTable[LOAD_IMM_64] = InstrDef{
		Name: "LOAD_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			if len(oargs) == 0 {
				return []interface{}{0, uint64(0)}
			}
			registerIndexA := min(12, int(oargs[0])%16)
			lx := 8
			if len(oargs) < 1+lx {
				// Pad with zeros if not enough bytes
				padded := make([]byte, 1+lx)
				copy(padded, oargs)
				vx := types.DecodeE_l(padded[1 : 1+lx])
				return []interface{}{registerIndexA, vx}
			}
			vx := types.DecodeE_l(oargs[1 : 1+lx])
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x", params[0], params[1])
		},
	}

	// A.5.4. Instructions with Arguments of Two Immediates
	instrTable[STORE_IMM_U8] = InstrDef{
		Name: "STORE_IMM_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			if len(args) == 0 {
				return []interface{}{uint64(0), uint64(0)}
			}
			lx := min(4, int(args[0])%8)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value
			return []interface{}{vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], %#x", params[0], params[1])
		},
	}

	instrTable[STORE_IMM_U16] = InstrDef{
		Name: "STORE_IMM_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			if len(args) == 0 {
				return []interface{}{uint64(0), uint64(0)}
			}
			lx := min(4, int(args[0])%8)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value
			return []interface{}{vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], %#x", params[0], params[1])
		},
	}

	instrTable[STORE_IMM_U32] = InstrDef{
		Name: "STORE_IMM_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			if len(oargs) == 0 {
				return []interface{}{uint64(0), uint64(0)}
			}
			// For STORE_IMM_U32, use same logic as runtime HandleTwoImms
			// Start with encoding byte to determine lengths
			lx := min(4, int(oargs[0])%8)
			// ly can be up to 4 bytes for full U32 immediate value
			ly := min(4, max(0, len(oargs)-lx-1))
			if ly == 0 {
				ly = 1
				oargs = append(oargs, 0)
			}
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))       // address
			vy := x_encode(types.DecodeE_l(oargs[1+lx:1+lx+ly]), uint32(ly)) // value
			return []interface{}{vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], %#x", params[0], params[1])
		},
	}

	instrTable[STORE_IMM_U64] = InstrDef{
		Name: "STORE_IMM_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			if len(args) == 0 {
				return []interface{}{uint64(0), uint64(0)}
			}
			lx := min(4, int(args[0])%8)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))       // offset
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly)) // value
			return []interface{}{vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], %#x", params[0], params[1])
		},
	}

	// A.5.5. Instructions with Arguments of One Offset
	instrTable[JUMP] = InstrDef{
		Name: "JUMP",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			lx := min(4, len(args))
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := z_encode(types.DecodeE_l(args[0:lx]), uint32(lx))
			return []interface{}{vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("%d", params[0])
		},
	}

	// A.5.6. Instructions with Arguments of One Register & One Immediate
	instrTable[JUMP_IND] = InstrDef{
		Name: "JUMP_IND",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x", params[0], params[1])
		},
	}

	instrTable[LOAD_IMM] = InstrDef{
		Name: "LOAD_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x", params[0], params[1])
		},
	}

	instrTable[LOAD_U8] = InstrDef{
		Name: "LOAD_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_I8] = InstrDef{
		Name: "LOAD_I8",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_U16] = InstrDef{
		Name: "LOAD_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_I16] = InstrDef{
		Name: "LOAD_I16",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_U32] = InstrDef{
		Name: "LOAD_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_I32] = InstrDef{
		Name: "LOAD_I32",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[LOAD_U64] = InstrDef{
		Name: "LOAD_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [%#x]", params[0], params[1])
		},
	}

	instrTable[STORE_U8] = InstrDef{
		Name: "STORE_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], r%d", params[1], params[0])
		},
	}

	instrTable[STORE_U16] = InstrDef{
		Name: "STORE_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], r%d", params[1], params[0])
		},
	}

	instrTable[STORE_U32] = InstrDef{
		Name: "STORE_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], r%d", params[1], params[0])
		},
	}

	instrTable[STORE_U64] = InstrDef{
		Name: "STORE_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, max(0, len(args))-1)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[%#x], r%d", params[1], params[0])
		},
	}

	// A.5.7. Instructions with Arguments of One Register & Two Immediates.
	instrTable[STORE_IMM_IND_U8] = InstrDef{
		Name: "STORE_IMM_IND_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], %#x", params[0], params[1], params[2])
		},
	}

	instrTable[STORE_IMM_IND_U16] = InstrDef{
		Name: "STORE_IMM_IND_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			ly := min(4, max(0, len(args)-lx-1))
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], %#x", params[0], params[1], params[2])
		},
	}

	instrTable[STORE_IMM_IND_U32] = InstrDef{
		Name: "STORE_IMM_IND_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], %#x", params[0], params[1], params[2])
		},
	}

	instrTable[STORE_IMM_IND_U64] = InstrDef{
		Name: "STORE_IMM_IND_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := x_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], %#x", params[0], params[1], params[2])
		},
	}

	// A.5.8 One Register, One Immediate and One Offset
	instrTable[LOAD_IMM_JUMP] = InstrDef{
		Name: "LOAD_IMM_JUMP",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_EQ_IMM] = InstrDef{
		Name: "BRANCH_EQ_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_NE_IMM] = InstrDef{
		Name: "BRANCH_NE_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_LT_U_IMM] = InstrDef{
		Name: "BRANCH_LT_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_LE_U_IMM] = InstrDef{
		Name: "BRANCH_LE_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GE_U_IMM] = InstrDef{
		Name: "BRANCH_GE_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GT_U_IMM] = InstrDef{
		Name: "BRANCH_GT_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_LT_S_IMM] = InstrDef{
		Name: "BRANCH_LT_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_LE_S_IMM] = InstrDef{
		Name: "BRANCH_LE_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GE_S_IMM] = InstrDef{
		Name: "BRANCH_GE_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GT_S_IMM] = InstrDef{
		Name: "BRANCH_GT_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			args := slices.Clone(oargs)
			registerIndexA := min(12, int(args[0])%16)
			lx := min(4, int(args[0])>>4)
			if lx == 0 {
				lx = 1
				args = append(args, 0)
			}
			ly := min(4, max(0, len(args)-lx-1))
			if ly == 0 {
				ly = 1
				args = append(args, 0)
			}
			vx := x_encode(types.DecodeE_l(args[1:1+lx]), uint32(lx))
			vy := z_encode(types.DecodeE_l(args[1+lx:1+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, %#x, %d", params[0], params[1], params[2])
		},
	}

	// A.5.9. Instructions with Arguments of Two Registers.
	instrTable[MOVE_REG] = InstrDef{
		Name: "MOVE_REG",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[SBRK] = InstrDef{
		Name: "SBRK",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	// A.5.11 Two Registers and One Offset
	instrTable[BRANCH_EQ] = InstrDef{
		Name: "BRANCH_EQ",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_NE] = InstrDef{
		Name: "BRANCH_NE",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	// A.5.13. Instructions with Arguments of Three Registers.
	instrTable[ADD_32] = InstrDef{
		Name: "ADD_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SUB_32] = InstrDef{
		Name: "SUB_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[ADD_64] = InstrDef{
		Name: "ADD_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SUB_64] = InstrDef{
		Name: "SUB_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_64] = InstrDef{
		Name: "MUL_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[AND] = InstrDef{
		Name: "AND",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[OR] = InstrDef{
		Name: "OR",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[XOR] = InstrDef{
		Name: "XOR",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[COUNT_SET_BITS_64] = InstrDef{
		Name: "COUNT_SET_BITS_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[COUNT_SET_BITS_32] = InstrDef{
		Name: "COUNT_SET_BITS_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[LEADING_ZERO_BITS_64] = InstrDef{
		Name: "LEADING_ZERO_BITS_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[LEADING_ZERO_BITS_32] = InstrDef{
		Name: "LEADING_ZERO_BITS_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[TRAILING_ZERO_BITS_64] = InstrDef{
		Name: "TRAILING_ZERO_BITS_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[TRAILING_ZERO_BITS_32] = InstrDef{
		Name: "TRAILING_ZERO_BITS_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[SIGN_EXTEND_8] = InstrDef{
		Name: "SIGN_EXTEND_8",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[SIGN_EXTEND_16] = InstrDef{
		Name: "SIGN_EXTEND_16",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[ZERO_EXTEND_16] = InstrDef{
		Name: "ZERO_EXTEND_16",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	instrTable[REVERSE_BYTES] = InstrDef{
		Name: "REVERSE_BYTES",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexD := min(12, int(oargs[0])%16)
			registerIndexA := min(12, int(oargs[0])>>4)
			return []interface{}{registerIndexD, registerIndexA}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d", params[0], params[1])
		},
	}

	// A.5.10 Two Registers and One Immediate
	instrTable[STORE_IND_U8] = InstrDef{
		Name: "STORE_IND_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], r%d", params[1], params[2], params[0])
		},
	}

	instrTable[STORE_IND_U16] = InstrDef{
		Name: "STORE_IND_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], r%d", params[1], params[2], params[0])
		},
	}

	instrTable[STORE_IND_U32] = InstrDef{
		Name: "STORE_IND_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], r%d", params[1], params[2], params[0])
		},
	}

	instrTable[STORE_IND_U64] = InstrDef{
		Name: "STORE_IND_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("[r%d+%#x], r%d", params[1], params[2], params[0])
		},
	}

	instrTable[LOAD_IND_U8] = InstrDef{
		Name: "LOAD_IND_U8",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_I8] = InstrDef{
		Name: "LOAD_IND_I8",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_U16] = InstrDef{
		Name: "LOAD_IND_U16",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_I16] = InstrDef{
		Name: "LOAD_IND_I16",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_U32] = InstrDef{
		Name: "LOAD_IND_U32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_I32] = InstrDef{
		Name: "LOAD_IND_I32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[LOAD_IND_U64] = InstrDef{
		Name: "LOAD_IND_U64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, [r%d+%#x]", params[0], params[1], params[2])
		},
	}

	instrTable[ADD_IMM_32] = InstrDef{
		Name: "ADD_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[ADD_IMM_64] = InstrDef{
		Name: "ADD_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[AND_IMM] = InstrDef{
		Name: "AND_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[XOR_IMM] = InstrDef{
		Name: "XOR_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[OR_IMM] = InstrDef{
		Name: "OR_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_IMM_32] = InstrDef{
		Name: "MUL_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_IMM_64] = InstrDef{
		Name: "MUL_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	// A.5.11 Two Registers and One Offset (additional branch instructions)
	instrTable[BRANCH_LT_U] = InstrDef{
		Name: "BRANCH_LT_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_LT_S] = InstrDef{
		Name: "BRANCH_LT_S",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GE_U] = InstrDef{
		Name: "BRANCH_GE_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	instrTable[BRANCH_GE_S] = InstrDef{
		Name: "BRANCH_GE_S",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := z_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %d", params[0], params[1], params[2])
		},
	}

	// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
	instrTable[LOAD_IMM_JUMP_IND] = InstrDef{
		Name: "LOAD_IMM_JUMP_IND",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, int(oargs[1])&0xF)
			if lx == 0 {
				lx = 1
			}
			ly := min(4, int(oargs[1])>>4)
			if ly == 0 {
				ly = 1
			}
			vx := x_encode(types.DecodeE_l(oargs[2:2+lx]), uint32(lx))
			vy := x_encode(types.DecodeE_l(oargs[2+lx:2+lx+ly]), uint32(ly))
			return []interface{}{registerIndexA, registerIndexB, vx, vy}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x, %#x", params[0], params[1], params[2], params[3])
		},
	}

	// A.5.13. More Three Register instructions
	instrTable[MUL_32] = InstrDef{
		Name: "MUL_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[DIV_U_32] = InstrDef{
		Name: "DIV_U_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[DIV_S_32] = InstrDef{
		Name: "DIV_S_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[DIV_U_64] = InstrDef{
		Name: "DIV_U_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[DIV_S_64] = InstrDef{
		Name: "DIV_S_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[REM_U_32] = InstrDef{
		Name: "REM_U_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[REM_S_32] = InstrDef{
		Name: "REM_S_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[REM_U_64] = InstrDef{
		Name: "REM_U_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[REM_S_64] = InstrDef{
		Name: "REM_S_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SET_LT_U] = InstrDef{
		Name: "SET_LT_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SET_LT_S] = InstrDef{
		Name: "SET_LT_S",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	// Additional Three Register instructions
	instrTable[SHLO_L_32] = InstrDef{
		Name: "SHLO_L_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_32] = InstrDef{
		Name: "SHLO_R_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_32] = InstrDef{
		Name: "SHAR_R_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_L_64] = InstrDef{
		Name: "SHLO_L_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_64] = InstrDef{
		Name: "SHLO_R_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_64] = InstrDef{
		Name: "SHAR_R_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_UPPER_S_S] = InstrDef{
		Name: "MUL_UPPER_S_S",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_UPPER_U_U] = InstrDef{
		Name: "MUL_UPPER_U_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MUL_UPPER_S_U] = InstrDef{
		Name: "MUL_UPPER_S_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[CMOV_IZ] = InstrDef{
		Name: "CMOV_IZ",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[CMOV_NZ] = InstrDef{
		Name: "CMOV_NZ",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_L_64] = InstrDef{
		Name: "ROT_L_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_L_32] = InstrDef{
		Name: "ROT_L_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_64] = InstrDef{
		Name: "ROT_R_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_32] = InstrDef{
		Name: "ROT_R_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[AND_INV] = InstrDef{
		Name: "AND_INV",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[OR_INV] = InstrDef{
		Name: "OR_INV",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[XNOR] = InstrDef{
		Name: "XNOR",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MAX] = InstrDef{
		Name: "MAX",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MAX_U] = InstrDef{
		Name: "MAX_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MIN] = InstrDef{
		Name: "MIN",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	instrTable[MIN_U] = InstrDef{
		Name: "MIN_U",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			registerIndexD := min(12, int(oargs[1])%16)
			return []interface{}{registerIndexD, registerIndexA, registerIndexB}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, r%d", params[0], params[1], params[2])
		},
	}

	// Additional Two Registers One Immediate instructions
	instrTable[SET_LT_U_IMM] = InstrDef{
		Name: "SET_LT_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SET_LT_S_IMM] = InstrDef{
		Name: "SET_LT_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SET_GT_U_IMM] = InstrDef{
		Name: "SET_GT_U_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SET_GT_S_IMM] = InstrDef{
		Name: "SET_GT_S_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_L_IMM_32] = InstrDef{
		Name: "SHLO_L_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_IMM_32] = InstrDef{
		Name: "SHLO_R_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_IMM_32] = InstrDef{
		Name: "SHAR_R_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_L_IMM_64] = InstrDef{
		Name: "SHLO_L_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_IMM_64] = InstrDef{
		Name: "SHLO_R_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_IMM_64] = InstrDef{
		Name: "SHAR_R_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[NEG_ADD_IMM_32] = InstrDef{
		Name: "NEG_ADD_IMM_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[NEG_ADD_IMM_64] = InstrDef{
		Name: "NEG_ADD_IMM_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_L_IMM_ALT_32] = InstrDef{
		Name: "SHLO_L_IMM_ALT_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_IMM_ALT_32] = InstrDef{
		Name: "SHLO_R_IMM_ALT_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_IMM_ALT_32] = InstrDef{
		Name: "SHAR_R_IMM_ALT_32",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_L_IMM_ALT_64] = InstrDef{
		Name: "SHLO_L_IMM_ALT_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHLO_R_IMM_ALT_64] = InstrDef{
		Name: "SHLO_R_IMM_ALT_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[SHAR_R_IMM_ALT_64] = InstrDef{
		Name: "SHAR_R_IMM_ALT_64",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[CMOV_IZ_IMM] = InstrDef{
		Name: "CMOV_IZ_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[CMOV_NZ_IMM] = InstrDef{
		Name: "CMOV_NZ_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_64_IMM] = InstrDef{
		Name: "ROT_R_64_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_64_IMM_ALT] = InstrDef{
		Name: "ROT_R_64_IMM_ALT",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_32_IMM] = InstrDef{
		Name: "ROT_R_32_IMM",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}

	instrTable[ROT_R_32_IMM_ALT] = InstrDef{
		Name: "ROT_R_32_IMM_ALT",
		Extract: func(oargs []byte) (params []interface{}) {
			registerIndexA := min(12, int(oargs[0])%16)
			registerIndexB := min(12, int(oargs[0])>>4)
			lx := min(4, max(0, len(oargs)-1))
			vx := x_encode(types.DecodeE_l(oargs[1:1+lx]), uint32(lx))
			return []interface{}{registerIndexA, registerIndexB, vx}
		},
		Format: func(params []interface{}) string {
			return fmt.Sprintf("r%d, r%d, %#x", params[0], params[1], params[2])
		},
	}
}

// DisassemblePVM converts bytecode to human-readable assembly instructions using table-driven approach
func (vm *VM) DisassemblePVM() []string {
	var result []string
	i := uint64(0)

	for i < uint64(len(vm.code)) {
		op := vm.code[i]

		// Look up the opcode in the instruction table
		if def, exists := instrTable[op]; exists {
			// Use VM's skip method to get the operand length
			operandLen := vm.skip(i)

			// Get the operands
			var operands []byte
			if operandLen > 0 && i+1+operandLen <= uint64(len(vm.code)) {
				operands = vm.code[i+1 : i+1+operandLen]
			}

			// Extract parameters
			params := def.Extract(operands)

			// Format the instruction
			formatted := def.Format(params)

			// Create the disassembly line with address, instruction name, and formatted operands
			line := fmt.Sprintf("%04x: %-15s %s", i, def.Name, formatted)
			result = append(result, line)

			// Advance by 1 (opcode) + operand bytes
			i += 1 + operandLen
		} else {
			// Unknown opcode - emit as raw data byte
			line := fmt.Sprintf("%04x: %-15s %#x", i, "DB", op)
			result = append(result, line)
			i++
		}
	}

	return result
}

func (vm *VM) DisassemblePVMOfficial() ([]byte, []string) {
	var result []string
	var opcodes []byte
	i := uint64(0)

	for i < uint64(len(vm.code)) {
		op := vm.code[i]
		opcodes = append(opcodes, op)
		// Look up the opcode in the instruction table
		if def, exists := instrTable[op]; exists {
			// Use VM's skip method to get the operand length
			operandLen := vm.skip(i)

			// Get the operands
			var operands []byte
			if operandLen > 0 && i+1+operandLen <= uint64(len(vm.code)) {
				operands = vm.code[i+1 : i+1+operandLen]
			}

			// Extract parameters
			params := def.Extract(operands)

			// Format the instruction using human-readable format
			line := formatInstructionHumanReadable(op, params, i)
			result = append(result, line)

			// Advance by 1 (opcode) + operand bytes
			i += 1 + operandLen
		} else {
			// Unknown opcode - emit as raw data byte
			line := fmt.Sprintf("%s %#x", "DB", op)
			result = append(result, line)
			i++
		}
	}

	return opcodes, result
}

// formatInstructionHumanReadable converts instructions to human-readable format matching test vectors
func formatInstructionHumanReadable(opcode byte, params []interface{}, pc uint64) string {
	switch opcode {
	case TRAP:
		return "trap"

	case MOVE_REG:
		return fmt.Sprintf("r%v = r%v", params[0], params[1])

	case LOAD_IMM:
		return fmt.Sprintf("r%v = %#x", params[0], params[1])

	case LOAD_IMM_64:
		return fmt.Sprintf("r%v = %#x", params[0], params[1])

	case ADD_32:
		return fmt.Sprintf("i32 r%v = r%v + r%v", params[0], params[1], params[2])

	case ADD_64:
		return fmt.Sprintf("r%v = r%v + r%v", params[0], params[1], params[2])

	case ADD_IMM_32:
		return fmt.Sprintf("i32 r%v = r%v + %#x", params[0], params[1], params[2])

	case ADD_IMM_64:
		return fmt.Sprintf("r%v = r%v + %#x", params[0], params[1], params[2])

	case SUB_32:
		return fmt.Sprintf("i32 r%v = r%v - r%v", params[0], params[1], params[2])

	case SUB_64:
		return fmt.Sprintf("r%v = r%v - r%v", params[0], params[1], params[2])

	case AND:
		return fmt.Sprintf("r%v = r%v & r%v", params[0], params[1], params[2])

	case AND_IMM:
		return fmt.Sprintf("r%v = r%v & %#x", params[0], params[1], params[2])

	case OR:
		return fmt.Sprintf("r%v = r%v | r%v", params[0], params[1], params[2])

	case OR_IMM:
		return fmt.Sprintf("r%v = r%v | %#x", params[0], params[1], params[2])

	case XOR:
		return fmt.Sprintf("r%v = r%v ^ r%v", params[0], params[1], params[2])

	case XOR_IMM:
		return fmt.Sprintf("r%v = r%v ^ %#x", params[0], params[1], params[2])

	case MUL_32:
		return fmt.Sprintf("i32 r%v = r%v * r%v", params[0], params[1], params[2])

	case MUL_64:
		return fmt.Sprintf("r%v = r%v * r%v", params[0], params[1], params[2])

	case MUL_IMM_32:
		return fmt.Sprintf("i32 r%v = r%v * %#x", params[0], params[1], params[2])

	case MUL_IMM_64:
		return fmt.Sprintf("r%v = r%v * %#x", params[0], params[1], params[2])

	case DIV_U_32:
		return fmt.Sprintf("i32 r%v = r%v /u r%v", params[0], params[1], params[2])

	case DIV_U_64:
		return fmt.Sprintf("r%v = r%v /u r%v", params[0], params[1], params[2])

	case DIV_S_32:
		return fmt.Sprintf("i32 r%v = r%v /s r%v", params[0], params[1], params[2])

	case DIV_S_64:
		return fmt.Sprintf("r%v = r%v /s r%v", params[0], params[1], params[2])

	case REM_U_32:
		return fmt.Sprintf("i32 r%v = r%v %%u r%v", params[0], params[1], params[2])

	case REM_U_64:
		return fmt.Sprintf("r%v = r%v %%u r%v", params[0], params[1], params[2])

	case REM_S_32:
		return fmt.Sprintf("i32 r%v = r%v %%s r%v", params[0], params[1], params[2])

	case REM_S_64:
		return fmt.Sprintf("r%v = r%v %%s r%v", params[0], params[1], params[2])

	case SHLO_L_32:
		return fmt.Sprintf("i32 r%v = r%v << r%v", params[0], params[1], params[2])

	case SHLO_L_64:
		return fmt.Sprintf("r%v = r%v << r%v", params[0], params[1], params[2])

	case SHLO_L_IMM_32:
		return fmt.Sprintf("i32 r%v = r%v << %#x", params[0], params[1], params[2])

	case SHLO_L_IMM_64:
		return fmt.Sprintf("r%v = r%v << %#x", params[0], params[1], params[2])

	case SHLO_L_IMM_ALT_32:
		return fmt.Sprintf("i32 r%v = %#x << r%v", params[0], params[2], params[1])

	case SHLO_L_IMM_ALT_64:
		return fmt.Sprintf("r%v = %#x << r%v", params[0], params[2], params[1])

	case SHLO_R_32:
		return fmt.Sprintf("i32 r%v = r%v >> r%v", params[0], params[1], params[2])

	case SHLO_R_64:
		return fmt.Sprintf("r%v = r%v >> r%v", params[0], params[1], params[2])

	case SHLO_R_IMM_32:
		return fmt.Sprintf("i32 r%v = r%v >> %#x", params[0], params[1], params[2])

	case SHLO_R_IMM_64:
		return fmt.Sprintf("r%v = r%v >> %#x", params[0], params[1], params[2])

	case SHLO_R_IMM_ALT_32:
		return fmt.Sprintf("i32 r%v = %#x >> r%v", params[0], params[2], params[1])

	case SHLO_R_IMM_ALT_64:
		return fmt.Sprintf("r%v = %#x >> r%v", params[0], params[2], params[1])

	case SHAR_R_32:
		return fmt.Sprintf("i32 r%v = r%v >>a r%v", params[0], params[1], params[2])

	case SHAR_R_64:
		return fmt.Sprintf("r%v = r%v >>a r%v", params[0], params[1], params[2])

	case SHAR_R_IMM_32:
		return fmt.Sprintf("i32 r%v = r%v >>a %#x", params[0], params[1], params[2])

	case SHAR_R_IMM_64:
		return fmt.Sprintf("r%v = r%v >>a %#x", params[0], params[1], params[2])

	case SHAR_R_IMM_ALT_32:
		return fmt.Sprintf("i32 r%v = %#x >>a r%v", params[0], params[2], params[1])

	case SHAR_R_IMM_ALT_64:
		return fmt.Sprintf("r%v = %#x >>a r%v", params[0], params[2], params[1])

	case BRANCH_EQ:
		var offset int64
		if v, ok := params[2].(int64); ok {
			offset = v
		} else {
			offset = int64(params[2].(uint64))
		}
		target := int64(pc) + offset
		return fmt.Sprintf("jump %d if r%v == r%v", target, params[0], params[1])

	case BRANCH_EQ_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		immVal := uint32(params[1].(uint64))
		return fmt.Sprintf("jump %d if r%v == %v", target, params[0], immVal)

	case BRANCH_NE:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v != r%v", target, params[0], params[1])

	case BRANCH_NE_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v != %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_LT_U:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <u r%v", target, params[0], params[1])

	case BRANCH_LT_U_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <u %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_LT_S:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <s r%v", target, params[0], params[1])

	case BRANCH_LT_S_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <s %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_GE_U:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >=u r%v", target, params[0], params[1])

	case BRANCH_GE_U_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >=u %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_GE_S:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >=s r%v", target, params[0], params[1])

	case BRANCH_GE_S_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >=s %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_LE_U_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <=u %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_LE_S_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v <=s %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_GT_U_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >u %v", target, params[0], uint32(params[1].(uint64)))

	case BRANCH_GT_S_IMM:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d if r%v >s %v", target, params[0], uint32(params[1].(uint64)))

	case JUMP:
		offset := params[0].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("jump %d", target)

	case JUMP_IND:
		if len(params) > 1 {
			offset := params[1]
			// Check if offset is numeric zero (could be int, uint, etc.)
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("jump [r%v + 0]", params[0])
			}
			return fmt.Sprintf("jump [r%v + %#x]", params[0], offset)
		}
		return fmt.Sprintf("jump [r%v]", params[0])

	case LOAD_U8:
		return fmt.Sprintf("r%v = u8 [%#x]", params[0], params[1])

	case LOAD_I8:
		return fmt.Sprintf("r%v = i8 [%#x]", params[0], params[1])

	case LOAD_U16:
		return fmt.Sprintf("r%v = u16 [%#x]", params[0], params[1])

	case LOAD_I16:
		return fmt.Sprintf("r%v = i16 [%#x]", params[0], params[1])

	case LOAD_U32:
		return fmt.Sprintf("r%v = u32 [%#x]", params[0], params[1])

	case LOAD_I32:
		return fmt.Sprintf("r%v = i32 [%#x]", params[0], params[1])

	case LOAD_U64:
		return fmt.Sprintf("r%v = u64 [%#x]", params[0], params[1])

	case LOAD_IND_U8:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = u8 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = u8 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = u8 [r%v]", params[0], params[1])

	case LOAD_IND_I8:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = i8 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = i8 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = i8 [r%v]", params[0], params[1])

	case LOAD_IND_U16:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = u16 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = u16 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = u16 [r%v]", params[0], params[1])

	case LOAD_IND_I16:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = i16 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = i16 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = i16 [r%v]", params[0], params[1])

	case LOAD_IND_U32:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = u32 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = u32 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = u32 [r%v]", params[0], params[1])

	case LOAD_IND_I32:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = i32 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = i32 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = i32 [r%v]", params[0], params[1])

	case LOAD_IND_U64:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("r%v = u64 [r%v + 0]", params[0], params[1])
			}
			return fmt.Sprintf("r%v = u64 [r%v + %#x]", params[0], params[1], offset)
		}
		return fmt.Sprintf("r%v = u64 [r%v]", params[0], params[1])

	case STORE_U8:
		return fmt.Sprintf("u8 [%#x] = r%v", params[1], params[0])

	case STORE_U16:
		return fmt.Sprintf("u16 [%#x] = r%v", params[1], params[0])

	case STORE_U32:
		return fmt.Sprintf("u32 [%#x] = r%v", params[1], params[0])

	case STORE_U64:
		return fmt.Sprintf("u64 [%#x] = r%v", params[1], params[0])

	case STORE_IMM_U8:
		return fmt.Sprintf("u8 [%#x] = %#x", params[0], params[1])

	case STORE_IMM_U16:
		return fmt.Sprintf("u16 [%#x] = %#x", params[0], params[1])

	case STORE_IMM_U32:
		return fmt.Sprintf("u32 [%#x] = %#x", params[0], params[1])

	case STORE_IMM_U64:
		return fmt.Sprintf("u64 [%#x] = %#x", params[0], params[1])

	case STORE_IND_U8:
		// Special formatting for STORE_IND_U8: register 8 should be "a1", others "r%d"
		formatRegForStoreInd := func(regIndex int) string {
			if regIndex == 8 {
				return "a1"
			}
			return fmt.Sprintf("r%d", regIndex)
		}

		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("u8 [r%d + 0] = %s", params[1].(int), formatRegForStoreInd(params[0].(int)))
			}
			return fmt.Sprintf("u8 [r%d + %#x] = %s", params[1].(int), offset, formatRegForStoreInd(params[0].(int)))
		}
		return fmt.Sprintf("u8 [%s] = %s", formatRegForStoreInd(params[1].(int)), formatRegForStoreInd(params[0].(int)))

	case STORE_IND_U16:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("u16 [r%v + 0] = r%v", params[1], params[0])
			}
			return fmt.Sprintf("u16 [r%v + %#x] = r%v", params[1], offset, params[0])
		}
		return fmt.Sprintf("u16 [r%v] = r%v", params[1], params[0])

	case STORE_IND_U32:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("u32 [r%v + 0] = r%v", params[1], params[0])
			}
			return fmt.Sprintf("u32 [r%v + %#x] = r%v", params[1], offset, params[0])
		}
		return fmt.Sprintf("u32 [r%v] = r%v", params[1], params[0])

	case STORE_IND_U64:
		if len(params) > 2 {
			offset := params[2]
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("u64 [r%v + 0] = r%v", params[1], params[0])
			}
			return fmt.Sprintf("u64 [r%v + %#x] = r%v", params[1], offset, params[0])
		}
		return fmt.Sprintf("u64 [r%v] = r%v", params[1], params[0])

	case STORE_IMM_IND_U8:
		offset := params[1]
		if fmt.Sprintf("%v", offset) == "0" {
			return fmt.Sprintf("u8 [r%v + 0] = %#x", params[0], params[2])
		}
		return fmt.Sprintf("u8 [r%v + %v] = %#x", params[0], offset, params[2])

	case STORE_IMM_IND_U16:
		offset := params[1]
		if fmt.Sprintf("%v", offset) == "0" {
			return fmt.Sprintf("u16 [r%v + 0] = %#x", params[0], params[2])
		}
		return fmt.Sprintf("u16 [r%v + %v] = %#x", params[0], offset, params[2])

	case STORE_IMM_IND_U32:
		offset := params[1]
		if fmt.Sprintf("%v", offset) == "0" {
			return fmt.Sprintf("u32 [r%v + 0] = %#x", params[0], params[2])
		}
		return fmt.Sprintf("u32 [r%v + %v] = %#x", params[0], offset, params[2])

	case STORE_IMM_IND_U64:
		offset := params[1]
		if fmt.Sprintf("%v", offset) == "0" {
			return fmt.Sprintf("u64 [r%v + 0] = %#x", params[0], params[2])
		}
		return fmt.Sprintf("u64 [r%v + %v] = %#x", params[0], offset, params[2])

	case SET_LT_U:
		return fmt.Sprintf("r%v = r%v <u r%v", params[0], params[1], params[2])

	case SET_LT_U_IMM:
		return fmt.Sprintf("r%v = r%v <u %#x", params[0], params[1], params[2])

	case SET_LT_S:
		return fmt.Sprintf("r%v = r%v <s r%v", params[0], params[1], params[2])

	case SET_LT_S_IMM:
		return fmt.Sprintf("r%v = r%v <s %#x", params[0], params[1], params[2])

	case SET_GT_S_IMM:
		return fmt.Sprintf("r%v = r%v >s %#x", params[0], params[1], params[2])

	case SET_GT_U_IMM:
		return fmt.Sprintf("r%v = r%v >u %#x", params[0], params[1], params[2])

	case CMOV_IZ:
		return fmt.Sprintf("r%v = r%v if r%v == 0", params[0], params[1], params[2])

	case CMOV_NZ:
		return fmt.Sprintf("r%v = r%v if r%v != 0", params[0], params[1], params[2])

	case CMOV_IZ_IMM:
		return fmt.Sprintf("r%v = %#x if r%v == 0", params[0], params[2], params[1])

	case CMOV_NZ_IMM:
		return fmt.Sprintf("r%v = %#x if r%v != 0", params[0], params[2], params[1])

	case NEG_ADD_IMM_32:
		return fmt.Sprintf("i32 r%v = %#x - r%v", params[0], params[2], params[1])

	case NEG_ADD_IMM_64:
		return fmt.Sprintf("r%v = %#x - r%v", params[0], params[2], params[1])

	// Memory management
	case SBRK:
		return fmt.Sprintf("r%v = sbrk(r%v)", params[0], params[1])

	// Bit manipulation operations
	case COUNT_SET_BITS_64:
		return fmt.Sprintf("r%v = popcnt64(r%v)", params[0], params[1])

	case COUNT_SET_BITS_32:
		return fmt.Sprintf("i32 r%v = popcnt32(r%v)", params[0], params[1])

	case LEADING_ZERO_BITS_64:
		return fmt.Sprintf("r%v = lzcnt64(r%v)", params[0], params[1])

	case LEADING_ZERO_BITS_32:
		return fmt.Sprintf("i32 r%v = lzcnt32(r%v)", params[0], params[1])

	case TRAILING_ZERO_BITS_64:
		return fmt.Sprintf("r%v = tzcnt64(r%v)", params[0], params[1])

	case TRAILING_ZERO_BITS_32:
		return fmt.Sprintf("i32 r%v = tzcnt32(r%v)", params[0], params[1])

	case SIGN_EXTEND_8:
		return fmt.Sprintf("r%v = sext.b(r%v)", params[0], params[1])

	case SIGN_EXTEND_16:
		return fmt.Sprintf("r%v = sext.h(r%v)", params[0], params[1])

	case ZERO_EXTEND_16:
		return fmt.Sprintf("r%v = zext.h(r%v)", params[0], params[1])

	case REVERSE_BYTES:
		return fmt.Sprintf("r%v = revbytes(r%v)", params[0], params[1])

	// Upper multiplication operations
	case MUL_UPPER_S_S:
		return fmt.Sprintf("r%v = r%v *upper_s r%v", params[0], params[1], params[2])

	case MUL_UPPER_U_U:
		return fmt.Sprintf("r%v = r%v *upper_u r%v", params[0], params[1], params[2])

	case MUL_UPPER_S_U:
		return fmt.Sprintf("r%v = r%v *upper_su r%v", params[0], params[1], params[2])

	// Rotation operations
	case ROT_L_64:
		return fmt.Sprintf("r%v = r%v rotL r%v", params[0], params[1], params[2])

	case ROT_L_32:
		return fmt.Sprintf("i32 r%v = r%v rotL r%v", params[0], params[1], params[2])

	case ROT_R_64:
		return fmt.Sprintf("r%v = r%v rotR r%v", params[0], params[1], params[2])

	case ROT_R_32:
		return fmt.Sprintf("i32 r%v = r%v rotR r%v", params[0], params[1], params[2])

	// Inverted logical operations
	case AND_INV:
		return fmt.Sprintf("r%v = r%v & ~r%v", params[0], params[1], params[2])

	case OR_INV:
		return fmt.Sprintf("r%v = r%v | ~r%v", params[0], params[1], params[2])

	case XNOR:
		return fmt.Sprintf("r%v = ~(r%v ^ r%v)", params[0], params[1], params[2])

	// Min/Max operations
	case MAX:
		return fmt.Sprintf("r%v = max(r%v, r%v)", params[0], params[1], params[2])

	case MAX_U:
		return fmt.Sprintf("r%v = max_u(r%v, r%v)", params[0], params[1], params[2])

	case MIN:
		return fmt.Sprintf("r%v = min(r%v, r%v)", params[0], params[1], params[2])

	case MIN_U:
		return fmt.Sprintf("r%v = min_u(r%v, r%v)", params[0], params[1], params[2])

	case LOAD_IMM_JUMP:
		offset := params[2].(int64)
		target := uint64(int64(pc) + offset)
		return fmt.Sprintf("r%v = %v, jump %d", params[0], params[1], target)

	case LOAD_IMM_JUMP_IND:
		// params[0] = destination register for immediate
		// params[1] = base register for jump
		// params[2] = immediate value
		// params[3] = offset for jump
		immVal := params[2]
		offset := params[3]

		// Special case: when destination register == base register for jump
		if params[0] == params[1] {
			if fmt.Sprintf("%v", offset) == "0" {
				return fmt.Sprintf("tmp = r%v, r%v = %v, jump [tmp + 0]", params[1], params[0], immVal)
			}
			return fmt.Sprintf("tmp = r%v, r%v = %v, jump [tmp + %#x]", params[1], params[0], immVal, offset)
		}

		// Normal case: different registers
		if fmt.Sprintf("%v", offset) == "0" {
			return fmt.Sprintf("r%v = %v, jump [r%v + 0]", params[0], immVal, params[1])
		}
		return fmt.Sprintf("r%v = %v, jump [r%v + %#x]", params[0], immVal, params[1], offset)

	case FALLTHROUGH:
		return "llthrough"

	case ECALLI:
		return fmt.Sprintf("ecalli %#x", params[0])

	default:
		// For any unhandled instructions, fall back to original format
		if def, exists := instrTable[opcode]; exists {
			formatted := def.Format(params)
			return fmt.Sprintf("%s %s", def.Name, formatted)
		}
		return fmt.Sprintf("unknown opcode %d", opcode)
	}
}

// DisassemblePVMCode is a convenience function that creates a VM instance to disassemble code
// This provides backwards compatibility for code that doesn't have a VM instance
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
