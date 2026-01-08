// Package pvmtypes consolidates shared types, interfaces, and constants for the PVM.
package pvmtypes

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// ============================================================================
// Machine State Constants (from vmstate)
// ============================================================================

const (
	HALT  = 0 // regular halt
	PANIC = 1 // panic
	FAULT = 2 // page-fault
	HOST  = 3 // host-call
	OOG   = 4 // out-of-gas
)

// ============================================================================
// Host Function Result Codes (from hostcode)
// ============================================================================

const (
	OK   uint64 = 0
	NONE uint64 = (1 << 64) - 1
	WHAT uint64 = (1 << 64) - 2
	OOB  uint64 = (1 << 64) - 3
	WHO  uint64 = (1 << 64) - 4
	FULL uint64 = (1 << 64) - 5
	CORE uint64 = (1 << 64) - 6
	CASH uint64 = (1 << 64) - 7
	LOW  uint64 = (1 << 64) - 8
	HUH  uint64 = (1 << 64) - 9
)

// ============================================================================
// Memory Page Permissions (from mem)
// ============================================================================

const (
	PageInaccessible = unix.PROT_NONE
	PageMutable      = unix.PROT_READ | unix.PROT_WRITE
	PageImmutable    = unix.PROT_READ
)

// ============================================================================
// ABI Constants (from abi)
// ============================================================================

const (
	Z_A = 2

	Z_P = (1 << 12)
	Z_Q = (1 << 16)
	Z_I = (1 << 24)
	Z_Z = (1 << 16)
)

func CeilingDivide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

func PFunc(x uint32) uint32 {
	return Z_P * CeilingDivide(x, Z_P)
}

func ZFunc(x uint32) uint32 {
	return Z_Z * CeilingDivide(x, Z_Z)
}

// ============================================================================
// HostVM Interface (from iface)
// ============================================================================

type HostVM interface {
	InvokeHostCall(hostFn int) (bool, error)
	GetResultCode() uint8
	GetMachineState() uint8
	SetResultCode(code uint8)
	SetMachineState(state uint8)
	SetTerminated(terminated bool)
}

type FakeHostVM struct {
	ResultCode   uint8
	MachineState uint8
	Terminated   bool
}

func (f *FakeHostVM) InvokeHostCall(hostFn int) (bool, error) {
	return false, nil
}

func (f *FakeHostVM) GetResultCode() uint8 {
	return f.ResultCode
}

func (f *FakeHostVM) GetMachineState() uint8 {
	return f.MachineState
}

func (f *FakeHostVM) SetResultCode(code uint8) {
	f.ResultCode = code
}

func (f *FakeHostVM) SetMachineState(state uint8) {
	f.MachineState = state
}

func (f *FakeHostVM) SetTerminated(terminated bool) {
	f.Terminated = terminated
}

// ============================================================================
// Host Function Constants and Debug Helpers (from hostfunc)
// ============================================================================

const (
	ColorReset       = "\033[0m"
	ColorBold        = "\033[1m"
	ColorRed         = "\033[31m"
	ColorGreen       = "\033[32m"
	ColorYellow      = "\033[33m"
	ColorBlue        = "\033[34m"
	ColorMagenta     = "\033[35m"
	ColorCyan        = "\033[36m"
	ColorWhite       = "\033[37m"
	ColorGray        = "\033[90m"
	ColorRedBold     = "\033[31m\033[1m"
	ColorGreenBold   = "\033[32m\033[1m"
	ColorBlueBold    = "\033[34m\033[1m"
	ColorMagentaBold = "\033[35m\033[1m"
	ColorCyanBold    = "\033[36m\033[1m"
	ColorYellowBold  = "\033[33m\033[1m"
)

var DebugHostFunctions = false

var DebugHostFunctionMap = map[int]int{}

var ResultMap = map[uint64]int{}

var debugHostFnMu sync.Mutex

