//go:build unicorn
// +build unicorn

package statedb

import (
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

type RecompilerSandbox struct {
	*recompiler.RecompilerSandboxVM
}

func NewRecompilerVMSandbox(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, initialGas uint64, pvmBackend string) *RecompilerSandbox {
	if len(pvmBackend) == 0 {
		panic("pvmBackend cannot be empty")
	}
	if len(code) == 0 {
		return nil
	}

	var p *Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte
	var err error

	if jam_ready_blob {
		p, o_size, w_size, z, s, o_byte, w_byte, err = DecodeProgram(code)
		if err != nil {
			log.Error("", "DecodeProgram failed", "error", err)
			return nil
		}
	} else {
		p, err = DecodeProgram_pure_pvm_blob(code)
		if err != nil {
			log.Error("", "DecodeProgram_pure_pvm_blob failed", "error", err)
			return nil
		}
		o_size = 0
		w_size = uint32(initialHeap)
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = make([]byte, w_size)
	}
	rvm_raw, err := recompiler.NewRecompilerVM(service_index, p.Code, initialRegs, initialPC)
	if err != nil {
		log.Error("", "RecompilerVM creation failed", "error", err)
		return nil
	}
	rvm, err := recompiler.NewRecompilerSandboxVM(rvm_raw)
	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end + z*Z_P
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	rvm.SetGas(initialGas) // Gas will be set later by specific execution methods
	rvm.ServiceMetadata = Metadata

	recompiler := &RecompilerSandbox{
		RecompilerSandboxVM: rvm,
	}
	return recompiler
}

func (rvm *RecompilerSandbox) Execute(VM *VM, entry uint32, logDir string) error {
	rvm.HostFunc = VM
	// Note: logDir is currently unused in sandbox recompiler backend
	// Tracing is handled differently in the sandboxed compiled code
	rvm.RecompilerSandboxVM.ExecuteSandBox(uint64(entry))
	state := rvm.RecompilerSandboxVM.MachineState
	VM.ResultCode = state
	return nil
}

func (rvm *RecompilerSandbox) Destroy() {
	rvm.RecompilerSandboxVM.Close()
}

func (rvm *RecompilerSandbox) ReadRegisters() [13]uint64 {
	var regs [13]uint64
	for i := 0; i < 13; i++ {
		regs[i] = rvm.RecompilerSandboxVM.ReadRegister(i)
	}
	return regs
}

func (rvm *RecompilerSandbox) InitStepwise(vm *VM, entryPoint uint32) error {
	panic("InitStepwise not implemented for RecompilerSandbox backend")
}

func (rvm *RecompilerSandbox) ExecuteStep(vm *VM) []byte {
	panic("ExecuteStep not implemented for RecompilerSandbox backend")
}
