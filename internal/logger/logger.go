// package logger provides structured debug logging for dotfile operations
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger handles writing logs to file and optionally to terminal
type Logger struct {
	file    *os.File
	mu      sync.Mutex
	verbose bool
	level   Level
}

var (
	globalLogger *Logger
	once         sync.Once
)

// Init initializes the global logger
// logDir is where log files will be written
// verbose enables terminal output
func Init(logDir string, verbose bool) error {
	var err error
	once.Do(func() {
		// ensure log directory exists
		if err = os.MkdirAll(logDir, 0755); err != nil {
			return
		}

		// create timestamped log file
		timestamp := time.Now().Format("20060102_150405")
		logPath := filepath.Join(logDir, fmt.Sprintf("dotpicker_%s.log", timestamp))

		var file *os.File
		file, err = os.Create(logPath)
		if err != nil {
			return
		}

		globalLogger = &Logger{
			file:    file,
			verbose: verbose,
			level:   DEBUG, // always log everything to file
		}

		// write header
		globalLogger.writeHeader()
	})

	return err
}

// Close closes the log file
func Close() {
	if globalLogger != nil {
		globalLogger.mu.Lock()
		defer globalLogger.mu.Unlock()
		if globalLogger.file != nil {
			globalLogger.file.Close()
		}
	}
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	log(DEBUG, format, args...)
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	log(INFO, format, args...)
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	log(WARN, format, args...)
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	log(ERROR, format, args...)
}

// Section writes a prominent section header
func Section(title string) {
	if globalLogger == nil {
		return
	}
	separator := "=========================================="
	Info("%s", separator)
	Info("%s", title)
	Info("%s", separator)
}

// log writes a log entry
func log(level Level, format string, args ...interface{}) {
	if globalLogger == nil {
		return
	}

	globalLogger.mu.Lock()
	defer globalLogger.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("%s [%s] %s\n", timestamp, level.String(), message)

	// always write to file
	if globalLogger.file != nil {
		globalLogger.file.WriteString(logLine)
	}

	// write to terminal if verbose mode
	if globalLogger.verbose {
		var out io.Writer = os.Stdout
		if level == ERROR {
			out = os.Stderr
		}
		fmt.Fprint(out, logLine)
	}
}

// writeHeader writes a header to the log file
func (l *Logger) writeHeader() {
	l.file.WriteString("================================================================================\n")
	l.file.WriteString(fmt.Sprintf("Dotfile Picker Debug Log - %s\n", time.Now().Format("2006-01-02 15:04:05")))
	l.file.WriteString("================================================================================\n\n")
}

// Enabled returns true if logging is enabled
func Enabled() bool {
	return globalLogger != nil
}

// FileMap logs a complete file map
func FileMap(fileMap map[string]string) {
	if globalLogger == nil || len(fileMap) == 0 {
		return
	}

	Info("File map contains %d entries:", len(fileMap))
	for source, target := range fileMap {
		Debug("  %s → %s", source, target)
	}
}

// DirListing logs the contents of a directory
func DirListing(path string, prefix string) {
	if globalLogger == nil {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		Debug("%sDirectory listing failed: %v", prefix, err)
		return
	}

	Debug("%sDirectory listing for: %s", prefix, path)
	for _, entry := range entries {
		if entry.IsDir() {
			Debug("%s  [DIR]  %s", prefix, entry.Name())
		} else {
			info, _ := entry.Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf(" (%d bytes)", info.Size())
			}
			Debug("%s  [FILE] %s%s", prefix, entry.Name(), size)
		}
	}
}
