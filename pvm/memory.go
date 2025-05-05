package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

const (
	PageSize     = 4096
	AddressSpace = 1 << 32
	TotalPages   = AddressSpace / PageSize
)

type AccessMode struct {
	Inaccessible bool `json:"inaccessible"`
	Writable     bool `json:"writable"`
	Readable     bool `json:"readable"`
}

type Page struct {
	Value  []byte     `json:"data"`
	Access AccessMode `json:"access"`
}

func (p *Page) ensureData() {
	if p.Value == nil {
		p.Value = make([]byte, PageSize)
	}
}

type RAM struct {
	Pages []*Page `json:"pages"`
}

func NewRAM() *RAM {
	return &RAM{
		Pages: make([]*Page, TotalPages),
	}
}

func (ram *RAM) getOrAllocatePage(idx uint32) (*Page, error) {
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

func (ram *RAM) SetPageAccess(pageIndex uint32, numPages uint32, mode AccessMode) uint64 {
	for i := pageIndex; i < pageIndex+numPages; i++ {
		page, err := ram.getOrAllocatePage(i)
		if err != nil {
			return OOB
		}
		page.Access = mode
	}
	return OK
}

func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	offset := address % PageSize
	remaining := uint32(len(data))

	for remaining > 0 {
		currentPage := address / PageSize
		pageOffset := offset

		page, err := ram.getOrAllocatePage(currentPage)
		if err != nil {
			return OOB
		}
		if !page.Access.Writable {
			return uint64(address)
		}
		page.ensureData()

		bytesToWrite := PageSize - pageOffset
		if bytesToWrite > remaining {
			bytesToWrite = remaining
		}
		copy(page.Value[pageOffset:pageOffset+bytesToWrite], data[:bytesToWrite])

		address += bytesToWrite
		data = data[bytesToWrite:]
		remaining -= bytesToWrite
		offset = 0
	}

	return OK
}

func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
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
		if page.Access.Inaccessible {
			return nil, uint64(address)
		}

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

