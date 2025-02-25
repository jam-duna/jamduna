package pvm

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug_pvm  = "pvm"
	debug_host = "host"

	regSize  = 13
	numCores = types.TotalCores
	W_E      = types.W_E
	W_S      = types.W_S

	W_X = 1024
	M   = 128
	Bs  = 100
	V   = 1023
	D   = 28800
	Z_A = 2
	Z_P = (1 << 12)
	Z_Q = (1 << 16)
	Z_I = (1 << 24)
	Z_Z = (1 << 16)
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

func (p *Page) zero() {
	p.Value = make([]byte, PageSize)
	p.Access = AccessMode{
		Inaccessible: false,
		Writable:     true,
		Readable:     true,
	}
}

func (p *Page) void() {
	p.Value = make([]byte, PageSize)
	p.Access = AccessMode{
		Inaccessible: true,
		Writable:     false,
		Readable:     false,
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
			log.Debug(debug_pvm, "WriteRAMBytes: Fail to get or allocate page to access address", "currentPage", currentPage, "address", address)
			return OOB
		}

		// Check if the page is writable
		if !page.Access.Writable {
			log.Debug(debug_pvm, "Page is not writable", "currentPage", currentPage, "address", address)
			return uint64(address)
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
			log.Debug(debug_pvm, "Fail to get or allocate page to access address", "currentPage", currentPage, "address", address)
			return nil, OOB
		}
		if page.Access.Inaccessible {
			log.Debug(debug_pvm, "Page is not readable", "currentPage", currentPage, "address", address, "length", length)
			return nil, uint64(address)
		}

		// Calculate how much data can be read from the current page
		bytesToRead := PageSize - pageOffset
		if bytesToRead > remaining {
			bytesToRead = remaining
		}

		// Check against the actual slice length to avoid out-of-range errors
		pageSize := uint32(len(page.Value))
		endPos := pageOffset + bytesToRead

		if endPos <= pageSize {
			// Entire slice is within valid range
			result = append(result, page.Value[pageOffset:endPos]...)
		} else {
			// Part or all of the required range goes out of bounds
			if pageOffset >= pageSize {
				// Completely out of range: fill everything with zero
				result = append(result, make([]byte, bytesToRead)...)
			} else {
				// Part of the data is valid, the rest should be zero
				validLen := pageSize - pageOffset
				result = append(result, page.Value[pageOffset:pageOffset+validLen]...)
				result = append(result, make([]byte, bytesToRead-validLen)...)
			}
		}

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
	JSize          uint64
	Z              uint8
	J              []uint32
	code           []byte
	bitmask        string // K in GP
	pc             uint64 // Program counter
	ResultCode     uint8
	HostResultCode uint64
	Fault_address  uint32
	terminated     bool
	hostCall       bool // ̵h in GP
	host_func_id   int  // h in GP
	// ram                 map[uint32][4096]byte
	Ram      *RAM
	register []uint64
	Gas      uint64
	hostenv  types.HostEnv

	VMs map[uint32]*VM

	// Work Package Inputs
	WorkItemIndex uint32
	WorkPackage   types.WorkPackage
	Extrinsics    types.ExtrinsicsBlobs
	Authorization []byte
	Imports       [][]byte

	// Invocation funtions entry point
	EntryPoint uint32

	// if logging = "author"
	logging string

	// standard program initialization parameters
	o_size uint32
	w_size uint32
	z      uint32
	s      uint32
	o_byte []byte
	w_byte []byte

	// Refine argument
	RefineM_map        map[uint32]*RefineM
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

	// Output
	Outputs []byte
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
	K string
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
	JUMP_IND  = 50
	LOAD_IMM  = 51
	LOAD_U8   = 52
	LOAD_I8   = 53
	LOAD_U16  = 54
	LOAD_I16  = 55
	LOAD_U32  = 56
	LOAD_I32  = 57
	LOAD_U64  = 58
	STORE_U8  = 59
	STORE_U16 = 60
	STORE_U32 = 61
	STORE_U64 = 62
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
	MOVE_REG              = 100
	SBRK                  = 101
	COUNT_SET_BITS_64     = 102
	COUNT_SET_BITS_32     = 103
	LEADING_ZERO_BITS_64  = 104
	LEADING_ZERO_BITS_32  = 105
	TRAILING_ZERO_BITS_64 = 106
	TRAILING_ZERO_BITS_32 = 107
	SIGN_EXTEND_8         = 108
	SIGN_EXTEND_16        = 109
	ZERO_EXTEND_16        = 110
	REVERSE_BYTES         = 111
)

// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
const (
	STORE_IND_U8      = 120
	STORE_IND_U16     = 121
	STORE_IND_U32     = 122
	STORE_IND_U64     = 123
	LOAD_IND_U8       = 124
	LOAD_IND_I8       = 125
	LOAD_IND_U16      = 126
	LOAD_IND_I16      = 127
	LOAD_IND_U32      = 128
	LOAD_IND_I32      = 129
	LOAD_IND_U64      = 130
	ADD_IMM_32        = 131
	AND_IMM           = 132
	XOR_IMM           = 133
	OR_IMM            = 134
	MUL_IMM_32        = 135
	SET_LT_U_IMM      = 136
	SET_LT_S_IMM      = 137
	SHLO_L_IMM_32     = 138
	SHLO_R_IMM_32     = 139
	SHAR_R_IMM_32     = 140
	NEG_ADD_IMM_32    = 141
	SET_GT_U_IMM      = 142
	SET_GT_S_IMM      = 143
	SHLO_L_IMM_ALT_32 = 144
	SHLO_R_IMM_ALT_32 = 145
	SHAR_R_IMM_ALT_32 = 146
	CMOV_IZ_IMM       = 147
	CMOV_NZ_IMM       = 148
	ADD_IMM_64        = 149
	MUL_IMM_64        = 150
	SHLO_L_IMM_64     = 151
	SHLO_R_IMM_64     = 152
	SHAR_R_IMM_64     = 153
	NEG_ADD_IMM_64    = 154
	SHLO_L_IMM_ALT_64 = 155
	SHLO_R_IMM_ALT_64 = 156
	SHAR_R_IMM_ALT_64 = 157

	// GP-0.5.4 new instructions
	ROT_R_64_IMM     = 158
	ROT_R_64_IMM_ALT = 159
	ROT_R_32_IMM     = 160
	ROT_R_32_IMM_ALT = 161
)

// A.5.11. Instructions with Arguments of Two Registers & One Offset.
const (
	BRANCH_EQ   = 170
	BRANCH_NE   = 171
	BRANCH_LT_U = 172
	BRANCH_LT_S = 173
	BRANCH_GE_U = 174
	BRANCH_GE_S = 175
)

// A.5.12. Instruction with Arguments of Two Registers and Two Immediates.
const (
	LOAD_IMM_JUMP_IND = 180
)

