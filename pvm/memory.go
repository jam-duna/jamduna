package pvm

import (
	"slices"
	"time"
)

type RAMInterface interface {
	WriteRAMBytes(address uint32, data []byte) uint64
	ReadRAMBytes(address uint32, length uint32) ([]byte, uint64)
	allocatePages(startPage uint32, count uint32)
	GetCurrentHeapPointer() uint32
	SetCurrentHeapPointer(pointer uint32)
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
}

func (ram *RAM) GetDirtyPages() []int {
	panic("GetDirtyPages not implemented for RAM")
}
func NewRAM(o_size uint32, w_size uint32, z uint32, o_byte []byte, w_byte []byte, s uint32) *RAM {
	// Z_Z, Z_I
	z_z := uint32(Z_Z)
	z_i := uint32(Z_I)

	// o - read-only
	ro_data_address := z_z
	p_o_size := P_func(o_size)
	ro_data_address_end := ro_data_address + p_o_size

	// w - read-write
	z_o_size := Z_func(o_size)
	rw_data_address := 2*z_z + z_o_size
	p_w_size := P_func(w_size)
	rw_data_address_end := rw_data_address + p_w_size
	current_heap_pointer := rw_data_address_end
	heap_end := rw_data_address + Z_func(current_heap_pointer)

	// s - stack
	p_s := P_func(s)
	max_uint32 := uint32(0xFFFFFFFF)
	stack_offset := 2*z_z + z_i + p_s
	stack_address := max_uint32 + 1 - stack_offset
	stack_address_end := max_uint32 + 1 - 2*z_z - z_i

	// a - argument outputs
	a_size := z_z + z_i - 1
	output_address := max_uint32 - z_z - z_i + 1

	// ro + rw slice lengths
	ro_len := ro_data_address_end - ro_data_address
	rw_len := heap_end - rw_data_address

	// Ensure input slices don't exceed calculated lengths
	o_copy_len := min(len(o_byte), int(ro_len))
	w_copy_len := min(len(w_byte), int(rw_len))

	ram := &RAM{
		ro_data_address:      ro_data_address,
		ro_data_address_end:  ro_data_address_end,
		rw_data_address:      rw_data_address,
		rw_data_address_end:  rw_data_address_end,
		stack_address:        stack_address,
		stack_address_end:    stack_address_end,
		current_heap_pointer: current_heap_pointer,
		output_address:       output_address,
		output_end:           max_uint32,
		stack:                nil,
		rw_data:              nil,
		ro_data:              nil,
		output:               nil,
	}

	t0 := time.Now()
	ro_data := make([]byte, ro_len)
	// Use slices.Clone to avoid copy when possible, otherwise copy the subset
	if len(o_byte) <= int(ro_len) {
		ro_data = slices.Clone(o_byte)
		// Resize if needed
		if len(ro_data) < int(ro_len) {
			ro_data = slices.Grow(ro_data, int(ro_len)-len(ro_data))[:ro_len]
		}
	} else {
		copy(ro_data, o_byte[:o_copy_len])
	}
	ram.ro_data = ro_data
	benchRec.Add("- NewRAM:ro_data", time.Since(t0))

	t0 = time.Now()
	rw_data := make([]byte, rw_len)
	if len(w_byte) <= int(rw_len) {
		rw_data = slices.Clone(w_byte)
		// Resize if needed
		if len(rw_data) < int(rw_len) {
			rw_data = slices.Grow(rw_data, int(rw_len)-len(rw_data))[:rw_len]
		}
	} else {
		copy(rw_data, w_byte[:w_copy_len])
	}
	ram.rw_data = rw_data
	benchRec.Add("- NewRAM:rw_data", time.Since(t0))

	t0 = time.Now()
	ram.stack = make([]byte, p_s)
	benchRec.Add("- NewRAM:stack", time.Since(t0))

	t0 = time.Now()
	ram.output = make([]byte, a_size)
	benchRec.Add("- NewRAM:output", time.Since(t0))

	return ram
}
func (ram *RAM) SetPageAccess(pageIndex int, access byte) {
	// TODO: match the model of below ... but how?
}

// inRange reports whether the half-open interval [address, address+length) lies fully within [start, end).  It is crafted to avoid uint32 overflow.

func inRange(address, length, start, end uint32) bool {
	if length == 0 {
		return true
	}
	// Must start inside region.
	if address < start || address >= end {
		return false
	}
	// address + length <= end  ⇔  address <= end - length
	// (works even when length is large; avoids overflow)
	return address <= end-length
}

func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	if len(data) == 0 {
		return OK
	}
	length := uint32(len(data))
	heapEnd := Z_func(ram.current_heap_pointer)

	// First 64K (special, write is OOB)
	if inRange(address, length, 0, 65536) {
		// Preserve existing semantics: writes to first64k return OOB.
		copy(ram.first64k[address:], data)
		return OOB
	}

	// Output region (writable)
	if inRange(address, length, ram.output_address, ram.output_end) {
		offset := address - ram.output_address
		copy(ram.output[offset:offset+length], data)
		return OK
	}

	// Stack region (writable)
	if inRange(address, length, ram.stack_address, ram.stack_address_end) {
		offset := address - ram.stack_address
		copy(ram.stack[offset:offset+length], data)
		return OK
	}

	// RW data / heap region (writable up to heapEnd)
	if inRange(address, length, ram.rw_data_address, heapEnd) {
		offset := address - ram.rw_data_address
		// Safety guard for dynamic growth; usually elided by compiler
		if offEnd := offset + length; offEnd > uint32(len(ram.rw_data)) {
			return OOB
		}
		copy(ram.rw_data[offset:offset+length], data)
		return OK
	}

	// RO data region (writes are OOB but we mimic prior behavior of copying then OOB)
	if inRange(address, length, ram.ro_data_address, ram.ro_data_address_end) {
		offset := address - ram.ro_data_address
		copy(ram.ro_data[offset:offset+length], data)
		return OOB
	}

	return OOB
}

func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	if length == 0 {
		return []byte{}, OK
	}
	heapEnd := Z_func(ram.current_heap_pointer)

	// Output region
	if inRange(address, length, ram.output_address, ram.output_end) {
		offset := address - ram.output_address
		if offset+length > uint32(len(ram.output)) {
			return nil, OOB
		}
		return ram.output[offset : offset+length], OK
	}

	// First 64K (special; read returns OOB per existing semantics)
	if inRange(address, length, 0, 65536) {
		return ram.first64k[address : address+length], OOB
	}

	// Stack
	if inRange(address, length, ram.stack_address, ram.stack_address_end) {
		offset := address - ram.stack_address
		if offset+length > uint32(len(ram.stack)) {
			return nil, OOB
		}
		return ram.stack[offset : offset+length], OK
	}

	// RW data / heap
	if inRange(address, length, ram.rw_data_address, heapEnd) {
		offset := address - ram.rw_data_address
		if offset+length > uint32(len(ram.rw_data)) {
			return nil, OOB
		}
		return ram.rw_data[offset : offset+length], OK
	}

	// RO data
	if inRange(address, length, ram.ro_data_address, ram.ro_data_address_end) {
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