func (ram *RAM) Bytes() []byte {
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

func (vm *VM) V1Hash() common.Hash {
	ramBytes := vm.Ram.Bytes()

	regSize := len(vm.register)
	registerBytes := make([]byte, (regSize+1)*8)
	for i, r := range vm.register {
		binary.LittleEndian.PutUint64(registerBytes[i*8:(i+1)*8], r)
	}
	binary.LittleEndian.PutUint64(registerBytes[regSize*8:(regSize+1)*8], uint64(vm.Gas))

	return common.Blake2Hash(append(ramBytes, registerBytes...))
}

func (ram *RAM) DebugStatus() {
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

// import (
// 	"bytes"
// 	"encoding/binary"
// 	"fmt"
// 	"sort"

// 	"github.com/colorfulnotion/jam/common"
// )

// const (
// 	PageSize     = 4096                    // Size of each page (2^12 bytes)
// 	AddressSpace = 4294967296              // Total addressable memory (2^32)
// 	TotalPages   = AddressSpace / PageSize // Total number of pages
// )

// // AccessMode represents the access permissions of a memory page
// type AccessMode struct {
// 	Inaccessible bool `json:"inaccessible"` // Indicates if the page is inaccessible
// 	Writable     bool `json:"writable"`     // Indicates if the page is writable
// 	Readable     bool `json:"readable"`     // Indicates if the page is readable
// 	Accessed     bool `json:"i"`            // Indicates if the page has been accessed and must be imported
// 	Dirty        bool `json:"e"`            // Indicates if the page has been modified and must be exported
// }

// // MemoryPage represents a single memory page
// type Page struct {
// 	Value  []byte     `json:"data"`   // The data stored in the page
// 	Access AccessMode `json:"access"` // The access mode of the page
// }

// // RAM represents the entire memory system
// type RAM struct {
// 	Pages map[uint32]*Page `json:"pages"` // The pages in the RAM
// }

// // V1Hash computes the hash of the VM's state by combining RAM bytes and register values.
// // It copies each 64-bit register into an 8-byte slice (little-endian) and appends the Gas value as the final 8 bytes.
// func (vm *VM) V1Hash() common.Hash {
// 	// Retrieve the RAM bytes.
// 	ramBytes := vm.Ram.Bytes()

// 	// Create a byte slice for registers; allocate space for each register plus an extra 8 bytes for Gas.
// 	regSize := len(vm.register)
// 	registerBytes := make([]byte, (regSize+1)*8)

// 	// copy 13 **little-endian** 64 bit regs registerBytes.
// 	for i, r := range vm.register {
// 		binary.LittleEndian.PutUint64(registerBytes[i*8:(i+1)*8], r)
// 	}

// 	// Append vm.Gas as the last 8 bytes.
// 	binary.LittleEndian.PutUint64(registerBytes[regSize*8:(regSize+1)*8], uint64(vm.Gas))

// 	// NOTE: in v2, we include a richer X + Y context, specifically the D service map

// 	// Return the Blake2b-256 hash of the concatenated RAM bytes and register bytes.
// 	return common.Blake2Hash(append(ramBytes, registerBytes...))
// }

// // Bytes returns a concatenated byte slice of all pages in ascending order of keys.
// // For each page, it encodes the page key into 4 bytes (big-endian) and then appends the page's data.
// func (ram *RAM) Bytes() []byte {
// 	// Collect all page keys.
// 	keys := make([]uint32, 0, len(ram.Pages))
// 	for k := range ram.Pages {
// 		keys = append(keys, k)
// 	}
// 	// Sort keys in ascending order.
// 	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

// 	buf := new(bytes.Buffer)
// 	// For each key, encode the key and then append the corresponding page data.
// 	for _, k := range keys {
// 		// Encode the page key into 4 bytes (big-endian).
// 		keyBytes := make([]byte, 4)
// 		binary.LittleEndian.PutUint32(keyBytes, k)
// 		buf.Write(keyBytes)
// 		buf.Write(ram.Pages[k].Value[:])
// 	}
// 	return buf.Bytes()
// }

// func NewRAM() *RAM {
// 	return &RAM{
// 		Pages: make(map[uint32]*Page),
// 	}
// }

// // Ensure data allocation for a page (lazy allocation)
// func (p *Page) ensureData() {
// 	if p.Value == nil {
// 		p.Value = make([]byte, PageSize)
// 	}
// }

// /*
// func (p *Page) zero() {
// 	p.Value = make([]byte, PageSize)
// 	p.Access = AccessMode{
// 		Inaccessible: false,
// 		Writable:     true,
// 		Readable:     true,
// 	}
// }

// func (p *Page) void() {
// 	p.Value = make([]byte, PageSize)
// 	p.Access = AccessMode{
// 		Inaccessible: true,
// 		Writable:     false,
// 		Readable:     false,
// 	}
// }
// */
// // Get or allocate a specific page
// func (ram *RAM) getOrAllocatePage(pageIndex uint32) (*Page, error) {
// 	if pageIndex >= TotalPages {
// 		return nil, fmt.Errorf("page index %d out of bounds (max %d)", pageIndex, TotalPages-1)
// 	}

// 	// Check if the page already exists
// 	if page, exists := ram.Pages[pageIndex]; exists {
// 		return page, nil
// 	}

// 	// Allocate a new page dynamically
// 	newPage := &Page{
// 		Access: AccessMode{
// 			Inaccessible: true, // Default to inaccessible
// 			Writable:     false,
// 			Readable:     false,
// 		},
// 	}
// 	ram.Pages[pageIndex] = newPage
// 	return newPage, nil
// }

// // Set the access mode for a specific page
// func (ram *RAM) SetPageAccess(pageIndex uint32, Numpages uint32, mode AccessMode) uint64 {
// 	for i := pageIndex; i < pageIndex+Numpages; i++ {
// 		page, err := ram.getOrAllocatePage(i)
// 		if err != nil {
// 			return OOB
// 		}
// 		page.Access = mode
// 	}
// 	return OK
// }

// // WriteRAMBytes writes data to a specific address in RAM
// func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
// 	offset := address % PageSize
// 	remaining := uint32(len(data))

// 	for remaining > 0 {
// 		currentPage := address / PageSize
// 		pageOffset := offset
// 		page, err := ram.getOrAllocatePage(currentPage)
// 		if err != nil {
// 			// log.Debug(debug_pvm, "WriteRAMBytes: Fail to get or allocate page to access address", "currentPage", currentPage, "address", address)
// 			return OOB
// 		}

// 		// Check if the page is writable
// 		if !page.Access.Writable {
// 			// log.Debug(debug_pvm, "Page is not writable", "currentPage", currentPage, "address", address)
// 			return uint64(address)
// 		}
// 		// Ensure data allocation before writing
// 		page.ensureData()

// 		page.Access.Dirty = true

// 		// Calculate how much data can be written to the current page
// 		bytesToWrite := PageSize - pageOffset
// 		if bytesToWrite > remaining {
// 			bytesToWrite = remaining
// 		}
// 		copy(page.Value[pageOffset:pageOffset+bytesToWrite], data[:bytesToWrite])

// 		// Update the address, data slice, and remaining bytes
// 		address += bytesToWrite
// 		data = data[bytesToWrite:]
// 		remaining -= bytesToWrite
// 		offset = 0 // Offset is only used for the first page
// 	}
// 	return OK
// }

// // ReadRAMBytes reads data from a specific address in RAM
// func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
// 	result := make([]byte, 0, length)
// 	offset := address % PageSize
// 	remaining := length

// 	for remaining > 0 {
// 		currentPage := address / PageSize
// 		pageOffset := offset

// 		// Ensure the page exists and is readable
// 		page, err := ram.getOrAllocatePage(currentPage)
// 		if err != nil {
// 			// log.Debug(debug_pvm, "Fail to get or allocate page to access address", "currentPage", currentPage, "address", address)
// 			return nil, OOB
// 		}
// 		if page.Access.Inaccessible {
// 			// log.Debug(debug_pvm, "Page is not readable", "currentPage", currentPage, "address", address, "length", length)
// 			return nil, uint64(address)
// 		}

// 		// Calculate how much data can be read from the current page
// 		bytesToRead := PageSize - pageOffset
// 		if bytesToRead > remaining {
// 			bytesToRead = remaining
// 		}

// 		// Check against the actual slice length to avoid out-of-range errors
// 		pageSize := uint32(len(page.Value))
// 		endPos := pageOffset + bytesToRead
// 		page.Access.Accessed = true

// 		if endPos <= pageSize {
// 			// Entire slice is within valid range
// 			result = append(result, page.Value[pageOffset:endPos]...)
// 		} else {
// 			// Part or all of the required range goes out of bounds
// 			if pageOffset >= pageSize {
// 				// Completely out of range: fill everything with zero
// 				result = append(result, make([]byte, bytesToRead)...)
// 			} else {
// 				// Part of the data is valid, the rest should be zero
// 				validLen := pageSize - pageOffset
// 				result = append(result, page.Value[pageOffset:pageOffset+validLen]...)
// 				result = append(result, make([]byte, bytesToRead-validLen)...)
// 			}
// 		}

// 		// Update the address and remaining bytes
// 		address += bytesToRead
// 		remaining -= bytesToRead
// 		offset = 0 // Offset is only used for the first page
// 	}
// 	return result, OK
// }

// // DebugStatus provides a snapshot of the RAM's state
// func (ram *RAM) DebugStatus() {
// 	fmt.Println("RAM Status:")
// 	for pageIndex, page := range ram.Pages {
// 		fmt.Printf("Page %d: Inaccessible=%v, Writable=%v, Readable=%v, Data Allocated=%v\n",
// 			pageIndex, page.Access.Inaccessible, page.Access.Writable, page.Access.Readable, page.Value != nil)
// 	}
// }