// A.5.13. Instructions with Arguments of Three Registers.
const (
	ADD_32        = 190
	SUB_32        = 191
	MUL_32        = 192
	DIV_U_32      = 193
	DIV_S_32      = 194
	REM_U_32      = 195
	REM_S_32      = 196
	SHLO_L_32     = 197
	SHLO_R_32     = 198
	SHAR_R_32     = 199
	ADD_64        = 200
	SUB_64        = 201
	MUL_64        = 202
	DIV_U_64      = 203
	DIV_S_64      = 204
	REM_U_64      = 205
	REM_S_64      = 206
	SHLO_L_64     = 207
	SHLO_R_64     = 208
	SHAR_R_64     = 209
	AND           = 210
	XOR           = 211
	OR            = 212
	MUL_UPPER_S_S = 213
	MUL_UPPER_U_U = 214
	MUL_UPPER_S_U = 215
	SET_LT_U      = 216
	SET_LT_S      = 217
	CMOV_IZ       = 218
	CMOV_NZ       = 219

	// GP-0.5.4 new instructions
	ROT_L_64 = 220
	ROT_L_32 = 221
	ROT_R_64 = 222
	ROT_R_32 = 223
	AND_INV  = 224
	OR_INV   = 225
	XNOR     = 226
	MAX      = 227
	MAX_U    = 228
	MIN      = 229
	MIN_U    = 230
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
	WHAT = (1 << 64) - 2  // 2^32 - 2
	OOB  = (1 << 64) - 3  // 2^32 - 3
	WHO  = (1 << 64) - 4  // 2^32 - 4
	FULL = (1 << 64) - 5  // 2^32 - 5
	CORE = (1 << 64) - 6  // 2^32 - 6
	CASH = (1 << 64) - 7  // 2^32 - 7
	LOW  = (1 << 64) - 8  // 2^32 - 8
	HIGH = (1 << 64) - 9  // 2^32 - 9
	HUH  = (1 << 64) - 10 // 2^32 - 10
	OK   = 0              // 0

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
	for i := 0; i < len(p.K); i += 8 {
		end := i + 8
		if end > len(p.K) {
			end = len(p.K)
		}
		group := p.K[i:end]
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

	log.Debug(debug_pvm, "OSIZE", o_size, "WSIZE", w_size, "Z", types.DecodeE_l(standard_z_byte), "S", types.DecodeE_l(standard_s_byte),
		"O", o_byte, "W", w_byte, "Code and Jump Table size: %d\n", types.DecodeE_l(standard_c_size_byte))
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

	program := &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: c_size,
		J:     j_array,
		Code:  c_byte,
		K:     kCombined,
	}
	return program, uint32(o_size), uint32(w_size), uint32(types.DecodeE_l(standard_z_byte)), uint32(types.DecodeE_l(standard_s_byte)), o_byte, w_byte

}

func DecodeProgram_pure_pvm_blob(p []byte) *Program {
	var pure_code []byte
	pure_code = p

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

	program := &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: c_size,
		J:     j_array,
		Code:  c_byte,
		K:     kCombined,
	}
	return program

}

// func Floor(x int64) int64 {
// 	if x < 0 {
// 		return x - 1
// 	}
// 	return x
// }

func FloorDiv(a, b int64) int64 {
	q := a / b
	r := a % b

	if r != 0 && (a^b) < 0 {
		q--
	}
	return q
}

func FloorDivBig(a, b *big.Int) *big.Int {
	var quotient, remainder big.Int
	quotient.QuoRem(a, b, &remainder)
	if remainder.Sign() != 0 && a.Sign() < 0 {
		quotient.Sub(&quotient, big.NewInt(1))
	}
	return &quotient
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
		// set up the initial values and access mode for the RAM
		var page_index, page_length uint32
		var access_mode AccessMode

		page_index = Z_Z / PageSize
		page_length = CelingDevide(vm.o_size, PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: false}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		log.Trace(vm.logging, "Standard Program Initialization", "page_index", page_index, "page_length", page_length, "access_mode", access_mode)

		vm.Ram.WriteRAMBytes(Z_Z, vm.o_byte)

		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		page_index = (Z_Z + vm.o_size) / PageSize
		page_length = CelingDevide((P_func(vm.o_size) - vm.o_size), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}

		// 1. Set Page Index and Page Length %d with Mode: page_index, page_length, access_mode
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		page_index = (2*Z_Z + Z_func(vm.o_size)) / PageSize
		page_length = CelingDevide(vm.w_size, PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		// 2. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode)
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		vm.Ram.WriteRAMBytes(2*Z_Z+Z_func(vm.o_size), vm.w_byte)

		page_index = (2*Z_Z + Z_func(vm.o_size) + vm.w_size) / PageSize
		page_length = CelingDevide(P_func(vm.w_size)+vm.z*Z_P-vm.w_size, PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		// 3. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		page_index = ((1 << 32) - 2*Z_Z - Z_I - P_func(vm.s)) / PageSize
		page_length = CelingDevide(P_func(vm.s), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}

		// 4. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		page_index = ((1 << 32) - Z_Z - Z_I) / PageSize
		page_length = CelingDevide(uint32(len(argument_data_a)), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: true}
		// 5. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		vm.Ram.WriteRAMBytes((1<<32)-Z_Z-Z_I, argument_data_a)

		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)

		page_index = ((1 << 32) - Z_Z - Z_I + uint32(len(argument_data_a))) / PageSize
		page_length = CelingDevide(P_func(uint32(len(argument_data_a)))-uint32(len(argument_data_a)), PageSize)
		access_mode = AccessMode{Inaccessible: false, Writable: false, Readable: true}
		vm.Ram.SetPageAccess(page_index, page_length, access_mode)
		// 6. Set Page Index %d and Page Length %d with Access Mode %v\n", page_index, page_length, access_mode
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
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, hostENV types.HostEnv, jam_ready_blob bool) *VM {
	if len(code) == 0 {
		panic("NO CODE\n")
	}
	var p *Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	if jam_ready_blob {
		p, o_size, w_size, z, s, o_byte, w_byte = DecodeProgram(code)
	} else {
		p = DecodeProgram_pure_pvm_blob(code)
		o_size = 0
		w_size = 0
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = []byte{}
	}

	// TODO: William - initialize RAM "a" @ 0xFEFF0000 (2^32 - Z_Q - Z_I) based on entrypoint:
	//  Refine a = E(s, y, p, c, a, o, x bar ...) Eq 271
	//  Accumulate a = E(o) Eq 275
	//  IsAuthorized - E(p,c) Eq 268
	//  Transfer - E(t) Eq 282
	vm := &VM{
		Gas:           0,
		JSize:         p.JSize,
		Z:             p.Z,
		J:             p.J,
		code:          p.Code,
		bitmask:       p.K, // pass in bitmask K
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

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, hostENV types.HostEnv) *VM {
	return NewVM(serviceIndex, code, []uint64{}, i, hostENV, true)
}

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

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash) (r types.Result, res uint64) {
	workitem := workPackage.WorkItems[workitemIndex]

	a := common.Uint32ToBytes(workitem.Service)
	a = append(a, workitem.Payload...)
	a = append(a, workPackage.Hash().Bytes()...)

	workPackage_RefineContext, _ := types.Encode(workPackage.RefineContext)
	a = append(a, workPackage_RefineContext...)
	a = append(a, p_a.Bytes()...)

	vm.WorkItemIndex = workitemIndex
	vm.Gas = workitem.RefineGasLimit
	vm.WorkPackage = workPackage
	// Sourabh , William pls validate this
	vm.Authorization = authorization.Ok
	//===================================
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments

	Standard_Program_Initialization(vm, a) // eq 264/265
	vm.Execute(types.EntryPointRefine)
	r, res = vm.getArgumentOutputs()
	return r, res
}

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, g uint64, elements []types.AccumulateOperandElements, X *types.XContext) (r types.Result, res uint64, xs *types.ServiceAccount) {
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()
	// for _, argument := range elements {
	input_bytes := make([]byte, 0)
	t_bytes := common.Uint32ToBytes(t)
	s_bytes := common.Uint32ToBytes(s)
	encoded_elements, _ := types.Encode(elements)
	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, encoded_elements...)
	Standard_Program_Initialization(vm, input_bytes) // eq 264/265
	vm.Gas = g
	vm.Execute(types.EntryPointAccumulate) // F ∈ Ω⟨(X, X)⟩
	// }
	xs, _ = vm.X.GetX_s()
	r, res = vm.getArgumentOutputs()
	return r, res, xs
}
func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account

	Standard_Program_Initialization(vm, arguments) // eq 264/265
	vm.Execute(types.EntryPointOnTransfer)
	// return vm.getArgumentOutputs()
	r.Err = vm.ResultCode
	r.Ok = []byte{}
	return r, 0
}

