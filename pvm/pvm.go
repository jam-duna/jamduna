package pvm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"

	//	"reflect"

	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug_pvm      = false
	debug_host     = false
	ram_permission = true

	regSize  = 13
	numCores = types.TotalCores
	W_E      = types.W_E
	W_S      = types.W_S
	W_X      = 1024
	M        = 128
	V        = 1023
	D        = 28800
	Z_A      = 2
	Z_P      = (1 << 12)
	Z_Q      = (1 << 16)
	Z_I      = (1 << 24)
	Z_Z      = (1 << 16)
)

const (
	PageSize     = 4096                    // Size of each page (2^12 bytes)
	AddressSpace = 4294967296              // Total addressable memory (2^32)
	TotalPages   = AddressSpace / PageSize // Total number of pages
)

// AccessMode represents the access permissions of a memory page
type AccessMode struct {
	Inaccessible bool `json:"inaccessible"` // Indicates if the page is inaccessible
	Writable     bool `json:"writable"`     // Indicates if the page is writable
	Readable     bool `json:"readable"`     // Indicates if the page is readable
}

// MemoryPage represents a single memory page
type Page struct {
	Value  []byte     `json:"data"`   // The data stored in the page
	Access AccessMode `json:"access"` // The access mode of the page
}

// RAM represents the entire memory system
type RAM struct {
	Pages map[uint32]*Page `json:"pages"` // The pages in the RAM
}

func NewRAM() *RAM {
	return &RAM{
		Pages: make(map[uint32]*Page),
	}
}

// Ensure data allocation for a page (lazy allocation)
func (p *Page) ensureData() {
	if p.Value == nil {
		p.Value = make([]byte, PageSize)
	}
}

// Get or allocate a specific page
func (ram *RAM) getOrAllocatePage(pageIndex uint32) (*Page, error) {
	if pageIndex >= TotalPages {
		return nil, fmt.Errorf("page index %d out of bounds (max %d)", pageIndex, TotalPages-1)
	}

	// Check if the page already exists
	if page, exists := ram.Pages[pageIndex]; exists {
		return page, nil
	}

	// Allocate a new page dynamically
	newPage := &Page{
		Access: AccessMode{
			Inaccessible: true, // Default to inaccessible
			Writable:     false,
			Readable:     false,
		},
	}
	ram.Pages[pageIndex] = newPage
	return newPage, nil
}

// Set the access mode for a specific page
func (ram *RAM) SetPageAccess(pageIndex uint32, Numpages uint32, mode AccessMode) uint64 {
	for i := pageIndex; i < pageIndex+Numpages; i++ {
		page, err := ram.getOrAllocatePage(i)
		if err != nil {
			return OOB
		}
		page.Access = mode
	}
	return OK
}

// WriteRAMBytes writes data to a specific address in RAM
func (ram *RAM) WriteRAMBytes(address uint32, data []byte) uint64 {
	offset := address % PageSize
	remaining := uint32(len(data))

	for remaining > 0 {
		currentPage := address / PageSize
		pageOffset := offset
		page, err := ram.getOrAllocatePage(currentPage)
		if err != nil {
			if debug_pvm {
				fmt.Printf("Fail to get or allocate page %d to access address %d\n", currentPage, address)
			}
			return OOB
		}

		// Check if the page is writable
		if page.Access.Inaccessible || !page.Access.Writable {
			if debug_pvm {
				fmt.Printf("Page %d is not writable, addess is %d\n", currentPage, address)
			}
			return OOB
		}

		// Ensure data allocation before writing
		page.ensureData()

		// Calculate how much data can be written to the current page
		bytesToWrite := PageSize - pageOffset
		if bytesToWrite > remaining {
			bytesToWrite = remaining
		}
		copy(page.Value[pageOffset:pageOffset+bytesToWrite], data[:bytesToWrite])

		// Update the address, data slice, and remaining bytes
		address += bytesToWrite
		data = data[bytesToWrite:]
		remaining -= bytesToWrite
		offset = 0 // Offset is only used for the first page
	}
	return OK
}

// ReadRAMBytes reads data from a specific address in RAM
func (ram *RAM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) {
	result := make([]byte, 0, length)
	offset := address % PageSize
	remaining := length

	for remaining > 0 {
		currentPage := address / PageSize
		pageOffset := offset

		// Ensure the page exists and is readable
		page, err := ram.getOrAllocatePage(currentPage)
		if err != nil {
			if debug_pvm {
				fmt.Printf("Fail to get or allocate page %d to access address %d\n", currentPage, address)
			}
			return nil, OOB
		}
		if page.Access.Inaccessible || !page.Access.Readable {
			if debug_pvm {
				fmt.Printf("Page %d is not readable, addess is %d, length is %d\n", currentPage, address, length)
			}
			return nil, OOB
		}

		// Calculate how much data can be read from the current page
		bytesToRead := PageSize - pageOffset
		if bytesToRead > remaining {
			bytesToRead = remaining
		}

		// Append data to the result
		result = append(result, page.Value[pageOffset:pageOffset+bytesToRead]...)

		// Update the address and remaining bytes
		address += bytesToRead
		remaining -= bytesToRead
		offset = 0 // Offset is only used for the first page
	}
	return result, OK
}

// DebugStatus provides a snapshot of the RAM's state
func (ram *RAM) DebugStatus() {
	fmt.Println("RAM Status:")
	for pageIndex, page := range ram.Pages {
		fmt.Printf("Page %d: Inaccessible=%v, Writable=%v, Readable=%v, Data Allocated=%v\n",
			pageIndex, page.Access.Inaccessible, page.Access.Writable, page.Access.Readable, page.Value != nil)
	}
}

type VM struct {
	JSize        uint64
	Z            uint8
	J            []uint32
	code         []byte
	bitmask      string // K in GP
	pc           uint32 // Program counter
	resultCode   uint8
	terminated   bool
	hostCall     bool // ̵h in GP
	host_func_id int  // h in GP
	// ram                 map[uint32][4096]byte
	Ram                 *RAM
	register            []uint64
	Gas                 int64
	hostenv             types.HostEnv
	writable_ram_start  uint32
	writable_ram_length uint32

	VMs map[uint32]*VM

	// Work Package Inputs
	Extrinsics [][]byte
	payload    []byte
	Imports    [][]byte

	// Invocation funtions entry point
	EntryPoint uint32

	// Malicious Setting
	IsMalicious bool

	// standard program initialization parameters
	o_size uint32
	w_size uint32
	z      uint32
	s      uint32
	o_byte []byte
	w_byte []byte

	// Refine argument
	RefineM_map        RefineM_map
	Exports            [][]byte
	ExportSegmentIndex uint32

	// Accumulate argument
	X        *types.XContext
	Y        types.XContext
	Timeslot uint32

	// Gereral argument
	ServiceAccount *types.ServiceAccount
	Service_index  uint32
	Delta          map[uint32]*types.ServiceAccount
}

type Forgets struct {
}

type Solicit struct {
	BlobHash common.Hash
	Length   uint32
}

type Program struct {
	JSize uint64
	Z     uint8 // 1 byte
	CSize uint64
	J     []uint32
	Code  []byte
	//K     []byte
	K []string
}

// Appendix A - Instuctions

// A.5.1. Instructions without Arguments.
const (
	TRAP        = 0
	FALLTHROUGH = 1
)

// A.5.2. Instructions with Arguments of One Immediate.
const (
	ECALLI = 10 // 0x0a
)

// A.5.3. Instructions with Arguments of One Register and One Extended Width Immediate.
const (
	LOAD_IMM_64 = 20 // 0x14
)

// A.5.4. Instructions with Arguments of Two Immediates.
const (
	STORE_IMM_U8  = 30
	STORE_IMM_U16 = 31
	STORE_IMM_U32 = 32
	STORE_IMM_U64 = 33 // NEW, 32-bit twin = store_imm_u32
)

// A.5.5. Instructions with Arguments of One Offset.
const (
	JUMP = 40 // 0x28
)

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
const (
	JUMP_IND  = 50 // 0x32
	LOAD_IMM  = 51 // 0x33 NOTE: 64-bit twin = load_imm_64
	LOAD_U8   = 52
	LOAD_I8   = 53 // NEW
	LOAD_U16  = 54
	LOAD_I16  = 55 // NEW
	LOAD_U32  = 56
	LOAD_I32  = 57 // NEW
	LOAD_U64  = 58
	STORE_U8  = 59
	STORE_U16 = 60
	STORE_U32 = 61
	STORE_U64 = 62 // NEW, 32-bit twin = store_u32
)

