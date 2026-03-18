package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestParseFlagsUsesHostedDefaults(t *testing.T) {
	flags, err := parseFlags(nil, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.dataDir != defaultHostedDataDir {
		t.Fatalf("dataDir = %q, want %q", flags.dataDir, defaultHostedDataDir)
	}
	if flags.port != defaultHostedPort {
		t.Fatalf("port = %q, want %q", flags.port, defaultHostedPort)
	}
	if flags.host != defaultHostedHost {
		t.Fatalf("host = %q, want %q", flags.host, defaultHostedHost)
	}
}

func TestParseFlagsAllowsOverrides(t *testing.T) {
	flags, err := parseFlags([]string{"--data-dir", "/tmp/site", "--port", "9090", "--host", "127.0.0.1"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.dataDir != "/tmp/site" {
		t.Fatalf("dataDir = %q, want %q", flags.dataDir, "/tmp/site")
	}
	if flags.port != "9090" {
		t.Fatalf("port = %q, want %q", flags.port, "9090")
	}
	if flags.host != "127.0.0.1" {
		t.Fatalf("host = %q, want %q", flags.host, "127.0.0.1")
	}
}

func TestHostedLogFormatDefaultsToJSON(t *testing.T) {
	t.Setenv("GITVISTA_LOG_FORMAT", "")
	t.Setenv("GITVISTA_LOG_LEVEL", "")

	var buf bytes.Buffer
	originalStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = originalStderr
	})

	originalDefault := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(originalDefault)
	})

	initLogger()
	slog.Info("hello", "key", "val")

	if err := w.Close(); err != nil {
		t.Fatalf("closing write pipe: %v", err)
	}
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("reading pipe output: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "{") {
		t.Fatalf("log output = %q, want JSON", line)
	}
	if !strings.Contains(line, `"msg":"hello"`) {
		t.Fatalf("log output missing message: %q", line)
	}
}
