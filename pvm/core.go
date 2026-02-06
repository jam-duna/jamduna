package pvm

import (
	"fmt"

	"github.com/jam-duna/jamduna/pvm/program"
	"github.com/jam-duna/jamduna/pvm/pvmtypes"
	"github.com/jam-duna/jamduna/types"
)

const (
	BackendInterpreter = "interpreter" // Go interpreter
	BackendCompiler    = "compiler"    // X86 recompiler
)

type Program = program.Program

type ExecutionVM interface {
	ReadRegister(int) uint64
	ReadRegisters() [13]uint64
	WriteRegister(int, uint64)
	ReadRAMBytes(uint32, uint32) ([]byte, uint64)
	WriteRAMBytes(uint32, []byte) uint64
	SetHeapPointer(uint32)
	SetPagesAccessRange(startPage, pageCount int, access int) error
	GetGas() int64
	SetGas(int64)
	GetPC() uint64
	SetPC(uint64)
	GetMachineState() uint8
	GetFaultAddress() uint64
	Panic(uint64)
	SetHostResultCode(uint64)
	Init(argument_data_a []byte) (err error)
	Execute(vm pvmtypes.HostVM, EntryPoint uint32, logDir string) error
	ExecuteAsChild(entryPoint uint32) error
	GetHostID() uint64
	Destroy()

	// to support ExecuteWorkPackageBundleSteps
	InitStepwise(vm pvmtypes.HostVM, entryPoint uint32) error
	ExecuteStep(vm pvmtypes.HostVM) []byte

	// CheckMemoryAccess checks if a memory range is readable or writable
	// Returns (canRead bool, canWrite bool)
	CheckMemoryAccess(address uint32, length uint32) (bool, bool)
}

func DecodeProgram(p []byte) (*Program, uint32, uint32, uint32, uint32, []byte, []byte, error) {
	prog, o, w, z, s, oB, wB, err := program.DecodeProgram(p)
	return (*Program)(prog), o, w, z, s, oB, wB, err
}

func DecodeProgram_pure_pvm_blob(p []byte) (*Program, error) {
	prog, err := decodeCorePart(p)
	return (*Program)(prog), err
}

// EncodeProgram encodes raw instruction code and bitmask into a PVM blob format.
// The result can be decoded by DecodeProgram_pure_pvm_blob.
func EncodeProgram(code []byte, bitmask []byte) []byte {
	jSize := uint64(0) // TODO: add J support
	z := uint64(0)     // TODO: add z support correctly
	cSize := uint64(len(code))

	jSizeEncoded := types.E(jSize)
	zEncoded := types.E(z)
	cSizeEncoded := types.E(cSize)

	kBytes := compressBits(bitmask)

	blob := make([]byte, 0, len(jSizeEncoded)+len(zEncoded)+len(cSizeEncoded)+len(code)+len(kBytes))
	blob = append(blob, jSizeEncoded...)
	blob = append(blob, zEncoded...)
	blob = append(blob, cSizeEncoded...)
	blob = append(blob, code...)
	blob = append(blob, kBytes...)

	fmt.Printf("EncodeProgram: j_size=%d, z=%d, c_size=%d, code_len=%d, bitmask_len=%d, k_bytes_len=%d, total_blob_len=%d\n",
		jSize, z, cSize, len(code), len(bitmask), len(kBytes), len(blob))
	return blob
}

// compressBits converts a bitmask bit array ([]byte with 0/1 values) to packed bytes.
func compressBits(bitmask []byte) []byte {
	if len(bitmask) == 0 {
		return []byte{}
	}

	numBytes := (len(bitmask) + 7) / 8
	compressed := make([]byte, numBytes)

	for i, bit := range bitmask {
		if bit != 0 {
			byteIndex := i / 8
			bitIndex := i % 8
			compressed[byteIndex] |= (1 << bitIndex)
		}
	}

	return compressed
}

var decodeCorePart = program.DecodeCorePart
