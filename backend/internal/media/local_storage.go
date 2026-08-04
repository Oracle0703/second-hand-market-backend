package media

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"second-hand-market-backend/backend/internal/common"
)

func LocalObjectPath(root, objectKey string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", common.ErrInvalidUpload
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	key := filepath.ToSlash(strings.TrimSpace(objectKey))
	if key == "" || strings.HasPrefix(key, "/") {
		return "", common.ErrInvalidUpload
	}
	parts := strings.Split(key, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", common.ErrInvalidUpload
		}
	}
	target := filepath.Join(absRoot, filepath.FromSlash(strings.Join(parts, "/")))
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == "." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", common.ErrInvalidUpload
	}
	return target, nil
}

func PublishObjectNoReplace(path string, content []byte, mode fs.FileMode) error {
	if strings.TrimSpace(path) == "" || len(content) == 0 {
		return common.ErrInvalidUpload
	}
	if mode == 0 {
		mode = 0o600
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return common.ErrInternal
	}

	tmpPath := path + "." + strconv.FormatInt(time.Now().UnixNano(), 10) + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return common.ErrInternal
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return common.ErrInternal
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return common.ErrInternal
	}
	if err := file.Close(); err != nil {
		return common.ErrInternal
	}

	if err := os.Link(tmpPath, path); err != nil {
		if os.IsExist(err) {
			return common.ErrConflict
		}
		return publishWithCreateExclusive(path, content, mode)
	}
	_ = syncParentDir(path)
	cleanupTmp = true
	return nil
}

func publishWithCreateExclusive(path string, content []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		if os.IsExist(err) {
			return common.ErrConflict
		}
		return common.ErrInternal
	}
	removeOnFailure := true
	defer func() {
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return common.ErrInternal
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return common.ErrInternal
	}
	if err := file.Close(); err != nil {
		return common.ErrInternal
	}
	removeOnFailure = false
	_ = syncParentDir(path)
	return nil
}

func RemoveLocalObject(root, objectKey string) error {
	path, err := LocalObjectPath(root, objectKey)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return common.ErrInternal
	}
	_ = os.Remove(path + ".tmp")
	return nil
}

func SHA256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	digest := sha256.New()
	size, err := copyToHash(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

func copyToHash(digest hash.Hash, reader io.Reader) (int64, error) {
	return io.Copy(digest, reader)
}

func syncParentDir(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
