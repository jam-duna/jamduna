package x86gencheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/pvm/recompiler"
	recompiler_c "github.com/colorfulnotion/jam/pvm/recompiler_c/recompiler_c"
)

func TestCompilePVM(t *testing.T) {
	pattern := filepath.Join("..", "..", "services", "*", "*.pvm")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("No .pvm files found with pattern %s", pattern)
	}

	for _, path := range matches {
		if strings.Contains(path, "_blob") {
			continue
		}
		path := path // capture
		name := filepath.Base(path)
		t.Logf("testing file: %s", path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read file %s: %v", path, err)
			}
			code, bitmask, jumpTable, err := ParsePvmByteCode(data)
			if err != nil {
				t.Fatalf("Failed to parse PVM bytecode from %s: %v", path, err)
			}
			testPvmFileCompiler(t, code, bitmask, jumpTable)
		})
	}
}

func testPvmFileCompiler(t *testing.T, code []byte, bitmask []byte, jumpTable []uint32) {
	fmt.Printf("PVM code length: %d, bitmask length: %d, jumpTable length: %d\n", len(code), len(bitmask), len(jumpTable))
	// go side
	compilerGo := recompiler.NewX86Compiler(code)
	err := compilerGo.SetBitMask(bitmask)
	if err != nil {
		t.Fatalf("SetBitMask failed: %v", err)
	}
	err = compilerGo.SetJumpTable(jumpTable)
	if err != nil {
		t.Fatalf("SetJumpTable failed: %v", err)
	}
	start := time.Now()
	x86Code, djumpAddress, InstMapPVMToX86, InstMapX86ToPvm := compilerGo.CompileX86Code(0)
	elapsed := time.Since(start)
	fmt.Printf("Go CompileX86Code execution time: %s\n", elapsed)
	if err != nil {
		t.Fatalf("Go CompileX86Code failed: %v", err)
	}
	fmt.Printf("Go CompileX86Code: x86Code length: %d, djumpAddress: %x, InstMapPVMToX86 length: %d, InstMapX86ToPvm length: %d\n", len(x86Code), djumpAddress, len(InstMapPVMToX86), len(InstMapX86ToPvm))

	// c side
	compilerC := recompiler_c.NewC_Compiler(code)
	err = compilerC.SetBitMask(bitmask)
	if err != nil {
		t.Fatalf("C SetBitMask failed: %v", err)
	}
	err = compilerC.SetJumpTable(jumpTable)
	if err != nil {
		t.Fatalf("C SetJumpTable failed: %v", err)
	}
	start = time.Now()
	x86CodeC, djumpAddressC, InstMapPVMToX86C, InstMapX86ToPvmC := compilerC.CompileX86Code(0)
	elapsed = time.Since(start)
	fmt.Printf("C CompileX86Code execution time: %s\n", elapsed)
	if err != nil {
		t.Fatalf("C CompileX86Code failed: %v", err)
	}
	fmt.Printf("C CompileX86Code: x86Code length: %d, djumpAddress: %x, InstMapPVMToX86 length: %d, InstMapX86ToPvm length: %d\n", len(x86CodeC), djumpAddressC, len(InstMapPVMToX86C), len(InstMapX86ToPvmC))
	var tmpInstructionOffset int
	var tmpOp byte

	instrsGo := recompiler.DisassembleInstructions(x86Code)
	instrsC := recompiler.DisassembleInstructions(x86CodeC)
	t.Logf("djumpAddress: Go %x, C %x\n", djumpAddress, djumpAddressC)
	for i, instr := range instrsGo {
		if i >= len(instrsC) {
			t.Fatalf("C x86 code has fewer instructions (%d) than Go (%d)", len(instrsC), len(instrsGo))
		}
		instrC := instrsC[i]
		if instr.Offset != instrC.Offset {
			// find the neartest pvm pc for context
			for index := instr.Offset; index >= 0; index-- {
				if pvmPc, ok := InstMapX86ToPvm[index]; ok {
					tmpInstructionOffset = index
					tmpOp = code[pvmPc]
					break
				}
			}
			t.Logf("code mismatch at pvm op %d (%s) ", tmpOp, recompiler.GetOpcodeStr(tmpOp))
			if i-10 >= 0 {
				for j := i - 10; j < i; j++ {
					t.Logf("Previous Go instruction:[0x%x] %s", instrsGo[j].Offset, instrsGo[j].Instruction.String())
					t.Logf("Previous C instruction: [0x%x] %s", instrsC[j].Offset, instrsC[j].Instruction.String())
				}
			}

			t.Logf("Go instruction: %s", instrsGo[i].Instruction.String())
			t.Logf(" C instruction:  %s", instrsC[i].Instruction.String())
			t.Fatalf("Mismatch in disassembled instructions at index %d: Go: %s, C: %s", i, instr.Instruction.String(), instrC.Instruction.String())
		}
	}

	for offset := 0; offset < len(x86Code); offset++ {
		if pvmPc, ok := InstMapX86ToPvm[offset]; ok {
			tmpInstructionOffset = offset
			tmpOp = code[pvmPc]
		}
		if x86Code[offset] != x86CodeC[offset] {
			// Print nearby code for context, safely
			start := offset - 25
			if start < 0 {
				start = 0
			}
			end := offset + 25
			if end > len(x86Code) {
				end = len(x86Code)
			}
			endC := offset + 25
			if endC > len(x86CodeC) {
				endC = len(x86CodeC)
			}
			go_disasm := recompiler.Disassemble(x86Code[start:end])
			c_disasm := recompiler.Disassemble(x86CodeC[start:endC])
			t.Logf("Nearby Go x86 code: % X\nNearby C x86 code:  % X\nGo ASM: %s\nC ASM:  %s", x86Code[start:end], x86CodeC[start:endC], go_disasm, c_disasm)

			t.Fatalf("Mismatch at x86 code offset %d (0x%x) for PVM instruction 0x%02x (%s) at PVM pc 0x%x (last instruction 0x%02x at x86 offset 0x%x)\n", offset, offset, tmpOp, recompiler.GetOpcodeStr(tmpOp), InstMapX86ToPvm[tmpInstructionOffset], tmpOp, tmpInstructionOffset)
		}
	}
}
