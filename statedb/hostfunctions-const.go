package statedb

import (
	"fmt"
	"sync"
)

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

// ============================================================================
// Host Function Name Mapping and Debug Helpers
// ============================================================================

var DebugHostFunctions = false

var DebugHostFunctionMap = map[int]int{}

var debugHostFnMu sync.Mutex

// DebugHostFunction prints colorized debug information about host function calls
func (vm *VM) DebugHostFunction(hostFn int, format string, a ...any) {
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
	fmt.Printf("%s[%d]***** HostFunction %s ", color, vm.Service_index, HostFnToName(hostFn))
	fmt.Printf(format, a...)
	fmt.Printf("%s\n", reset)
}

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

// ============================================================================
// JAM Protocol Core Host Functions (Appendix B)
// ============================================================================

const (
	// JAM Protocol Host Function IDs (0-26, 100)
	GAS                 = 0   // Gas metering
	FETCH               = 1   // Fetch work package data
	LOOKUP              = 2   // Lookup service state
	READ                = 3   // Read from service state
	WRITE               = 4   // Write to service state
	INFO                = 5   // Query service information
	HISTORICAL_LOOKUP   = 6   // Lookup historical state
	EXPORT              = 7   // Export data
	MACHINE             = 8   // Create child VM
	PEEK                = 9   // Peek child VM memory
	POKE                = 10  // Poke child VM memory
	PAGES               = 11  // Query VM memory pages
	INVOKE              = 12  // Invoke child VM
	EXPUNGE             = 13  // Destroy child VM
	BLESS               = 14  // Bless service code
	ASSIGN              = 15  // Assign service code
	DESIGNATE           = 16  // Designate service
	CHECKPOINT          = 17  // Create checkpoint
	NEW                 = 18  // Create new service
	UPGRADE             = 19  // Upgrade service
	TRANSFER            = 20  // Transfer service ownership
	EJECT               = 21  // Eject service
	QUERY               = 22  // Query service
	SOLICIT             = 23  // Solicit service
	FORGET              = 24  // Forget service
	YIELD               = 25  // Yield control
	PROVIDE             = 26  // Provide data
	LOG                 = 100 // Debug logging
	VERIFY_VERKLE_PROOF = 253 // Majik Verkle proof verification
	FETCH_WITNESS       = 254 // Majik CE139 fetch/cache
	FETCH_VERKLE        = 255 // Majik Verkle fetch (unified balance/nonce/code/storage)
)
