package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/cli"
)

func TestParseFlagsDefaultsToServe(t *testing.T) {
	flags, err := parseFlags(nil, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandServe {
		t.Fatalf("parseFlags command = %q, want %q", flags.command, commandServe)
	}
	if flags.port != "8080" {
		t.Fatalf("parseFlags default port = %q, want %q", flags.port, "8080")
	}
}

func TestParseFlagsAcceptsLongPort(t *testing.T) {
	flags, err := parseFlags([]string{"--port", "9090"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandServe {
		t.Fatalf("parseFlags command = %q, want %q", flags.command, commandServe)
	}
	if flags.port != "9090" {
		t.Fatalf("parseFlags port = %q, want %q", flags.port, "9090")
	}
}

func TestParseFlagsRecognizesServe(t *testing.T) {
	flags, err := parseFlags([]string{"serve", "--port", "9090"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandServe {
		t.Fatalf("parseFlags command = %q, want %q", flags.command, commandServe)
	}
	if flags.port != "9090" {
		t.Fatalf("parseFlags port = %q, want %q", flags.port, "9090")
	}
}

func TestParseFlagsOpenTarget(t *testing.T) {
	flags, err := parseFlags([]string{"open", "HEAD~2"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandOpen {
		t.Fatalf("parseFlags command = %q, want %q", flags.command, commandOpen)
	}
	if flags.targetRev != "HEAD~2" {
		t.Fatalf("parseFlags targetRev = %q, want %q", flags.targetRev, "HEAD~2")
	}
}

func TestResolveBindHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "defaults to loopback",
			host: "",
			want: "127.0.0.1",
		},
		{
			name: "explicit host wins",
			host: "0.0.0.0",
			want: "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBindHost(tt.host)
			if got != tt.want {
				t.Errorf("resolveBindHost(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestApplyInvocationDefaults(t *testing.T) {
	t.Run("defaults to current directory", func(t *testing.T) {
		flags := appFlags{}
		applyInvocationDefaults(&flags)
		if flags.repoPath != "." {
			t.Fatalf("repoPath = %q, want %q", flags.repoPath, ".")
		}
	})

	t.Run("explicit repo path wins", func(t *testing.T) {
		flags := appFlags{repoPath: "/tmp/repo"}
		applyInvocationDefaults(&flags)
		if flags.repoPath != "/tmp/repo" {
			t.Fatalf("repoPath = %q, want %q", flags.repoPath, "/tmp/repo")
		}
	})
}

func TestBuildURLs(t *testing.T) {
	base, open := buildURLs("127.0.0.1:8080", launchTarget{
		CommitHash: "abcdef1234567890abcdef1234567890abcdef12",
		Path:       "internal/server",
	})
	if base != "http://127.0.0.1:8080" {
		t.Fatalf("base = %q", base)
	}
	wantOpen := "http://127.0.0.1:8080?path=internal%2Fserver#abcdef1234567890abcdef1234567890abcdef12"
	if open != wantOpen {
		t.Fatalf("open = %q, want %q", open, wantOpen)
	}
}

func TestParseFlagsHelpCommand(t *testing.T) {
	flags, err := parseFlags([]string{"help", "url"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if !flags.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if flags.command != commandURL {
		t.Fatalf("command = %q, want %q", flags.command, commandURL)
	}
	if flags.color != "auto" {
		t.Fatalf("color = %q, want %q", flags.color, "auto")
	}
}

func TestParseFlagsBareHelpCommand(t *testing.T) {
	flags, err := parseFlags([]string{"help"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if !flags.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if flags.command != commandHelp {
		t.Fatalf("command = %q, want %q", flags.command, commandHelp)
	}
	if flags.color != "auto" {
		t.Fatalf("color = %q, want %q", flags.color, "auto")
	}
}

func TestParseFlagsURLCommandOptions(t *testing.T) {
	flags, err := parseFlags([]string{"url", "--branch", "main", "--path", "internal/server", "--json"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandURL {
		t.Fatalf("command = %q, want %q", flags.command, commandURL)
	}
	if flags.branch != "main" {
		t.Fatalf("branch = %q, want %q", flags.branch, "main")
	}
	if flags.targetPath != "internal/server" {
		t.Fatalf("targetPath = %q, want %q", flags.targetPath, "internal/server")
	}
	if !flags.jsonOutput {
		t.Fatal("jsonOutput = false, want true")
	}
}

func TestParseFlagsDoctorJSON(t *testing.T) {
	flags, err := parseFlags([]string{"doctor", "--json"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandDoctor {
		t.Fatalf("command = %q, want %q", flags.command, commandDoctor)
	}
	if !flags.jsonOutput {
		t.Fatal("jsonOutput = false, want true")
	}
}

func TestParseFlagsRecognizesUpdate(t *testing.T) {
	flags, err := parseFlags([]string{"update"}, func(key, fallback string) string {
		return fallback
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if flags.command != commandUpdate {
		t.Fatalf("command = %q, want %q", flags.command, commandUpdate)
	}
}

func TestParseFlagsRejectsUnexpectedArgument(t *testing.T) {
	_, err := parseFlags([]string{"serve", "extra"}, func(key, fallback string) string {
		return fallback
	})
	if err == nil {
		t.Fatal("expected parseFlags to reject unexpected argument")
	}
	if err.Error() != "unexpected argument: extra" {
		t.Fatalf("error = %q, want %q", err.Error(), "unexpected argument: extra")
	}
}

func TestParseFlagsReportsFlagErrors(t *testing.T) {
	_, err := parseFlags([]string{"--bad-flag"}, func(key, fallback string) string {
		return fallback
	})
	if err == nil {
		t.Fatal("expected parseFlags to reject unknown flag")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestParseFlagsRejectsMissingPortValue(t *testing.T) {
	_, err := parseFlags([]string{"--port"}, func(key, fallback string) string {
		return fallback
	})
	if err == nil {
		t.Fatal("expected parseFlags to reject missing port value")
	}
	if !strings.Contains(err.Error(), "flag needs an argument") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestUpdateInstructionForPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "homebrew cellar install",
			path: "/opt/homebrew/Cellar/gitvista/1.2.3/bin/gitvista",
			want: "brew upgrade gitvista",
		},
		{
			name: "direct install",
			path: "/usr/local/bin/gitvista",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateInstructionForPath(tt.path)
			if got != tt.want {
				t.Fatalf("updateInstructionForPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRunUpdateRejectsHomebrewInstall(t *testing.T) {
	restore := stubUpdateDeps(
		func(string) (string, error) { return "v1.2.4", nil },
		func(string, string, string) error { return nil },
		func() (string, error) { return "/opt/homebrew/Cellar/gitvista/1.2.3/bin/gitvista", nil },
	)
	defer restore()

	version = "v1.2.3"
	code := runUpdate(cli.NewWriter(os.Stdout, cli.ColorNever))
	if code != 1 {
		t.Fatalf("runUpdate() = %d, want 1", code)
	}
}

func TestRunUpdateNoopWhenAlreadyCurrent(t *testing.T) {
	restore := stubUpdateDeps(
		func(string) (string, error) { return "v1.2.3", nil },
		func(string, string, string) error {
			t.Fatal("performUpdateFunc should not be called")
			return nil
		},
		func() (string, error) { return "/usr/local/bin/gitvista", nil },
	)
	defer restore()

	version = "v1.2.3"
	code := runUpdate(cli.NewWriter(os.Stdout, cli.ColorNever))
	if code != 0 {
		t.Fatalf("runUpdate() = %d, want 0", code)
	}
}

func TestRunUpdatePerformsUpdate(t *testing.T) {
	var gotRepo, gotProject, gotVersion string
	restore := stubUpdateDeps(
		func(string) (string, error) { return "v1.2.4", nil },
		func(repo, project, latest string) error {
			gotRepo = repo
			gotProject = project
			gotVersion = latest
			return nil
		},
		func() (string, error) { return "/usr/local/bin/gitvista", nil },
	)
	defer restore()

	version = "v1.2.3"
	code := runUpdate(cli.NewWriter(os.Stdout, cli.ColorNever))
	if code != 0 {
		t.Fatalf("runUpdate() = %d, want 0", code)
	}
	if gotRepo != "rybkr/gitvista" || gotProject != "gitvista" || gotVersion != "v1.2.4" {
		t.Fatalf("update args = (%q, %q, %q)", gotRepo, gotProject, gotVersion)
	}
}

func TestRunUpdatePropagatesCheckLatestError(t *testing.T) {
	restore := stubUpdateDeps(
		func(string) (string, error) { return "", errors.New("boom") },
		func(string, string, string) error { return nil },
		func() (string, error) { return "/usr/local/bin/gitvista", nil },
	)
	defer restore()

	version = "v1.2.3"
	code := runUpdate(cli.NewWriter(os.Stdout, cli.ColorNever))
	if code != 1 {
		t.Fatalf("runUpdate() = %d, want 1", code)
	}
}

func stubUpdateDeps(
	checkLatest func(string) (string, error),
	performUpdate func(string, string, string) error,
	resolveExecPath func() (string, error),
) func() {
	oldVersion := version
	oldCheckLatest := checkLatestFunc
	oldPerformUpdate := performUpdateFunc
	oldResolveExecPath := resolveExecPathFunc

	checkLatestFunc = checkLatest
	performUpdateFunc = performUpdate
	resolveExecPathFunc = resolveExecPath

	return func() {
		version = oldVersion
		checkLatestFunc = oldCheckLatest
		performUpdateFunc = oldPerformUpdate
		resolveExecPathFunc = oldResolveExecPath
	}
}
