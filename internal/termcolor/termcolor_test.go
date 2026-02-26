package termcolor

import (
	"os"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		input   string
		want    ColorMode
		wantErr bool
	}{
		{"auto", ColorAuto, false},
		{"always", ColorAlways, false},
		{"never", ColorNever, false},
		{"", ColorAuto, true},
		{"yes", ColorAuto, true},
		{"Auto", ColorAuto, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseColorMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseColorMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseColorMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriterColorNever(t *testing.T) {
	f, err := os.CreateTemp("", "colortest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	w := NewWriter(f, ColorNever)

	if w.Enabled() {
		t.Error("expected Enabled() = false for ColorNever")
	}

	tests := []struct {
		name string
		fn   func(string) string
	}{
		{"Red", w.Red},
		{"Green", w.Green},
		{"Yellow", w.Yellow},
		{"Cyan", w.Cyan},
		{"Bold", w.Bold},
		{"BoldCyan", w.BoldCyan},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn("hello")
			if got != "hello" {
				t.Errorf("%s(\"hello\") = %q, want %q", tt.name, got, "hello")
			}
		})
	}
}

func TestWriterColorAlways(t *testing.T) {
	f, err := os.CreateTemp("", "colortest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	w := NewWriter(f, ColorAlways)

	if !w.Enabled() {
		t.Error("expected Enabled() = true for ColorAlways")
	}

	tests := []struct {
		name     string
		fn       func(string) string
		wantCode string
	}{
		{"Red", w.Red, red},
		{"Green", w.Green, green},
		{"Yellow", w.Yellow, yellow},
		{"Cyan", w.Cyan, cyan},
		{"Bold", w.Bold, bold},
		{"BoldCyan", w.BoldCyan, boldCyan},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn("hello")
			want := tt.wantCode + "hello" + reset
			if got != want {
				t.Errorf("%s(\"hello\") = %q, want %q", tt.name, got, want)
			}
		})
	}
}

func TestShouldColorize_Pipe(t *testing.T) {
	// A pipe fd is not a terminal, so ShouldColorize should return false.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if ShouldColorize(r) {
		t.Error("ShouldColorize(pipe) = true, want false")
	}
}

func TestShouldColorize_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	// Even if we pass a real file, NO_COLOR should force false.
	f, err := os.CreateTemp("", "colortest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if ShouldColorize(f) {
		t.Error("ShouldColorize with NO_COLOR set = true, want false")
	}
}

func TestNewWriterAutoMode_Pipe(t *testing.T) {
	// In auto mode with a pipe, color should be disabled.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	cw := NewWriter(r, ColorAuto)
	if cw.Enabled() {
		t.Error("NewWriter(pipe, ColorAuto).Enabled() = true, want false")
	}
}
