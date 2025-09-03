package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	AddressSpace = 1 << 32
	PageSize     = 4096
	TotalPages   = AddressSpace / PageSize
)

type RAMInterface interface {
	WriteRAMBytes(address uint32, data []byte) uint64
	ReadRAMBytes(address uint32, length uint32) ([]byte, uint64)
	allocatePages(startPage uint32, count uint32)
	GetCurrentHeapPointer() uint32
	SetCurrentHeapPointer(pointer uint32)
}

type AccessMode struct {
	Inaccessible bool `json:"inaccessible"`
	Writable     bool `json:"writable"`
	Readable     bool `json:"readable"`
}

type Page struct {
	Value  []byte     `json:"data"`
	Access AccessMode `json:"access"`
	Dirty  bool       `json:"dirty"`
}

func (p *Page) ensureData() {
	if p.Value == nil {
		p.Value = make([]byte, PageSize)
	}
}

type RawRAM struct {
	current_heap_pointer uint32
	register             []uint64

	Pages []*Page `json:"pages"`
}

func NewRawRAM() *RawRAM {
	ram := &RawRAM{
		Pages:                make([]*Page, TotalPages),
		register:             make([]uint64, regSize),
		current_heap_pointer: 51 * 4096,
	}
	for i := uint32(0); i < 51; i++ {
		ram.getOrAllocatePage(i)
	}
	return ram
}

func (ram *RawRAM) GetDirtyPages() []int {
	pages := make([]int, 0)
	for idx, page := range ram.Pages {
		if page != nil && page.Dirty {
			pages = append(pages, idx)
		}
	}
	return pages
}

func (ram *RawRAM) GetCurrentHeapPointer() uint32 {
	return ram.current_heap_pointer
}

func (ram *RawRAM) SetCurrentHeapPointer(pointer uint32) {
	ram.current_heap_pointer = pointer
	//fmt.Printf("SetCurrentHeapPointer: %x\n", ram.current_heap_pointer)

}

func (ram *RawRAM) allocatePages(startPage uint32, count uint32) {
	for i := uint32(0); i < count; i++ {
		ram.getOrAllocatePage(startPage + i)
	}
}

func (ram *RawRAM) getOrAllocatePage(idx uint32) (*Page, error) {
	if idx >= TotalPages {
		return nil, fmt.Errorf("page index %d out of bounds (max %d)", idx, TotalPages-1)
	}
	p := ram.Pages[idx]
	if p == nil {
		p = &Page{
			Access: AccessMode{Inaccessible: true},
		}
		ram.Pages[idx] = p
	}
	return p, nil
}

func (ram *RawRAM) SetPageAccess(pageIndex uint32, numPages uint32, mode AccessMode) uint64 {
	for i := pageIndex; i < pageIndex+numPages; i++ {
		page, err := ram.getOrAllocatePage(i)
		if err != nil {
			return OOB
		}
		page.Access = mode
	}
	return OK
}

func (ram *RawRAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	offset := address % PageSize
	remaining := uint32(len(data))

	for remaining > 0 {
		currentPage := address / PageSize
		pageOffset := offset

		page, err := ram.getOrAllocatePage(currentPage)
		if err != nil {
			return OOB
		}
		// if !page.Access.Writable {
		// 	fmt.Printf("WriteRAMBytes: page %d is not writable @ addresss %x\n", currentPage, address)
		// 	panic(555)
		// 	return uint64(address)
		// }
		page.ensureData()

		bytesToWrite := PageSize - pageOffset
		if bytesToWrite > remaining {
			bytesToWrite = remaining
		}
		copy(page.Value[pageOffset:pageOffset+bytesToWrite], data[:bytesToWrite])
		page.Dirty = true
		address += bytesToWrite
		data = data[bytesToWrite:]
		remaining -= bytesToWrite
		offset = 0
	}

	return OK
}

func (ram *RawRAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	result := make([]byte, 0, length)
	offset := address % PageSize
	remaining := length

	for remaining > 0 {
		currentPage := address / PageSize
		pageOffset := offset

		page, err := ram.getOrAllocatePage(currentPage)
		if err != nil {

			return nil, OOB
		}
		// if page.Access.Inaccessible {
		// 	panic(999)
		// 	return nil, uint64(address)
		// }

		bytesToRead := PageSize - pageOffset
		if bytesToRead > remaining {
			bytesToRead = remaining
		}

		pageSize := uint32(len(page.Value))
		endPos := pageOffset + bytesToRead

		if endPos <= pageSize {
			result = append(result, page.Value[pageOffset:endPos]...)
		} else {
			if pageOffset < pageSize {
				result = append(result, page.Value[pageOffset:pageSize]...)
			}
			result = append(result, make([]byte, bytesToRead-(pageSize-pageOffset))...)
		}

		address += bytesToRead
		remaining -= bytesToRead
		offset = 0
	}

	return result, OK
}

func (ram *RawRAM) Bytes() []byte {
	buf := new(bytes.Buffer)
	for idx, page := range ram.Pages {
		if page == nil {
			continue
		}
		keyBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(keyBytes, uint32(idx))
		buf.Write(keyBytes)
		buf.Write(page.Value)
	}
	return buf.Bytes()
}

func (ram *RawRAM) DebugStatus() {
	fmt.Println("RAM Status:")
	for idx, page := range ram.Pages {
		if page == nil {
			fmt.Printf("Page %d: <nil>\n", idx)
			continue
		}
		fmt.Printf(
			"Page %d: Inaccessible=%v, Writable=%v, Readable=%v, DataAllocated=%v\n",
			idx,
			page.Access.Inaccessible,
			page.Access.Writable,
			page.Access.Readable,
			page.Value != nil,
		)
	}
}
