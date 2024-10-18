package pvm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug = false

	regSize  = 13
	numCores = types.TotalCores
	W_E      = types.W_E
	W_S      = types.W_S
	W_X      = 1024
	M        = 128
	V        = 1023
	D        = 28800
	Z_A      = 2
	Z_P      = (1 << 14)
	Z_Q      = (1 << 16)
	Z_I      = (1 << 24)
)

// PageMap
type PageMap struct {
	Address    uint32 `json:"address"`
	Length     uint32 `json:"length"`
	IsWritable bool   `json:"is-writable"`
	IsReadable bool   `json:"is-readable"`
}

// Page holds a byte array
type Page struct {
	Address  uint32 `json:"address"`
	Contents []byte `json:"contents"`
}

// Define access mode enumeration
type AccessMode int

const (
	NoAccess AccessMode = iota
	ReadOnly
	ReadWrite
)

// PermissionRange represents a range of addresses with a specific access mode
type PermissionRange struct {
	Start uint32
	End   uint32
	Mode  AccessMode
}

// Modified RAM structure
type RAM struct {
	memory      map[uint32]byte   // Actual memory data
	permissions []PermissionRange // Access permissions represented as ranges
}

// Initialize RAM
func NewRAM() *RAM {
	return &RAM{
		memory:      make(map[uint32]byte),
		permissions: make([]PermissionRange, 0),
	}
}

// SetAccessMode sets the access mode for a specified range in the VM's RAM
func (vm *VM) SetAccessMode(start, end uint32, mode AccessMode) error {
	if vm.ram == nil {
		return errors.New("RAM not initialized")
	}
	if start > end {
		return errors.New("Invalid range")
	}
	vm.ram.permissions = append(vm.ram.permissions, PermissionRange{
		Start: start,
		End:   end,
		Mode:  mode,
	})
	return nil
}

// Merge permission ranges, should be called after all SetAccessMode calls
func (ram *RAM) FinalizePermissions() {
	if len(ram.permissions) == 0 {
		return
	}
	// Sort
	sort.Slice(ram.permissions, func(i, j int) bool {
		return ram.permissions[i].Start < ram.permissions[j].Start
	})
	// Merge
	merged := []PermissionRange{}
	current := ram.permissions[0]
	for _, pr := range ram.permissions[1:] {
		if pr.Start <= current.End && pr.Mode == current.Mode {
			if pr.End > current.End {
				current.End = pr.End
			}
		} else {
			merged = append(merged, current)
			current = pr
		}
	}
	merged = append(merged, current)
	ram.permissions = merged
}

// Get the access mode for a specific address
func (ram *RAM) GetAccessMode(addr uint32) AccessMode {
	left, right := 0, len(ram.permissions)-1
	for left <= right {
		mid := (left + right) / 2
		pr := ram.permissions[mid]
		if addr >= pr.Start && addr < pr.End {
			return pr.Mode
		} else if addr < pr.Start {
			right = mid - 1
		} else {
			left = mid + 1
		}
	}
	return NoAccess
}

// Write a single byte to the specified address in the VM's RAM
func (vm *VM) writeRAM(addr uint32, data byte) uint32 {
	if vm.ram == nil {
		fmt.Println("RAM not initialized")
		return PANIC
	}
	if vm.ram.GetAccessMode(addr) != ReadWrite {
		fmt.Println("Write access denied")
		return PANIC
	}
	vm.ram.memory[addr] = data
	return OK
}

// Read a single byte from the specified address in the VM's RAM
func (vm *VM) readRAM(addr uint32) (byte, uint32) {
	if vm.ram == nil {
		fmt.Println("RAM not initialized")
		return 0, PANIC
	}
	mode := vm.ram.GetAccessMode(addr)
	if mode == NoAccess {
		fmt.Println("Read access denied")
		return 0, PANIC
	}
	data, exists := vm.ram.memory[addr]
	if !exists {
		data = 0
	}
	return data, OK
}

// Write multiple bytes to the specified range in the VM's RAM
func (vm *VM) writeRAMBytes(addr uint32, data []byte) uint32 {
	if vm.ram == nil {
		fmt.Println("RAM not initialized")
		return PANIC
	}
	for i := uint32(0); i < uint32(len(data)); i++ {
		if vm.ram.GetAccessMode(addr+i) != ReadWrite {
			fmt.Println("Write access denied for one or more addresses")
			return PANIC
		}
		vm.ram.memory[addr+i] = data[i]
	}
	return OK
}

// Read multiple bytes from the specified range in the VM's RAM
func (vm *VM) readRAMBytes(addr uint32, length int) ([]byte, uint32) {
	if vm.ram == nil {
		fmt.Println("RAM not initialized")
		return nil, PANIC
	}
	if length < 0 {
		fmt.Println("Invalid length")
		return nil, PANIC
	}
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		if vm.ram.GetAccessMode(addr+uint32(i)) == NoAccess {
			fmt.Println("Read access denied for one or more addresses")
			return nil, PANIC
		}
		data, exists := vm.ram.memory[addr+uint32(i)]
		if !exists {
			data = 0
		}
		result[i] = data
	}
	return result, OK
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
	hostCall     bool   // ̵h in GP
	host_func_id uint32 // h in GP
	// ram                 map[uint32][4096]byte
	ram                 *RAM
	register            []uint32
	ξ                   uint64
	hostenv             types.HostEnv
	writable_ram_start  uint32
	writable_ram_length uint32
	service_index       uint32

	// Pagemap definne whether the page is writable or not
	pagemaps []PageMap

	VMs map[uint32]*VM

	// Work Package Inputs
	Extrinsics [][]byte
	payload    []byte
	Imports    [][]byte
	Exports    [][]byte

	// SOLICITS+FORGETS
	Solicits []Solicit
	Forgets  []Forgets

	X types.XContext
	Y types.XContext

	// Invocation funtions entry point
	EntryPoint uint32

	// Malicious Setting
	IsMalicious bool
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
	FALLTHROUGH = 17
)