// E(p, c)
func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16) (r types.Result) {
	a := p.Bytes()
	a = append(a, common.Uint16ToBytes(c)...)
	// vm.setArgumentInputs(a)
	Standard_Program_Initialization(vm, a) // eq 264/265
	vm.Execute(types.EntryPointAuthorization)
	r, _ = vm.getArgumentOutputs()
	return r
}

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int) error {
	vm.terminated = false

	// A.2 deblob
	if vm.code == nil {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("No code to execute")
	}

	if len(vm.code) == 0 {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("No code to execute")
	}

	if len(vm.bitmask) == 0 {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return errors.New("Failed to decode bitmask")
	}

	vm.pc = uint64(entryPoint)
	stepn := 1
	for !vm.terminated {
		if err := vm.step(stepn); err != nil {
			return err
		}
		// host call invocation
		if vm.hostCall {
			vm.InvokeHostCall(vm.host_func_id)
			vm.hostCall = false
			vm.terminated = false
		}
		stepn++;
	}
	log.Trace(vm.logging, "PVM Complete", "pc", vm.pc)
	// if vm finished without error, set result code to OK
	if !vm.terminated {
		vm.ResultCode = types.RESULT_OK
	}
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
	if vm.ResultCode == types.RESULT_OOG {
		r.Err = types.RESULT_OOG
		return r, 0
	}
	o, _ := vm.ReadRegister(7)
	l, _ := vm.ReadRegister(8)
	output, res := vm.Ram.ReadRAMBytes(uint32(o), uint32(l))
	if vm.ResultCode == types.RESULT_OK && res == 0 {
		r.Ok = output
		return r, res
	}
	if vm.ResultCode == types.RESULT_OK && res != 0 {
		r.Ok = []byte{}
		return r, res
	}
	r.Err = types.RESULT_PANIC
	return r, 0
}


func opcode_str(opcode byte) string {
	opcodeMap := map[byte]string{
		10:  "ECALLI",
		20:  "LOAD_IMM_64",
		30:  "STORE_IMM_U8",
		31:  "STORE_IMM_U16",
		32:  "STORE_IMM_U32",
		33:  "STORE_IMM_U64",
		40:  "JUMP",
		50:  "JUMP_IND",
		51:  "LOAD_IMM",
		52:  "LOAD_U8",
		53:  "LOAD_I8",
		54:  "LOAD_U16",
		55:  "LOAD_I16",
		56:  "LOAD_U32",
		57:  "LOAD_I32",
		58:  "LOAD_U64",
		59:  "STORE_U8",
		60:  "STORE_U16",
		61:  "STORE_U32",
		62:  "STORE_U64",
		70:  "STORE_IMM_IND_U8",
		71:  "STORE_IMM_IND_U16",
		72:  "STORE_IMM_IND_U32",
		73:  "STORE_IMM_IND_U64",
		80:  "LOAD_IMM_JUMP",
		81:  "BRANCH_EQ_IMM",
		82:  "BRANCH_NE_IMM",
		83:  "BRANCH_LT_U_IMM",
		84:  "BRANCH_LE_U_IMM",
		85:  "BRANCH_GE_U_IMM",
		86:  "BRANCH_GT_U_IMM",
		87:  "BRANCH_LT_S_IMM",
		88:  "BRANCH_LE_S_IMM",
		89:  "BRANCH_GE_S_IMM",
		90:  "BRANCH_GT_S_IMM",
		100: "MOVE_REG",
		101: "SBRK",
		102: "COUNT_SET_BITS_64",
		103: "COUNT_SET_BITS_32",
		104: "LEADING_ZERO_BITS_64",
		105: "LEADING_ZERO_BITS_32",
		106: "TRAILING_ZERO_BITS_64",
		107: "TRAILING_ZERO_BITS_32",
		108: "SIGN_EXTEND_8",
		109: "SIGN_EXTEND_16",
		110: "ZERO_EXTEND_16",
		111: "REVERSE_BYTES",
		120: "STORE_IND_U8",
		121: "STORE_IND_U16",
		122: "STORE_IND_U32",
		123: "STORE_IND_U64",
		124: "LOAD_IND_U8",
		125: "LOAD_IND_I8",
		126: "LOAD_IND_U16",
		127: "LOAD_IND_I16",
		128: "LOAD_IND_U32",
		129: "LOAD_IND_I32",
		130: "LOAD_IND_U64",
		131: "ADD_IMM_32",
		132: "AND_IMM",
		133: "XOR_IMM",
		134: "OR_IMM",
		135: "MUL_IMM_32",
		136: "SET_LT_U_IMM",
		137: "SET_LT_S_IMM",
		138: "SHLO_L_IMM_32",
		139: "SHLO_R_IMM_32",
		140: "SHAR_R_IMM_32",
		141: "NEG_ADD_IMM_32",
		142: "SET_GT_U_IMM",
		143: "SET_GT_S_IMM",
		144: "SHLO_L_IMM_ALT_32",
		145: "SHLO_R_IMM_ALT_32",
		146: "SHAR_R_IMM_ALT_32",
		147: "CMOV_IZ_IMM",
		148: "CMOV_NZ_IMM",
		149: "ADD_IMM_64",
		150: "MUL_IMM_64",
		151: "SHLO_L_IMM_64",
		152: "SHLO_R_IMM_64",
		153: "SHAR_R_IMM_64",
		154: "NEG_ADD_IMM_64",
		155: "SHLO_L_IMM_ALT_64",
		156: "SHLO_R_IMM_ALT_64",
		157: "SHAR_R_IMM_ALT_64",
		158: "ROT_R_64_IMM",
		159: "ROT_R_64_IMM_ALT",
		160: "ROT_R_32_IMM",
		161: "ROT_R_32_IMM_ALT",
		170: "BRANCH_EQ",
		171: "BRANCH_NE",
		172: "BRANCH_LT_U",
		173: "BRANCH_LT_S",
		174: "BRANCH_GE_U",
		175: "BRANCH_GE_S",
		180: "LOAD_IMM_JUMP_IND",
		190: "ADD_32",
		191: "SUB_32",
		192: "MUL_32",
		193: "DIV_U_32",
		194: "DIV_S_32",
		195: "REM_U_32",
		196: "REM_S_32",
		197: "SHLO_L_32",
		198: "SHLO_R_32",
		199: "SHAR_R_32",
		200: "ADD_64",
		201: "SUB_64",
		202: "MUL_64",
		203: "DIV_U_64",
		204: "DIV_S_64",
		205: "REM_U_64",
		206: "REM_S_64",
		207: "SHLO_L_64",
		208: "SHLO_R_64",
		209: "SHAR_R_64",
		210: "AND",
		211: "XOR",
		212: "OR",
		213: "MUL_UPPER_S_S",
		214: "MUL_UPPER_U_U",
		215: "MUL_UPPER_S_U",
		216: "SET_LT_U",
		217: "SET_LT_S",
		218: "CMOV_IZ",
		219: "CMOV_NZ",
		220: "ROT_L_64",
		221: "ROT_L_32",
		222: "ROT_R_64",
		223: "ROT_R_32",
		224: "AND_INV",
		225: "OR_INV",
		226: "XNOR",
		227: "MAX",
		228: "MAX_U",
		229: "MIN",
		230: "MIN_U",
	}

	if name, exists := opcodeMap[opcode]; exists {
		return name
	}
	return fmt.Sprintf("OPCODE %d", opcode)
}

