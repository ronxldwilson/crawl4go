// Package log provides a structured, levelled logger inspired by Python
// Crawl4AI's AsyncLogger. Output is slog-compatible (key=value pairs).
package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel controls which messages are emitted.
type LogLevel int

const (
	Debug LogLevel = iota
	Info
	Warn
	Error
)

// String returns the uppercase label for a log level.
func (l LogLevel) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a case-insensitive string to a LogLevel.
// It returns Info if the string is not recognised.
func ParseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return Debug
	case "info":
		return Info
	case "warn", "warning":
		return Warn
	case "error":
		return Error
	default:
		return Info
	}
}

// ANSI colour codes used when colour output is enabled.
var levelColour = map[LogLevel]string{
	Debug: "\033[36m", // cyan
	Info:  "\033[32m", // green
	Warn:  "\033[33m", // yellow
	Error: "\033[31m", // red
}

const colourReset = "\033[0m"

// Logger is a structured, synchronous logger with configurable level,
// tag, output writer, optional file output and optional colour.
type Logger struct {
	mu    sync.Mutex
	tag   string
	level LogLevel
	out   io.Writer
	file  *os.File
	color bool
}

// LoggerOption is a functional option for NewLogger.
type LoggerOption func(*Logger)

// WithOutput sets the primary output writer (default: os.Stderr).
func WithOutput(w io.Writer) LoggerOption {
	return func(l *Logger) { l.out = w }
}

// WithFile opens path for append and writes a copy of every log line to it.
func WithFile(path string) LoggerOption {
	return func(l *Logger) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// Best-effort: if the file cannot be opened, skip file output.
			return
		}
		l.file = f
	}
}

// WithColor enables or disables ANSI colour codes in the primary output.
func WithColor(enabled bool) LoggerOption {
	return func(l *Logger) { l.color = enabled }
}

// NewLogger creates a Logger with the given tag, minimum level, and options.
func NewLogger(tag string, level LogLevel, opts ...LoggerOption) *Logger {
	l := &Logger{
		tag:   tag,
		level: level,
		out:   os.Stderr,
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Close releases resources held by the logger (e.g. the log file).
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Debug logs a message at Debug level.
func (l *Logger) Debug(msg string, fields ...any) { l.log(Debug, msg, fields) }

// Info logs a message at Info level.
func (l *Logger) Info(msg string, fields ...any) { l.log(Info, msg, fields) }

// Warn logs a message at Warn level.
func (l *Logger) Warn(msg string, fields ...any) { l.log(Warn, msg, fields) }

// Error logs a message at Error level.
func (l *Logger) Error(msg string, fields ...any) { l.log(Error, msg, fields) }

// log formats and writes a structured log line if lvl >= l.level.
// Format: time=<RFC3339> level=<LEVEL> tag=<tag> msg=<msg> key=value ...
func (l *Logger) log(lvl LogLevel, msg string, fields []any) {
	if lvl < l.level {
		return
	}

	now := time.Now().Format(time.RFC3339)
	levelStr := lvl.String()

	var b strings.Builder
	b.Grow(128)

	fmt.Fprintf(&b, "time=%s level=%s tag=%s msg=%q", now, levelStr, l.tag, msg)

	// Append key=value pairs from fields (alternating key, value).
	for i := 0; i+1 < len(fields); i += 2 {
		fmt.Fprintf(&b, " %v=%v", fields[i], fields[i+1])
	}
	// If odd number of fields, append the trailing key with no value.
	if len(fields)%2 != 0 {
		fmt.Fprintf(&b, " %v=<missing>", fields[len(fields)-1])
	}

	plain := b.String() + "\n"

	l.mu.Lock()
	defer l.mu.Unlock()

	// Write to primary output, optionally with colour.
	if l.color {
		c := levelColour[lvl]
		fmt.Fprint(l.out, c+plain+colourReset)
	} else {
		fmt.Fprint(l.out, plain)
	}

	// Mirror to file (always without colour).
	if l.file != nil {
		fmt.Fprint(l.file, plain)
	}
}
