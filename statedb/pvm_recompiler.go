package statedb

import (
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

type Recompiler struct {
	*recompiler.RecompilerVM
}

func NewRecompilerVM(service_index uint32, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, initialGas uint64, p *Program, o_size uint32, w_size uint32, z uint32, s uint32, o_byte []byte, w_byte []byte) *Recompiler {
	code := p.Code
	if len(code) == 0 {
		return nil
	}
	rvm, err := recompiler.NewRecompilerVM(service_index, p.Code, initialRegs, initialPC)
	if err != nil {
		log.Error("", "RecompilerVM creation failed", "error", err)
		return nil
	}

	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size) + z*Z_P
	current_heap_pointer := rw_data_address_end
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	rvm.SetGas(initialGas) // Gas will be set later by specific execution methods
	rvm.ServiceMetadata = Metadata

	recompiler := &Recompiler{
		RecompilerVM: rvm,
	}
	return recompiler
}

func NewRecompilerVMWithoutSetup(service_index uint32, initialRegs []uint64, initialPC uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, initialGas uint64, p *Program) *Recompiler {
	code := p.Code
	if len(code) == 0 {
		return nil
	}
	rvm, err := recompiler.NewRecompilerVM(service_index, p.Code, initialRegs, initialPC)
	if err != nil {
		log.Error("", "RecompilerVM creation failed", "error", err)
		return nil
	}
	rvm.SetMemoryBounds(0, 0, 0, 0, nil, nil)
	rvm.SetGas(initialGas) // Gas will be set later by specific execution methods
	rvm.ServiceMetadata = Metadata
	rvm.SetHeapPointer(0)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)

	// Child VMs use per-instruction gas mode to match interpreter trace for verification
	rvm.SetCompilerGasMode(recompiler.GasModeInstruction)
	rvm.SetCompilerIsChild(true)
	rvm.IsChild = true

	recompilerVM := &Recompiler{
		RecompilerVM: rvm,
	}
	return recompilerVM
}

func (rvm *Recompiler) Execute(VM *VM, entry uint32, logDir string) error {
	rvm.HostFunc = VM
	// Set LogDir for trace verification support
	rvm.RecompilerVM.LogDir = logDir
	rvm.RecompilerVM.Execute(entry)
	VM.ResultCode = rvm.GetResultCode()
	return nil
}

func (rvm *Recompiler) GetFaultAddress() uint64 {
	return uint64(rvm.RecompilerVM.FaultAddr)
}

func (rvm *Recompiler) Destroy() {
	rvm.RecompilerVM.CloseVerifier()
	rvm.RecompilerVM.Close()
}

func (rvm *Recompiler) SetPage(uint32, uint32, uint8) {

}

func (rvm *Recompiler) InitStepwise(vm *VM, entryPoint uint32) error {
	panic("InitStepwise not implemented for Recompiler backend")
}

func (rvm *Recompiler) ExecuteStep(vm *VM) []byte {
	panic("ExecuteStep not implemented for Recompiler backend")
}
