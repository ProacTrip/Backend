package pagination

import (
	"encoding/base64"
	"testing"
)

func TestEncodeCursor(t *testing.T) {
	tests := []struct {
		name   string
		offset int
	}{
		{"zero offset", 0},
		{"positive offset", 10},
		{"large offset", 99999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor := EncodeCursor(tt.offset)
			if cursor == "" {
				t.Error("expected non-empty cursor")
			}
		})
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name     string
		cursor   string
		want     int
		wantErr  bool
	}{
		{
			name:   "empty cursor returns zero",
			cursor: "",
			want:   0,
		},
		{
			name:   "valid cursor",
			cursor: EncodeCursor(42),
			want:   42,
		},
		{
			name:   "zero offset",
			cursor: EncodeCursor(0),
			want:   0,
		},
		{
			name:   "large offset roundtrip",
			cursor: EncodeCursor(50000),
			want:   50000,
		},
		{
			name:    "malformed base64 returns zero gracefully",
			cursor:  "!!!not-valid-base64!!!",
			want:    0,
			wantErr: false,
		},
		{
			name:    "valid base64 but invalid json returns zero gracefully",
			cursor:  base64.StdEncoding.EncodeToString([]byte(`{bad json`)),
			want:    0,
			wantErr: false,
		},
		{
			name:    "valid json but missing offset field returns zero",
			cursor:  base64.StdEncoding.EncodeToString([]byte(`{"foo":"bar"}`)),
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeCursor(tt.cursor)
			if tt.wantErr && err == nil {
				t.Errorf("DecodeCursor(%q) expected error, got nil", tt.cursor)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("DecodeCursor(%q) unexpected error: %v", tt.cursor, err)
			}
			if got != tt.want {
				t.Errorf("DecodeCursor(%q) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	offsets := []int{0, 1, 10, 50, 100, 999, 10000}
	for _, offset := range offsets {
		cursor := EncodeCursor(offset)
		decoded, err := DecodeCursor(cursor)
		if err != nil {
			t.Errorf("roundtrip offset=%d: unexpected error: %v", offset, err)
		}
		if decoded != offset {
			t.Errorf("roundtrip offset=%d: got %d", offset, decoded)
		}
	}
}
