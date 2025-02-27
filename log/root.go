package log

import (
	"log/slog"
	"os"
	"sync/atomic"
)

var root atomic.Value

func init() {
	root.Store(&logger{slog.New(DiscardHandler())})
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

// --- Module management ---
// moduleEnabled keeps track of whether a module’s logging is enabled.
var moduleEnabled = map[string]bool{
	"authoring": false,
}

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
	Root().Write(LevelTrace, msg, newCtx...)
}

// Debug logs a message at the debug level for a specific module.
func Debug(module string, msg string, ctx ...interface{}) {

	if !isModuleEnabled(module) {
		return
	}
	//newCtx := append([]interface{}{"module", module}, ctx...)
	Root().Write(slog.LevelDebug, msg, ctx...)
}

// The rest of the logging functions (Info, Warn, Error, Crit, New) dont filter on module
func Info(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelInfo, msg, ctx...)
}

func Warn(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelWarn, msg, ctx...)
}

func Error(module string, msg string, ctx ...interface{}) {
	Root().Write(slog.LevelError, msg, ctx...)
}

func Crit(msg string, ctx ...interface{}) {
	Root().Write(LevelCrit, msg, ctx...)
	os.Exit(1)
}

func New(ctx ...interface{}) Logger {
	return Root().With(ctx...)
}
