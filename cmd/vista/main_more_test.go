package main

import "testing"

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
