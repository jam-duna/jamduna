package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

const (
	// PVM Context (vm.logging)
	PvmAuthoring            = "pvm_authoring"   // PVM Authoring
	PvmValidating           = "pvm_validator"   // PVM Validator
	FirstGuarantorOrAuditor = "first_guarantor" // First Guarantor or Auditor
	OtherGuarantor          = "other_guarantor" // 2nd/3rd Guarantor

	GeneralAuthoring  = "authoring"  // Generic Authoring  (excluding pvm)
	GeneralValidating = "validating" // Generic Validating (excluding pvm)

	BlockMonitoring      = "blk_mod"     // Block module log
	DAMonitoring         = "da_mod"      // Data Availability module log
	NodeMonitoring       = "n_mod"       // General Node Ops
	SegmentMonitoring    = "seg_mod"     // Segment module log
	StateDBMonitoring    = "statedb_mod" // stateDB module log
	GrandpaMonitoring    = "grandpa_mod" // Grandpa module log
	JamwebMonitoring     = "jamweb_mod"  // Jamweb module log
	QuicStreamMonitoring = "q_mod"       // Quicstream module log
	GuaranteeMonitoring  = "g_mod"       // Guarantee module log
)

var root atomic.Value

func init() {
	root.Store(&logger{slog.New(DiscardHandler()), nil, false, make([]slog.Record, 0)})
	DisableModule(PvmValidating)
	DisableModule(OtherGuarantor)
	DisableModule(GeneralValidating)
}

func ParseLevel(lvl string) (slog.Level, error) {
	switch strings.ToUpper(lvl) {
	case "MAX", "MAXVERBOSITY":
		return levelMaxVerbosity, nil
	case "TRACE":
		return LevelTrace, nil
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	case "CRIT", "CRITICAL":
		return LevelCrit, nil
	default:
		return 0, fmt.Errorf("invalid level: %s", lvl)
	}
}

func InitLogger(logLevel string) {
	logLvl, err := ParseLevel(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	SetDefault(NewLogger(NewTerminalHandlerWithLevel(os.Stderr, logLvl, true)))
}

// SetDefault sets the default global logger
func SetDefault(l Logger) {
	root.Store(l)
	if lg, ok := l.(*logger); ok {
		slog.SetDefault(lg.inner)
	}
}

// Root returns the root logger
func Root() Logger {
	return root.Load().(Logger)
}

func init_module(moduleList []string, moduleEnabled []string) map[string]bool {
	moduleMap := make(map[string]bool, 0)
	for _, module := range moduleList {
		moduleMap[module] = false
	}
	for _, module := range moduleEnabled {
		moduleMap[module] = true
	}
	return moduleMap
}

var defaultKnownModules = []string{GeneralAuthoring, PvmAuthoring, FirstGuarantorOrAuditor, BlockMonitoring, DAMonitoring, NodeMonitoring, SegmentMonitoring, StateDBMonitoring, GrandpaMonitoring, JamwebMonitoring, "G"} // no idea what is G
var defaultModuleEnabled = []string{}

// --- Module management ---
// moduleEnabled keeps track of whether a module’s logging is enabled.
var moduleEnabled = init_module(defaultKnownModules, defaultModuleEnabled)

// EnableModule enables logging for the specified module.
func EnableModule(module string) {
	moduleEnabled[module] = true
}

// DisableModule disables logging for the specified module.
func DisableModule(module string) {
	moduleEnabled[module] = false
}

// isModuleEnabled checks if logging is enabled for the given module.
func isModuleEnabled(module string) bool {
	enabled, ok := moduleEnabled[module]
	return ok && enabled
}

// --- Adjusted logging functions ---

// Trace logs a message at the trace level for a specific module.
func Trace(module string, msg string, ctx ...interface{}) {
	if !isModuleEnabled(module) {
		return
	}
	// Prepend the module name into the context.
	newCtx := append([]interface{}{"module", module}, ctx...)
	Root().Write(LevelTrace, module, msg, newCtx...)
}

// Debug logs a message at the debug level for a specific module.
func Debug(module string, msg string, ctx ...interface{}) {
	if !isModuleEnabled(module) {
		return
	}
	//newCtx := append([]interface{}{"module", module}, ctx...)
	Root().Write(slog.LevelDebug, module, msg, ctx...)
}

// The rest of the logging functions (Info, Warn, Error, Crit, New) dont filter on module
func Info(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelInfo, module, msg, ctx...)
}

func Warn(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelWarn, module, msg, ctx...)
}

func Error(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelError, module, msg, ctx...)
}

func Crit(module string, msg string, ctx ...interface{}) {
	Root().Write(LevelCrit, module, msg, ctx...)
	os.Exit(1)
}

func RecordLogs() {
	Root().RecordLogs()
}

func GetRecordedLogs() ([]byte, error) {
	return Root().GetRecordedLogs()
}

func New(ctx ...interface{}) Logger {
	return Root().With(ctx...)
}
