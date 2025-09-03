package pvm

// #include "pvm.h"
import "C"
import (
	"encoding/binary"
)

func (ram *VM) SetPageAccess(pageIndex int, access byte) {
	// TODO: match the model of below ... but how?
}

func (ram *VM) WriteRAMBytes64(address uint32, data uint64) uint64 {
	// Use C implementation if CGO integration is active
	if ram.cVM != nil {
		return uint64(C.vm_write_ram_bytes_64((*C.VM)(ram.cVM), C.uint32_t(address), C.uint64_t(data)))
	}

	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap region (writable up to heapEnd)
	if address >= ram.rw_data_address && address <= heapEnd-8 {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offEnd := offset + 8; offEnd > uint32(len(ram.rw_data)) {
			return OOB
		}
		binary.LittleEndian.PutUint64(ram.rw_data[offset:offset+8], data)
		return OK
	}

	// Output region (writable)
	if address >= ram.output_address && address <= ram.output_end-8 {
		offset := address - ram.output_address
		binary.LittleEndian.PutUint64(ram.output[offset:offset+8], data)
		return OK
	}

	// Stack region (writable)
	if address >= ram.stack_address && address <= ram.stack_address_end-8 {
		offset := address - ram.stack_address
		binary.LittleEndian.PutUint64(ram.stack[offset:offset+8], data)
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-8 {
		offset := address - ram.ro_data_address
		binary.LittleEndian.PutUint64(ram.ro_data[offset:offset+8], data)
		return OOB
	}

	return OOB
}

func (ram *VM) WriteRAMBytes32(address uint32, data uint32) uint64 {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap region (writable up to heapEnd)
	if address >= ram.rw_data_address && address <= heapEnd-4 {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offEnd := offset + 4; offEnd > uint32(len(ram.rw_data)) {
			return OOB
		}
		binary.LittleEndian.PutUint32(ram.rw_data[offset:offset+4], data)
		return OK
	}

	// Output region (writable)
	if address >= ram.output_address && address <= ram.output_end-4 {
		offset := address - ram.output_address
		binary.LittleEndian.PutUint32(ram.output[offset:offset+4], data)
		return OK
	}

	// Stack region (writable)
	if address >= ram.stack_address && address <= ram.stack_address_end-4 {
		offset := address - ram.stack_address
		binary.LittleEndian.PutUint32(ram.stack[offset:offset+4], data)
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-4 {
		offset := address - ram.ro_data_address
		binary.LittleEndian.PutUint32(ram.ro_data[offset:offset+4], data)
		return OOB
	}

	return OOB
}

func (ram *VM) WriteRAMBytes16(address uint32, data uint16) uint64 {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap region (writable up to heapEnd)
	if address >= ram.rw_data_address && address <= heapEnd-2 {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offEnd := offset + 2; offEnd > uint32(len(ram.rw_data)) {
			return OOB
		}
		binary.LittleEndian.PutUint16(ram.rw_data[offset:offset+2], data)
		return OK
	}

	// Output region (writable)
	if address >= ram.output_address && address <= ram.output_end-2 {
		offset := address - ram.output_address
		binary.LittleEndian.PutUint16(ram.output[offset:offset+2], data)
		return OK
	}

	// Stack region (writable)
	if address >= ram.stack_address && address <= ram.stack_address_end-2 {
		offset := address - ram.stack_address
		binary.LittleEndian.PutUint16(ram.stack[offset:offset+2], data)
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-2 {
		offset := address - ram.ro_data_address
		binary.LittleEndian.PutUint16(ram.ro_data[offset:offset+2], data)
		return OOB
	}

	return OOB
}

func (ram *VM) WriteRAMBytes8(address uint32, data uint8) uint64 {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap region (writable up to heapEnd)
	if address >= ram.rw_data_address && address < heapEnd {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offset >= uint32(len(ram.rw_data)) {
			return OOB
		}
		ram.rw_data[offset] = data
		return OK
	}

	// Output region (writable)
	if address >= ram.output_address && address < ram.output_end {
		offset := address - ram.output_address
		ram.output[offset] = data
		return OK
	}

	// Stack region (writable)
	if address >= ram.stack_address && address < ram.stack_address_end {
		offset := address - ram.stack_address
		ram.stack[offset] = data
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if address >= ram.ro_data_address && address < ram.ro_data_address_end {
		offset := address - ram.ro_data_address
		ram.ro_data[offset] = data
		return OOB
	}

	return OOB
}

func (ram *VM) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}
	length := uint32(len(data))
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap region (writable up to heapEnd)
	if address >= ram.rw_data_address && address <= heapEnd-length {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offEnd := offset + length; offEnd > uint32(len(ram.rw_data)) {
			return OOB
		}
		copy(ram.rw_data[offset:offset+length], data)
		return OK
	}

	// Output region (writable)
	if address >= ram.output_address && address <= ram.output_end-length {
		offset := address - ram.output_address
		copy(ram.output[offset:offset+length], data)
		return OK
	}

	// Stack region (writable)
	if address >= ram.stack_address && address <= ram.stack_address_end-length {
		offset := address - ram.stack_address
		copy(ram.stack[offset:offset+length], data)
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-length {
		offset := address - ram.ro_data_address
		copy(ram.ro_data[offset:offset+length], data)
		return OOB
	}

	return OOB
}

func (ram *VM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap
	if address >= ram.rw_data_address && address <= heapEnd-length {
		offset := address - ram.rw_data_address
		// Range check already validates offset+length <= heap bounds
		return ram.rw_data[offset : offset+length], OK
	}

	// RO data
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-length {
		offset := address - ram.ro_data_address
		// Range check already validates offset+length <= ro bounds
		return ram.ro_data[offset : offset+length], OK
	}
	// Output region
	if address >= ram.output_address && address <= ram.output_end-length {
		offset := address - ram.output_address
		// Range check already validates offset+length <= output bounds
		return ram.output[offset : offset+length], OK
	}

	// Stack
	if address >= ram.stack_address && address <= ram.stack_address_end-length {
		offset := address - ram.stack_address
		// Range check already validates offset+length <= stack bounds
		return ram.stack[offset : offset+length], OK
	}

	return nil, OOB
}

func (ram *VM) ReadRAMBytes8(address uint32) (uint8, uint64) {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap
	if address >= ram.rw_data_address && address < heapEnd {
		offset := address - ram.rw_data_address
		if offset >= uint32(len(ram.rw_data)) {
			return 0, OOB
		}
		return ram.rw_data[offset], OK
	}

	// RO data
	if address >= ram.ro_data_address && address < ram.ro_data_address_end {
		offset := address - ram.ro_data_address
		if offset >= uint32(len(ram.ro_data)) {
			return 0, OOB
		}
		return ram.ro_data[offset], OK
	}

	// Output region
	if address >= ram.output_address && address < ram.output_end {
		offset := address - ram.output_address
		if offset >= uint32(len(ram.output)) {
			return 0, OOB
		}
		return ram.output[offset], OK
	}

	// Stack
	if address >= ram.stack_address && address < ram.stack_address_end {
		offset := address - ram.stack_address
		if offset >= uint32(len(ram.stack)) {
			return 0, OOB
		}
		return ram.stack[offset], OK
	}

	return 0, OOB
}

func (ram *VM) ReadRAMBytes16(address uint32) (uint16, uint64) {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap
	if address >= ram.rw_data_address && address <= heapEnd-2 {
		offset := address - ram.rw_data_address
		return binary.LittleEndian.Uint16(ram.rw_data[offset : offset+2]), OK
	}

	// RO data
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-2 {
		offset := address - ram.ro_data_address
		return binary.LittleEndian.Uint16(ram.ro_data[offset : offset+2]), OK
	}

	// Output region
	if address >= ram.output_address && address <= ram.output_end-2 {
		offset := address - ram.output_address
		return binary.LittleEndian.Uint16(ram.output[offset : offset+2]), OK
	}

	// Stack
	if address >= ram.stack_address && address <= ram.stack_address_end-2 {
		offset := address - ram.stack_address
		return binary.LittleEndian.Uint16(ram.stack[offset : offset+2]), OK
	}

	return 0, OOB
}

func (ram *VM) ReadRAMBytes32(address uint32) (uint32, uint64) {
	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap
	if address >= ram.rw_data_address && address <= heapEnd-4 {
		offset := address - ram.rw_data_address
		return binary.LittleEndian.Uint32(ram.rw_data[offset : offset+4]), OK
	}

	// RO data
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-4 {
		offset := address - ram.ro_data_address
		if offset+4 > uint32(len(ram.ro_data)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint32(ram.ro_data[offset : offset+4]), OK
	}

	// Output region
	if address >= ram.output_address && address <= ram.output_end-4 {
		offset := address - ram.output_address
		if offset+4 > uint32(len(ram.output)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint32(ram.output[offset : offset+4]), OK
	}

	// Stack
	if address >= ram.stack_address && address <= ram.stack_address_end-4 {
		offset := address - ram.stack_address
		if offset+4 > uint32(len(ram.stack)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint32(ram.stack[offset : offset+4]), OK
	}

	return 0, OOB
}

func (ram *VM) ReadRAMBytes64(address uint32) (uint64, uint64) {
	// Use C implementation if CGO integration is active
	if ram.cVM != nil {
		var errCode C.int
		result := C.vm_read_ram_bytes_64((*C.VM)(ram.cVM), C.uint32_t(address), &errCode)
		return uint64(result), uint64(errCode)
	}

	heapEnd := Z_func(ram.current_heap_pointer)

	// RW data / heap
	if address >= ram.rw_data_address && address <= heapEnd-8 {
		offset := address - ram.rw_data_address
		return binary.LittleEndian.Uint64(ram.rw_data[offset : offset+8]), OK
	}

	// RO data
	if address >= ram.ro_data_address && address <= ram.ro_data_address_end-8 {
		offset := address - ram.ro_data_address
		if offset+8 > uint32(len(ram.ro_data)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint64(ram.ro_data[offset : offset+8]), OK
	}

	// Output region
	if address >= ram.output_address && address <= ram.output_end-8 {
		offset := address - ram.output_address
		if offset+8 > uint32(len(ram.output)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint64(ram.output[offset : offset+8]), OK
	}

	// Stack
	if address >= ram.stack_address && address <= ram.stack_address_end-8 {
		offset := address - ram.stack_address
		if offset+8 > uint32(len(ram.stack)) {
			return 0, OOB
		}
		return binary.LittleEndian.Uint64(ram.stack[offset : offset+8]), OK
	}

	return 0, OOB
}

func (ram *VM) allocatePages(startPage uint32, count uint32) {
	required := (startPage + count) * Z_P
	if uint32(len(ram.rw_data)) < required {
		// Grow rw_data to fit new allocation
		newData := make([]byte, required)
		copy(newData, ram.rw_data)
		ram.rw_data = newData
	}
}

func (ram *VM) GetCurrentHeapPointer() uint32 {
	return ram.current_heap_pointer
}

func (ram *VM) SetCurrentHeapPointer(pointer uint32) {
	ram.current_heap_pointer = pointer
}
