package crawl

import (
	"fmt"
	"net/http"
)

// CrawlErrorCode identifies the category of crawl failure.
type CrawlErrorCode string

const (
	ErrCodeTimeout          CrawlErrorCode = "timeout"
	ErrCodeConnectionRefused CrawlErrorCode = "connection_refused"
	ErrCodeDNSFailure       CrawlErrorCode = "dns_failure"
	ErrCodeTLSFailure       CrawlErrorCode = "tls_failure"
	ErrCodeHTTPStatus       CrawlErrorCode = "http_status"
	ErrCodeBlocked          CrawlErrorCode = "blocked"
	ErrCodeRateLimited      CrawlErrorCode = "rate_limited"
	ErrCodeContentEmpty     CrawlErrorCode = "content_empty"
	ErrCodeRobotsBlocked    CrawlErrorCode = "robots_blocked"
	ErrCodeParseFailure     CrawlErrorCode = "parse_failure"
	ErrCodeBrowserError     CrawlErrorCode = "browser_error"
	ErrCodeContextCancelled CrawlErrorCode = "context_cancelled"
)

// CrawlError is a structured error with code, status, and retry guidance.
type CrawlError struct {
	Code       CrawlErrorCode `json:"code"`
	Message    string         `json:"message"`
	StatusCode int            `json:"status_code,omitempty"`
	URL        string         `json:"url,omitempty"`
	Retryable  bool           `json:"retryable"`
	Cause      error          `json:"-"`
}

func (e *CrawlError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CrawlError) Unwrap() error {
	return e.Cause
}

// NewTimeoutError creates a timeout error.
func NewTimeoutError(url string, cause error) *CrawlError {
	return &CrawlError{
		Code: ErrCodeTimeout, Message: "request timed out",
		URL: url, Retryable: true, Cause: cause,
	}
}

// NewHTTPStatusError creates an error from an HTTP response status.
func NewHTTPStatusError(url string, statusCode int) *CrawlError {
	retryable := statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode >= 500

	code := ErrCodeHTTPStatus
	if statusCode == http.StatusTooManyRequests {
		code = ErrCodeRateLimited
	}
	if statusCode == http.StatusForbidden || statusCode == http.StatusUnavailableForLegalReasons {
		code = ErrCodeBlocked
	}

	return &CrawlError{
		Code: code, Message: http.StatusText(statusCode),
		StatusCode: statusCode, URL: url, Retryable: retryable,
	}
}

// NewBlockedError creates an error for anti-bot blocking.
func NewBlockedError(url string, reason string) *CrawlError {
	return &CrawlError{
		Code: ErrCodeBlocked, Message: reason,
		URL: url, Retryable: true,
	}
}

// NewConnectionError creates an error for connection failures.
func NewConnectionError(url string, cause error) *CrawlError {
	return &CrawlError{
		Code: ErrCodeConnectionRefused, Message: cause.Error(),
		URL: url, Retryable: true, Cause: cause,
	}
}

// NewBrowserError creates an error for CDP/browser failures.
func NewBrowserError(url string, cause error) *CrawlError {
	return &CrawlError{
		Code: ErrCodeBrowserError, Message: cause.Error(),
		URL: url, Retryable: true, Cause: cause,
	}
}

// NewContentEmptyError creates an error when page content is empty/insufficient.
func NewContentEmptyError(url string) *CrawlError {
	return &CrawlError{
		Code: ErrCodeContentEmpty, Message: "page returned empty or insufficient content",
		URL: url, Retryable: true,
	}
}

// IsRetryable checks if an error (possibly wrapped) is retryable.
func IsRetryable(err error) bool {
	if ce, ok := err.(*CrawlError); ok {
		return ce.Retryable
	}
	return false
}

// ErrorCode extracts the CrawlErrorCode from an error, or empty string if not a CrawlError.
func ErrorCode(err error) CrawlErrorCode {
	if ce, ok := err.(*CrawlError); ok {
		return ce.Code
	}
	return ""
}
