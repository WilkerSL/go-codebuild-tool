package project

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

func Within(root, path string) error {
	realRoot, err := resolveExisting(root)
	if err != nil {
		return newError(KindUsage, "invalid_dir", err.Error())
	}
	real, err := resolveExisting(path)
	if err != nil {
		return newError(KindUsage, "invalid_dir", err.Error())
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || !filepath.IsLocal(rel) {
		e := newError(KindUsage, "outside_root", fmt.Sprintf("%s resolves to %s, which is outside %s", path, real, realRoot))
		e.Details = map[string]string{"path": path, "resolved": real, "root": realRoot}
		return e
	}
	return nil
}

func resolveExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		real, err := filepath.EvalSymlinks(abs)
		if err == nil {
			return filepath.Join(append([]string{real}, missing...)...), nil
		}
		parent := filepath.Dir(abs)
		if !errors.Is(err, fs.ErrNotExist) || parent == abs {
			return "", err
		}
		missing = append([]string{filepath.Base(abs)}, missing...)
		abs = parent
	}
}
