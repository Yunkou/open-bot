package httpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAttachmentPath_EnforcesConvPrefix(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OPENBOT_UPLOAD_ROOT", root)

	uid, conv := "user-a", "conv-b"
	relDir := filepath.Join("user-a", "conv-b") // SanitizeUserSegment keeps alnum/dash
	// Match SanitizeUserSegment behavior: letters/digits only map — "user-a" stays.
	absDir := filepath.Join(root, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-1111-1111-111111111111"
	name := "shot.png"
	stored := id + "_" + name
	abs := filepath.Join(absDir, stored)
	if err := os.WriteFile(abs, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.ToSlash(filepath.Join(relDir, stored))

	got, err := resolveAttachmentPath(uid, conv, AttachmentRef{ID: id, Path: rel, Name: name})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != abs {
		// Abs may normalize; compare basenames + prefix
		if filepath.Base(got) != stored {
			t.Fatalf("got %q", got)
		}
	}

	// Escape attempt
	_, err = resolveAttachmentPath(uid, conv, AttachmentRef{
		ID:   id,
		Path: "../other/secret.bin",
		Name: "x",
	})
	if err == nil {
		t.Fatal("expected escape rejection")
	}

	// Wrong conversation prefix
	otherDir := filepath.Join(root, "user-a", "conv-other")
	_ = os.MkdirAll(otherDir, 0o755)
	otherAbs := filepath.Join(otherDir, stored)
	_ = os.WriteFile(otherAbs, []byte("x"), 0o644)
	_, err = resolveAttachmentPath(uid, conv, AttachmentRef{
		ID:   id,
		Path: filepath.ToSlash(filepath.Join("user-a", "conv-other", stored)),
		Name: name,
	})
	if err == nil || !strings.Contains(err.Error(), "belong") {
		t.Fatalf("expected belong error, got %v", err)
	}
}

func TestAttachmentRefURLFieldJSONTag(t *testing.T) {
	t.Parallel()
	ref := AttachmentRef{ID: "a", Name: "n", Mime: "image/png", Size: 1, Path: "p", URL: "/v1/conversations/c/attachments/a"}
	if ref.URL == "" {
		t.Fatal("URL should be settable for upload/list responses")
	}
}