// A.5.7. Instructions with Arguments of One Register & Two Immediates.
const (
	STORE_IMM_IND_U8  = 70 // 0x46
	STORE_IMM_IND_U16 = 71 // 0x47
	STORE_IMM_IND_U32 = 72 // 0x48
	STORE_IMM_IND_U64 = 73 // 0x49 NEW, 32-bit twin = store_imm_ind_u32
)

// A.5.8. Instructions with Arguments of One Register, One Immediate and One Offset.
const (
	LOAD_IMM_JUMP   = 80
	BRANCH_EQ_IMM   = 81 // 0x51
	BRANCH_NE_IMM   = 82
	BRANCH_LT_U_IMM = 83
	BRANCH_LE_U_IMM = 84
	BRANCH_GE_U_IMM = 85
	BRANCH_GT_U_IMM = 86
	BRANCH_LT_S_IMM = 87
	BRANCH_LE_S_IMM = 88
	BRANCH_GE_S_IMM = 89
	BRANCH_GT_S_IMM = 90
)

// A.5.9. Instructions with Arguments of Two Registers.
const (
	MOVE_REG = 100 // 0x64
	SBRK     = 101
)

// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
const (
	STORE_IND_U8      = 110 // 0x6e
	STORE_IND_U16     = 111 // 0x6f
	STORE_IND_U32     = 112 // 0x70
	STORE_IND_U64     = 113 // 0x71 NEW, 32-bit twin = store_ind_u32
	LOAD_IND_U8       = 114
	LOAD_IND_I8       = 115
	LOAD_IND_U16      = 116
	LOAD_IND_I16      = 117 // 0x75
	LOAD_IND_U32      = 118 // 0x76
	LOAD_IND_I32      = 119 // 0x77 NEW, no twin.
	LOAD_IND_U64      = 120 // 0x78 NEW, 32-bit twin = load_ind_u32
	ADD_IMM_32        = 121 // 0x79
	AND_IMM           = 122
	XOR_IMM           = 123
	OR_IMM            = 124
	MUL_IMM_32        = 125
	MUL_IMM           = 125
	SET_LT_U_IMM      = 126
	SET_LT_S_IMM      = 127
	SHLO_L_IMM_32     = 128
	SHLO_R_IMM_32     = 129
	SHAR_R_IMM_32     = 130
	NEG_ADD_IMM_32    = 131
	SET_GT_U_IMM      = 132
	SET_GT_S_IMM      = 133
	SHLO_L_IMM_ALT_32 = 134
	SHLO_R_IMM_ALT_32 = 135
	SHAR_R_IMM_ALT_32 = 136
	CMOV_IZ_IMM       = 137
	CMOV_NZ_IMM       = 138
	ADD_IMM_64        = 139 // 0x8b NEW, 32-bit twin = add_imm_32
	MUL_IMM_64        = 140 // 0x8c NEW, 32-bit twin = mul_imm_32
	SHLO_L_IMM_64     = 141 // 0x8d NEW, 32-bit twin = shlo_l_imm_32
	SHLO_R_IMM_64     = 142 // 0x8e NEW, 32-bit twin = shlo_r_imm_32
	SHAR_R_IMM_64     = 143 // 0x8f NEW, 32-bit twin = shar_r_imm_32
	NEG_ADD_IMM_64    = 144 // 0x90 NEW, 32-bit twin = neg_add_imm_32
	SHLO_L_IMM_ALT_64 = 145 // 0x91 NEW, 32-bit twin = shlo_l_imm_alt_32
	SHLO_R_IMM_ALT_64 = 146 // 0x92 NEW, 32-bit twin = shlo_r_imm_alt_32
	SHAR_R_IMM_ALT_64 = 147 // 0x93 NEW, 32-bit twin = shar_r_imm_alt_32
)

// A.5.11. Instructions with Arguments of Two Registers & One Offset.
const (
	BRANCH_EQ   = 150
	BRANCH_NE   = 151
	BRANCH_LT_U = 152
	BRANCH_LT_S = 153
	BRANCH_GE_U = 154
	BRANCH_GE_S = 155
)

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
const (
	LOAD_IMM_JUMP_IND = 160
)

// A.5.13. Instructions with Arguments of Three Registers.
const (
	ADD_32        = 170 // 0xaa
	SUB_32        = 171
	MUL_32        = 172
	DIV_U_32      = 173
	DIV_S_32      = 174
	REM_U_32      = 175
	REM_S_32      = 176
	SHLO_L_32     = 177
	SHLO_R_32     = 178
	SHAR_R_32     = 179
	ADD_64        = 180 // 0xb4 NEW, 32-bit twin = ADD_32
	SUB_64        = 181 // NEW, 32-bit twin = SUB_32
	MUL_64        = 182 // NEW, 32-bit twin = MUL_32
	DIV_U_64      = 183 // NEW, 32-bit twin = DIV_U_32
	DIV_S_64      = 184 // NEW, 32-bit twin = DIV_S_32
	REM_U_64      = 185 // NEW, 32-bit twin = REM_U_32
	REM_S_64      = 186 // NEW, 32-bit twin = REM_S_32
	SHLO_L_64     = 187 // NEW, 32-bit twin =  SHLO_L_32
	SHLO_R_64     = 188 // NEW, 32-bit twin = SHLO_R_32
	SHAR_R_64     = 189 // NEW, 32-bit twin =  SHAR_R_32
	AND           = 190
	XOR           = 191 // 0xbf
	OR            = 192
	MUL_UPPER_S_S = 193
	MUL_UPPER_U_U = 194
	MUL_UPPER_S_U = 195
	SET_LT_U      = 196
	SET_LT_S      = 197
	CMOV_IZ       = 198
	CMOV_NZ       = 199
)

// Termination Instructions
var T = map[int]struct{}{
	TRAP:            {},
	FALLTHROUGH:     {},
	JUMP:            {},
	JUMP_IND:        {},
	LOAD_IMM_JUMP:   {},
	BRANCH_EQ_IMM:   {},
	BRANCH_NE_IMM:   {},
	BRANCH_LT_U_IMM: {},
	BRANCH_LE_U_IMM: {},
	BRANCH_GE_U_IMM: {},
	BRANCH_GT_U_IMM: {},
	BRANCH_LT_S_IMM: {},
	BRANCH_LE_S_IMM: {},
	BRANCH_GE_S_IMM: {},
	BRANCH_GT_S_IMM: {},
	BRANCH_EQ:       {},
	BRANCH_NE:       {},
	BRANCH_LT_U:     {},
	BRANCH_LT_S:     {},
	BRANCH_GE_U:     {},
	BRANCH_GE_S:     {},
}

const (
	NONE = (1 << 64) - 1  // 2^32 - 1
	OOB  = (1 << 64) - 2  // 2^32 - 2
	WHO  = (1 << 64) - 3  // 2^32 - 3
	FULL = (1 << 64) - 4  // 2^32 - 4
	CORE = (1 << 64) - 5  // 2^32 - 5
	CASH = (1 << 64) - 6  // 2^32 - 6
	LOW  = (1 << 64) - 7  // 2^32 - 7
	HIGH = (1 << 64) - 8  // 2^32 - 8
	WAT  = (1 << 64) - 9  // 2^32 - 9
	HUH  = (1 << 64) - 10 // 2^32 - 10
	OK   = 0              // 0

	HALT  = 0              // 0
	PANIC = (1 << 64) - 12 // 2^32 - 12
	FAULT = (1 << 64) - 13 // 2^32 - 13
	HOST  = (1 << 64) - 14 // 2^32 - 14

// BAD = 1111
// S   = 2222
// BIG = 3333
)

