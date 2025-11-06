package recompiler

type TestCaseNew struct {
	Name          string            `json:"name"`
	InitialPC     uint64            `json:"initial-pc"`
	InitialGas    uint64            `json:"initial-gas"`
	Program       []byte            `json:"program"`
	Steps         []Step            `json:"steps"`
	BlockGasCosts map[string]uint64 `json:"block-gas-costs"`
}

type Step struct {
	Kind string `json:"kind"` // "map" | "write" | "set-reg" | "run" | "assert"`

	// for kind == "map"
	Address    uint32 `json:"address,omitempty"`
	Length     uint32 `json:"length,omitempty"`
	IsWritable bool   `json:"is_writable,omitempty"`

	// for kind == "write"
	Contents []byte `json:"contents,omitempty"`

	// for kind == "set-reg"
	Reg   int    `json:"reg,omitempty"`
	Value uint64 `json:"value,omitempty"`

	// for kind == "assert"
	Status string      `json:"status,omitempty"`
	Gas    uint64      `json:"gas,omitempty"`
	PC     uint64      `json:"pc,omitempty"`
	Regs   []uint64    `json:"regs,omitempty"`
	Memory []MemAssert `json:"memory,omitempty"`
}

type MemAssert struct {
	Address  uint32 `json:"address"`
	Contents []byte `json:"contents"`
}