// step performs a single step in the PVM
func (vm *VM) step(stepn int) error {
	if vm.pc >= uint64(len(vm.code)) {
		return errors.New("program counter out of bounds")
	}

	instr := vm.code[vm.pc]
	opcode := instr
	len_operands := skip(vm.pc, vm.bitmask)
	x := vm.pc + 4
	if x > uint64(len(vm.code)) {
		x = uint64(len(vm.code))
	}
	operands := vm.code[vm.pc+1 : vm.pc+1+len_operands]
	startPC := vm.pc
	//beforeGas := vm.Gas
	switch instr {
	case TRAP:
		log.Trace(vm.logging, "TERMINATED")
		if instr == TRAP {
			vm.ResultCode = types.RESULT_PANIC
		} else {
			vm.ResultCode = types.RESULT_OK
		}
		vm.terminated = true
	case FALLTHROUGH:
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

		vm.branch(uint64(int64(vm.pc)+vx), true)
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
		// vx := types.DecodeE_l(originalOperands[1 : 1+lx])
		vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

		valueA, _ := vm.ReadRegister(registerIndexA)
		j := valueA + vx
		vm.djump(j % (1 << 32))
	case LOAD_IMM_JUMP:
		vm.loadImmJump(operands)
	case LOAD_IMM_JUMP_IND:
		vm.loadImmJumpInd(operands)
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		vm.branchReg(opcode, operands)
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		vm.branchCond(opcode, operands)
	case ECALLI:
		vm.HostResultCode = vm.ecalli(operands)
		vm.pc += 1 + len_operands
		// if vm.HostResultCode != OK {
		// 	vm.terminated = true
		// }
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32, STORE_IMM_U64:
		vm.storeImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case LOAD_IMM:
		vm.loadImm(operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case LOAD_U8, LOAD_U16, LOAD_U32, LOAD_U64, LOAD_I8, LOAD_I16, LOAD_I32:
		vm.load(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case STORE_U8, STORE_U16, STORE_U32, STORE_U64:
		vm.store(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32, STORE_IMM_IND_U64:
		vm.storeImmInd(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32, STORE_IND_U64:
		vm.storeInd(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32, LOAD_IND_I32, LOAD_IND_U64:
		vm.loadInd(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
		// MUL_UPPER_S_S_IMM, MUL_UPPER_S_S_IMM
	case ADD_IMM_32, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM_32, SET_LT_U_IMM, SET_LT_S_IMM, MUL_IMM_64, NEG_ADD_IMM_64, SHLO_L_IMM_ALT_64, SHLO_R_IMM_ALT_64, ROT_R_64_IMM, ROT_R_64_IMM_ALT, ROT_R_32_IMM, ROT_R_32_IMM_ALT:
		vm.aluImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case CMOV_IZ_IMM, CMOV_NZ_IMM:
		vm.cmovImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case SHLO_R_IMM_32, SHLO_L_IMM_32, SHAR_R_IMM_32, NEG_ADD_IMM_32, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT_32, SHLO_L_IMM_ALT_32, SHAR_R_IMM_ALT_32, SHAR_R_IMM_64, SHAR_R_IMM_ALT_64:
		vm.shiftImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case ADD_32, ADD_64, SUB_32, SUB_64, AND, XOR, OR, MUL_32, MUL_64, MUL_UPPER_S_S, MUL_UPPER_U_U, MUL_UPPER_S_U, DIV_U_32, DIV_U_64, DIV_S_32, DIV_S_64, REM_U_32, REM_U_64, REM_S_32, REM_S_64, CMOV_IZ, CMOV_NZ, SHLO_L_32, SHLO_L_64, SHLO_R_32, SHLO_R_64, SHAR_R_32, SHAR_R_64, SET_LT_U, SET_LT_S, ROT_L_64, ROT_L_32, ROT_R_64, ROT_R_32, AND_INV, OR_INV, XNOR, MAX, MAX_U, MIN, MIN_U:
		vm.aluReg(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case MOVE_REG:
		vm.moveReg(operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case SBRK:
		vm.sbrk(operands)
		break
	// New opcodes
	case LOAD_IMM_64:
		vm.Instructions_with_Arguments_of_One_Register_and_One_Extended_Width_Immediate(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case ADD_IMM_64, SHLO_L_IMM_64, SHLO_R_IMM_64:
		vm.Instructions_with_Arguments_of_Two_Registers_and_One_Immediate(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case COUNT_SET_BITS_64, COUNT_SET_BITS_32, LEADING_ZERO_BITS_64, LEADING_ZERO_BITS_32, TRAILING_ZERO_BITS_64, TRAILING_ZERO_BITS_32, SIGN_EXTEND_8, SIGN_EXTEND_16, ZERO_EXTEND_16, REVERSE_BYTES:
		vm.Instructions_with_Arguments_of_Two_Register(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		log.Debug(vm.logging, "terminated: unknown opcode", "opcode", opcode)
		return nil
	}

	log.Debug(vm.logging, fmt.Sprintf("%d: PC %d %s", stepn, startPC, opcode_str(opcode)), "g", vm.Gas, "reg", vm.ReadRegisters())

	return nil
}

func reverseBytes(b []byte) []byte {
	n := len(b)
	for i := 0; i < n/2; i++ {
		b[i], b[n-1-i] = b[n-1-i], b[i]
	}
	return b
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
func skip(pc uint64, bitmask string) uint64 {
	// Convert the bitmask string to a slice of bytes
	bitmaskBytes := []byte(bitmask)

	// Iterate through the bitmask starting from the given pc position
	var i uint64
	for i = pc + 1; i < pc+25 && i < uint64(len(bitmaskBytes)); i++ {
		// Ensure we do not access out of bounds
		if bitmaskBytes[i] == '1' {
			return i - pc - 1
		}
	}
	// If no '1' is found within the next 24 positions, check the last index
	if i < pc+25 {
		return i - pc - 1
	}
	return uint64(24)
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

func (vm *VM) djump(a uint64) {
	if a == uint64((1<<32)-(1<<16)) {
		vm.terminated = true
		vm.ResultCode = types.PVM_HALT
	} else if a == 0 || a > uint64(len(vm.J)*Z_A) || a%Z_A != 0 {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		// types.PVM_PANIC("OOB")
	} else {
		vm.pc = uint64(vm.J[(a/Z_A)-1])
	}
}

func (vm *VM) ReadRegister(index int) (uint64, uint64) {
	if index < 0 || index >= len(vm.register) {
		return 0, OOB
	}
	return vm.register[index], OK
}

func (vm *VM) WriteRegister(index int, value uint64) uint64 {
	if index < 0 || index >= len(vm.register) {
		return OOB
	}
	vm.register[index] = value
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

	targetIndex := uint64(a/ZA - 1)
	if targetIndex >= uint64(len(vm.code)) {
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
	vm.ResultCode = types.PVM_HOST
	return types.PVM_HOST
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
		return vm.Ram.WriteRAMBytes(uint32(vx), []byte{byte(value)})
	case STORE_IMM_U16:
		value := types.E_l(vy%(1<<16), 2)
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	case STORE_IMM_U32:
		value := types.E_l(vy%(1<<32), 4)
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	case STORE_IMM_U64:
		value := types.E_l(vy, 8)
		return vm.Ram.WriteRAMBytes(uint32(vx), value)
	}

	return OOB
}

// load_imm (opcode 4)
func (vm *VM) loadImm(operands []byte) {
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
	vm.WriteRegister(registerIndexA, uint64(vx))
}

// LOAD_U8, LOAD_U16, LOAD_U32, LOAD_U64, LOAD_I8, LOAD_I16, LOAD_I32
func (vm *VM) load(opcode byte, operands []byte) {
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
	case LOAD_U8:
		value, errCode := vm.Ram.ReadRAMBytes((uint32(vx)), 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, uint64(value[0]))
	case LOAD_I8:
		value, errCode := vm.Ram.ReadRAMBytes((uint32(vx)), 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(uint64(value[0]), 1))
	case LOAD_U16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	case LOAD_I16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(types.DecodeE_l(value), 2))
	case LOAD_U32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	case LOAD_I32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, x_encode(types.DecodeE_l(value), 4))
	case LOAD_U64: // TODO: NEW, check this
		value, errCode := vm.Ram.ReadRAMBytes(uint32(vx), 8)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		vm.WriteRegister(registerIndexA, types.DecodeE_l(value))
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

// STORE_U8, STORE_U16, STORE_U32
func (vm *VM) store(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	valueA, _ := vm.ReadRegister(registerIndexA)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	switch opcode {
	case STORE_U8:
		value := valueA % (1 << 8)
		errCode := vm.Ram.WriteRAMBytes(uint32(vx), []byte{byte(value)})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U16:
		value := types.E_l(uint64(valueA%(1<<16)), 2)
		errCode := vm.Ram.WriteRAMBytes(uint32(vx), value)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U32:
		value := types.E_l(uint64(valueA%(1<<32)), 4)
		errCode := vm.Ram.WriteRAMBytes(uint32(vx), value)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_U64:
		value := types.E_l(uint64(valueA), 8)
		errCode := vm.Ram.WriteRAMBytes(uint32(vx), value)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

// storeImmInd implements STORE_IMM_{U8, U16, U32}
func (vm *VM) storeImmInd(opcode byte, operands []byte) {
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

	valueA, _ := vm.ReadRegister(registerIndexA)

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := x_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly))

	switch opcode {
	case STORE_IMM_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), []byte{byte(vy % (1 << 8))})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy%1<<16), 2))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy%1<<32), 4))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IMM_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueA)+uint32(vx), types.E_l(uint64(vy), 8))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

// Implement loadImmJump logic
func (vm *VM) loadImmJump(operands []byte) {
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
	vy := uint64(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly)))

	vm.WriteRegister(registerIndexA, uint64(vx))

	vm.branch(vy, true)
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []byte) {
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

	valueB, _ := vm.ReadRegister(registerIndexB)

	vm.WriteRegister(registerIndexA, uint64(vx))
	vm.djump((valueB + vy) % (1 << 32))
}

// move_reg (opcode 82)
func (vm *VM) moveReg(operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexD := min(12, int(originalOperands[0])%16)
	registerIndexA := min(12, int(originalOperands[0])/16)

	valueA, _ := vm.ReadRegister(registerIndexA)
	vm.WriteRegister(registerIndexD, valueA)
}

func (vm *VM) sbrk(operands []byte) error {
	// TODO
	fmt.Printf("sbrk not implemented -- operands %v", operands)
	/*	srcIndex, destIndex := splitRegister(operands[0])

		amount := int(operands[0])
		newRAM := make([]byte, len(vm.ram)+amount)
		copy(newRAM, vm.ram)
		vm.ram = newRAM */
	return nil
}

func (vm *VM) branch(vx uint64, condition bool) {
	if condition {
		vm.pc = vx
	} else {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

func (vm *VM) branchCond(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, (int((originalOperands[0])/16) % 8))
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	vy := uint64(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1+lx:1+lx+ly]), uint32(ly)))

	valueA, _ := vm.ReadRegister(registerIndexA)

	switch opcode {
	case BRANCH_EQ_IMM:
		if valueA == vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_NE_IMM:
		if valueA != vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_U_IMM:
		if valueA < vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_S_IMM:
		if z_encode(valueA, 8) < z_encode(vx, 8) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LE_U_IMM:
		if valueA <= vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LE_S_IMM:
		if z_encode(valueA, 8) <= z_encode(vx, 8) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_U_IMM:
		if valueA >= vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_S_IMM:
		if z_encode(valueA, 8) >= z_encode(vx, 8) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GT_U_IMM:
		if valueA > vx {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GT_S_IMM:
		if z_encode(valueA, 8) > z_encode(vx, 8) {
			vm.branch(vy, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	}
}

func (vm *VM) storeInd(opcode byte, operands []byte) {
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

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))

	switch opcode {
	case STORE_IND_U8:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), []byte{byte(valueA % (1 << 8))})
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IND_U16:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA)%(1<<16), 2))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IND_U32:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA)%(1<<32), 4))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	case STORE_IND_U64:
		errCode := vm.Ram.WriteRAMBytes(uint32(valueB)+uint32(vx), types.E_l(uint64(valueA), 8))
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}

}

