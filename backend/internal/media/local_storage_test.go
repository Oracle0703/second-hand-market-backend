package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"second-hand-market-backend/backend/internal/common"
)

func TestLocalObjectPathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{
		"",
		".",
		"..",
		"../escape.jpg",
		"/escape.jpg",
		`product_image\..\escape.jpg`,
		"product_image/detail-v1/../escape.jpg",
	} {
		if path, err := LocalObjectPath(root, key); err == nil {
			t.Fatalf("unsafe key %q resolved to %q", key, path)
		}
	}

	path, err := LocalObjectPath(root, "product_image/detail-v1/F1.jpg")
	if err != nil {
		t.Fatalf("valid object key rejected: %v", err)
	}
	want := filepath.Join(root, "product_image", "detail-v1", "F1.jpg")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestPublishObjectNoReplaceKeepsExistingBytes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "product_image", "detail-v1", "F1.jpg")
	if err := PublishObjectNoReplace(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("initial publish: %v", err)
	}

	err := PublishObjectNoReplace(target, []byte("new"), 0o600)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("overwrite error = %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "old" {
		t.Fatalf("target was overwritten: %q", content)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

func TestRemoveLocalObjectIsIdempotent(t *testing.T) {
	root := t.TempDir()
	key := "product_image/detail-v1/F1.jpg"
	path, err := LocalObjectPath(root, key)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if err := PublishObjectNoReplace(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("publish target: %v", err)
	}
	if err := RemoveLocalObject(root, key); err != nil {
		t.Fatalf("remove existing object: %v", err)
	}
	if err := RemoveLocalObject(root, key); err != nil {
		t.Fatalf("remove missing object should be idempotent: %v", err)
	}
}
