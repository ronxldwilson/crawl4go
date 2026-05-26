package log

import (
	"strings"
	"testing"
)

// ---- LogLevel.String --------------------------------------------------------

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{Debug, "DEBUG"},
		{Info, "INFO"},
		{Warn, "WARN"},
		{Error, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.level.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- ParseLevel -------------------------------------------------------------

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", Debug},
		{"DEBUG", Debug},
		{"info", Info},
		{"INFO", Info},
		{"warn", Warn},
		{"WARN", Warn},
		{"warning", Warn},
		{"WARNING", Warn},
		{"error", Error},
		{"ERROR", Error},
		{"unknown", Info},  // falls back to Info
		{"", Info},         // empty falls back to Info
		{"verbose", Info},  // unrecognised falls back to Info
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := ParseLevel(tc.input); got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ---- NewLogger + basic output -----------------------------------------------

func TestLogger_InfoOutput(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("test", Info, WithOutput(&buf))
	l.Info("hello world")

	out := buf.String()
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("output missing level=INFO: %q", out)
	}
	if !strings.Contains(out, `msg="hello world"`) {
		t.Errorf("output missing msg: %q", out)
	}
	if !strings.Contains(out, "tag=test") {
		t.Errorf("output missing tag=test: %q", out)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("filter", Warn, WithOutput(&buf))

	l.Debug("should be suppressed")
	l.Info("also suppressed")
	l.Warn("this appears")
	l.Error("this too")

	out := buf.String()
	if strings.Contains(out, "should be suppressed") {
		t.Error("Debug message should be suppressed at Warn level")
	}
	if strings.Contains(out, "also suppressed") {
		t.Error("Info message should be suppressed at Warn level")
	}
	if !strings.Contains(out, "this appears") {
		t.Error("Warn message should appear")
	}
	if !strings.Contains(out, "this too") {
		t.Error("Error message should appear")
	}
}

func TestLogger_KeyValueFields(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("kv", Debug, WithOutput(&buf))
	l.Debug("metrics", "count", 42, "status", "ok")

	out := buf.String()
	if !strings.Contains(out, "count=42") {
		t.Errorf("expected count=42 in output: %q", out)
	}
	if !strings.Contains(out, "status=ok") {
		t.Errorf("expected status=ok in output: %q", out)
	}
}

func TestLogger_OddFields(t *testing.T) {
	// Odd number of fields: trailing key should appear with <missing> value.
	var buf strings.Builder
	l := NewLogger("odd", Debug, WithOutput(&buf))
	l.Debug("msg", "key1", "val1", "orphan")

	out := buf.String()
	if !strings.Contains(out, "orphan=<missing>") {
		t.Errorf("expected orphan=<missing> in output: %q", out)
	}
}

func TestLogger_AllLevels(t *testing.T) {
	levels := []struct {
		name string
		fn   func(*Logger, string, ...any)
		want string
	}{
		{"debug", (*Logger).Debug, "DEBUG"},
		{"info", (*Logger).Info, "INFO"},
		{"warn", (*Logger).Warn, "WARN"},
		{"error", (*Logger).Error, "ERROR"},
	}
	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			l := NewLogger("t", Debug, WithOutput(&buf))
			tc.fn(l, "msg")
			if !strings.Contains(buf.String(), "level="+tc.want) {
				t.Errorf("expected level=%s in output: %q", tc.want, buf.String())
			}
		})
	}
}

// ---- WithColor --------------------------------------------------------------

func TestLogger_WithColor(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("color", Info, WithOutput(&buf), WithColor(true))
	l.Info("colored")

	out := buf.String()
	// ANSI escape sequence should be present.
	if !strings.Contains(out, "\033[") {
		t.Errorf("expected ANSI escape in colored output: %q", out)
	}
}

func TestLogger_WithoutColor(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("nocolor", Info, WithOutput(&buf), WithColor(false))
	l.Info("plain")

	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Errorf("unexpected ANSI escape in non-colored output: %q", out)
	}
}

// ---- Close ------------------------------------------------------------------

func TestLogger_Close_NoFile(t *testing.T) {
	l := NewLogger("close", Info)
	if err := l.Close(); err != nil {
		t.Errorf("Close without file should return nil, got %v", err)
	}
}

// ---- Concurrent safety ------------------------------------------------------

func TestLogger_ConcurrentWrites(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("concurrent", Debug, WithOutput(&buf))
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			l.Info("concurrent message")
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// ---- RFC3339 timestamp presence ---------------------------------------------

func TestLogger_TimestampFormat(t *testing.T) {
	var buf strings.Builder
	l := NewLogger("ts", Info, WithOutput(&buf))
	l.Info("check timestamp")

	out := buf.String()
	if !strings.HasPrefix(out, "time=") {
		t.Errorf("expected output to start with time=, got: %q", out)
	}
}
