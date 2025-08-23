package pvm

import (
	"fmt"

	"github.com/colorfulnotion/jam/log"
)

type RAMInterface interface {
	WriteRAMBytes(address uint32, data []byte) uint64
	ReadRAMBytes(address uint32, length uint32) ([]byte, uint64)
	allocatePages(startPage uint32, count uint32)
	GetCurrentHeapPointer() uint32
	SetCurrentHeapPointer(pointer uint32)
	//SetPageAccess(pageIndex int, access byte)
	ReadRegister(index int) (uint64, uint64)
	WriteRegister(index int, value uint64) uint64
	ReadRegisters() []uint64
	GetDirtyPages() []int

	// WriteContextSlot(slot_index int, value uint64) error
	// ReadContextSlot(slot_index int) (uint64, error)
}

type RAM struct {
	stack_address        uint32
	stack_address_end    uint32
	rw_data_address      uint32
	rw_data_address_end  uint32
	ro_data_address      uint32
	ro_data_address_end  uint32
	current_heap_pointer uint32
	output_address       uint32
	output_end           uint32

	first64k [65536]byte // first 64K of memory, used for special purposes
	stack    []byte
	rw_data  []byte
	ro_data  []byte
	output   []byte
	register []uint64
}

func (ram *RAM) GetDirtyPages() []int {
	panic("GetDirtyPages not implemented for RAM")
}

//		vm.Ram = NewRAM(o_size, w_size, z, s, o_byte, w_byte)

func NewRAM(o_size uint32, w_size uint32, z uint32, o_byte []byte, w_byte []byte, s uint32) *RAM {
	// o - read-only
	ro_data_address := uint32(Z_Z)
	ro_data_address_end := ro_data_address + P_func(o_size)
	//fmt.Printf("o_size: %d copied to %d up to %d\n", o_size, ro_data_address, ro_data_address_end)

	// w - read-write
	rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
	rw_data_address_end := rw_data_address + P_func(w_size)
	current_heap_pointer := rw_data_address_end
	heap_end := rw_data_address + Z_func(current_heap_pointer)
	//fmt.Printf("w_size: %d copied to %d up to %d\n", w_size, rw_data_address, rw_data_address_end)

	//fmt.Printf("current_heap_pointer: %d (dec) %x (hex)\n", current_heap_pointer, current_heap_pointer)

	// s - stack
	p_s := P_func(s)
	stack_address := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I) - uint64(p_s))
	stack_address_end := uint32(uint64(1<<32) - uint64(2*Z_Z) - uint64(Z_I))
	// fmt.Printf("s_size: %d [hex %x] stack at %d [hex %x] up to %d [hex %x]\n", s, s, stack_address, stack_address, stack_address_end, stack_address_end)
	// a - argument outputs
	a_size := uint32(Z_Z + Z_I - 1)
	output_address := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	output_end := uint32(0xFFFFFFFF)

	ram := &RAM{
		// o_bytes are be copied here Z_Z
		ro_data_address:     ro_data_address,
		ro_data_address_end: ro_data_address_end,

		// w_bytes should be copied into here
		rw_data_address:     rw_data_address,
		rw_data_address_end: rw_data_address_end,

		stack_address:     stack_address,
		stack_address_end: stack_address_end,

		current_heap_pointer: current_heap_pointer,
		output_address:       output_address,
		output_end:           output_end,
		register:             make([]uint64, regSize),
		stack:                make([]byte, p_s),
		rw_data:              make([]byte, heap_end-rw_data_address),
		ro_data:              make([]byte, ro_data_address_end-ro_data_address),
		output:               make([]byte, a_size),
	}
	copy(ram.ro_data[:], o_byte[:])
	copy(ram.rw_data[:], w_byte[:])

	log.Trace("pvm", "NewRAM",
		"output_address", fmt.Sprintf("%x", ram.output_address), "output_end", fmt.Sprintf("%x", ram.output_end),
		"stack_address", fmt.Sprintf("%x", ram.stack_address), "stack_end", fmt.Sprintf("%x", ram.stack_address_end),
		"rw_data_address", fmt.Sprintf("%x", ram.rw_data_address), "current_heap_pointer", fmt.Sprintf("%x", Z_func(ram.current_heap_pointer)),
		"ro_data_address", fmt.Sprintf("%x", ram.ro_data_address), "ro_data_end", fmt.Sprintf("%x", ram.ro_data_address_end))

	return ram
}
func (ram *RAM) SetPageAccess(pageIndex int, access byte) {
	// TODO: match the model of below ... but how?
}

