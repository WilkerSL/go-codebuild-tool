package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ApplyResult struct {
	Written []string `json:"written"`
	Deleted []string `json:"deleted"`
}

func Apply(p *Plan) (*ApplyResult, error) {
	if conflicts := p.Conflicts(); len(conflicts) > 0 {
		lines := make([]string, len(conflicts))
		for i, c := range conflicts {
			lines[i] = "  " + c.Path + ": " + c.Reason
		}
		e := newError(KindOwnership, "ownership_conflict", "codebuild refuses to overwrite these files:\n"+strings.Join(lines, "\n"))
		e.Details = map[string]string{}
		for _, c := range conflicts {
			e.Details[c.Path] = c.Reason
		}
		return nil, e
	}
	result := &ApplyResult{Written: []string{}, Deleted: []string{}}
	for _, f := range p.Files {
		full := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		switch f.Action {
		case ActionCreate, ActionUpdate:
			if err := writeFile(full, f.content); err != nil {
				return result, newError(KindInternal, "write_file", fmt.Sprintf("write %s: %v", f.Path, err))
			}
			result.Written = append(result.Written, f.Path)
		case ActionDelete:
			content, err := os.ReadFile(full)
			if err != nil || !hasHeader(content) {
				continue
			}
			if err := os.Remove(full); err != nil {
				return result, newError(KindInternal, "delete_file", fmt.Sprintf("delete %s: %v", f.Path, err))
			}
			result.Deleted = append(result.Deleted, f.Path)
		}
	}
	return result, nil
}

func writeFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".codebuild-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
