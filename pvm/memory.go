package pvm

import (
	"fmt"

	"github.com/colorfulnotion/jam/log"
)

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

	stack   []byte
	rw_data []byte
	ro_data []byte
	output  []byte
}

func NewRAM(o_size uint32, w_size uint32, p_s uint32) *RAM {
	// read-only
	ro_data_address := uint32(Z_Z)
	ro_data_address_end := ro_data_address + o_size

	// read-write
	rw_data_address := uint32(2 * Z_Z)
	rw_data_address_end := rw_data_address + Z_func(o_size)
	current_heap_pointer := rw_data_address_end + 1024*1024

	// stack
	stack_address := uint32(0xFFFFFFFF) - 2*Z_Z - Z_I - p_s + 1
	stack_address_end := stack_address + p_s

	// output
	a_size := uint32(Z_Z + Z_I - 1)
	output_address := uint32(0xFFFFFFFF) - Z_Z - Z_I + 1
	output_end := uint32(0xFFFFFFFF)

	ram := &RAM{
		stack_address:        stack_address,
		stack_address_end:    stack_address_end,
		rw_data_address:      rw_data_address,
		rw_data_address_end:  rw_data_address_end,
		ro_data_address:      ro_data_address,
		ro_data_address_end:  ro_data_address_end,
		current_heap_pointer: current_heap_pointer,
		output_address:       output_address,
		output_end:           output_end,
		stack:                make([]byte, p_s),
		rw_data:              make([]byte, current_heap_pointer-rw_data_address),
		ro_data:              make([]byte, ro_data_address_end-ro_data_address),
		output:               make([]byte, a_size),
	}
	log.Trace("pvm", "NewRAM", 
		"output_address", fmt.Sprintf("%x", ram.output_address), "output_end", fmt.Sprintf("%x", ram.output_end),
		"stack_address", fmt.Sprintf("%x", ram.stack_address), "stack_end", fmt.Sprintf("%x", ram.stack_address_end),
		"rw_data_address", fmt.Sprintf("%x", ram.rw_data_address), "current_heap_pointer", fmt.Sprintf("%x", Z_func(ram.current_heap_pointer)),
		"ro_data_address", fmt.Sprintf("%x", ram.ro_data_address), "ro_data_end", fmt.Sprintf("%x", ram.ro_data_address_end))

	return ram
}


func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	length := uint32(len(data))
	end := address + length

	switch {
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
		return OK
	default:
		return OOB
	}
}

func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	end := address + length

	if address >= ram.output_address && end <= ram.output_end {
		offset := address - ram.output_address
		if offset+length > uint32(len(ram.output)) {
			panic(322)
			return nil, OOB
		}
		return ram.output[offset : offset+length], OK
	}

	if address >= ram.stack_address && end <= ram.stack_address_end {
		offset := address - ram.stack_address
		if offset+length > uint32(len(ram.stack)) {
			panic(324)
			return nil, OOB
		}
		return ram.stack[offset : offset+length], OK
	}

	if address >= ram.rw_data_address && end <= Z_func(ram.current_heap_pointer) {
		offset := address - ram.rw_data_address
		if offset+length > uint32(len(ram.rw_data)) {
			fmt.Printf("ADDRESS %x rw_data_address %x offset %x\n", address, ram.rw_data_address, offset)
			panic(326)
			return nil, OOB
		}
		return ram.rw_data[offset : offset+length], OK
	}

	if address >= ram.ro_data_address && end <= ram.ro_data_address_end {
		offset := address - ram.ro_data_address
		if offset+length > uint32(len(ram.ro_data)) {
			panic(328)
			return nil, OOB
		}
		return ram.ro_data[offset : offset+length], OK
	}

	log.Trace("pvm", "invalid ReadRAMBytes", "addr", fmt.Sprintf("%x", address), "end", end, "l", length,
		"output_address", fmt.Sprintf("%x", ram.output_address), "output_end", fmt.Sprintf("%x", ram.output_end),
		"stack_address", fmt.Sprintf("%x", ram.stack_address), "stack_end", fmt.Sprintf("%x", ram.stack_address_end),
		"rw_data_address", fmt.Sprintf("%x", ram.rw_data_address), "current_heap_pointer", fmt.Sprintf("%x", Z_func(ram.current_heap_pointer)),
		"ro_data_address", fmt.Sprintf("%x", ram.ro_data_address), "ro_data_end", fmt.Sprintf("%x", ram.ro_data_address_end))

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
