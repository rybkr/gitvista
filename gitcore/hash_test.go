package gitcore

import "testing"

func TestFromHexChar(t *testing.T) {
	tests := []struct {
		name string
		in   byte
		want int
	}{
		{name: "digit", in: '7', want: 7},
		{name: "lowercase", in: 'b', want: 11},
		{name: "uppercase", in: 'E', want: 14},
		{name: "invalid", in: 'x', want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromHexChar(tt.in); got != tt.want {
				t.Fatalf("fromHexChar(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
