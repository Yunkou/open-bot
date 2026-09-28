package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointAndRestore(t *testing.T) {
	root := t.TempDir()
	m := NewManager(Config{DataRoot: root})
	_, err := m.EnsureLayout("u", "b1", ModeTeam)
	if err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(m.SharedHost("u"), "a.txt")
	if err := os.WriteFile(shared, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst, err := m.Checkpoint(context.Background(), "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "shared", "a.txt")); err != nil {
		t.Fatal(err)
	}
	// Wipe live shared and restore
	_ = os.RemoveAll(m.SharedHost("u"))
	_ = os.RemoveAll(filepath.Join(m.UserHomeHost("u"), "bots"))
	if err := m.Restore(context.Background(), "u"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(shared)
	if err != nil || string(b) != "data" {
		t.Fatalf("restore: %v %q", err, b)
	}
}
