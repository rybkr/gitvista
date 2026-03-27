package gitcore

import (
	"testing"
	"time"
)

func TestNewSignature(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Signature
		wantErr bool
	}{
		{
			name:  "valid signature with positive timezone",
			input: "Jane Doe <jane@example.com> 1700000000 +0200",
			want: Signature{
				Name:  "Jane Doe",
				Email: "jane@example.com",
				When:  time.Unix(1700000000, 0).In(time.FixedZone("+0200", 2*3600)),
			},
		},
		{
			name:  "valid signature with negative timezone",
			input: "John Smith <john@example.com> 1700000000 -0500",
			want: Signature{
				Name:  "John Smith",
				Email: "john@example.com",
				When:  time.Unix(1700000000, 0).In(time.FixedZone("-0500", -5*3600)),
			},
		},
		{
			name:  "valid signature with UTC offset",
			input: "Bot <bot@ci.example.com> 1700000000 +0000",
			want: Signature{
				Name:  "Bot",
				Email: "bot@ci.example.com",
				When:  time.Unix(1700000000, 0).In(time.FixedZone("+0000", 0)),
			},
		},
		{
			name:  "extra whitespace around name and email",
			input: "  Jane Doe  <  jane@example.com  > 1700000000 +0000",
			want: Signature{
				Name:  "Jane Doe",
				Email: "jane@example.com",
				When:  time.Unix(1700000000, 0).In(time.FixedZone("+0000", 0)),
			},
		},
		{
			name:  "missing timezone falls back to UTC",
			input: "Jane Doe <jane@example.com> 1700000000",
			want: Signature{
				Name:  "Jane Doe",
				Email: "jane@example.com",
				When:  time.Unix(1700000000, 0).In(time.UTC),
			},
		},
		{
			name:    "missing angle brackets",
			input:   "Jane Doe jane@example.com 1700000000 +0000",
			wantErr: true,
		},
		{
			name:    "missing timestamp",
			input:   "Jane Doe <jane@example.com>",
			wantErr: true,
		},
		{
			name:    "non-numeric timestamp",
			input:   "Jane Doe <jane@example.com> notanumber +0000",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSignature(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewSignature(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.want.Name)
			}
			if got.Email != tt.want.Email {
				t.Errorf("Email: got %q, want %q", got.Email, tt.want.Email)
			}
			if !got.When.Equal(tt.want.When) {
				t.Errorf("When: got %v, want %v", got.When, tt.want.When)
			}
			if got.When.Location().String() != tt.want.When.Location().String() {
				t.Errorf("Location: got %q, want %q",
					got.When.Location(), tt.want.When.Location())
			}
		})
	}
}

func TestParseTimezone(t *testing.T) {
	tests := []struct {
		input      string
		wantOffset int
		wantNil    bool
	}{
		{"+0000", 0, false},
		{"+0200", 2 * 3600, false},
		{"-0500", -5 * 3600, false},
		{"+0530", 5*3600 + 30*60, false},
		{"-0930", -(9*3600 + 30*60), false},
		{"UTC", 0, true},
		{"", 0, true},
		{"+25:00", 0, true},
		{"+0X00", 0, true},
		{"*0000", 0, true},
		{"+00xx", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			loc := parseTimezone(tt.input)
			if tt.wantNil {
				if loc != nil {
					t.Errorf("parseTimezone(%q) = %v, want nil", tt.input, loc)
				}
				return
			}
			if loc == nil {
				t.Fatalf("parseTimezone(%q) = nil, want offset %d", tt.input, tt.wantOffset)
			}
			_, gotOffset := time.Now().In(loc).Zone()
			if gotOffset != tt.wantOffset {
				t.Errorf("parseTimezone(%q) offset = %d, want %d", tt.input, gotOffset, tt.wantOffset)
			}
		})
	}
}
