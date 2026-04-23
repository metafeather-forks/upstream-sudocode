package projectid

import (
	"testing"
)

func TestGenerate(t *testing.T) {
	// Reference outputs from the TypeScript implementation
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/my-app", "my-app-b9703f12"},
		{"/Users/metafeather/Developer/src/metafeather.net/metafeather-forks/vendor-sudocode", "vendor-sudocode-b20d41b9"},
		{"/tmp/test", "test-0872effe"},
		{"/home/user/My Cool Project!!!", "my-cool-project-eef0e7fc"},
		{"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/this-is-a-very-long-directory-name-that-exceeds-32-chars", "this-is-a-very-long-directory-na-93e7470d"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := Generate(tt.path)
			if got != tt.want {
				t.Errorf("Generate(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestHashUUIDToBase36(t *testing.T) {
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		length int
		want   string
	}{
		{4, "s6d5"},
		{5, "cituj"},
		{6, "5kd9kg"},
	}
	for _, tt := range tests {
		got := HashUUIDToBase36(uuid, tt.length)
		if got != tt.want {
			t.Errorf("HashUUIDToBase36(%q, %d) = %q, want %q", uuid, tt.length, got, tt.want)
		}
	}
}

func TestGetAdaptiveHashLength(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{0, 4},
		{979, 4},
		{980, 5},
		{5899, 5},
		{5900, 6},
		{34999, 6},
		{35000, 7},
		{211999, 7},
		{212000, 8},
	}
	for _, tt := range tests {
		got := GetAdaptiveHashLength(tt.count)
		if got != tt.want {
			t.Errorf("GetAdaptiveHashLength(%d) = %d, want %d", tt.count, got, tt.want)
		}
	}
}

func TestIsLegacyID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"SPEC-001", true},
		{"ISSUE-123", true},
		{"s-x7k9", false},
		{"i-a3f2", false},
		{"random", false},
	}
	for _, tt := range tests {
		got := IsLegacyID(tt.id)
		if got != tt.want {
			t.Errorf("IsLegacyID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestIsHashID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"s-x7k9", true},
		{"i-a3f2", true},
		{"s-x7k9p1a4b", false}, // 9 chars after prefix, max is 8
		{"SPEC-001", false},
		{"random", false},
	}
	for _, tt := range tests {
		got := IsHashID(tt.id)
		if got != tt.want {
			t.Errorf("IsHashID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestGetEntityTypeFromID(t *testing.T) {
	tests := []struct {
		id      string
		want    string
		wantErr bool
	}{
		{"i-x7k9", "issue", false},
		{"s-a3f2", "spec", false},
		{"ISSUE-001", "issue", false},
		{"SPEC-001", "spec", false},
		{"random", "", true},
	}
	for _, tt := range tests {
		got, err := GetEntityTypeFromID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("GetEntityTypeFromID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("GetEntityTypeFromID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