func extractBytes(input []byte) ([]byte, []byte) {
	/*
		In GP_0.36 (272):
		If the input value of (272) is large, "l" will also increase and vice versa.
		"l" is than be used to encode first byte and the reaming "l" bytes.
		If the first byte is large, that means the number of the entire encoded bytes is large and vice versa.
		So the first byte can be used to determine the number of bytes to extract and the rule is as follows:
	*/

	if len(input) == 0 {
		return nil, input
	}

	firstByte := input[0]
	var numBytes int

	// Determine the number of bytes to extract based on the value of the 0th byte.
	switch {
	case firstByte >= 1 && firstByte < 128:
		numBytes = 1
	case firstByte >= 128 && firstByte < 192:
		numBytes = 2
	case firstByte >= 192 && firstByte < 224:
		numBytes = 3
	case firstByte >= 224 && firstByte < 240:
		numBytes = 4
	case firstByte >= 240 && firstByte < 248:
		numBytes = 5
	case firstByte >= 248 && firstByte < 252:
		numBytes = 6
	case firstByte >= 252 && firstByte < 254:
		numBytes = 7
	case firstByte >= 254:
		numBytes = 8
	default:
		numBytes = 1
	}

	// If the input length is insufficient to extract the specified number of bytes, return the original input.
	if len(input) < numBytes {
		return input, nil
	}

	// Extract the specified number of bytes and return the remaining bytes.
	extracted := input[:numBytes]
	remaining := input[numBytes:]

	return extracted, remaining
}

func value_check(value []byte) []byte {
	if len(value) == 0 {
		return []byte{0}
	}
	return value
}

func PrintProgam(p *Program) {
	if debug_pvm {
		fmt.Printf("JSize=%d\n", p.JSize)
		fmt.Printf("Z=%d\n", p.Z)
		fmt.Printf("CSize=%d\n", p.CSize)
		fmt.Printf("J=%v\n", p.J)
		fmt.Printf("Code=%v\n", p.Code)
		fmt.Printf("K=%v\n", p.K)
	}
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func binaryStringToByte(binaryStr string) byte {
	var result byte
	for i, bit := range binaryStr {
		if bit == '1' {
			result |= 1 << (len(binaryStr) - 1 - i)
		}
	}
	return result
}

func EncodeProgram(p *Program) []byte {
	// Encode the program
	encoded := make([]byte, 0)

	encodedJSize := types.E(p.JSize)
	encoded = append(encoded, encodedJSize...)

	encodedZ := types.E_l(uint64(p.Z), 1)
	encoded = append(encoded, encodedZ...)

	encodedCSize := types.E(p.CSize)
	encoded = append(encoded, encodedCSize...)
	for _, j := range p.J {
		encodedj := types.E_l(uint64(j), uint32(p.Z))
		encoded = append(encoded, encodedj...)
	}
	encoded = append(encoded, p.Code...)

	var k_bytes []byte
	for i := 0; i < len(p.K[0]); i += 8 {
		end := i + 8
		if end > len(p.K[0]) {
			end = len(p.K[0])
		}
		group := p.K[0][i:end]
		reversedGroup := reverseString(group)
		if len(reversedGroup) < 8 {
			reversedGroup = strings.Repeat("1", 8-len(reversedGroup)) + reversedGroup
		}

		byteValue := binaryStringToByte(reversedGroup)
		k_bytes = append(k_bytes, byteValue)
	}
	encoded = append(encoded, k_bytes...)
	encoded = append(encoded, []byte{0}...)
	return encoded
}

func DecodeProgram(p []byte) (*Program, uint32, uint32, uint32, uint32, []byte, []byte) {
	// Check if the program include ascii byte code
	var pure_code []byte
	pure_code = p

	// extract standard program information
	o_size_byte := pure_code[:3]
	o_size := types.DecodeE_l(o_size_byte)
	w_size_byte := pure_code[3:6]
	w_size := types.DecodeE_l(w_size_byte)
	standard_z_byte := pure_code[6:8]
	standard_s_byte := pure_code[8:11]
	var o_byte []byte
	if 11+o_size > uint64(len(pure_code)) {
		o_byte = make([]byte, o_size)
	} else {
		o_byte = pure_code[11 : 11+o_size]
	}
	var w_byte []byte
	if 11+o_size+w_size > uint64(len(pure_code)) {
		w_byte = make([]byte, w_size)
	} else {
		w_byte = pure_code[11+o_size : 11+o_size+w_size]
	}
	var standard_c_size_byte []byte
	if 11+o_size+w_size+4 > uint64(len(pure_code)) {
		standard_c_size_byte = pure_code[11+0+0 : 11+0+0+4]
		pure_code = pure_code[11+0+0+4:]
	} else {
		standard_c_size_byte = pure_code[11+o_size+w_size : 11+o_size+w_size+4]
		pure_code = pure_code[11+o_size+w_size+4:]
	}

	if debug_pvm {
		fmt.Printf("OSIZE=%d\n", o_size)
		fmt.Printf("WSIZE=%d\n", w_size)
		fmt.Printf("Z=%d\n", types.DecodeE_l(standard_z_byte))
		fmt.Printf("S=%d\n", types.DecodeE_l(standard_s_byte))
		fmt.Printf("O=%v\n", o_byte)
		fmt.Printf("W=%v\n", w_byte)
		fmt.Printf("Code and Jump Table size: %d\n", types.DecodeE_l(standard_c_size_byte))
	}
	var j_size_byte, z_byte, c_size_byte []byte
	j_size_byte, pure_code = extractBytes(pure_code)
	z_byte, pure_code = extractBytes(pure_code)
	c_size_byte, pure_code = extractBytes(pure_code)
	j_size, _ := types.DecodeE(j_size_byte)
	z, _ := types.DecodeE(z_byte)
	c_size, _ := types.DecodeE(c_size_byte)
	j_byte := pure_code[:j_size*z]
	c_byte := pure_code[j_size*z : j_size*z+c_size]
	k_bytes := pure_code[j_size*z+c_size:]
	var kCombined string
	for _, b := range k_bytes {
		binaryStr := fmt.Sprintf("%08b", b)
		kCombined += reverseString(binaryStr)
	}
	if len(kCombined) > int(c_size) {
		kCombined = kCombined[:int(c_size)]
	}
	// process j_array
	var j_array []uint32
	for i := uint64(0); i < uint64(len(j_byte)); i += z {
		end := i + z
		if end > uint64(len(j_byte)) {
			end = uint64(len(j_byte))
		}
		slice := j_byte[i:end]
		decodedValue := types.DecodeE_l(slice)
		j_array = append(j_array, uint32(decodedValue))
	}

	if debug_pvm {
		fmt.Printf("JSize=%d\n", j_size)
		fmt.Printf("Z=%d\n", z)
		fmt.Printf("CSize=%d\n", c_size)
		fmt.Println("Jump Table: ", j_array)
		fmt.Printf("Code: %x\n", c_byte)
		fmt.Printf("K(bitmask): %v\n", kCombined)
		fmt.Println("================================================================")
	}
	program := &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: c_size,
		J:     j_array,
		Code:  c_byte,
		K:     []string{kCombined},
	}
	return program, uint32(o_size), uint32(w_size), uint32(types.DecodeE_l(standard_z_byte)), uint32(types.DecodeE_l(standard_s_byte)), o_byte, w_byte

}

func CelingDevide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

func P_func(x uint32) uint32 {
	return Z_P * CelingDevide(x, Z_P)
}

func Q_func(x uint32) uint32 {
	return Z_Q * CelingDevide(x, Z_Q)
}

func Z_func(x uint32) uint32 {
	return Z_Z * CelingDevide(x, Z_Z)
}

