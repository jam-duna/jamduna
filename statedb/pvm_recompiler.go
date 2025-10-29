package statedb

import (
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/recompiler"
	"github.com/colorfulnotion/jam/types"
)

type Recompiler struct {
	*recompiler.RecompilerVM
}

func NewRecompilerVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, initialGas uint64, pvmBackend string) *Recompiler {
	if len(pvmBackend) == 0 {
		panic("pvmBackend cannot be empty")
	}
	if len(code) == 0 {
		return nil
	}

	var p *Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	if jam_ready_blob {
		p, o_size, w_size, z, s, o_byte, w_byte = DecodeProgram(code)
	} else {
		p = DecodeProgram_pure_pvm_blob(code)
		o_size = 0
		w_size = uint32(initialHeap)
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = make([]byte, w_size)
	}
	rvm, err := recompiler.NewRecompilerVM(service_index, p.Code, initialRegs, initialPC)
	if err != nil {
		log.Error("", "RecompilerVM creation failed", "error", err)
		return nil
	}

	rvm.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	rvm.SetHeapPointer(current_heap_pointer)
	rvm.SetBitMask(p.K)
	rvm.SetJumpTable(p.J)
	rvm.SetGas(int64(initialGas)) // Gas will be set later by specific execution methods
	rvm.ServiceMetadata = Metadata

	recompiler := &Recompiler{
		RecompilerVM: rvm,
	}
	return recompiler
}

func (rvm *Recompiler) Execute(VM *VM, entry uint32) error {
	rvm.HostFunc = VM
	rvm.RecompilerVM.Execute(entry)
	return nil
}

func (rvm *Recompiler) Destroy() {
	rvm.RecompilerVM.Close()
}

func (rvm *Recompiler) SetPage(uint32, uint32, uint8) {

}
