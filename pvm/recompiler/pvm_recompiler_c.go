package recompiler

import (
	"github.com/colorfulnotion/jam/pvm/recompiler_c/recompiler_c"
)

type RecompilerC struct {
	recompiler_c.C_Compiler
}

func NewRecompilerC(code []byte) *RecompilerC {
	cCompiler := recompiler_c.NewC_Compiler(code)
	if cCompiler == nil {
		return nil
	}
	return &RecompilerC{
		C_Compiler: *cCompiler,
	}
}

func (rc *RecompilerC) GetBasicBlock(pvmPC uint64) *BasicBlock {
	return nil
}
