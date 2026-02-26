package cli

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"log", "lgo", 2},   // transposition
		{"diff", "dif", 1},  // deletion
		{"stat", "stats", 1}, // insertion
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshtein(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			// Verify symmetry.
			got2 := levenshtein(tt.b, tt.a)
			if got2 != got {
				t.Errorf("levenshtein(%q, %q) = %d, but reverse = %d", tt.a, tt.b, got, got2)
			}
		})
	}
}

func TestSuggest(t *testing.T) {
	commands := []string{"log", "cat-file", "diff", "status", "version"}

	tests := []struct {
		input string
		want  string
	}{
		{"lgo", "log"},       // transposition
		{"logg", "log"},      // extra char
		{"lo", "log"},        // deletion
		{"dif", "diff"},      // missing char
		{"stauts", "status"}, // transposition
		{"cat-flie", "cat-file"}, // transposition in compound
		{"xxxxxx", ""},       // no match
		{"", ""},             // empty input
		{"version", "version"}, // exact match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Suggest(tt.input, commands)
			if got != tt.want {
				t.Errorf("Suggest(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