func (vm *VM) loadInd(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, _ := vm.ReadRegister(registerIndexB)
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
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := uint64(value[0])
		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I8:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 1)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := uint64(z_decode(z_encode(uint64(value[0]), 1), 8))
		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := uint64(types.DecodeE_l(value))
		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I16:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 2)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := z_decode(z_encode(types.DecodeE_l(value), 2), 8)

		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := uint64(types.DecodeE_l(value))
		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_I32:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 4)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := z_decode(z_encode(types.DecodeE_l(value), 4), 8)
		vm.WriteRegister(registerIndexA, result)
	case LOAD_IND_U64:
		value, errCode := vm.Ram.ReadRAMBytes(uint32(valueB)+uint32(vx), 8)
		if errCode != OK {
			vm.ResultCode = types.PVM_FAULT
			vm.terminated = true
			vm.Fault_address = uint32(errCode)
			return
		}
		result := uint64(types.DecodeE_l(value))
		vm.WriteRegister(registerIndexA, result)
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

func splitRegister(operand byte) (int, int) {
	registerIndexA := int(operand & 0xF)
	registerIndexB := int((operand >> 4) & 0xF)
	return registerIndexA, registerIndexB
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, _ := vm.ReadRegister(registerIndexB)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := uint64(x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))) // CHECK

	var result uint64
	switch opcode {
	case ADD_IMM_32:
		result = x_encode((valueB+vx)%(1<<32), 4)
	case AND_IMM:
		result = valueB & vx
	case XOR_IMM:
		result = valueB ^ vx
	case OR_IMM:
		result = valueB | vx
	case MUL_IMM_32:
		result = x_encode((valueB*vx)%(1<<32), 4)
	case SET_LT_U_IMM:
		if valueB < vx {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		if z_encode(valueB, 8) < z_encode(vx, 8) { // CHECK
			result = 1
		} else {
			result = 0
		}
	case MUL_IMM_64:
		result = (valueB * vx)
	case NEG_ADD_IMM_64:
		result = vx - valueB + maxUint64 + 1
	case SHLO_L_IMM_ALT_64:
		result = vx * (1 << (valueB % 64))
	case SHLO_R_IMM_ALT_64:
		result = vx / (1 << (valueB % 64))
	case ROT_R_64_IMM:
		b_valueB := B_encode(valueB)
		shift := int(vx % 64)
		b_valueA := b_valueB[64-shift:] + b_valueB[:64-shift]
		result = x_encode(B_decode(b_valueA), 8)
	case ROT_R_64_IMM_ALT:
		b_vx := B_encode(vx)
		shift := int(valueB % 64)
		b_valueA := b_vx[64-shift:] + b_vx[:64-shift]
		result = x_encode(B_decode(b_valueA), 8)
	case ROT_R_32_IMM:
		b_valueB := B_encode(valueB)[32:]
		shift := int(vx % 32)
		b_valueA := b_valueB[32-shift:] + b_valueB[:32-shift]
		result = x_encode(B_decode(b_valueA), 4)
	case ROT_R_32_IMM_ALT:
		b_vx := B_encode(vx)[32:]
		shift := int(valueB % 32)
		b_valueA := b_vx[32-shift:] + b_vx[:32-shift]
		result = x_encode(B_decode(b_valueA), 4)

	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexA, result)
}