func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}
	length := uint32(len(data))
	end := address + length

	switch {
	case address < 65536:
		copy(ram.first64k[address:], data)
		return OOB
	case address >= ram.output_address && end <= ram.output_end:
		offset := address - ram.output_address
		copy(ram.output[offset:], data)
		return OK
	case address >= ram.stack_address && end <= ram.stack_address_end:
		offset := address - ram.stack_address
		copy(ram.stack[offset:], data)
		return OK
	case address >= ram.rw_data_address && end <= Z_func(ram.current_heap_pointer):
		offset := address - ram.rw_data_address
		copy(ram.rw_data[offset:], data)
		return OK
	case address >= ram.ro_data_address && end <= ram.ro_data_address_end:
		offset := address - ram.ro_data_address
		copy(ram.ro_data[offset:], data)
		return OOB
	default:
		log.Error(log.GeneralAuthoring, "WriteRAMBytes Invalid RAM", "address", fmt.Sprintf("%x", address), "end", fmt.Sprintf("%x", end))
		return OOB
	}
}

func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	end := address + length
	if length == 0 {
		return []byte{}, OK
	}
	if address >= ram.output_address && end <= ram.output_end {
		offset := address - ram.output_address
		if offset+length > uint32(len(ram.output)) {
			return nil, OOB
		}
		return ram.output[offset : offset+length], OK
	}
	if address < 65536 {
		if address+length > 65536 {
			return nil, OOB
		}
		return ram.first64k[address : address+length], OOB
	}
	if address >= ram.output_address && end <= ram.output_end {
		offset := address - ram.output_address
		if offset+length > uint32(len(ram.output)) {
			return nil, OOB
		}
		return ram.output[offset : offset+length], OK
	}

	if address >= ram.stack_address && end <= ram.stack_address_end {
		offset := address - ram.stack_address
		if offset+length > uint32(len(ram.stack)) {
			return nil, OOB
		}
		return ram.stack[offset : offset+length], OK
	}

	if address >= ram.rw_data_address && end <= Z_func(ram.current_heap_pointer) {
		//fmt.Printf("ReadRAMBytes: address %x, length %d, current_heap_pointer %x\n", address, length, ram.current_heap_pointer)
		offset := address - ram.rw_data_address
		if offset+length > uint32(len(ram.rw_data)) {
			fmt.Printf("ADDRESS %x rw_data_address %x offset %x\n", address, ram.rw_data_address, offset)
			// panic(555)
			return nil, OOB
		}
		return ram.rw_data[offset : offset+length], OK
	}

	if address >= ram.ro_data_address && end <= ram.ro_data_address_end {
		offset := address - ram.ro_data_address
		if offset+length > uint32(len(ram.ro_data)) {
			return nil, OOB
		}
		return ram.ro_data[offset : offset+length], OK
	}

	return nil, OOB
}

func (ram *RAM) allocatePages(startPage uint32, count uint32) {
	required := (startPage + count) * Z_P
	if uint32(len(ram.rw_data)) < required {
		// Grow rw_data to fit new allocation
		newData := make([]byte, required)
		copy(newData, ram.rw_data)
		ram.rw_data = newData
	}
}

func (ram *RAM) GetCurrentHeapPointer() uint32 {
	return ram.current_heap_pointer

}

func (ram *RAM) SetCurrentHeapPointer(pointer uint32) {

	ram.current_heap_pointer = pointer
	//fmt.Printf("SetCurrentHeapPointer: %x\n", ram.current_heap_pointer)

}

func (ram *RAM) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= len(ram.register) {
		return 0, OOB
	}
	return ram.register[index], OK
}

func (ram *RAM) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= len(ram.register) {
		return OOB
	}
	ram.register[index] = value
	return OK
}

func (ram *RAM) ReadRegisters() []uint64 {
	return ram.register
}