// DebugHostFunction prints colorized debug information about host function calls.
func DebugHostFunction(serviceIndex uint32, hostFn int, format string, a ...any) {
	if !DebugHostFunctions {
		return
	}
	debugHostFnMu.Lock()
	DebugHostFunctionMap[hostFn]++
	debugHostFnMu.Unlock()

	colors := []string{
		"\x1b[32m", // green
		"\x1b[33m", // yellow
		"\x1b[34m", // blue
		"\x1b[35m", // magenta
		"\x1b[36m", // cyan
	}
	reset := "\x1b[0m"
	color := colors[hostFn%len(colors)]
	fmt.Printf("%s[%d]***** HostFunction %s ", color, serviceIndex, HostFnToName(hostFn))
	fmt.Printf(format, a...)
	fmt.Printf("%s\n", reset)
}

// JAM Protocol Host Function IDs (0-26, 100)
const (
	GAS               = 0   // Gas metering
	FETCH             = 1   // Fetch work package data
	LOOKUP            = 2   // Lookup service state
	READ              = 3   // Read from service state
	WRITE             = 4   // Write to service state
	INFO              = 5   // Query service information
	HISTORICAL_LOOKUP = 6   // Lookup historical state
	EXPORT            = 7   // Export data
	MACHINE           = 8   // Create child VM
	PEEK              = 9   // Peek child VM memory
	POKE              = 10  // Poke child VM memory
	PAGES             = 11  // Query VM memory pages
	INVOKE            = 12  // Invoke child VM
	EXPUNGE           = 13  // Destroy child VM
	BLESS             = 14  // Bless service code
	ASSIGN            = 15  // Assign service code
	DESIGNATE         = 16  // Designate service
	CHECKPOINT        = 17  // Create checkpoint
	NEW               = 18  // Create new service
	UPGRADE           = 19  // Upgrade service
	TRANSFER          = 20  // Transfer service ownership
	EJECT             = 21  // Eject service
	QUERY             = 22  // Query service
	SOLICIT           = 23  // Solicit service
	FORGET            = 24  // Forget service
	YIELD             = 25  // Yield control
	PROVIDE           = 26  // Provide data
	LOG               = 100 // Debug logging
	FETCH_WITNESS     = 254 // Majik CE139 fetch/cache
	FETCH_VERKLE      = 255 // Majik Verkle fetch (unified balance/nonce/code/storage)
)

// hostFnNames maps host function IDs to human-readable names
var hostFnNames = map[int]string{
	// JAM Protocol Core Host Functions
	GAS:               "GAS",
	FETCH:             "FETCH",
	LOOKUP:            "LOOKUP",
	READ:              "READ",
	WRITE:             "WRITE",
	INFO:              "INFO",
	HISTORICAL_LOOKUP: "HISTORICAL_LOOKUP",
	EXPORT:            "EXPORT",
	MACHINE:           "MACHINE",
	PEEK:              "PEEK",
	POKE:              "POKE",
	PAGES:             "PAGES",
	INVOKE:            "INVOKE",
	EXPUNGE:           "EXPUNGE",
	BLESS:             "BLESS",
	ASSIGN:            "ASSIGN",
	DESIGNATE:         "DESIGNATE",
	CHECKPOINT:        "CHECKPOINT",
	NEW:               "NEW",
	UPGRADE:           "UPGRADE",
	TRANSFER:          "TRANSFER",
	EJECT:             "EJECT",
	QUERY:             "QUERY",
	SOLICIT:           "SOLICIT",
	FORGET:            "FORGET",
	YIELD:             "YIELD",
	PROVIDE:           "PROVIDE",
	LOG:               "DEBUG",

	// Custom Host Functions
	FETCH_WITNESS: "FETCH_WITNESS",
	FETCH_VERKLE:  "FETCH_VERKLE",
}

// HostFnToName returns the human-readable name for a host function ID
func HostFnToName(hostFn int) string {
	if name, ok := hostFnNames[hostFn]; ok {
		return name
	}
	return "UNKNOWN"
}
