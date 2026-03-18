package main

import (
	"bytes"
	"log/slog"
	"os"
	"reflect"
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

func TestParseCORSOrigins(t *testing.T) {
	got := parseCORSOrigins(" https://a.example , ,https://b.example,https://a.example ")
	want := map[string]bool{
		"https://a.example": true,
		"https://b.example": true,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCORSOrigins() = %#v, want %#v", got, want)
	}
}

func TestParseAllowedHosts(t *testing.T) {
	got := parseAllowedHosts(" GitHub.com , ,EXAMPLE.com,github.com ")
	want := []string{"github.com", "example.com", "github.com"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAllowedHosts() = %#v, want %#v", got, want)
	}
}

func TestParseAllowedHostsEmpty(t *testing.T) {
	if got := parseAllowedHosts(""); got != nil {
		t.Fatalf("parseAllowedHosts(\"\") = %#v, want nil", got)
	}
}
