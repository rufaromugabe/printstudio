package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nFOO=bar\nexport BAZ=qux\nQUOTED=\"hello world\"\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOO", "keep")
	if !loadEnvFile(path) {
		t.Fatal("expected loadEnvFile to succeed")
	}
	if got := os.Getenv("FOO"); got != "keep" {
		t.Fatalf("FOO=%q, want keep (existing env must win)", got)
	}
	if got := os.Getenv("BAZ"); got != "qux" {
		t.Fatalf("BAZ=%q, want qux", got)
	}
	if got := os.Getenv("QUOTED"); got != "hello world" {
		t.Fatalf("QUOTED=%q, want hello world", got)
	}
}