// Implement cmov_nz_imm, cmov_nz_imm
func (vm *VM) cmovImm(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)
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
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexA, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []byte) {
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

	valueB, _ := vm.ReadRegister(registerIndexB)

	var result uint64
	switch opcode {
	case SHLO_R_IMM_32:
		result = x_encode(valueB%(1<<32)/(1<<(vx%32)), 4)
	case SHLO_L_IMM_32:
		result = x_encode(valueB*(1<<(vx%32))%(1<<32), 4)
	case SHAR_R_IMM_32:
		numerator := z_encode(valueB%(1<<32), 4)
		denominator := int64(1 << (vx % 32))
		floorQuotient := FloorDiv(numerator, denominator)
		result = z_decode(floorQuotient, 8)
	case NEG_ADD_IMM_32:
		result = x_encode((vx+uint64(1<<32)-valueB)%uint64(1<<32), 4)
	case SET_GT_U_IMM:
		if valueB > vx {
			result = 1
		} else {
			result = 0
		}
	case SET_GT_S_IMM:
		if z_encode(valueB, 8) > z_encode(vx, 8) {
			result = 1
		} else {
			result = 0
		}
	case SHLO_L_IMM_ALT_32:
		result = x_encode((vx*(1<<(valueB%32)))%(1<<32), 4)
	case SHLO_R_IMM_ALT_32:
		result = x_encode((vx%(1<<32))/(1<<(valueB%32)), 4)
	case SHAR_R_IMM_ALT_32:
		numerator := z_encode(vx%(1<<32), 4)
		denominator := int64(1 << (valueB % 32))
		floorQuotient := FloorDiv(numerator, denominator)
		result = z_decode(floorQuotient, 8)
	case SHAR_R_IMM_64:
		numerator := z_encode(valueB, 8)
		denominator := int64(1 << (vx % 64))
		floorQuotient := FloorDiv(numerator, denominator)
		result = z_decode(floorQuotient, 8)
	case SHAR_R_IMM_ALT_64:
		numerator := z_encode(vx, 8)
		denominator := int64(1 << (valueB % 64))
		floorQuotient := FloorDiv(numerator, denominator)
		result = z_decode(floorQuotient, 8)
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexA, result)
}