func Standard_Program_Initialization(vm *VM, argument_data_a []byte) {
	// Standard Program Initialization
	// check condition for standard program GP-0.4.0 (261))
	if len(argument_data_a) == 0 {
		argument_data_a = []byte{0}
	}
	condition := uint64(5*Z_Z+Q_func(vm.o_size)+Q_func(vm.w_size+vm.z*Z_P)+Q_func(vm.s)+Z_I) <= uint64((1 << 32))
	if condition {
		if debug_pvm {
			fmt.Println("Standard Program Initialization...")
		}
		// set up the initial values and access mode for the RAM
		var page_index, page_length uint32
		var access_mode AccessMode

		page_index = (Z_Z + vm.o_size) / PageSize
		page_length = CelingDevide((P_func(vm.o_size) - vm.o_size), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}

		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("1. Set Page Index %d and Page Length %d with Mode %v\n", page_index, page_length, access_mode)
		}

		page_index = (2*Z_Z + Z_func(vm.o_size)) / PageSize
		page_length = CelingDevide(vm.w_size, PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("2. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		}
		vm.Ram.WriteRAMBytes(2*Z_Z+Z_func(vm.o_size), vm.w_byte)

		page_index = (2*Z_Z + Z_func(vm.o_size) + vm.w_size) / PageSize
		page_length = CelingDevide(P_func(vm.w_size)+vm.z*Z_P-vm.w_size, PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("3. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		}

		page_index = ((1 << 32) - 2*Z_Z - Z_I - P_func(vm.s)) / PageSize
		page_length = CelingDevide(P_func(vm.s), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("4. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		}

		page_index = ((1 << 32) - Z_Z - Z_I) / PageSize
		page_length = CelingDevide(uint32(len(argument_data_a)), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("5. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		}
		vm.Ram.WriteRAMBytes((1<<32)-Z_Z-Z_I, argument_data_a)

		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		page_index = ((1 << 32) - Z_Z - Z_I + uint32(len(argument_data_a))) / PageSize
		page_length = CelingDevide(P_func(uint32(len(argument_data_a)))-uint32(len(argument_data_a)), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		if debug_pvm {
			fmt.Printf("6. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		}

		// set up the initial values for the registers
		vm.WriteRegister(0, (1<<32)-(1<<16))
		vm.WriteRegister(1, (1<<32)-2*Z_Z-Z_I)
		vm.WriteRegister(7, (1<<32)-Z_Z-Z_I)
		vm.WriteRegister(8, uint64(len(argument_data_a)))
	} else {
		panic("Standard Program Initialization Error\n")
	}
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint32, hostENV types.HostEnv) *VM {
	if len(code) == 0 {
		panic("NO CODE\n")
	}
	p, o_size, w_size, z, s, o_byte, w_byte := DecodeProgram(code)
	_ = o_byte
	_ = w_byte

	// TODO: William - initialize RAM "a" @ 0xFEFF0000 (2^32 - Z_Q - Z_I) based on entrypoint:
	//  Refine a = E(s, y, p, c, a, o, x bar ...) Eq 271
	//  Accumulate a = E(o) Eq 275
	//  IsAuthorized - E(p,c) Eq 268
	//  Transfer - E(t) Eq 282
	vm := &VM{
		JSize:         p.JSize,
		Z:             p.Z,
		J:             p.J,
		code:          p.Code,
		bitmask:       p.K[0], // pass in bitmask K
		register:      make([]uint64, regSize),
		pc:            initialPC,
		Ram:           NewRAM(),
		hostenv:       hostENV, //check if we need this
		Exports:       make([][]byte, 0),
		Service_index: service_index,
		o_size:        o_size,
		w_size:        w_size,
		z:             z,
		s:             s,
		o_byte:        o_byte,
		w_byte:        w_byte,
	}
	// for _, pg := range pages {
	// 	vm.writeRAMBytes(pg.Address, pg.Contents)
	// }
	copy(vm.register, initialRegs)
	// Standard Program Initialization
	// Standard_Program_Initialization(vm, []byte{})
	return vm
}

// NewVM with entrypoint and IsMalicious
// func NewVM_With_EntryPoint(service_index uint32, code []byte, initialRegs []uint32, initialPC uint32, pagemap []PageMap, pages []Page, hostENV types.HostEnv, Entrypoint uint32, IsMalicious bool) *VM {
// 	if len(code) == 0 {
// 		panic("NO CODE\n")
// 	}
// 	fmt.Println("Code: ", code)
// 	p := DecodeProgram(code)
// 	fmt.Printf("Code: %v K(bitmask): %v\n", p.Code, p.K[0])
// 	fmt.Println("================================================================")
// 	vm := &VM{
// 		JSize:         p.JSize,
// 		Z:             p.Z,
// 		J:             p.J,
// 		code:          p.Code,
// 		bitmask:       p.K[0], // pass in bitmask K
// 		register:      make([]uint32, regSize),
// 		pc:            initialPC,
// 		ram:           make(map[uint32][4096]byte),
// 		hostenv:       hostENV, //check if we need this
// 		Exports:       make([][]byte, 0),
// 		service_index: service_index,
// 		EntryPoint:    Entrypoint,
// 		IsMalicious:   IsMalicious,
// 	}
// 	// set vm.pc with entrypoint
// 	vm.pc = Entrypoint
// 	for _, pg := range pages {
// 		vm.writeRAMBytes(pg.Address, pg.Contents)
// 	}
// 	copy(vm.register, initialRegs)
// 	return vm
// }

// func NewVMFromParseProgramTest(code []byte) {
// 	p := DecodeProgram(code)
// 	fmt.Printf("Code: %v K(bitmask): %v\n", p.Code, p.K[0])
// 	fmt.Println("================================================================")
// }

func NewVMFromCode(serviceIndex uint32, code []byte, i uint32, hostENV types.HostEnv) *VM {
	return NewVM(serviceIndex, code, []uint64{}, i, hostENV)
}

// func NewVMFromCode_With_EntryPoint(serviceIndex uint32, code []byte, i uint32, hostENV types.HostEnv, Entrypoint uint32, IsMalicious bool) *VM {
// 	return NewVM_With_EntryPoint(serviceIndex, code, []uint32{}, i, []PageMap{}, []Page{}, hostENV, Entrypoint, IsMalicious)
// }

func NewForceCreateVM(code []byte, bitmask string, hostENV types.HostEnv) *VM {
	return &VM{
		code:    code,
		bitmask: bitmask,
		hostenv: hostENV,
	}
}

func NewVMFortest(hostENV types.HostEnv) *VM {
	return &VM{
		register: make([]uint64, regSize),
		hostenv:  hostENV,
		Ram:      NewRAM(),
	}
}

func (vm *VM) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VM) GetServiceIndex() uint32 {
	return vm.Service_index
}

func (vm *VM) ExecuteRefine(s uint32, y []byte, workPackageHash common.Hash, codeHash common.Hash, authorizerCodeHash common.Hash, authorization []byte, extrinsicsBlobs [][]byte) (r types.Result, res uint64) {
	// TODO: William -- work with Sean on encode/decode of argument inputs here
	// Refine inputs: let a = E(s, y, p, c, a, o, ↕[↕x S x <− x])
	a := common.Uint32ToBytes(s)                 // s - the service index
	a = append(a, y...)                          //  work payload, y,
	a = append(a, workPackageHash.Bytes()...)    // p - work package hash
	a = append(a, codeHash.Bytes()...)           //  c - the prediction of the hash of that service’s code c at the time of reporting,
	a = append(a, authorizerCodeHash.Bytes()...) //
	a = append(a, authorization...)              // authorization
	a = append(a, common.ConcatenateByteSlices(extrinsicsBlobs)...)
	// vm.setArgumentInputs(a)

	Standard_Program_Initialization(vm, a) // eq 264/265
	vm.Execute(types.EntryPointRefine)
	return vm.getArgumentOutputs()
}

func (vm *VM) ExecuteAccumulate(elements []types.AccumulateOperandElements, X *types.XContext) (r types.Result, res uint64) {
	var arguments []types.AccumulateOperandElements
	a, _ := types.Encode(elements)
	// need to figure out how the encoded elements are used in the PVM
	decoded, _, err := types.Decode(a, reflect.TypeOf(arguments))
	if err != nil {
		panic(err)
	}

	arguments, ok := decoded.([]types.AccumulateOperandElements)
	if !ok {
		panic("decoded data is not of type []types.AccumulateOperandElements")
	}
	for _, argument := range arguments {
		Standard_Program_Initialization(vm, argument.Results.Ok) // eq 264/265
		vm.X = X
		vm.Y = X.Clone()
		vm.Execute(types.EntryPointAccumulate)
	}

	// return vm.getArgumentOutputs()
	r.Err = vm.resultCode
	r.Ok = []byte{}
	return r, 0
}
func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account

	Standard_Program_Initialization(vm, arguments) // eq 264/265
	vm.Execute(types.EntryPointOnTransfer)
	// return vm.getArgumentOutputs()
	r.Err = vm.resultCode
	r.Ok = []byte{}
	return r, 0
}

// E(p, c)
func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint32) (r types.Result, res uint64) {
	a := p.Bytes()
	a = append(a, common.Uint32ToBytes(c)...)
	// vm.setArgumentInputs(a)
	Standard_Program_Initialization(vm, a) // eq 264/265
	vm.Execute(types.EntryPointOnTransfer)
	return vm.getArgumentOutputs()
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int) error {
	vm.pc = uint32(entryPoint)
	for !vm.terminated {
		if err := vm.step(); err != nil {
			return err
		}
		// host call invocation
		if vm.hostCall {
			if debug_pvm {
				fmt.Println("Invocate Host Function: ", vm.host_func_id)
			}
			vm.InvokeHostCall(vm.host_func_id)
			vm.hostCall = false
			vm.terminated = false
		}
		if debug_pvm {
			fmt.Println("-----------------------------------------------------------------------")
		}
	}
	if debug_pvm {
		fmt.Println("last pc: ", vm.pc)
	}
	// if vm finished without error, set result code to OK
	vm.resultCode = types.RESULT_OK
	return nil
}

// set ups extrinsics and payload
func (vm *VM) SetExtrinsicsPayload(extrinsics [][]byte, payload []byte) error {
	vm.Extrinsics = extrinsics
	vm.payload = payload
	return nil
}

// set up Imports
func (vm *VM) SetImports(imports [][]byte) error {
	vm.Imports = imports
	return nil
}

// copy a into 2^32 - Z_Q - Z_I and initialize registers
func (vm *VM) setArgumentInputs(a []byte) error {
	vm.WriteRegister(0, 0)
	vm.WriteRegister(1, 0xFFFF0000) // 2^32 - 2^16
	vm.WriteRegister(2, 0xFEFE0000) // 2^32 - 2 * 2^16 - 2^24
	for r := 3; r < 10; r++ {
		vm.WriteRegister(r, 0)
	}
	vm.WriteRegister(10, 0xFEFF0000) // 2^32 - 2^16 - 2^24
	vm.WriteRegister(11, uint64(len(a)))
	for r := 12; r < 16; r++ {
		vm.WriteRegister(r, 0)
	}
	vm.Ram.WriteRAMBytes(0xFEFF0000, a)
	return nil
}

func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	o, _ := vm.ReadRegister(10)
	l, _ := vm.ReadRegister(11)

	output, res := vm.Ram.ReadRAMBytes(uint32(o), uint32(l))
	r.Err = vm.resultCode
	if r.Err == types.RESULT_OK {
		r.Ok = output
	}
	return r, res
}

// step performs a single step in the PVM
func (vm *VM) step() error {
	if vm.pc >= uint32(len(vm.code)) {
		return errors.New("program counter out of bounds")
	}

	instr := vm.code[vm.pc]
	opcode := instr
	len_operands := skip(vm.pc, vm.bitmask)
	x := vm.pc + 4
	if x > uint32(len(vm.code)) {
		x = uint32(len(vm.code))
	}
	operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
	if debug_pvm {
		fmt.Printf("pc: %d opcode: %d - operands: %v, len(operands) = %d\n", vm.pc, opcode, operands, len_operands)
	}
	switch instr {
	case TRAP:
		if debug_pvm {
			fmt.Printf("TERMINATED\n")
		}
		if instr == TRAP {
			vm.resultCode = types.RESULT_PANIC
		} else {
			vm.resultCode = types.RESULT_OK
		}
		vm.terminated = true
	case FALLTHROUGH:
		if debug_pvm {
			fmt.Println("FALLTHROUGH")
		}
		vm.pc += 1
	case JUMP:
		// handle no operand means 0
		originalOperands := make([]byte, len(operands))
		copy(originalOperands, operands)
		lx := min(4, len(originalOperands))
		if lx == 0 {
			lx = 1
			originalOperands = append(originalOperands, 0)
		}
		vx := z_encode(types.DecodeE_l(originalOperands[0:lx]), uint32(lx))

		if debug_pvm {
			fmt.Println("Jump to: ", uint32(int64(vm.pc)+vx))
		}
		errCode := vm.branch(uint32(int64(vm.pc)+vx), true)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case JUMP_IND:
		// handle no operand means 0
		originalOperands := make([]byte, len(operands))
		copy(originalOperands, operands)

		registerIndexA := min(12, int(originalOperands[0])%16)
		lx := min(4, max(0, len(originalOperands))-1)
		if lx == 0 {
			lx = 1
			originalOperands = append(originalOperands, 0)
		}
		vx := uint32(types.DecodeE_l(originalOperands[1 : 1+lx]))

		valueA, errCode := vm.ReadRegister(registerIndexA)
		j := uint32(valueA) + uint32(vx)
		if debug_pvm {
			fmt.Println("Djump to: ", j)
		}
		vm.djump(j)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP:
		errCode := vm.loadImmJump(operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP_IND:
		errCode := vm.loadImmJumpInd(operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		errCode := vm.branchReg(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		errCode := vm.branchCond(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case ECALLI:
		errCode := vm.ecalli(operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

		if debug_pvm {
			fmt.Printf("TERMINATED\n")
		}
		if errCode != OK {
			vm.resultCode = types.RESULT_FAULT
			vm.terminated = true
		}
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32, STORE_IMM_U64:
		errCode := vm.storeImm(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IMM:
		errCode := vm.loadImm(operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_U8, LOAD_U16, LOAD_U32, LOAD_U64, LOAD_I8, LOAD_I16, LOAD_I32:
		errCode := vm.load(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_U8, STORE_U16, STORE_U32:
		errCode := vm.store(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32, STORE_IMM_IND_U64:
		errCode := vm.storeImmInd(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32, STORE_IND_U64:
		errCode := vm.storeInd(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32, LOAD_IND_I32, LOAD_IND_U64:
		errCode := vm.loadInd(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
		// MUL_UPPER_S_S_IMM, MUL_UPPER_S_S_IMM
	case ADD_IMM_32, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM, SET_LT_U_IMM, SET_LT_S_IMM:
		errCode := vm.aluImm(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case CMOV_IZ_IMM, CMOV_NZ_IMM:
		errCode := vm.cmovImm(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

	case SHLO_R_IMM_32, SHLO_L_IMM_32, SHAR_R_IMM_32, NEG_ADD_IMM_32, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT_32, SHLO_L_IMM_ALT_32, SHAR_R_IMM_ALT_32:
		errCode := vm.shiftImm(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case ADD_32, ADD_64, SUB_32, SUB_64, AND, XOR, OR, MUL_32, MUL_64, MUL_UPPER_S_S, MUL_UPPER_U_U, MUL_UPPER_S_U, DIV_U_32, DIV_U_64, DIV_S_32, DIV_S_64, REM_U_32, REM_U_64, REM_S_32, REM_S_64, CMOV_IZ, CMOV_NZ, SHLO_L_32, SHLO_L_64, SHLO_R_32, SHLO_R_64, SHAR_R_32, SHAR_R_64, SET_LT_U, SET_LT_S:
		errCode := vm.aluReg(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case MOVE_REG:
		errCode := vm.moveReg(operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case SBRK:
		vm.sbrk(operands)
		break

	// New opcodes
	case LOAD_IMM_64:
		errCode := vm.Instructions_with_Arguments_of_One_Register_and_One_Extended_Width_Immediate(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		vm.pc += 1 + len_operands
	case ADD_IMM_64, SHLO_L_IMM_64, SHLO_R_IMM_64:
		errCode := vm.Instructions_with_Arguments_of_Two_Registers_and_One_Immediate(opcode, operands)
		if debug_pvm {
			fmt.Println("Error: ", errCode)
		}
		vm.pc += 1 + len_operands

	default:
		vm.terminated = true
		vm.resultCode = types.RESULT_PANIC
		if debug_pvm {
			fmt.Printf("Unknown opcode: %d\n", opcode)
			fmt.Printf("----\n")
		}
		return nil
		/* Handle host call
		halt, err := vm.handleHostCall(opcode, operands)
		if err != nil {
			return err
		}
		if halt {
			// Host-call halt condition
			vm.hostCall = true
			vm.pc-- // Decrement PC to point to the host-call instruction
			return nil
		}*/
	}

	//vm.pc += 1 + uint32(skip(opcode))

	return nil
}

func reverseBytes(b []byte) []byte {
	n := len(b)
	for i := 0; i < n/2; i++ {
		b[i], b[n-1-i] = b[n-1-i], b[i]
	}
	return b
}

// handleHostCall handles host-call instructions
func (vm *VM) handleHostCall(opcode byte, operands []byte) (bool, uint64) {
	if vm.hostenv == nil {
		return false, HUH
	}
	// TODO: vm.hostenv.InvokeHostCall(opcode, operands, vm)
	return false, OOB
}

// skip calculates the skip distance based on the opcode
// func skip(opcode byte) uint32 {
// 	switch opcode {
// 	case JUMP, LOAD_IMM_JUMP, LOAD_IMM_JUMP_IND:
// 		return uint32(1)
// 	case BRANCH_EQ, BRANCH_NE, BRANCH_GE_U, BRANCH_GE_S, BRANCH_LT_U, BRANCH_LT_S:
// 		return uint32(2)
// 	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
// 		return uint32(3)
// 	default:
// 		return uint32(0)
// 	}
// }

// skip function calculates the distance to the next instruction
func skip(pc uint32, bitmask string) uint32 {
	// Convert the bitmask string to a slice of bytes
	bitmaskBytes := []byte(bitmask)

	// Iterate through the bitmask starting from the given pc position
	var i uint32
	for i = pc + 1; i < pc+25 && i < uint32(len(bitmaskBytes)); i++ {
		// Ensure we do not access out of bounds
		if bitmaskBytes[i] == '1' {
			return i - pc - 1
		}
	}
	// If no '1' is found within the next 24 positions, check the last index
	if i < pc+25 {
		return i - pc - 1
	}
	return uint32(24)
}

func (vm *VM) get_varpi(opcodes []byte, bitmask string) map[int]struct{} {
	result := make(map[int]struct{})
	for i, opcode := range opcodes {
		if bitmask[i] == '1' {
			if _, exists := T[int(opcode)]; exists {
				result[int(opcode)] = struct{}{}
			}
		}
	}
	return result
}

// func (vm *VM) djump(operands []byte) uint32 {

// 	if len(operands) != 1 {
// 		return OOB
// 	}

// 	registerIndexA := minInt(12, int(operands[0])%16)
// 	immIndexX := minInt(4, (len(operands)-1)%256)

// 	var vx uint32
// 	if 1+immIndexX < len(operands) {
// 		vx = get_elided_uint32(operands[1 : 1+immIndexX])
// 	} else {
// 		vx = 0
// 	}

// 	valueA, errCode := vm.readRegister(registerIndexA)
// 	if errCode != OK {
// 		return errCode
// 	}

// 	var target uint32
// 	Za := uint32(4)
// 	terminationInstructions := vm.get_varpi(vm.code, vm.bitmask)

// 	if (valueA/Za - 1) < uint32(len(vm.J)) {
// 		target = uint32(vm.J[(valueA/Za - 1)])
// 	} else {
// 		target = uint32(99999)
// 	}

// 	_, exists := terminationInstructions[int(target)]

// 	if valueA == uint32((1<<32)-(1<<16)) {
// 		fmt.Printf("TERMINATED\n")
// 		vm.resultCode = types.RESULT_OK
// 		vm.terminated = true
// 	} else if valueA == 0 || valueA > vx*Za || valueA%Za != 0 || exists {
// 		vm.resultCode = types.RESULT_OOB
// 		vm.terminated = true
// 		return OOB
// 	} else {
// 		vm.pc = vm.pc + target
// 	}

// 	return OK
// }

func (vm *VM) djump(a uint32) {
	if a == uint32((1<<32)-(1<<16)) {
		vm.terminated = true
	} else if a == 0 || a > uint32(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		// panic("OOB")
	} else {
		vm.pc = uint32(vm.J[(a/Z_A)-1])
	}
}

func (vm *VM) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= len(vm.register) {
		return 0, OOB
	}
	// fmt.Printf(" REGISTERS %v (index=%d => %d)\n", vm.register, index, vm.register[index])
	return vm.register[index], OK
}

func (vm *VM) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= len(vm.register) {
		return OOB
	}
	vm.register[index] = value
	if debug_pvm {
		fmt.Printf("Register[%d] = %d\n", index, value)
	}
	return OK
}

func (vm *VM) ReadRegisters() []uint64 {
	return vm.register
}

// Implement the dynamic jump logic
func (vm *VM) dynamicJump(operands []byte) uint64 {
	if len(operands) != 1 {
		return OOB
	}
	a := int(operands[0])
	const ZA = 4

	if a == 0 || a > 0x7FFFFFFF {
		return OOB
	}

	targetIndex := uint32(a/ZA - 1)
	if targetIndex >= uint32(len(vm.code)) {
		return OOB
	}

	vm.pc = targetIndex
	return OK
}

// Implement ecall logic
func (vm *VM) ecalli(operands []byte) uint64 {
	// Implement ecalli logic here
	lx := uint32(types.DecodeE_l(operands))
	vm.hostCall = true
	vm.host_func_id = int(lx)
	return HOST
}

// Implement storeImm logic
func (vm *VM) storeImm(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	lx := min(4, int(originalOperands[0])%8)
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly))

	switch opcode {
	case STORE_IMM_U8:
		value := vy % (1 << 8)
		if debug_pvm {
			fmt.Printf("STORE_IMM_U8: %d %v\n", vx, []byte{byte(value)})
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), []byte{byte(value)})
	case STORE_IMM_U16:
		value := types.E_l(uint64(vy%(1<<16)), 2)
		if debug_pvm {
			fmt.Printf("STORE_IMM_U16: %d %v\n", vx, value)
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	case STORE_IMM_U32:
		value := types.E_l(uint64(vy), 4)
		if debug_pvm {
			fmt.Printf("STORE_IMM_U32: %d %v\n", vx, value)
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	case STORE_IMM_U64:
		// TODO: NEW - check this
		value := types.E_l(uint64(vy), 8)
		if debug_pvm {
			fmt.Printf("STORE_IMM_U64: %d %v\n", vx, value)
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	}

	return OOB
}

// load_imm (opcode 4)
func (vm *VM) loadImm(operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	return vm.WriteRegister(registerIndexA, uint64(vx))
}

// LOAD_U8, LOAD_U16, LOAD_U32, LOAD_U64, LOAD_I8, LOAD_I16, LOAD_I32
func (vm *VM) load(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	switch opcode {
	case LOAD_U8, LOAD_I8: // TODO: CHECK NEW LOAD_I8
		value, errCode := vm.Ram.ReadRAMBytes((uint32(vx)), 1)
		if errCode != OK {
			return errCode
		}
		if debug_pvm {
			fmt.Printf("LOAD_U8: %d %v\n", vx, value)
		}
		return vm.WriteRegister(registerIndexA, uint64(value[0]))
	case LOAD_U16, LOAD_I16: // TODO: CHECK NEW LOAD_I16
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 2)
		if errCode != OK {
			return errCode
		}
		if debug_pvm {
			fmt.Printf("LOAD_U16: %d %v\n", vx, value)
		}
		return vm.WriteRegister(registerIndexA, uint64(types.DecodeE_l(value)))
	case LOAD_U32, LOAD_I32: // TODO: CHECK NEW LOAD_I32
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 4)
		if errCode != OK {
			return errCode
		}
		if debug_pvm {
			fmt.Printf("LOAD_U32: %d %v\n", vx, value)
		}
		return vm.WriteRegister(registerIndexA, uint64(types.DecodeE_l(value)))
	case LOAD_U64: // TODO: NEW, check this
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 8)
		if errCode != OK {
			return errCode
		}
		if debug_pvm {
			fmt.Printf("LOAD_U64: %d %v\n", vx, value)
		}
		return vm.WriteRegister(registerIndexA, uint64(types.DecodeE_l(value)))
	}
	return OK
}

func get_elided_uint32(o []byte) uint32 {
	/* GP:
	Immediate arguments are encoded in little-endian format with the most-significant bit being the sign bit.
	They may be compactly encoded by eliding more significant octets. Elided octets are assumed to be zero if the MSB of the value is zero, and 255 otherwise.
	*/
	if len(o) == 0 {
		return 0
	}

	if len(o) < 4 {
		newNumbers := make([]byte, 4)
		copy(newNumbers, o)
		var fillValue byte
		if o[len(o)-1] > 127 {
			fillValue = byte(255)
		} else {
			fillValue = byte(0)
		}
		for i := len(o); i < 4; i++ {
			newNumbers[i] = fillValue
		}
		o = newNumbers
	}

	x := make([]byte, 4)
	if len(o) > 0 {
		x[3] = o[0]
	}
	if len(o) > 1 {
		x[2] = o[1]
	}
	if len(o) > 2 {
		x[1] = o[2]
	}
	if len(o) > 3 {
		x[0] = o[3]
	}

	if debug_pvm {
		fmt.Printf("get_elided_uint32 %v from %v\n", x, o)
	}
	return binary.BigEndian.Uint32(x)
}

func get_elided_int32(o []byte) int32 {
	x := make([]byte, 4)
	x[0] = 0xff
	x[1] = 0xff
	x[2] = 0xff
	x[3] = 0xff
	// Copy the input bytes to the right end of x
	copy(x[4-len(o):], o)
	if debug_pvm {
		fmt.Printf("get_elided_int32 %v from %v\n", x, o)
	}
	return int32(binary.BigEndian.Uint32(x))
}

// STORE_U8, STORE_U16, STORE_U32
func (vm *VM) store(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	switch opcode {
	case STORE_U8:
		value := valueA % (1 << 8)
		if debug_pvm {
			fmt.Printf("STORE_u8: %d %v\n", vx, []byte{byte(value)})
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), []byte{byte(value)})
	case STORE_U16:
		value := types.E_l(uint64(valueA%(1<<16)), 2)
		if debug_pvm {
			fmt.Printf("STORE_U16: %d %v\n", vx, value)
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	case STORE_U32:
		value := types.E_l(uint64(valueA), 4)
		if debug_pvm {
			fmt.Printf("STORE_U32: %d %v\n", vx, value)
		}
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	}
	return OOB
}

// storeImmInd implements STORE_IMM_{U8, U16, U32}
func (vm *VM) storeImmInd(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int(originalOperands[0])/16)%8)
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return OOB
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly))

	switch opcode {
	case STORE_IMM_IND_U8:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IMM_IND_U8: %d %v\n", valueA+vx, []byte{byte(vy % 8)})
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), []byte{byte(vy % 8)})
	case STORE_IMM_IND_U16:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IMM_IND_U16: %d %v\n", valueA+vx, types.E_l(uint64(vy%1<<16), 2))
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy%1<<16), 2))
	case STORE_IMM_IND_U32:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IMM_IND_U32: %d %v\n", valueA+vx, types.E_l(uint64(vy), 4))
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy), 4))
	case STORE_IMM_IND_U64:
		return vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy), 8))
	}
	return OOB
}

// Implement loadImmJump logic
func (vm *VM) loadImmJump(operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int(originalOperands[0]) / 16 % 8))
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := uint32(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly)))

	vm.WriteRegister(registerIndexA, uint64(vx))

	vm.branch(vy, true)
	return OK
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, (int(originalOperands[1]) % 8))
	ly := min(4, max(0, len(originalOperands)-lx-2))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx := x_encode(types.DecodeE_l(originalOperands[2:2+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[2+lx:2+lx+ly]), uint32(ly))

	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	vm.WriteRegister(registerIndexA, uint64(vx))
	vm.djump(uint32(valueB) + uint32(vy))

	return OK
}

