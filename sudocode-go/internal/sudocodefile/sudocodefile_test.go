package sudocodefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSudocodeDir_Directory(t *testing.T) {
	repoPath := t.TempDir()
	sudocodeDir := filepath.Join(repoPath, ".sudocode")
	if err := os.Mkdir(sudocodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSudocodeDir(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sudocodeDir {
		t.Errorf("got %q, want %q", got, sudocodeDir)
	}
}

func TestResolveSudocodeDir_FileAbsolutePath(t *testing.T) {
	repoPath := t.TempDir()
	targetDir := t.TempDir()

	content := "sudocodedir: " + targetDir + "\n"
	if err := os.WriteFile(filepath.Join(repoPath, ".sudocode"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSudocodeDir(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != targetDir {
		t.Errorf("got %q, want %q", got, targetDir)
	}
}

func TestResolveSudocodeDir_FileRelativePath(t *testing.T) {
	repoPath := t.TempDir()
	targetDir := filepath.Join(repoPath, "shared", ".sudocode-data")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "sudocodedir: shared/.sudocode-data\n"
	if err := os.WriteFile(filepath.Join(repoPath, ".sudocode"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSudocodeDir(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != targetDir {
		t.Errorf("got %q, want %q", got, targetDir)
	}
}

func TestResolveSudocodeDir_NotFound(t *testing.T) {
	repoPath := t.TempDir()

	_, err := ResolveSudocodeDir(repoPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrNotFound {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestResolveSudocodeDir_MalformedFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty file", ""},
		{"missing prefix", "/some/path\n"},
		{"wrong prefix", "gitdir: /some/path\n"},
		{"prefix only", "sudocodedir:\n"},
		{"prefix with space only", "sudocodedir: \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := t.TempDir()
			if err := os.WriteFile(filepath.Join(repoPath, ".sudocode"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := ResolveSudocodeDir(repoPath)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestWriteSudocodeFile(t *testing.T) {
	repoPath := t.TempDir()
	targetDir := "/some/external/.sudocode/projects/my-project"

	if err := WriteSudocodeFile(repoPath, targetDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(repoPath, ".sudocode"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	want := "sudocodedir: " + targetDir + "\n"
	if string(content) != want {
		t.Errorf("got %q, want %q", string(content), want)
	}
}

func TestWriteSudocodeFile_OverwritesExisting(t *testing.T) {
	repoPath := t.TempDir()
	filePath := filepath.Join(repoPath, ".sudocode")

	// Write initial
	if err := os.WriteFile(filePath, []byte("sudocodedir: /old/path\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	newDir := "/new/path"
	if err := WriteSudocodeFile(repoPath, newDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	want := "sudocodedir: " + newDir + "\n"
	if string(content) != want {
		t.Errorf("got %q, want %q", string(content), want)
	}
}