// A.5.2. Instructions with Arguments of One Immediate.
const (
	ECALLI = 78
)

// A.5.3. Instructions with Arguments of Two Immediates.
const (
	STORE_IMM_U8  = 62
	STORE_IMM_U16 = 79
	STORE_IMM_U32 = 38
)

// A.5.4. Instructions with Arguments of One Offset.
const (
	JUMP = 5
)

// A.5.5. Instructions with Arguments of One Register & One Immediate.
const (
	JUMP_IND  = 19
	LOAD_IMM  = 4
	LOAD_U8   = 60
	LOAD_U16  = 76
	LOAD_U32  = 10
	STORE_U8  = 71
	STORE_U16 = 69
	STORE_U32 = 22
)

// A.5.6. Instructions with Arguments of One Register & Two Immediates.
const (
	STORE_IMM_IND_U8  = 26
	STORE_IMM_IND_U16 = 54
	STORE_IMM_IND_U32 = 13
)

// A.5.7. Instructions with Arguments of One Register, One Immediate and One Offset.
const (
	LOAD_IMM_JUMP   = 6
	BRANCH_EQ_IMM   = 7
	BRANCH_NE_IMM   = 15
	BRANCH_LT_U_IMM = 44
	BRANCH_LE_U_IMM = 59
	BRANCH_GE_U_IMM = 52
	BRANCH_GT_U_IMM = 50
	BRANCH_LT_S_IMM = 32
	BRANCH_LE_S_IMM = 46
	BRANCH_GE_S_IMM = 45
	BRANCH_GT_S_IMM = 53
)

// A.5.8. Instructions with Arguments of Two Registers.
const (
	MOVE_REG = 82
	SBRK     = 87
)

// A.5.9. Instructions with Arguments of Two Registers & One Immediate.
const (
	STORE_IND_U8      = 16
	STORE_IND_U16     = 29
	STORE_IND_U32     = 3
	LOAD_IND_U8       = 11
	LOAD_IND_I8       = 21
	LOAD_IND_U16      = 37
	LOAD_IND_I16      = 33
	LOAD_IND_U32      = 1
	ADD_IMM           = 2
	AND_IMM           = 18
	XOR_IMM           = 31
	OR_IMM            = 49
	MUL_IMM           = 35
	MUL_UPPER_S_S_IMM = 65
	MUL_UPPER_U_U_IMM = 63
	SET_LT_U_IMM      = 27
	SET_LT_S_IMM      = 56
	SHLO_L_IMM        = 9
	SHLO_R_IMM        = 14
	SHAR_R_IMM        = 25
	NEG_ADD_IMM       = 40
	SET_GT_U_IMM      = 39
	SET_GT_S_IMM      = 61
	SHLO_R_IMM_ALT    = 72
	SHLO_L_IMM_ALT    = 75
	SHAR_R_IMM_ALT    = 80
	CMOV_IMM_IZ       = 85 //added base on 2024/7/1 GP
	CMOV_IMM_NZ       = 86 //added base on 2024/7/1 GP
)

// A.5.10. Instructions with Arguments of Two Registers & One Offset.
const (
	BRANCH_EQ   = 24
	BRANCH_NE   = 30
	BRANCH_LT_U = 47
	BRANCH_LT_S = 48
	BRANCH_GE_U = 41
	BRANCH_GE_S = 43
)

// A.5.11. Instruction with Arguments of Two Registers and Two Immediates.
const (
	LOAD_IMM_JUMP_IND = 42
)

