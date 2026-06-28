package pikpak

import "testing"

func TestIsFileID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		// Valid IDs (exactly 26 chars, mixed case + digits, may have -)
		{"VOw7XmbR7CNXy-Fk9WWu7cQho2", true},  // 26 chars with -
		{"VOtJdblUjGOYYEEcFFYJmo8oo2", true},  // 26 chars without -
		{"VOwDrfHEL1n6I5mr2b3SsoUdo2", true},  // 26 chars
		{"abcDEF123xyzUVWXYZ456qrst0", true},  // 26 chars mixed case + digits
		{"Aa1Bb2Cc3Dd4Ee5Ff6Gg7Hh8Ij", true},  // 26 chars mixed

		// Contains '/' - definitely a path
		{"/path/to/file", false},
		{"folder/subfolder", false},
		{"VOw7XmbR7CNXy-Fk9WWu7cQho2/sub", false}, // even if first part looks like ID

		// Wrong length (not 26)
		{"VOw7XmbR7CNXy-Fk9WWu7cQf", false},   // 24 chars, too short
		{"abc123ABC-0987654321", false},       // 22 chars
		{"short", false},
		{"", false},
		{"a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5", false}, // 32 chars, too long

		// 26 chars but missing required character types
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", false}, // only uppercase
		{"abcdefghijklmnopqrstuvwxyz", false}, // only lowercase
		{"12345678901234567890123456", false}, // only digits
		{"aBcDeFgHiJkLmNoPqRsTuVwXyZ", false}, // mixed case but no digits

		// 26 chars but contains invalid characters
		{"My_Documents_Folder_2024!", false},  // 26 chars but has _ and !
		{"My_Documents_Folder_2024", false},   // 26 chars but has _
		{"My Pack and more files!!", false},   // has spaces and !
		{"file.txt.backup.old.2024", false},   // has dots

		// 26 chars, mixed case, has digits, but has invalid chars
		{"aBc123XyZ-test_file_Name", false},   // has _ (underscore not allowed)
		{"aBc123XyZ test file Name", false},   // has spaces
		{"aBc123XyZ.test.file.Name", false},   // has dots
	}
	for _, tt := range tests {
		got := IsFileID(tt.s)
		if got != tt.want {
			t.Errorf("IsFileID(%q) = %v, want %v (length=%d)", tt.s, got, tt.want, len(tt.s))
		}
	}
}
