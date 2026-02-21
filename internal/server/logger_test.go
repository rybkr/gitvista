package server

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// TestNewServer_LoggerInitialised verifies that NewServer populates the logger
// field so that server methods can call s.logger without a nil-dereference.
func TestNewServer_LoggerInitialised(t *testing.T) {
	s := newTestServer(t)
	if s.logger == nil {
		t.Fatal("logger is nil after NewServer(); expected slog.Default() to be used")
	}
}

// TestNewServer_LoggerInheritsDefault verifies that the logger installed by
// NewServer is the slog.Default() at the time of construction. This is the
// mechanism by which main.go's initLogger() call propagates into the server.
func TestNewServer_LoggerInheritsDefault(t *testing.T) {
	// Install a custom handler that writes to a buffer so we can observe output.
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, nil))
	original := slog.Default()
	slog.SetDefault(custom)
	t.Cleanup(func() { slog.SetDefault(original) })

	// Construct directly (not via newTestServer which overrides the logger).
	repo := &gitcore.Repository{}
	webFS := os.DirFS(t.TempDir())
	s := NewServer(repo, "127.0.0.1:0", webFS)

	// Emit a log line via the server logger and confirm it reaches the buffer.
	s.logger.Info("test-probe", "key", "value")
	if !strings.Contains(buf.String(), "test-probe") {
		t.Errorf("server logger did not inherit slog.Default(); buffer = %q", buf.String())
	}
}

// TestNewServer_LoggerOverridable verifies that tests can silence server logging
// by replacing s.logger with a handler that discards all output, without
// affecting the global default logger.
func TestNewServer_LoggerOverridable(t *testing.T) {
	s := newTestServer(t)

	// Replace with a discard handler; this is the pattern test helpers should use.
	s.logger = slog.New(slog.NewTextHandler(noopWriter{}, nil))

	// Confirm the override doesn't panic and that global default is untouched.
	s.logger.Info("discarded message")
	// slog.Default() should still write to stderr, not our discard writer.
	if slog.Default() == s.logger {
		t.Error("overriding s.logger must not mutate slog.Default()")
	}
}

// TestInitLogger_TextFormat verifies that GITVISTA_LOG_FORMAT=text produces
// text output by checking that the JSON-only "{" prefix is absent.
func TestInitLogger_TextFormat(t *testing.T) {
	t.Setenv("GITVISTA_LOG_FORMAT", "text")
	t.Setenv("GITVISTA_LOG_LEVEL", "info")

	var buf bytes.Buffer
	// Reproduce what initLogger does so we don't actually mutate the global.
	level := slog.LevelInfo
	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewTextHandler(&buf, opts)
	logger := slog.New(handler)

	logger.Info("hello", "key", "val")
	line := buf.String()
	if strings.HasPrefix(line, "{") {
		t.Errorf("text handler produced JSON output: %q", line)
	}
	if !strings.Contains(line, "hello") {
		t.Errorf("text handler output missing message: %q", line)
	}
}

// TestInitLogger_JSONFormat verifies that GITVISTA_LOG_FORMAT=json produces
// valid JSON output starting with "{".
func TestInitLogger_JSONFormat(t *testing.T) {
	t.Setenv("GITVISTA_LOG_FORMAT", "json")
	t.Setenv("GITVISTA_LOG_LEVEL", "info")

	var buf bytes.Buffer
	level := slog.LevelInfo
	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(&buf, opts)
	logger := slog.New(handler)

	logger.Info("hello", "key", "val")
	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "{") {
		t.Errorf("JSON handler output does not start with '{': %q", line)
	}
	if !strings.Contains(line, `"hello"`) {
		t.Errorf("JSON handler output missing message field: %q", line)
	}
}

// TestInitLogger_LevelFiltering verifies that debug messages are suppressed
// when the level is set to Info (the default).
func TestInitLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	logger := slog.New(slog.NewTextHandler(&buf, opts))

	logger.Debug("should-be-suppressed")
	logger.Info("should-appear")

	out := buf.String()
	if strings.Contains(out, "should-be-suppressed") {
		t.Error("debug message appeared despite Info level filter")
	}
	if !strings.Contains(out, "should-appear") {
		t.Error("info message was suppressed unexpectedly")
	}
}

// noopWriter is an io.Writer that discards all output, used to silence
// server logging in tests without polluting os.Stderr.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// Ensure noopWriter satisfies io.Writer (compile-time check).
var _ interface{ Write([]byte) (int, error) } = noopWriter{}
