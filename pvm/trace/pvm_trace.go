package trace

import "github.com/jam-duna/jamduna/common"

type SimpleInstruction struct {
	Opcode  uint8    `json:"opcode"`
	SrcRegs []int    `json:"srcRegs,omitempty"`
	DstRegs []int    `json:"dstRegs,omitempty"`
	Imm     []uint64 `json:"imm,omitempty"`
}

type TraceStep struct {
	Instruction SimpleInstruction `json:"instruction"`
	OpcodeStr   string            `json:"opcodeStr,omitempty"`

	PostGas             uint64      `json:"postGas"`
	PostMachineState    *string     `json:"postMachineState,omitempty"` // TODO : replaced with polkajam format
	PostFaultAddress    *uint64     `json:"postFaultAddress,omitempty"`
	PostRegister        *[13]uint64 `json:"postRegister,omitempty"` // TODO : replaced with polkajam format
	ChangedMemoryAddr   *uint64     `json:"changedMemoryAddr,omitempty"`
	ChangedMemoryLength *uint64     `json:"changedMemoryLength,omitempty"`
	ChangedMemoryBytes  []byte      `json:"changedMemoryBytes,omitempty"` // if more than 32 bytes changed -> store the hash of the bytes
}

func NewTraceStep(inst SimpleInstruction) *TraceStep {
	return &TraceStep{
		Instruction: inst,
	}
}

func (ts *TraceStep) SetPostRegister(regs *[13]uint64) {
	copied := *regs
	ts.PostRegister = &copied
}

func (ts *TraceStep) SetChangedMemory(addr uint64, length uint64, bytes []byte) {
	ts.ChangedMemoryAddr = &addr
	ts.ChangedMemoryLength = &length
	ts.ChangedMemoryBytes = make([]byte, len(bytes))
	copy(ts.ChangedMemoryBytes, bytes)
	if len(ts.ChangedMemoryBytes) == 0 {
		ts.ChangedMemoryBytes = nil
	} else if len(ts.ChangedMemoryBytes) > 32 {
		ts.ChangedMemoryBytes = common.Blake2Hash(ts.ChangedMemoryBytes).Bytes()
	}
}

func (ts *TraceStep) SetPostMachineState(state string) {
	ts.PostMachineState = &state
}

func (ts *TraceStep) SetPostFaultAddress(addr uint64) {
	ts.PostFaultAddress = &addr
}
