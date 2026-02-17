package server

import (
	"reflect"
	"testing"
)

func TestParsePorcelainStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   *WorkingTreeStatus
	}{
		{
			name:   "empty status",
			output: "",
			want: &WorkingTreeStatus{
				Staged:    []FileStatus{},
				Modified:  []FileStatus{},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "untracked files",
			output: "?? file1.txt\n?? src/file2.go\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{},
				Modified: []FileStatus{},
				Untracked: []FileStatus{
					{Path: "file1.txt", StatusCode: "?"},
					{Path: "src/file2.go", StatusCode: "?"},
				},
			},
		},
		{
			name:   "staged files",
			output: "A  new-file.txt\nM  modified.go\nD  deleted.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "new-file.txt", StatusCode: "A"},
					{Path: "modified.go", StatusCode: "M"},
					{Path: "deleted.txt", StatusCode: "D"},
				},
				Modified:  []FileStatus{},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "worktree modifications",
			output: " M file1.txt\n D file2.go\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{},
				Modified: []FileStatus{
					{Path: "file1.txt", StatusCode: "M"},
					{Path: "file2.go", StatusCode: "D"},
				},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "staged and modified",
			output: "MM both-changed.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "both-changed.txt", StatusCode: "M"},
				},
				Modified: []FileStatus{
					{Path: "both-changed.txt", StatusCode: "M"},
				},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "rename in index",
			output: "R  old-name.txt -> new-name.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "new-name.txt", StatusCode: "R"},
				},
				Modified:  []FileStatus{},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "copy in index",
			output: "C  original.txt -> copy.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "original.txt -> copy.txt", StatusCode: "C"},
				},
				Modified:  []FileStatus{},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "mixed status",
			output: "M  staged.go\n M modified.go\n?? untracked.txt\nA  new.go\n D deleted.go\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "staged.go", StatusCode: "M"},
					{Path: "new.go", StatusCode: "A"},
				},
				Modified: []FileStatus{
					{Path: "modified.go", StatusCode: "M"},
					{Path: "deleted.go", StatusCode: "D"},
				},
				Untracked: []FileStatus{
					{Path: "untracked.txt", StatusCode: "?"},
				},
			},
		},
		{
			name:   "lines too short are ignored",
			output: "M  valid.txt\nXY\n?? another.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "valid.txt", StatusCode: "M"},
				},
				Modified: []FileStatus{},
				Untracked: []FileStatus{
					{Path: "another.txt", StatusCode: "?"},
				},
			},
		},
		{
			name:   "rename with spaces in path",
			output: "R  old file.txt -> new file.txt\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{
					{Path: "new file.txt", StatusCode: "R"},
				},
				Modified:  []FileStatus{},
				Untracked: []FileStatus{},
			},
		},
		{
			name:   "trailing newline handling",
			output: "?? file.txt\n\n",
			want: &WorkingTreeStatus{
				Staged: []FileStatus{},
				Modified: []FileStatus{},
				Untracked: []FileStatus{
					{Path: "file.txt", StatusCode: "?"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePorcelainStatus(tt.output)

			if !reflect.DeepEqual(got.Staged, tt.want.Staged) {
				t.Errorf("Staged mismatch\ngot:  %+v\nwant: %+v", got.Staged, tt.want.Staged)
			}
			if !reflect.DeepEqual(got.Modified, tt.want.Modified) {
				t.Errorf("Modified mismatch\ngot:  %+v\nwant: %+v", got.Modified, tt.want.Modified)
			}
			if !reflect.DeepEqual(got.Untracked, tt.want.Untracked) {
				t.Errorf("Untracked mismatch\ngot:  %+v\nwant: %+v", got.Untracked, tt.want.Untracked)
			}
		})
	}
}

func TestParsePorcelainStatus_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		output string
		desc   string
	}{
		{
			name:   "unicode paths",
			output: "?? 日本語.txt\n?? файл.go\n",
			desc:   "should handle non-ASCII filenames",
		},
		{
			name:   "paths with special chars",
			output: "M  file-with-dash.txt\nA  file_with_underscore.go\n?? file.test.js\n",
			desc:   "should handle filenames with special characters",
		},
		{
			name:   "deeply nested paths",
			output: "M  a/b/c/d/e/f/g/h/i/j/file.txt\n",
			desc:   "should handle deeply nested paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := parsePorcelainStatus(tt.output)

			// Just verify it doesn't panic and returns a valid structure
			if status == nil {
				t.Fatal("parsePorcelainStatus returned nil")
			}
			if status.Staged == nil || status.Modified == nil || status.Untracked == nil {
				t.Error("parsePorcelainStatus returned nil slices")
			}
		})
	}
}
