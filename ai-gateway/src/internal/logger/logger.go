package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

var (
	level    = LevelInfo
	mu       sync.RWMutex
	stdLog   = log.New(os.Stdout, "", log.LstdFlags)
	auditLog = log.New(os.Stdout, "[AUDIT] ", log.LstdFlags)
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func SetLevel(l string) {
	mu.Lock()
	defer mu.Unlock()
	switch l {
	case "debug":
		level = LevelDebug
	case "info":
		level = LevelInfo
	case "warn":
		level = LevelWarn
	case "error":
		level = LevelError
	default:
		level = LevelInfo
	}
}

func Debug(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelDebug {
		stdLog.Output(2, fmt.Sprintf("[DEBUG] "+format, args...))
	}
}

func Info(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelInfo {
		stdLog.Output(2, fmt.Sprintf("[INFO] "+format, args...))
	}
}

func Warn(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelWarn {
		stdLog.Output(2, fmt.Sprintf("[WARN] "+format, args...))
	}
}

func Error(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelError {
		stdLog.Output(2, fmt.Sprintf("[ERROR] "+format, args...))
	}
}

func Audit(clientIP, method, path string, status int) {
	auditLog.Output(2, fmt.Sprintf("%s %s %s → %d", clientIP, method, path, status))
}