// GP.0.5.2-A.7
func z_encode(a uint64, n uint32) int64 {
	modValue := new(big.Int).Lsh(big.NewInt(1), uint(8*n-1))
	bigA := new(big.Int).SetUint64(a)

	if bigA.Cmp(modValue) < 0 {
		return bigA.Int64()
	}

	fullMod := new(big.Int).Lsh(big.NewInt(1), uint(8*n))
	result := new(big.Int).Sub(bigA, fullMod)
	return result.Int64()
}

// GP.0.5.2-A.8
func z_decode(a int64, n uint32) uint64 {
	mod := new(big.Int).Lsh(big.NewInt(1), uint(8*n))
	bigA := big.NewInt(a)

	result := new(big.Int).Add(mod, bigA)
	result.Mod(result, mod)

	return result.Uint64()
}

// GP.0.5.4-A.9
func B_encode(n uint64) string {
	return fmt.Sprintf("%064b", n)
}

// GP.0.5.4-A.10
func B_decode(s string) uint64 {
	n, _ := strconv.ParseUint(s, 2, 64)
	return n
}

// GP_.0.3.8(224)
func x_encode(x uint64, n uint32) uint64 {
	if n == 0 {
		return 0
	}

	// 1. Calculate 2^(8*n-1)
	modValue := new(big.Int).Lsh(big.NewInt(1), uint(8*n-1))

	// 2. Calculate 2^64 - 2^(8*n)
	two64 := new(big.Int).Lsh(big.NewInt(1), 64)
	two8n := new(big.Int).Lsh(big.NewInt(1), uint(8*n))
	factor := new(big.Int).Sub(two64, two8n)

	// 3. Calculate x / 2^(8*n-1)
	bigX := new(big.Int).SetUint64(x)
	quotient := new(big.Int).Div(bigX, modValue)

	// 4. Calculate x + quotient * factor
	temp := new(big.Int).Mul(quotient, factor)
	result := new(big.Int).Add(bigX, temp)

	// 5. Ensure the result can be returned as uint64
	return result.Uint64()
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []byte) {
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
	vx := uint64(int64(vm.pc) + z_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx)))

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)

	switch opcode {
	case BRANCH_EQ:
		if valueA == valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_NE:

		if valueA != valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_U:
		if valueA < valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_LT_S:
		if z_encode(valueA, 8) < z_encode(valueB, 8) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_U:
		if valueA >= valueB {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	case BRANCH_GE_S:
		if z_encode(valueA, 8) >= z_encode(valueB, 8) {
			vm.branch(vx, true)
		} else {
			vm.pc += uint64(1 + len(operands))
		}
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
}

// Implement ALU operations with register values
func (vm *VM) aluReg(opcode byte, operands []byte) {

	registerIndexA := min(12, int(operands[0])%16)
	registerIndexB := min(12, int(operands[0])/16)
	registerIndexD := min(12, int(operands[1]))

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)
	valueD, _ := vm.ReadRegister(registerIndexD)

	var result uint64
	switch opcode {
	case ADD_32:
		result = x_encode((valueA+valueB)%(1<<32), 4)
	case ADD_64:
		result = valueA + valueB
	case SUB_32:
		result = x_encode((valueA+(1<<32)-(valueB%(1<<32)))%(1<<32), 4)
	case SUB_64:
		result = valueA + maxUint64 - valueB + 1
	case AND:
		result = valueA & valueB
	case XOR:
		result = valueA ^ valueB
	case OR:
		result = valueA | valueB
	case MUL_32:
		result = x_encode((valueA*valueB)%(1<<32), 4)
	case MUL_64:
		result = valueA * valueB
	case MUL_UPPER_S_S:
		A := new(big.Int).SetInt64(z_encode(valueA, 8))
		B := new(big.Int).SetInt64(z_encode(valueB, 8))
		res := new(big.Int).Mul(A, B)
		res.Rsh(res, 64)
		result = z_decode(res.Int64(), 8)
	case MUL_UPPER_U_U:
		A := new(big.Int).SetUint64(valueA)
		B := new(big.Int).SetUint64(valueB)
		res := new(big.Int).Mul(A, B)
		res.Rsh(res, 64)
		result = res.Uint64()
	case MUL_UPPER_S_U:
		A := new(big.Int).SetInt64(z_encode(valueA, 8))
		B := new(big.Int).SetUint64(valueB)
		res := new(big.Int).Mul(A, B)
		res.Rsh(res, 64)
		result = z_decode(res.Int64(), 8)
	case DIV_U_32:
		if valueB == 0 {
			result = maxUint64
		} else {
			result = x_encode(valueA%(1<<32)/valueB%(1<<32), 4)
		}
	case DIV_U_64:
		if valueB == 0 {
			result = maxUint64
		} else {
			result = valueA / valueB
		}
	case DIV_S_32:
		S_valueA := z_encode(valueA%(1<<32), 4)
		S_valueB := z_encode(valueB%(1<<32), 4)
		if valueB == 0 {
			result = maxUint64
		} else if S_valueA == -(1<<31) && S_valueB == -1 {
			result = uint64(S_valueA)
		} else {
			result = z_decode(S_valueA/S_valueB, 8)
		}
	case DIV_S_64:
		if valueB == 0 {
			result = maxUint64
		} else if z_encode(valueA, 8) == -(1<<63) && z_encode(valueB, 8) == -1 {
			result = valueA
		} else {
			numerator := z_encode(valueA, 8)
			denominator := z_encode(valueB, 8)
			floorQuotient := FloorDiv(numerator, denominator)
			result = z_decode(floorQuotient, 8)
		}
	case REM_U_32:
		if valueB%(1<<32) == 0 {
			result = x_encode(valueA%(1<<32), 4) // not sure whether this is correct
		} else {
			result = x_encode((valueA%(1<<32))%(valueB%(1<<32)), 4)
		}
	case REM_U_64:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
	case REM_S_32:
		S_valueA := z_encode(valueA%(1<<32), 4)
		S_valueB := z_encode(valueB%(1<<32), 4)

		if S_valueB == 0 {
			result = z_decode(S_valueA, 8)
		} else if S_valueA == -(1<<31) && S_valueB == -1 {
			result = 0
		} else {
			result = z_decode(S_valueA%S_valueB, 8)
		}
	case REM_S_64:

		if valueB == 0 {
			result = valueA
		} else if z_encode(valueA, 8) == -(1<<63) && z_encode(valueB, 8) == -1 {
			result = 0
		} else {
			result = z_decode(z_encode(valueA, 8)%z_encode(valueB, 8), 8)
		}
	case CMOV_IZ:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueD
		}
	case CMOV_NZ:
		if valueB != 0 {
			result = valueA
		} else {
			result = valueD
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

	case SHLO_L_32:
		result = x_encode(valueA*(1<<(valueB%32))%(1<<32), 4)
	case SHLO_L_64:
		result = valueA * (1 << (valueB % 64))
	case SHLO_R_32:
		result = x_encode((valueA%(1<<32))/(1<<(valueB%32)), 4)
	case SHLO_R_64:
		result = valueA / (1 << (valueB % 64))
	case SHAR_R_32:
		numerator := z_encode(valueA%(1<<32), 4)
		denominator := int64(1 << (valueB % 32))
		floorQuotient := FloorDiv(numerator, denominator)
		result = z_decode(floorQuotient, 8)
	case SHAR_R_64:
		// numerator := z_encode(valueA, 8)
		// denominator := int64(uint64(1) << (valueB % 64))
		// floorQuotient := FloorDiv(numerator, denominator)
		// result = z_decode(floorQuotient, 8)

		numeratorVal := z_encode(valueA, 8)
		var bigNum big.Int
		bigNum.SetInt64(numeratorVal)
		shift := uint(valueB % 64)
		var bigDen big.Int
		bigDen.SetUint64(1)
		bigDen.Lsh(&bigDen, shift)
		floorQuo := FloorDivBig(&bigNum, &bigDen)
		result = z_decode(floorQuo.Int64(), 8)
	case ROT_L_64:
		b_valueA := B_encode(valueA)
		shift := int(valueB % 64)
		b_valueD := b_valueA[shift:] + b_valueA[:shift]
		result = B_decode(b_valueD)
	case ROT_R_64:
		b_valueA := B_encode(valueA)
		shift := int(valueB % 64)
		b_valueD := b_valueA[64-shift:] + b_valueA[:64-shift]
		result = B_decode(b_valueD)
	case ROT_L_32:
		full64 := B_encode(valueA)
		b_low32 := full64[32:]
		shift := int(valueB % 32)
		b_rot32 := b_low32[shift:] + b_low32[:shift]
		result = x_encode(B_decode(b_rot32), 4)
	case ROT_R_32:
		full64 := B_encode(valueA)
		b_low32 := full64[32:]
		shift := int(valueB % 32)
		b_rot32 := b_low32[32-shift:] + b_low32[:32-shift]
		result = x_encode(B_decode(b_rot32), 4)

	case AND_INV:
		result = valueA & (^valueB)
	case OR_INV:
		result = valueA | (^valueB)
	case XNOR:
		result = ^(valueA ^ valueB)
	case MAX:
		result = uint64(max(z_encode(valueA, 8), z_encode(valueB, 8)))
	case MAX_U:
		result = max(valueA, valueB)
	case MIN:
		result = uint64(min(z_encode(valueA, 8), z_encode(valueB, 8)))
	case MIN_U:
		result = min(valueA, valueB)
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexD, result)
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (vm *VM) CreateVM(serviceAcct uint32, code []byte, i uint64) uint32 {
	maxN := uint32(0)
	for n := range vm.VMs {
		if n > maxN {
			maxN = n
		}
	}
	if vm.VMs == nil {
		vm.VMs = make(map[uint32]*VM)
	}
	fmt.Printf("CreateVM: %d\n", maxN)
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

func (vm *VM) HostCheat(input string) (errcode uint64) {
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
	} else if input == "zero" {
		vm.hostZero()
	}
	return errcode
}

func (vm *VM) Instructions_with_Arguments_of_Two_Register(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexD := min(12, int(originalOperands[0])%16)
	registerIndexA := min(12, int(originalOperands[0])/16)

	valueA, _ := vm.ReadRegister(registerIndexA)

	var result uint64
	switch opcode {
	case COUNT_SET_BITS_64:
		b_valueA := B_encode(valueA)
		count := strings.Count(b_valueA, "1")
		result = uint64(count)
	case COUNT_SET_BITS_32:
		b_full64 := B_encode(valueA)
		b_low32 := b_full64[32:]
		count := strings.Count(b_low32, "1")
		result = uint64(count)
	case LEADING_ZERO_BITS_64:
		b_valueA := B_encode(valueA)
		idx := strings.Index(b_valueA, "1")
		if idx == -1 {
			result = 64
		} else {
			result = uint64(idx)
		}
	case LEADING_ZERO_BITS_32:
		b_full64 := B_encode(valueA)
		b_low32 := b_full64[32:]
		idx := strings.Index(b_low32, "1")
		if idx == -1 {
			result = 32
		} else {
			result = uint64(idx)
		}
	case TRAILING_ZERO_BITS_64:
		b_valueA := B_encode(valueA)
		idx := strings.LastIndex(b_valueA, "1")
		if idx == -1 {
			result = 64
		} else {
			result = uint64(63 - idx)
		}
	case TRAILING_ZERO_BITS_32:
		b_full64 := B_encode(valueA)
		b_low32 := b_full64[32:]
		idx := strings.LastIndex(b_low32, "1")
		if idx == -1 {
			result = 32
		} else {
			result = uint64(31 - idx)
		}
	case SIGN_EXTEND_8:
		result = z_decode(z_encode(valueA%(1<<8), 1), 8)
	case SIGN_EXTEND_16:
		result = z_decode(z_encode(valueA%(1<<16), 2), 8)
	case ZERO_EXTEND_16:
		result = valueA % (1 << 16)
	case REVERSE_BYTES:
		result = types.DecodeE_l(reverseBytes(types.E_l(valueA, 8)))
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}
	vm.WriteRegister(registerIndexD, result)
}

func (vm *VM) Instructions_with_Arguments_of_One_Register_and_One_Extended_Width_Immediate(opcode byte, operands []byte) {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := 8
	vx := types.DecodeE_l(originalOperands[1 : 1+lx])
	vm.WriteRegister(registerIndexA, uint64(vx))
}

func (vm *VM) Instructions_with_Arguments_of_Two_Registers_and_One_Immediate(opcode byte, operands []byte) {
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

	valueA, _ := vm.ReadRegister(registerIndexA)
	valueB, _ := vm.ReadRegister(registerIndexB)

	vx := x_encode(types.DecodeE_l(originalOperands[1:1+lx]), uint32(lx))
	var result uint64
	switch opcode {
	case ADD_IMM_64:
		valueA = valueB + vx
		result = valueA
	case SHLO_L_IMM_64:
		valueA = x_encode(valueB*(1<<(vx%64)), 8)
		result = valueA
	case SHLO_R_IMM_64:
		valueA = x_encode(valueB/(1<<(vx%64)), 8)
		result = valueA
	default:
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
	}

	vm.WriteRegister(registerIndexA, result)
}
