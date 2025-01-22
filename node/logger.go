package node

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type DebugLogger struct {
	loggers map[string]*log.Logger
	mu      sync.Mutex
}

func NewLoggerManager() *DebugLogger {
	return &DebugLogger{
		loggers: make(map[string]*log.Logger),
	}
}

const (
	testing_record = "testing_record"
	EG_status      = "EG_status"
	EG_error       = "EG_error"
	stream_error   = "stream_error"
	grandpa_status = "grandpa_status"
	grandpa_error  = "grandpa_error"
	audit_status   = "audit_status"
	audit_error    = "audit_error"
)

var DefaultLogger = []string{
	testing_record,
	EG_status,
	EG_error,
	stream_error,
	grandpa_status,
	grandpa_error,
	audit_status,
	audit_error,
}

const logDir = "./logs"
const clearOnStart = false

func (lm *DebugLogger) SetLogger(name string) *log.Logger {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}
	fileFlags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if clearOnStart {
		fileFlags = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}
	logFile := filepath.Join(logDir, name+".log")
	file, err := os.OpenFile(logFile, fileFlags, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file %s: %v", logFile, err)
	}

	logger := log.New(file, name+": ", log.Ldate|log.Ltime)
	lm.loggers[name] = logger
	return logger
}

func (lm *DebugLogger) GetLogger(name string) (*log.Logger, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if logger, ok := lm.loggers[name]; ok {
		return logger, nil
	} else {
		return nil, fmt.Errorf("logger %s not found", name)
	}
}

func InitDefaultLoggers() *DebugLogger {
	lm := NewLoggerManager()
	for _, name := range DefaultLogger {
		lm.SetLogger(name)
	}
	return lm
}

func (lm *DebugLogger) RecordLogs(name string, log string, with_timestamp bool) error {
	if lm == nil {
		return nil
	}
	logger, err := lm.GetLogger(name)
	if err != nil {
		return err
	}
	if with_timestamp {
		currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
		logger.Printf("[%d] %s", currJCE, log)
	} else {
		logger.Printf("%s", log)
	}
	return nil
}