// move_reg (opcode 82)
func (vm *VM) moveReg(operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexD := min(12, int(originalOperands[0])%16)
	registerIndexA := min(12, int(originalOperands[0])/16)

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	if debug_pvm {
		fmt.Printf("MOVE_REG: %d %d\n", registerIndexA, registerIndexD)
	}
	return vm.WriteRegister(registerIndexD, valueA)
}

func (vm *VM) sbrk(operands []byte) error {
	/*	srcIndex, destIndex := splitRegister(operands[0])

		amount := int(operands[0])
		newRAM := make([]byte, len(vm.ram)+amount)
		copy(newRAM, vm.ram)
		vm.ram = newRAM */
	return nil
}

func (vm *VM) branch(vx uint32, condition bool) uint32 {
	if condition {
		vm.pc = uint32(vx)
	}
	return OK
}

func (vm *VM) branchCond(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int(originalOperands[0]) / 16 % 8))
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := uint32(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly)))

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}

	switch opcode {
	case BRANCH_EQ_IMM:
		if valueA == uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_EQ_IMM: %valueA=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_NE_IMM:
		if valueA != uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_NE_IMM: %valueA!=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_U_IMM:
		if valueA < uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_LT_U_IMM: %valueA<%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_S_IMM:
		if z_encode(valueA, 4) < z_encode(vx, 4) {
			if debug_pvm {
				fmt.Printf("BRANCH_LT_S_IMM: %valueA<%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_U_IMM:
		if valueA <= uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_LE_U_IMM: %valueA<=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_S_IMM:
		if z_encode(valueA, 4) <= z_encode(vx, 4) {
			if debug_pvm {
				fmt.Printf("BRANCH_LE_S_IMM: %valueA<=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_U_IMM:
		if valueA >= uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_GE_U_IMM: %valueA>=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_S_IMM:
		if z_encode(valueA, 4) >= z_encode(vx, 4) {
			if debug_pvm {
				fmt.Printf("BRANCH_GE_S_IMM: %valueA>=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_U_IMM:
		if valueA > uint64(vx) {
			if debug_pvm {
				fmt.Printf("BRANCH_GT_U_IMM: %valueA>%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_S_IMM:
		if z_encode(valueA, 4) > z_encode(vx, 4) {
			if debug_pvm {
				fmt.Printf("BRANCH_GT_S_IMM: %valueA>%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	}
	return OK

}

func (vm *VM) storeInd(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	switch opcode {
	case STORE_IND_U8:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IND_U8: %d %v\n", valueB+vx, []byte{byte(valueA % 8)})
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), []byte{byte(valueA % 8)})
	case STORE_IND_U16:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IND_U16: %d %v\n", valueB+vx, types.E_l(uint64(valueA%1<<16), 2))
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA), 2))
	case STORE_IND_U32:
		// if debug_pvm {
		// 	fmt.Printf("STORE_IND_U32: %d %v\n", valueB+vx, types.E_l(uint64(valueA), 4))
		// }
		return vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA), 4))
	case STORE_IND_U64:
		return vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA), 8))
	default:
		return OOB
	}

}

func (vm *VM) loadInd(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	switch opcode {
	case LOAD_IND_U8:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 1)
		if errCode != OK {
			return errCode
		}
		result := uint64(value[0])
		if debug_pvm {
			fmt.Printf("LOAD_IND_U8: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I8:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 1)
		if errCode != OK {
			return errCode
		}
		result := uint64(z_decode(z_encode(uint64(value[0]), 1), 4))
		if debug_pvm {
			fmt.Printf("LOAD_IND_I8: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 2)
		if errCode != OK {
			return errCode
		}
		result := uint64(types.DecodeE_l(value))
		if debug_pvm {
			fmt.Printf("LOAD_IND_U16: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 2)
		if errCode != OK {
			return errCode
		}
		result := z_decode(z_encode(types.DecodeE_l(value), 2), 4)
		if debug_pvm {
			fmt.Printf("LOAD_IND_I16: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 4)
		if errCode != OK {
			return errCode
		}
		result := uint64(types.DecodeE_l(value))
		if debug_pvm {
			fmt.Printf("LOAD_IND_U32: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 4)
		if errCode != OK {
			return errCode
		}
		result := z_decode(z_encode(types.DecodeE_l(value), 4), 4)
		return vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U64:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 8)
		if errCode != OK {
			return errCode
		}
		result := uint64(types.DecodeE_l(value))
		return vm.WriteRegister(registerIndexA, result)
	}
	return OOB
}

func splitRegister(operand byte) (int, int) {
	registerIndexA := int(operand & 0xF)
	registerIndexB := int((operand >> 4) & 0xF)
	return registerIndexA, registerIndexB
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := uint64(x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))) // CHECK

	var result uint64
	switch opcode {
	case ADD_IMM_32:
		result = uint64(valueB) + vx
	case AND_IMM:
		result = valueB & vx
	case XOR_IMM:
		result = valueB ^ vx
	case OR_IMM:
		result = valueB | vx
	case MUL_IMM:
		result = valueB * vx
	//case MUL_UPPER_S_S_IMM:
	//	result = uint32(z_decode((z_encode(valueB, 4) * z_encode(vx, 4)), 4))
	//case MUL_UPPER_U_U_IMM:
	//	result = valueB * vx
	case SET_LT_U_IMM:
		if uint32(valueB) < uint32(vx) {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		if z_encode(valueB, 4) < z_encode(vx, 4) { // CHECK
			result = 1
		} else {
			result = 0
		}
	default:
		return OOB
	}
	if debug_pvm {
		fmt.Printf("aluImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.WriteRegister(registerIndexA, result)
}

// Implement cmov_nz_imm, cmov_nz_imm
func (vm *VM) cmovImm(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := uint64(x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx)))

	var result uint64
	switch opcode {
	case CMOV_IZ_IMM:
		if valueB == 0 {
			result = vx
		} else {
			result = valueA
		}
	case CMOV_NZ_IMM:
		if valueB != 0 {
			result = vx
		} else {
			result = valueA
		}
	default:
		return OOB
	}
	if debug_pvm {
		fmt.Printf("cmovImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.WriteRegister(registerIndexA, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)

	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint64
	switch opcode {
	case SHLO_R_IMM_32:
		result = valueB / (1 << (vx % 32))
	case SHLO_L_IMM_32:
		result = valueB * (1 << (vx % 32))
	case SHAR_R_IMM_32:
		result = uint64(z_decode(z_encode(valueB, 4)/(1<<(vx%32)), 4)) // CHECK
	case NEG_ADD_IMM_32:
		result = uint64(uint64(vx) + uint64(1<<32) - uint64(valueB))
	case SET_GT_U_IMM:
		if valueB > vx {
			result = 1
		} else {
			result = 0
		}
	case SET_GT_S_IMM:
		if z_encode(valueB, 4) > z_encode(vx, 4) { // CHECK
			result = 1
		} else {
			result = 0
		}
	case SHLO_L_IMM_ALT_32:
		result = uint64(vx * (1 << (valueB % 32)))
	case SHLO_R_IMM_ALT_32:
		result = uint64(vx / (1 << (valueB % 32)))
	case SHAR_R_IMM_ALT_32:
		result = uint64(z_decode(z_encode(vx, 4)/(1<<(valueB%32)), 4))
	default:
		return OOB
	}
	if debug_pvm {
		fmt.Printf("shiftImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.WriteRegister(registerIndexA, result)
}

// GP_.0.3.6(219)
func z_encode(a uint64, n uint32) int64 {
	modValue := uint64(1 << (8*n - 1))
	if a < modValue {
		return int64(a)
	} else {
		return int64(a) - int64(1<<(8*n))
	}
}

// GP_.0.3.6(220)
func z_decode(a int64, n uint32) uint64 {
	return (uint64(math.Pow(2, float64(8*n))) + uint64(a)) % uint64(math.Pow(2, float64(8*n)))
}

// GP_.0.3.8(224)
func x_encode(x uint64, n uint32) uint64 {
	if n == 0 {
		return x
	}
	return x + (uint64(x)/(1<<(8*n-1)))*(uint64(1<<32)-(1<<(8*n)))
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := uint32(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx)))

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	switch opcode {
	case BRANCH_EQ:
		if valueA == valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_NE:
		if valueA != valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_U:
		if valueA < valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_S:
		if z_encode(valueA, 4) < z_encode(valueB, 4) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_U:
		if valueA >= valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_S:
		if z_encode(valueA, 4) >= z_encode(valueB, 4) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	default:
		return OOB
	}

	return OK
}

// Implement ALU operations with register values
func (vm *VM) aluReg(opcode byte, operands []byte) uint64 {

	registerIndexA := min(12, int(operands[0])%16)
	registerIndexB := min(12, int(operands[0])/16)
	registerIndexD := min(12, int(operands[1]))

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint64
	switch opcode {
	case ADD_32, ADD_64:
		// condition on IsMalicious
		if vm.IsMalicious {
			result = valueA * valueB
		} else {
			result = valueA + valueB
		}
	case SUB_32, SUB_64:
		result = valueA - valueB
	case AND:
		result = valueA & valueB
	case XOR:
		result = valueA ^ valueB
	case OR:
		result = valueA | valueB
	case MUL_32, MUL_64:
		result = valueA * valueB
	case MUL_UPPER_S_S:
		result = uint64(int(valueA) * int(valueB) >> 32)
	case MUL_UPPER_U_U:
		result = uint64((uint(valueA) * uint(valueB)) >> 32)
	case MUL_UPPER_S_U:
		result = uint64((int(valueA) * int(valueB)) >> 32)
	case DIV_U_32, DIV_U_64: // TODO: CHECK
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else {
			result = valueA / valueB
		}
	case DIV_S_32, DIV_S_64: // TODO: check
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else if int32(valueA) == -(1<<31) && int32(valueB) == -(1<<0) {
			result = valueA
		} else {
			result = uint64(int32(valueA) / int32(valueB))
		}
	case REM_U_32, REM_U_64: // TODO: check
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
	case REM_S_32, REM_S_64: // TODO: check
		if debug_pvm {
			fmt.Printf(" REM_S %d %d \n", int32(valueA), int32(valueB))
		}
		if valueB == 0 {
			result = valueA
		} else if valueA == 0x80 && valueB == 0xFF {
			return OOB
		} else {
			result = uint64(int32(valueA) % int32(valueB))
		}
	case CMOV_IZ:
		if valueB == 0 {
			result = valueA
		} else {
			result = 0
		}
	case CMOV_NZ:
		if valueB == 0 {
			result = 0
		} else {
			result = valueA
		}
	case SET_LT_U:
		if valueA < valueB {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S:
		if int64(valueA) < int64(valueB) {
			result = 1
		} else {
			result = 0
		}

	case SHLO_L_32, SHLO_L_64: // TODO: split
		result = valueA << (valueB % 32)
	case SHLO_R_32, SHLO_R_64: // TODO: split
		result = valueA >> (valueB % 32)
	case SHAR_R_32, SHAR_R_64: // TODO: split
		if int32(valueA)/(1<<(valueB%32)) < 0 && int32(valueA)%(1<<(valueB%32)) != 0 {
			result = uint64((int32(valueA) / (1 << (valueB % 32))) - 1)
		} else {
			result = uint64(int32(valueA) / (1 << (valueB % 32)))
		}

	default:
		return OOB // unknown ALU register
	}

	if debug_pvm {
		fmt.Printf("aluReg - rA[%d]=%d  regB[%d]=%d regD[%d]=%d\n", registerIndexA, valueA, registerIndexB, valueB, registerIndexD, result)
	}
	return vm.WriteRegister(registerIndexD, result)
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (vm *VM) CreateVM(serviceAcct uint32, code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n := range vm.VMs {
		if n > maxN {
			maxN = n
		}
	}
	if vm.VMs == nil {
		vm.VMs = make(map[uint32]*VM)
	}
	vm.VMs[maxN+1] = NewVMFromCode(serviceAcct, code, i, vm.hostenv)
	return maxN + 1
}

func (vm *VM) GetVM(n uint32) (*VM, bool) {
	vm, ok := vm.VMs[n]
	if !ok {
		return nil, false
	}
	return vm, true
}

func (vm *VM) ExpungeVM(n uint32) bool {
	_, ok := vm.VMs[n]
	if !ok {
		return false
	}
	delete(vm.VMs, n)
	return true
}

// shawn : add a pub function to get the register value
func (vm *VM) GetRegisterValue(registerIndex int) (uint64, uint64) {
	value, errCode := vm.ReadRegister(registerIndex)
	return value, errCode
}

func (vm *VM) SetRegisterValue(registerIndex int, value uint64) uint64 {
	return vm.WriteRegister(registerIndex, value)
}

func (vm *VM) HostCheat(input string) {
	if input == "machine" {
		vm.hostMachine()
	} else if input == "poke" {
		vm.hostPoke()
	} else if input == "peek" {
		vm.hostPeek()
	} else if input == "invoke" {
		vm.hostInvoke()
	} else if input == "expunge" {
		vm.hostExpunge()
	}
}

func (vm *VM) Instructions_with_Arguments_of_One_Register_and_One_Extended_Width_Immediate(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := types.DecodeE_l(originalOperands[1 : 1+lx])
	return vm.WriteRegister(registerIndexA, uint64(vx))
}

func (vm *VM) Instructions_with_Arguments_of_Two_Registers_and_One_Immediate(opcode byte, operands []byte) uint64 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}

	valueA, errCode := vm.ReadRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.ReadRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	var result uint64
	switch opcode {
	case ADD_IMM_64:
		valueA = valueB + uint64(vx)
		result = valueA

	case SHLO_L_IMM_64:
		valueA = x_encode(valueB*(1<<(vx%64)), 8)
		result = valueA
	case SHLO_R_IMM_64:
		valueA = x_encode(valueB/(1<<(vx%64)), 8)
		result = valueA
	default:
		return OOB // unknown ALU register
	}

	return vm.WriteRegister(registerIndexA, result)
}
