//go:build unix

package gitcore

import (
	"math"
	"os"
	"testing"
)

func TestMapPackFile(t *testing.T) {
	tests := []struct {
		name    string
		size    int64
		wantErr bool
		wantNil bool
	}{
		{
			name:    "zero size returns nil",
			size:    0,
			wantNil: true,
		},
		{
			name:    "negative size returns error",
			size:    -1,
			wantErr: true,
		},
		{
			name:    "size exceeds MaxInt returns error",
			size:    math.MaxInt64,
			wantErr: true,
		},
		{
			name: "valid file maps successfully",
			size: 29,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "pack-*.bin")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			size := tt.size
			if size == 29 {
				content := make([]byte, 4096)
				if _, err := f.Write(content); err != nil {
					t.Fatal(err)
				}
				size = 4096
			}

			data, err := mapPackFile(f, size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mapPackFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantNil && data != nil {
				t.Errorf("mapPackFile() = %v, want nil", data)
			}
			if err == nil && data != nil {
				if err := unmapPackData(data); err != nil {
					t.Errorf("unmapPackData() error = %v", err)
				}
			}
		})
	}
}

func TestUnmapPackData(t *testing.T) {
	t.Run("empty slice is a no-op", func(t *testing.T) {
		if err := unmapPackData([]byte{}); err != nil {
			t.Errorf("unmapPackData(empty) error = %v", err)
		}
	})

	t.Run("nil slice is a no-op", func(t *testing.T) {
		if err := unmapPackData(nil); err != nil {
			t.Errorf("unmapPackData(nil) error = %v", err)
		}
	})

	t.Run("mapped data unmaps cleanly", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "pack-*.bin")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		content := make([]byte, 4096)
		if _, err := f.Write(content); err != nil {
			t.Fatal(err)
		}

		data, err := mapPackFile(f, 4096)
		if err != nil {
			t.Fatalf("mapPackFile() error = %v", err)
		}
		if err := unmapPackData(data); err != nil {
			t.Errorf("unmapPackData() error = %v", err)
		}
	})
}
