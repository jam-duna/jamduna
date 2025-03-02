package log

import (
	"context"
	"fmt"
	//"strings"
	"log/slog"
	"log/syslog"
	"math"
	"os"
	"runtime"
	"time"
)

const errorKey = "LOG_ERROR"

const (
	legacyLevelCrit = iota
	legacyLevelError
	legacyLevelWarn
	legacyLevelInfo
	legacyLevelDebug
	legacyLevelTrace
)

const (
	levelMaxVerbosity slog.Level = math.MinInt
	LevelTrace        slog.Level = -8
	LevelDebug                   = slog.LevelDebug
	LevelInfo                    = slog.LevelInfo
	LevelWarn                    = slog.LevelWarn
	LevelError                   = slog.LevelError
	LevelCrit         slog.Level = 12

	// for backward-compatibility
	LvlTrace = LevelTrace
	LvlInfo  = LevelInfo
	LvlDebug = LevelDebug
)

// FromLegacyLevel converts from old Geth verbosity level constants
// to levels defined by slog
func FromLegacyLevel(lvl int) slog.Level {
	switch lvl {
	case legacyLevelCrit:
		return LevelCrit
	case legacyLevelError:
		return slog.LevelError
	case legacyLevelWarn:
		return slog.LevelWarn
	case legacyLevelInfo:
		return slog.LevelInfo
	case legacyLevelDebug:
		return slog.LevelDebug
	case legacyLevelTrace:
		return LevelTrace
	default:
		break
	}

	if lvl > legacyLevelTrace {
		return LevelTrace
	}
	return LevelCrit
}

// LevelAlignedString returns a 5-character string containing the name of a Lvl.
func LevelAlignedString(l slog.Level) string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO "
	case slog.LevelWarn:
		return "WARN "
	case slog.LevelError:
		return "ERROR"
	case LevelCrit:
		return "CRIT "
	default:
		return "unknown level"
	}
}

// Logger writes key/value pairs to a Handler
type Logger interface {
	// With returns a new Logger that has this logger's attributes plus the given attributes
	With(ctx ...interface{}) Logger

	// New returns a new Logger that has this logger's attributes plus the given attributes. Identical to 'With'.
	New(ctx ...interface{}) Logger

	// Log logs a message at the specified level with context key/value pairs
	Log(level slog.Level, module string, msg string, ctx ...interface{})

	// Trace logs a message at the trace level with context key/value pairs
	Trace(module string, msg string, ctx ...interface{})

	// Debug logs a message at the debug level with context key/value pairs
	Debug(module string, msg string, ctx ...interface{})

	// Info logs a message at the info level with context key/value pairs
	Info(module string, msg string, ctx ...interface{})

	// Warn logs a message at the warn level with context key/value pairs
	Warn(module string, msg string, ctx ...any)

	// Error logs a message at the error level with context key/value pairs
	Error(module string, msg string, ctx ...interface{})

	// Crit logs a message at the crit level with context key/value pairs, and exits
	Crit(module string, msg string, ctx ...interface{})

	// Write logs a message at the specified level
	Write(level slog.Level, module string, msg string, attrs ...any)

	// Enabled reports whether l emits log records at the given context and level.
	Enabled(ctx context.Context, level slog.Level) bool

	// Handler returns the underlying handler of the inner logger.
	Handler() slog.Handler
}

type logger struct {
	inner  *slog.Logger
	writer *syslog.Writer
}

// NewLogger returns a logger with the specified handler set
func NewLogger(h slog.Handler) Logger {
	writer, _ := syslog.Dial("tcp", "dev.jamduna.org:5000", syslog.LOG_INFO, "jamduna")
	return &logger{
		inner:  slog.New(h),
		writer: writer,
	}
}

func (l *logger) Handler() slog.Handler {
	return l.inner.Handler()
}

func LevelString(l slog.Level) string {
	switch l {
	case LevelTrace:
		return "trace"
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	case LevelCrit:
		return "crit"
	default:
		return "unknown"
	}
}

// Write logs a message at the specified level.
func (l *logger) Write(level slog.Level, module string, msg string, attrs ...any) {
	if !l.inner.Enabled(context.Background(), level) {
		return
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(attrs...)

	if l.writer != nil {
		str := fmt.Sprintf("%s|%s|%s|%s\n", LevelAlignedString(r.Level), module, r.Message, attrs)
		//str := fmt.Sprintf("%s|%s|%v\n", LevelAlignedString(r.Level), r.Message, attrs)
		switch r.Level {
		case LevelCrit:
			l.writer.Crit(str)
		case slog.LevelError:
			l.writer.Err(str)
		case slog.LevelWarn:
			l.writer.Warning(str)
		case slog.LevelInfo:
			l.writer.Info(str)
		case slog.LevelDebug, LevelTrace:
			l.writer.Debug(str)
		default:
			l.writer.Info(str)
		}
	}
	l.inner.Handler().Handle(context.Background(), r)

}

func (l *logger) Log(level slog.Level, module string, msg string, attrs ...any) {
	l.Write(level, module, msg, attrs...)
}

func (l *logger) With(ctx ...interface{}) Logger {
	return &logger{l.inner.With(ctx...), nil}
}

func (l *logger) New(ctx ...interface{}) Logger {
	return l.With(ctx...)
}

// Enabled reports whether l emits log records at the given context and level.
func (l *logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l.inner.Enabled(ctx, level)
}

func (l *logger) Trace(module string, msg string, ctx ...interface{}) {
	l.Write(LevelTrace, module, msg, ctx...)
}

func (l *logger) Debug(module string, msg string, ctx ...interface{}) {
	l.Write(slog.LevelDebug, module, msg, ctx...)
}

func (l *logger) Info(module string, msg string, ctx ...interface{}) {
	l.Write(slog.LevelInfo, module, msg, ctx...)
}

func (l *logger) Warn(module string, msg string, ctx ...any) {
	l.Write(slog.LevelWarn, module, msg, ctx...)
}

func (l *logger) Error(module string, msg string, ctx ...interface{}) {
	l.Write(slog.LevelError, module, msg, ctx...)
}

func (l *logger) Crit(module string, msg string, ctx ...interface{}) {
	l.Write(LevelCrit, module, msg, ctx...)
	os.Exit(1)
}
