package log

import (
	"log/slog"
	"os"
	"sync/atomic"
)

const (
	GeneralAuthoring = "authoring"       // Generic Authoring log (excluding pvm)
	PvmAuthoring     = "pvm_authoring"   // PVM Authoring log
	FirstGuarantor   = "first_guarantor" // First Authoring Guarantor log
)

var root atomic.Value

func init() {
	root.Store(&logger{slog.New(DiscardHandler()), nil})

}

func InitLogger() {
	SetDefault(NewLogger(NewTerminalHandlerWithLevel(os.Stderr, LevelDebug, true)))
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

// TODO: this list can be provided externally
var defaultKnownModules = []string{GeneralAuthoring, PvmAuthoring, FirstGuarantor, "blk_mod", "statedb", "G", "grandpa", "segment"}
var defaultModuleEnabled = []string{GeneralAuthoring}

// --- Module management ---
// moduleEnabled keeps track of whether a module’s logging is enabled.
var moduleEnabled = init_module(defaultKnownModules, defaultModuleEnabled)

// EnableModule enables logging for the specified module.
func EnableModule(module string) {
	//fmt.Printf("!!!! Enabling module %s\n", module)
	moduleEnabled[module] = true
}

// DisableModule disables logging for the specified module.
func DisableModule(module string) {
	moduleEnabled[module] = false
}

// isModuleEnabled checks if logging is enabled for the given module.
func isModuleEnabled(module string) bool {
	//if module == "pvm_authoring" {
	//	return true
	//}
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

func New(ctx ...interface{}) Logger {
	return Root().With(ctx...)
}
