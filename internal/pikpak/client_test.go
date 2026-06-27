package pikpak

import "testing"

func TestIsFileID(t *testing.T) {
	id1 := "VOw7XmbR7CNXy-Fk9WWu7cQho2"      // 26 chars, with '-'
	id2 := "VOtJdblUjGOYYEEcFFYJmo8oo2"      // 26 chars, without '-'
	tests := []struct {
		s    string
		want bool
	}{
		{id1, true},
		{id2, true},
		{"short", false},
		{"toolong123456789012345678901", false}, // 27 chars
		{"", false},
		{"My Pack", false},
		{"/path/to/file", false},
	}
	for _, tt := range tests {
		got := IsFileID(tt.s)
		if got != tt.want {
			t.Errorf("IsFileID(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
