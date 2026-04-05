package logger

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	level    = LevelInfo
	mu       sync.RWMutex
	stdLog   = log.New(os.Stdout, "", log.LstdFlags)
	auditLog = log.New(os.Stdout, "[AUDIT] ", log.LstdFlags)
)

// reSensitive masks API keys, tokens, and bearer values in log output.
var reSensitive = regexp.MustCompile(
	`(?i)(sk-ant-api\d{2}-[a-zA-Z0-9_-]{6})[a-zA-Z0-9_-]*` + `|` +
		`(sk-[a-zA-Z0-9]{6})[a-zA-Z0-9]*` + `|` +
		`(AIzaSy[a-zA-Z0-9_-]{6})[a-zA-Z0-9_-]*` + `|` +
		`(Bearer\s+)\S+` + `|` +
		`(key=)[a-zA-Z0-9_-]+`,
)

func maskSensitive(s string) string {
	return reSensitive.ReplaceAllStringFunc(s, func(match string) string {
		lower := strings.ToLower(match)
		if strings.HasPrefix(lower, "bearer ") {
			return match[:7] + "***"
		}
		if strings.HasPrefix(lower, "key=") {
			return "key=***"
		}
		// Keep recognizable prefix, mask rest
		cut := len(match)
		if cut > 14 {
			cut = 14
		}
		return match[:cut] + "***"
	})
}

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
		stdLog.Output(2, maskSensitive(fmt.Sprintf("[DEBUG] "+format, args...)))
	}
}

func Info(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelInfo {
		stdLog.Output(2, maskSensitive(fmt.Sprintf("[INFO] "+format, args...)))
	}
}

func Warn(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelWarn {
		stdLog.Output(2, maskSensitive(fmt.Sprintf("[WARN] "+format, args...)))
	}
}

func Error(format string, args ...any) {
	mu.RLock()
	l := level
	mu.RUnlock()
	if l <= LevelError {
		stdLog.Output(2, maskSensitive(fmt.Sprintf("[ERROR] "+format, args...)))
	}
}

func Audit(clientIP, method, path string, status int) {
	auditLog.Output(2, fmt.Sprintf("%s %s %s → %d", clientIP, method, path, status))
}