// A.5.12. Instructions with Arguments of Three Registers.
const (
	ADD_REG           = 8
	SUB_REG           = 20
	AND_REG           = 23
	XOR_REG           = 28
	OR_REG            = 12
	MUL_REG           = 34
	MUL_UPPER_S_S_REG = 67
	MUL_UPPER_U_U_REG = 57
	MUL_UPPER_S_U_REG = 81
	DIV_U             = 68
	DIV_S             = 64
	REM_U             = 73
	REM_S             = 70
	SET_LT_U          = 36
	SET_LT_S          = 58
	SHLO_L            = 55
	SHLO_R            = 51
	SHAR_R            = 77
	CMOV_IZ           = 83
	CMOV_NZ           = 84
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
	NONE = (1 << 32) - 1  // 2^32 - 1
	OOB  = (1 << 32) - 2  // 2^32 - 2
	WHO  = (1 << 32) - 3  // 2^32 - 3
	FULL = (1 << 32) - 4  // 2^32 - 4
	CORE = (1 << 32) - 5  // 2^32 - 5
	CASH = (1 << 32) - 6  // 2^32 - 6
	LOW  = (1 << 32) - 7  // 2^32 - 7
	HIGH = (1 << 32) - 8  // 2^32 - 8
	WAT  = (1 << 32) - 9  // 2^32 - 9
	HUH  = (1 << 32) - 10 // 2^32 - 10
	OK   = 0              // 0

	HALT  = 0              // 0
	PANIC = (1 << 32) - 12 // 2^32 - 12
	FAULT = (1 << 32) - 13 // 2^32 - 13
	HOST  = (1 << 32) - 14 // 2^32 - 14

	BAD = 1111
	S   = 2222
	BIG = 3333
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
	if debug {
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

	if debug {
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

	if debug {
		fmt.Printf("JSize=%d\n", j_size)
		fmt.Printf("Z=%d\n", z)
		fmt.Printf("CSize=%d\n", c_size)
		fmt.Println("Jump Table: ", j_array)
		fmt.Printf("Code: %v\n", c_byte)
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

func P_func(x uint32) uint32 {
	return Z_P * (x/Z_P + 1)
}

func Q_func(x uint32) uint32 {
	return Z_Q * (x/Z_Q + 1)
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint32, initialPC uint32, pagemap []PageMap, pages []Page, hostENV types.HostEnv) *VM {
	if len(code) == 0 {
		panic("NO CODE\n")
	}
	p, o_size, w_size, z, s, o_byte, w_byte := DecodeProgram(code)
	_ = o_byte
	_ = w_byte

	vm := &VM{
		JSize:    p.JSize,
		Z:        p.Z,
		J:        p.J,
		code:     p.Code,
		bitmask:  p.K[0], // pass in bitmask K
		register: make([]uint32, regSize),
		pc:       initialPC,
		// ram:           make(map[uint32][4096]byte),
		ram:           NewRAM(),
		hostenv:       hostENV, //check if we need this
		Exports:       make([][]byte, 0),
		service_index: service_index,
	}
	// for _, pg := range pages {
	// 	vm.writeRAMBytes(pg.Address, pg.Contents)
	// }
	copy(vm.register, initialRegs)

	// Standard Program Initialization
	// check condition for standard program GP-0.4.0 (261))
	condition := uint64(5*Z_Q+Q_func(o_size)+Q_func(w_size+z*Z_P)+Q_func(s)+Z_I) <= uint64((1 << 32))
	if condition {
		if debug {
			fmt.Println("Standard Program Initialization...")
		}
		// set up the initial values and access mode for the RAM
		vm.SetAccessMode(Z_Q+o_size, Z_Q+P_func(o_size), ReadOnly)
		vm.SetAccessMode(2*Z_Q+Q_func(o_size), 2*Z_Q+Q_func(o_size)+w_size, ReadWrite)
		vm.SetAccessMode(2*Z_Q+Q_func(o_size)+w_size, 2*Z_Q+Q_func(o_size)+P_func(w_size)+z*Z_P, ReadWrite)
		vm.SetAccessMode((1<<32)-2*Z_Q-Z_I-P_func(s), (1<<32)-2*Z_Q-Z_I, ReadWrite)
		argument_data_a := []byte{0, 0, 0, 0} // initialize a with random data, should be replaced with actual data
		vm.SetAccessMode((1<<32)-2*Z_Q-Z_I, (1<<32)-Z_Q-Z_I+uint32(len(argument_data_a)), ReadWrite)
		vm.writeRAMBytes((1<<32)-Z_Q-Z_I, argument_data_a)
		vm.SetAccessMode((1<<32)-2*Z_Q-Z_I, (1<<32)-Z_Q-Z_I+uint32(len(argument_data_a)), ReadOnly)
		vm.SetAccessMode((1<<32)-Z_Q-Z_I+uint32(len(argument_data_a)), (1<<32)-Z_Q-Z_I+P_func(uint32(len(argument_data_a))), ReadOnly)
		vm.ram.FinalizePermissions()

		// set up the initial values for the registers
		vm.writeRegister(0, (1<<32)-(1<<16))
		vm.writeRegister(1, (1<<32)-2*Z_Q-Z_I)
		vm.writeRegister(7, (1<<32)-Z_Q-Z_I)
		vm.writeRegister(8, uint32(len(argument_data_a)))

		return vm
	} else {
		panic("Standard Program Initialization Error\n")
	}
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
	return NewVM(serviceIndex, code, []uint32{}, i, []PageMap{}, []Page{}, hostENV)
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

// for hostfuntion test
// func NewVMforhostfun(initialRegs []uint32, pagemap []PageMap, pages []Page, hostENV types.HostEnv) *VM {

// 	vm := &VM{
// 		register: make([]uint32, regSize),
// 		ram:      make(map[uint32][4096]byte),
// 		hostenv:  hostENV,
// 	}
// 	for _, pg := range pages {
// 		vm.writeRAMBytes(pg.Address, pg.Contents)
// 	}

// 	if pagemap[0].IsWritable {
// 		vm.writable_ram_start = uint32(pagemap[0].Address)
// 		vm.writable_ram_length = uint32(pagemap[0].Length)
// 	}
// 	copy(vm.register, initialRegs)
// 	return vm
// }

// Execute runs the program until it terminates
func (vm *VM) Execute(entryPoint int) error {
	vm.pc = uint32(entryPoint)
	for !vm.terminated {
		if err := vm.step(); err != nil {
			return err
		}
		// host call invocation
		if vm.hostCall {
			if debug {
				fmt.Println("Invocate Host Function: ", vm.host_func_id)
			}
			vm.InvokeHostCall(int(vm.host_func_id))
			vm.hostCall = false
			vm.terminated = false
		}
		if debug {
			fmt.Println("-----------------------------------------------------------------------")
		}
	}
	if debug {
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
func (vm *VM) SetArgumentInputs(a []byte) error {
	vm.writeRegister(0, 0)
	vm.writeRegister(1, 0xFFFF0000) // 2^32 - 2^16
	vm.writeRegister(2, 0xFEFE0000) // 2^32 - 2 * 2^16 - 2^24
	for r := 3; r < 10; r++ {
		vm.writeRegister(r, 0)
	}
	vm.writeRegister(10, 0xFEFF0000) // 2^32 - 2^16 - 2^24
	vm.writeRegister(11, uint32(len(a)))
	for r := 12; r < 16; r++ {
		vm.writeRegister(r, 0)
	}
	vm.writeRAMBytes(0xFEFF0000, a)
	return nil
}

func (vm *VM) GetArgumentOutputs() (r types.Result, res uint32) {
	o, _ := vm.readRegister(10)
	l, _ := vm.readRegister(11)

	output, res := vm.readRAMBytes(o, int(l))
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
	if debug {
		fmt.Printf("pc: %d opcode: %d - operands: %v, len(operands) = %d\n", vm.pc, opcode, operands, len_operands)
	}
	switch instr {
	case TRAP:
		if debug {
			fmt.Printf("TERMINATED\n")
		}
		if instr == TRAP {
			vm.resultCode = types.RESULT_PANIC
		} else {
			vm.resultCode = types.RESULT_OK
		}
		vm.terminated = true
	case FALLTHROUGH:
		if debug {
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
		vx := z_encode(uint32(types.DecodeE_l(originalOperands[0:lx])), uint32(lx))

		if debug {
			fmt.Println("Jump to: ", uint32(int64(vm.pc)+vx))
		}
		errCode := vm.branch(uint32(int64(vm.pc)+vx), true)
		if debug {
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

		valueA, errCode := vm.readRegister(registerIndexA)
		if debug {
			fmt.Println("Djump to: ", uint32(valueA+vx))
		}
		vm.djump(valueA + vx)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP:
		errCode := vm.loadImmJump(operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case LOAD_IMM_JUMP_IND:
		errCode := vm.loadImmJumpInd(operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ, BRANCH_NE, BRANCH_LT_U, BRANCH_LT_S, BRANCH_GE_U, BRANCH_GE_S:
		errCode := vm.branchReg(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case BRANCH_EQ_IMM, BRANCH_NE_IMM, BRANCH_LT_U_IMM, BRANCH_LT_S_IMM, BRANCH_LE_U_IMM, BRANCH_LE_S_IMM, BRANCH_GE_U_IMM, BRANCH_GE_S_IMM, BRANCH_GT_U_IMM, BRANCH_GT_S_IMM:
		errCode := vm.branchCond(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
	case ECALLI:
		errCode := vm.ecalli(operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

		if debug {
			fmt.Printf("TERMINATED\n")
		}
		if errCode != OK {
			vm.resultCode = types.RESULT_FAULT
			vm.terminated = true
		}
	case STORE_IMM_U8, STORE_IMM_U16, STORE_IMM_U32:
		errCode := vm.storeImm(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IMM:
		errCode := vm.loadImm(operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_U8, LOAD_U16, LOAD_U32:
		errCode := vm.load(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_U8, STORE_U16, STORE_U32:
		errCode := vm.store(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IMM_IND_U8, STORE_IMM_IND_U16, STORE_IMM_IND_U32:
		errCode := vm.storeImmInd(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case STORE_IND_U8, STORE_IND_U16, STORE_IND_U32:
		errCode := vm.storeInd(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case LOAD_IND_U8, LOAD_IND_I8, LOAD_IND_U16, LOAD_IND_I16, LOAD_IND_U32:
		errCode := vm.loadInd(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case ADD_IMM, AND_IMM, XOR_IMM, OR_IMM, MUL_IMM, MUL_UPPER_S_S_IMM, MUL_UPPER_U_U_IMM, SET_LT_U_IMM, SET_LT_S_IMM:
		errCode := vm.aluImm(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case CMOV_IMM_IZ, CMOV_IMM_NZ:
		errCode := vm.cmovImm(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands

	case SHLO_R_IMM, SHLO_L_IMM, SHAR_R_IMM, NEG_ADD_IMM, SET_GT_U_IMM, SET_GT_S_IMM, SHLO_R_IMM_ALT, SHLO_L_IMM_ALT, SHAR_R_IMM_ALT:
		errCode := vm.shiftImm(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case ADD_REG, SUB_REG, AND_REG, XOR_REG, OR_REG, MUL_REG, MUL_UPPER_S_S_REG, MUL_UPPER_U_U_REG, MUL_UPPER_S_U_REG, DIV_U, DIV_S, REM_U, REM_S, CMOV_IZ, CMOV_NZ, SHLO_L, SHLO_R, SHAR_R, SET_LT_U, SET_LT_S:
		errCode := vm.aluReg(opcode, operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case MOVE_REG:
		errCode := vm.moveReg(operands)
		if debug {
			fmt.Println("Error: ", errCode)
		}
		// vm.writeRegister(0, errCode)
		vm.pc += 1 + len_operands
	case SBRK:
		vm.sbrk(operands)
		break
	default:
		vm.terminated = true
		vm.resultCode = types.RESULT_PANIC
		if debug {
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
func (vm *VM) handleHostCall(opcode byte, operands []byte) (bool, uint32) {
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

// func (vm *VM) readRAM(address uint32) (byte, uint32) {
// 	// Determine the page and offset within the page
// 	page := address / 4096
// 	offset := address % 4096

// 	// Retrieve the page from the RAM, return 0 if the page does not exist
// 	pageData, exists := vm.ram[page]
// 	if !exists {
// 		return 0, OK
// 	}

// 	// Return the byte at the specified address and the status code OK
// 	return pageData[offset], OK
// }

// func (vm *VM) writeRAM(address uint32, value byte) uint32 {
// 	// Determine the page and offset within the page
// 	page := address / 4096
// 	offset := address % 4096

// 	// Retrieve the page from the RAM or create a new page if it doesn't exist
// 	pageData, exists := vm.ram[page]
// 	if !exists {
// 		pageData = [4096]byte{}
// 	}

// 	// Write the byte to the specified address
// 	pageData[offset] = value

// 	// Store the updated page back in the RAM
// 	vm.ram[page] = pageData

// 	// Return the status code OK
// 	return OK
// }

// func (vm *VM) checkWriteable(address uint32) bool {
// 	if vm.pagemaps == nil {
// 		for _, pagemap := range vm.pagemaps {
// 			if address >= pagemap.Address && address < pagemap.Address+pagemap.Length {
// 				return pagemap.IsWritable
// 			}
// 		}
// 	}
// 	return true
// }

// func (vm *VM) checkReadable(address uint32) bool {
// 	if vm.pagemaps == nil {
// 		for _, pagemap := range vm.pagemaps {
// 			if address >= pagemap.Address && address < pagemap.Address+pagemap.Length {
// 				return pagemap.IsReadable || pagemap.IsWritable
// 			}
// 		}
// 	}
// 	return true
// }

// func (vm *VM) readRAMBytes(address uint32, numBytes int) ([]byte, uint32) {
// 	var result []byte
// 	remainingBytes := numBytes
// 	currentAddress := address

// 	if vm.checkReadable(address) == false {
// 		fmt.Printf("READ OOB\n")
// 		return result, PANIC
// 	}

// 	for remainingBytes > 0 {
// 		// Determine the current page and offset within the page
// 		currentPage := currentAddress / 4096
// 		currentOffset := currentAddress % 4096

// 		// Determine how many bytes can be read from the current page
// 		bytesToRead := int(4096 - currentOffset)
// 		if bytesToRead > remainingBytes {
// 			bytesToRead = remainingBytes
// 		}

// 		// Retrieve the page data, return 0s if the page does not exist
// 		pageData, exists := vm.ram[currentPage]
// 		if !exists {
// 			result = append(result, make([]byte, bytesToRead)...)
// 		} else {
// 			result = append(result, pageData[currentOffset:currentOffset+uint32(bytesToRead)]...)
// 		}

// 		// Update the current address and the number of remaining bytes
// 		currentAddress += uint32(bytesToRead)
// 		remainingBytes -= bytesToRead
// 	}

// 	return result, OK
// }

// func (vm *VM) writeRAMBytes(address uint32, value []byte) uint32 {
// 	remainingBytes := len(value)
// 	currentAddress := address
// 	bytesWritten := 0

// 	if vm.checkWriteable(address) == false {
// 		fmt.Printf("WRITE OOB\n")
// 		return PANIC
// 	}

// 	for remainingBytes > 0 {
// 		// Determine the current page and offset within the page
// 		currentPage := currentAddress / 4096
// 		currentOffset := currentAddress % 4096

// 		// Determine how many bytes can be written to the current page
// 		bytesToWrite := int(4096 - currentOffset)
// 		if bytesToWrite > remainingBytes {
// 			bytesToWrite = remainingBytes
// 		}

// 		// Retrieve the page data or create a new page if it doesn't exist
// 		pageData, exists := vm.ram[currentPage]
// 		if !exists {
// 			pageData = [4096]byte{}
// 		}

// 		// Write the bytes to the current page
// 		copy(pageData[currentOffset:], value[bytesWritten:bytesWritten+bytesToWrite])

// 		// Store the updated page back in the RAM
// 		vm.ram[currentPage] = pageData

// 		// Update the current address and the number of remaining bytes
// 		currentAddress += uint32(bytesToWrite)
// 		bytesWritten += bytesToWrite
// 		remainingBytes -= bytesToWrite
// 	}

// 	return OK
// }

func (vm *VM) readRegister(index int) (uint32, uint32) {
	if index < 0 || index >= len(vm.register) {
		return 0, OOB
	}
	// fmt.Printf(" REGISTERS %v (index=%d => %d)\n", vm.register, index, vm.register[index])
	return vm.register[index], OK
}

func (vm *VM) writeRegister(index int, value uint32) uint32 {
	if index < 0 || index >= len(vm.register) {
		return OOB
	}
	vm.register[index] = value
	if debug {
		fmt.Printf("Register[%d] = %d\n", index, value)
	}
	return OK
}

// Implement the dynamic jump logic
func (vm *VM) dynamicJump(operands []byte) uint32 {
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
func (vm *VM) ecalli(operands []byte) uint32 {
	// Implement ecalli logic here
	lx := uint32(types.DecodeE_l(operands))
	vm.hostCall = true
	vm.host_func_id = lx
	return HOST
}

// Implement storeImm logic
func (vm *VM) storeImm(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	lx := min(4, int(originalOperands[0])%8)
	ly := min(4, max(0, len(originalOperands)-lx-1))
	if ly == 0 {
		ly = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	vy := x_encode(uint32(types.DecodeE_l(originalOperands[1+lx:1+lx+ly])), uint32(ly))

	switch opcode {
	case STORE_IMM_U8:
		value := vy % (1 << 8)
		if debug {
			fmt.Printf("STORE_IMM_U8: %d %v\n", vx, []byte{byte(value)})
		}
		return vm.writeRAMBytes(vx, []byte{byte(value)})
	case STORE_IMM_U16:
		value := types.E_l(uint64(vy%(1<<16)), 2)
		if debug {
			fmt.Printf("STORE_IMM_U16: %d %v\n", vx, value)
		}
		return vm.writeRAMBytes(vx, value)
	case STORE_IMM_U32:
		value := types.E_l(uint64(vy), 4)
		if debug {
			fmt.Printf("STORE_IMM_U32: %d %v\n", vx, value)
		}
		return vm.writeRAMBytes(vx, value)
	}

	return OOB
}

// load_imm (opcode 4)
func (vm *VM) loadImm(operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	return vm.writeRegister(registerIndexA, vx)
}

// LOAD_U8, LOAD_U16, LOAD_U32
func (vm *VM) load(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	switch opcode {
	case LOAD_U8:
		value, errCode := vm.readRAM(vx)
		if errCode != OK {
			return errCode
		}
		if debug {
			fmt.Printf("LOAD_U8: %d %v\n", vx, []byte{value})
		}
		return vm.writeRegister(registerIndexA, uint32(value))
	case LOAD_U16:
		value, errCode := vm.readRAMBytes(vx, 2)
		if errCode != OK {
			return errCode
		}
		if debug {
			fmt.Printf("LOAD_U16: %d %v\n", vx, value)
		}
		return vm.writeRegister(registerIndexA, uint32(types.DecodeE_l(value)))
	case LOAD_U32:
		value, errCode := vm.readRAMBytes(vx, 4)
		if errCode != OK {
			return errCode
		}
		if debug {
			fmt.Printf("LOAD_U32: %d %v\n", vx, value)
		}
		return vm.writeRegister(registerIndexA, uint32(types.DecodeE_l(value)))
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

	if debug {
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
	if debug {
		fmt.Printf("get_elided_int32 %v from %v\n", x, o)
	}
	return int32(binary.BigEndian.Uint32(x))
}

// STORE_U8, STORE_U16, STORE_U32
func (vm *VM) store(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	switch opcode {
	case STORE_U8:
		value := valueA % (1 << 8)
		if debug {
			fmt.Printf("STORE_u8: %d %v\n", vx, []byte{byte(value)})
		}
		return vm.writeRAMBytes(vx, []byte{byte(value)})
	case STORE_U16:
		value := types.E_l(uint64(valueA%(1<<16)), 2)
		if debug {
			fmt.Printf("STORE_U16: %d %v\n", vx, value)
		}
		return vm.writeRAMBytes(vx, value)
	case STORE_U32:
		value := types.E_l(uint64(valueA), 4)
		if debug {
			fmt.Printf("STORE_U32: %d %v\n", vx, value)
		}
		return vm.writeRAMBytes(vx, value)
	}
	return OOB
}

// storeImmInd implements STORE_IMM_{U8, U16, U32}
func (vm *VM) storeImmInd(opcode byte, operands []byte) uint32 {
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

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return OOB
	}

	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	vy := x_encode(uint32(types.DecodeE_l(originalOperands[1+lx:1+lx+ly])), uint32(ly))

	switch opcode {
	case STORE_IMM_IND_U8:
		if debug {
			fmt.Printf("STORE_IMM_IND_U8: %d %v\n", valueA+vx, []byte{byte(vy % 8)})
		}
		return vm.writeRAMBytes(valueA+vx, []byte{byte(vy % 8)})
	case STORE_IMM_IND_U16:
		if debug {
			fmt.Printf("STORE_IMM_IND_U16: %d %v\n", valueA+vx, types.E_l(uint64(vy%1<<16), 2))
		}
		return vm.writeRAMBytes(valueA+vx, types.E_l(uint64(vy%1<<16), 2))
	case STORE_IMM_IND_U32:
		if debug {
			fmt.Printf("STORE_IMM_IND_U32: %d %v\n", valueA+vx, types.E_l(uint64(vy), 4))
		}
		return vm.writeRAMBytes(valueA+vx, types.E_l(uint64(vy), 4))
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

	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	vy := uint32(int64(vm.pc) + z_encode(uint32(types.DecodeE_l(originalOperands[1+lx:1+lx+ly])), uint32(ly)))

	vm.writeRegister(registerIndexA, vx)

	vm.branch(vy, true)
	return OK
}

// Implement loadImmJumpInd logic
func (vm *VM) loadImmJumpInd(operands []byte) uint32 {
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

	vx := x_encode(uint32(types.DecodeE_l(originalOperands[2:2+lx])), uint32(lx))
	vy := x_encode(uint32(types.DecodeE_l(originalOperands[2+lx:2+lx+ly])), uint32(ly))

	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	vm.writeRegister(registerIndexA, vx)
	vm.djump(valueB + vy)

	return OK
}

// move_reg (opcode 82)
func (vm *VM) moveReg(operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexD := min(12, int(originalOperands[0])%16)
	registerIndexA := min(12, int(originalOperands[0])/16)

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	if debug {
		fmt.Printf("MOVE_REG: %d %d\n", registerIndexA, registerIndexD)
	}
	return vm.writeRegister(registerIndexD, valueA)
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

func (vm *VM) branchCond(opcode byte, operands []byte) uint32 {
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

	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	vy := uint32(int64(vm.pc) + z_encode(uint32(types.DecodeE_l(originalOperands[1+lx:1+lx+ly])), uint32(ly)))

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}

	switch opcode {
	case BRANCH_EQ_IMM:
		if valueA == vx {
			if debug {
				fmt.Printf("BRANCH_EQ_IMM: %valueA=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_NE_IMM:
		if valueA != vx {
			if debug {
				fmt.Printf("BRANCH_NE_IMM: %valueA!=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_U_IMM:
		if valueA < vx {
			if debug {
				fmt.Printf("BRANCH_LT_U_IMM: %valueA<%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LT_S_IMM:
		if z_encode(valueA, 4) < z_encode(vx, 4) {
			if debug {
				fmt.Printf("BRANCH_LT_S_IMM: %valueA<%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_U_IMM:
		if valueA <= vx {
			if debug {
				fmt.Printf("BRANCH_LE_U_IMM: %valueA<=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_LE_S_IMM:
		if z_encode(valueA, 4) <= z_encode(vx, 4) {
			if debug {
				fmt.Printf("BRANCH_LE_S_IMM: %valueA<=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_U_IMM:
		if valueA >= vx {
			if debug {
				fmt.Printf("BRANCH_GE_U_IMM: %valueA>=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GE_S_IMM:
		if z_encode(valueA, 4) >= z_encode(vx, 4) {
			if debug {
				fmt.Printf("BRANCH_GE_S_IMM: %valueA>=%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_U_IMM:
		if valueA > vx {
			if debug {
				fmt.Printf("BRANCH_GT_U_IMM: %valueA>%d, jump to %d\n", valueA, vx, vy)
			}
			vm.branch(vy, true)
		} else {
			vm.pc += uint32(1 + len(operands))
			return OK
		}
	case BRANCH_GT_S_IMM:
		if z_encode(valueA, 4) > z_encode(vx, 4) {
			if debug {
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

func (vm *VM) storeInd(opcode byte, operands []byte) uint32 {
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

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	switch opcode {
	case STORE_IND_U8:
		if debug {
			fmt.Printf("STORE_IND_U8: %d %v\n", valueB+vx, []byte{byte(valueA % 8)})
		}
		return vm.writeRAMBytes(valueB+vx, []byte{byte(valueA % 8)})
	case STORE_IND_U16:
		if debug {
			fmt.Printf("STORE_IND_U16: %d %v\n", valueB+vx, types.E_l(uint64(valueA%1<<16), 2))
		}
		return vm.writeRAMBytes(valueB+vx, types.E_l(uint64(valueA), 2))
	case STORE_IND_U32:
		if debug {
			fmt.Printf("STORE_IND_U32: %d %v\n", valueB+vx, types.E_l(uint64(valueA), 4))
		}
		return vm.writeRAMBytes(valueB+vx, types.E_l(uint64(valueA), 4))
	default:
		return OOB
	}

}

// Implement loadInd logic
// func (vm *VM) loadInd(opcode byte, operands []byte) uint32 {
// 	destRegisterIndex, registerIndexB := splitRegister(operands[0])
// 	wB, errCode := vm.readRegister(registerIndexB)
// 	if errCode != OK {
// 		return errCode
// 	}
// 	// handle no operand means 0
// 	var immediate uint32
// 	if len(operands) < 2 {
// 		immediate = uint32(0)
// 	} else {
// 		immediate = get_elided_uint32(operands[1:])
// 	}

// 	address := wB + immediate
// 	switch opcode {
// 	case LOAD_IND_U8:
// 		value, errCode := vm.readRAM(address)
// 		if errCode != OK {
// 			return errCode
// 		}
// 		return vm.writeRegister(destRegisterIndex, uint32(value))
// 	case LOAD_IND_I8:
// 		value, errCode := vm.readRAM(address)
// 		if errCode != OK {
// 			return errCode
// 		}
// 		v := int(value)
// 		if v > 127 {
// 			v -= 256
// 		}
// 		return vm.writeRegister(destRegisterIndex, uint32(v))
// 	case LOAD_IND_U16:
// 		value, errCode := vm.readRAMBytes(address, 2)
// 		if errCode != OK {
// 			return errCode

// 		}
// 		return vm.writeRegister(destRegisterIndex, uint32(binary.LittleEndian.Uint16(value)))
// 	case LOAD_IND_I16:
// 		value, errCode := vm.readRAMBytes(address, 2)
// 		if errCode != OK {
// 			return errCode
// 		}
// 		return vm.writeRegister(destRegisterIndex, uint32(binary.LittleEndian.Uint16(value)))
// 	case LOAD_IND_U32:
// 		value, errCode := vm.readRAMBytes(address, 4)
// 		if errCode != OK {
// 			return errCode
// 		}
// 		return vm.writeRegister(destRegisterIndex, uint32(binary.LittleEndian.Uint32(value)))
// 	}
// 	return OOB
// }

func (vm *VM) loadInd(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))
	switch opcode {
	case LOAD_IND_U8:
		value, errCode := vm.readRAM(valueB + vx)
		if errCode != OK {
			return errCode
		}
		result := uint32(value)
		if debug {
			fmt.Printf("LOAD_IND_U8: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.writeRegister(registerIndexA, result)
	case LOAD_IND_I8:
		value, errCode := vm.readRAM(valueB + vx)
		if errCode != OK {
			return errCode
		}
		result := uint32(z_decode(z_encode(uint32(value), 1), 4))
		if debug {
			fmt.Printf("LOAD_IND_I8: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.writeRegister(registerIndexA, result)
	case LOAD_IND_U16:
		value, errCode := vm.readRAMBytes(valueB+vx, 2)
		if errCode != OK {
			return errCode
		}
		result := uint32(types.DecodeE_l(value))
		if debug {
			fmt.Printf("LOAD_IND_U16: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.writeRegister(registerIndexA, result)
	case LOAD_IND_I16:
		value, errCode := vm.readRAMBytes(valueB+vx, 2)
		if errCode != OK {
			return errCode
		}
		result := uint32(z_decode(z_encode(uint32(types.DecodeE_l(value)), 2), 4))
		if debug {
			fmt.Printf("LOAD_IND_I16: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.writeRegister(registerIndexA, result)
	case LOAD_IND_U32:
		value, errCode := vm.readRAMBytes(valueB+vx, 4)
		if errCode != OK {
			return errCode
		}
		result := uint32(types.DecodeE_l(value))
		if debug {
			fmt.Printf("LOAD_IND_U32: ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
		}
		return vm.writeRegister(registerIndexA, result)
	}
	return OOB
}

func splitRegister(operand byte) (int, int) {
	registerIndexA := int(operand & 0xF)
	registerIndexB := int((operand >> 4) & 0xF)
	return registerIndexA, registerIndexB
}

// Implement ALU operations with immediate values
func (vm *VM) aluImm(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	var result uint32
	switch opcode {
	case ADD_IMM:
		result = valueB + vx
	case AND_IMM:
		result = valueB & vx
	case XOR_IMM:
		result = valueB ^ vx
	case OR_IMM:
		result = valueB | vx
	case MUL_IMM:
		result = valueB * vx
	case MUL_UPPER_S_S_IMM:
		result = uint32(z_decode((z_encode(valueB, 4) * z_encode(vx, 4)), 4))
	case MUL_UPPER_U_U_IMM:
		result = valueB * vx
	case SET_LT_U_IMM:
		if uint32(valueB) < uint32(vx) {
			result = 1
		} else {
			result = 0
		}
	case SET_LT_S_IMM:
		if z_encode(valueB, 4) < z_encode(vx, 4) {
			result = 1
		} else {
			result = 0
		}
	default:
		return OOB
	}
	if debug {
		fmt.Printf("aluImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.writeRegister(registerIndexA, result)
}

// Implement cmov_nz_imm, cmov_nz_imm
func (vm *VM) cmovImm(opcode byte, operands []byte) uint32 {
	// handle no operand means 0
	originalOperands := make([]byte, len(operands))
	copy(originalOperands, operands)

	registerIndexA := min(12, int(originalOperands[0])%16)
	registerIndexB := min(12, int(originalOperands[0])/16)
	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}
	lx := min(4, max(0, len(originalOperands)-1))
	if lx == 0 {
		lx = 1
		originalOperands = append(originalOperands, 0)
	}
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	var result uint32
	switch opcode {
	case CMOV_IMM_IZ:
		if valueB == 0 {
			result = vx
		} else {
			result = valueA
		}
	case CMOV_IMM_NZ:
		if valueB != 0 {
			result = vx
		} else {
			result = valueA
		}
	default:
		return OOB
	}
	if debug {
		fmt.Printf("cmovImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.writeRegister(registerIndexA, result)
}

// Implement shift operations with immediate values
func (vm *VM) shiftImm(opcode byte, operands []byte) uint32 {
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
	vx := x_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx))

	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint32
	switch opcode {
	case SHLO_R_IMM:
		result = valueB / (1 << (vx % 32))
	case SHLO_L_IMM:
		result = valueB * (1 << (vx % 32))
	case SHAR_R_IMM:
		result = uint32(z_decode(z_encode(valueB, 4)/(1<<(vx%32)), 4))

	case NEG_ADD_IMM:
		result = uint32(uint64(vx) + uint64(1<<32) - uint64(valueB))
	case SET_GT_U_IMM:
		if valueB > vx {
			result = 1
		} else {
			result = 0
		}
	case SET_GT_S_IMM:
		if z_encode(valueB, 4) > z_encode(vx, 4) {
			result = 1
		} else {
			result = 0
		}
	case SHLO_L_IMM_ALT:
		result = vx * (1 << (valueB % 32))
	case SHLO_R_IMM_ALT:
		result = vx / (1 << (valueB % 32))
	case SHAR_R_IMM_ALT:
		result = uint32(z_decode(z_encode(vx, 4)/(1<<(valueB%32)), 4))
	default:
		return OOB
	}
	if debug {
		fmt.Printf("shiftImm ra=%d rb=%d valueB=%d lx=%d vx=%d\n", registerIndexA, registerIndexB, valueB, lx, vx)
	}
	return vm.writeRegister(registerIndexA, result)
}

// GP_.0.3.6(219)
func z_encode(a uint32, n uint32) int64 {
	modValue := uint32(1 << (8*n - 1))
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
func x_encode(x uint32, n uint32) uint32 {
	if n == 0 {
		return x
	}
	return x + uint32((uint64(x)/(1<<(8*n-1)))*(uint64(1<<32)-(1<<(8*n))))
}

// Implement branch logic for two registers and one offset
func (vm *VM) branchReg(opcode byte, operands []byte) uint32 {
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
	vx := uint32(int64(vm.pc) + z_encode(uint32(types.DecodeE_l(originalOperands[1:1+lx])), uint32(lx)))

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
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
func (vm *VM) aluReg(opcode byte, operands []byte) uint32 {

	registerIndexA := min(12, int(operands[0])%16)
	registerIndexB := min(12, int(operands[0])/16)
	registerIndexD := min(12, int(operands[1]))

	valueA, errCode := vm.readRegister(registerIndexA)
	if errCode != OK {
		return errCode
	}
	valueB, errCode := vm.readRegister(registerIndexB)
	if errCode != OK {
		return errCode
	}

	var result uint32
	switch opcode {
	case ADD_REG:
		// set Malicious flag
		if vm.IsMalicious {
			result = valueA * valueB
		} else {
			result = valueA + valueB
		}
	case SUB_REG:
		result = valueA - valueB
	case AND_REG:
		result = valueA & valueB
	case XOR_REG:
		result = valueA ^ valueB
	case OR_REG:
		result = valueA | valueB
	case MUL_REG:
		result = valueA * valueB
	case MUL_UPPER_S_S_REG:
		result = uint32(int(valueA) * int(valueB) >> 32)
	case MUL_UPPER_U_U_REG:
		result = uint32((uint(valueA) * uint(valueB)) >> 32)
	case MUL_UPPER_S_U_REG:
		result = uint32((int(valueA) * int(valueB)) >> 32)
	case DIV_U:
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else {
			result = valueA / valueB
		}
	case DIV_S:
		if valueB == 0 {
			result = 0xFFFFFFFF
		} else if int32(valueA) == -(1<<31) && int32(valueB) == -(1<<0) {
			result = valueA
		} else {
			result = uint32(int32(valueA) / int32(valueB))
		}
	case REM_U:
		if valueB == 0 {
			result = valueA
		} else {
			result = valueA % valueB
		}
	case REM_S:
		if debug {
			fmt.Printf(" REM_S %d %d \n", int32(valueA), int32(valueB))
		}
		if valueB == 0 {
			result = valueA
		} else if valueA == 0x80 && valueB == 0xFF {
			return OOB
		} else {
			result = uint32(int32(valueA) % int32(valueB))
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
		if int32(valueA) < int32(valueB) {
			result = 1
		} else {
			result = 0
		}

	case SHLO_L:
		result = valueA << (valueB % 32)
	case SHLO_R:
		result = valueA >> (valueB % 32)
	case SHAR_R:
		if int32(valueA)/(1<<(valueB%32)) < 0 && int32(valueA)%(1<<(valueB%32)) != 0 {
			result = uint32((int32(valueA) / (1 << (valueB % 32))) - 1)
		} else {
			result = uint32(int32(valueA) / (1 << (valueB % 32)))
		}

	default:
		return OOB // unknown ALU register
	}

	if debug {
		fmt.Printf("aluReg - rA[%d]=%d  regB[%d]=%d regD[%d]=%d\n", registerIndexA, valueA, registerIndexB, valueB, registerIndexD, result)
	}
	return vm.writeRegister(registerIndexD, result)
}

// VM Management: CreateVM, GetVM, ExpungeVM
func (vm *VM) CreateVM(serviceAcct uint32, code []byte, i uint32) uint32 {
	maxN := uint32(0)
	for n := range vm.VMs {
		if n > maxN {
			maxN = n
		}
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
	vm.VMs[n] = nil
	return true
}
