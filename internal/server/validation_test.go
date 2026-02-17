package server

import (
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{name: "empty path (root)", path: "", wantErr: false},
		{name: "simple file", path: "file.txt", wantErr: false},
		{name: "simple directory", path: "src", wantErr: false},
		{name: "nested path", path: "src/internal/server", wantErr: false},
		{name: "path with dots in filename", path: "src/file.test.js", wantErr: false},
		{name: "hidden file", path: ".gitignore", wantErr: false},
		{name: "hidden directory", path: ".github/workflows", wantErr: false},

		// Invalid paths - directory traversal
		{name: "parent directory", path: "..", wantErr: true},
		{name: "parent then child", path: "../other", wantErr: true},
		{name: "nested traversal", path: "src/../../etc", wantErr: true},
		{name: "traversal in middle", path: "src/../../../etc/passwd", wantErr: true},
		{name: "encoded traversal", path: "src%2f..%2f..%2fetc", wantErr: true},
		{name: "dot dot slash", path: "src/../lib", wantErr: true},

		// Invalid paths - absolute
		{name: "unix absolute", path: "/etc/passwd", wantErr: true},
		{name: "windows absolute", path: "C:\\Windows\\System32", wantErr: true},

		// Invalid paths - null bytes
		{name: "null byte", path: "file\x00.txt", wantErr: true},
		{name: "null byte in path", path: "src/\x00/file.txt", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		// Valid paths that should be cleaned
		{name: "empty", path: "", want: "", wantErr: false},
		{name: "simple file", path: "file.txt", want: "file.txt", wantErr: false},
		{name: "remove dot slash", path: "./file.txt", want: "file.txt", wantErr: false},
		{name: "nested with dot slash", path: "./src/file.txt", want: "src/file.txt", wantErr: false},
		{name: "clean redundant slashes", path: "src//internal///server", want: "src/internal/server", wantErr: false},
		{name: "normalize backslashes", path: "src\\internal\\server", want: "src/internal/server", wantErr: false},

		// Invalid paths should error
		{name: "parent directory", path: "..", want: "", wantErr: true},
		{name: "absolute path", path: "/etc/passwd", want: "", wantErr: true},
		{name: "null byte", path: "file\x00.txt", want: "", wantErr: true},
		{name: "traversal", path: "src/../../../etc", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
