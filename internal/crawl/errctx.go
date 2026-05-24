package crawl

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorContext provides structured error reporting with the call site info.
type ErrorContext struct {
	Error    error  `json:"-"`
	Message  string `json:"message"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
	URL      string `json:"url,omitempty"`
}

func (ec *ErrorContext) String() string {
	return fmt.Sprintf("%s at %s:%d (%s)", ec.Message, ec.File, ec.Line, ec.Function)
}

// GetErrorContext captures the error with caller information.
// skip=1 means the caller of GetErrorContext, skip=2 means the caller's caller.
func GetErrorContext(err error, url string, skip int) *ErrorContext {
	pc, file, line, ok := runtime.Caller(skip)
	ec := &ErrorContext{
		Error:   err,
		Message: err.Error(),
		URL:     url,
	}
	if ok {
		// Shorten file path to last 2 components
		parts := strings.Split(strings.ReplaceAll(file, "\\", "/"), "/")
		if len(parts) > 2 {
			file = strings.Join(parts[len(parts)-2:], "/")
		}
		ec.File = file
		ec.Line = line
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			name := fn.Name()
			// Keep only the last part after the last /
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			ec.Function = name
		}
	}
	return ec
}
