package project

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const LockFile = ".codebuild.lock"

const staleLock = 2 * time.Minute

type ApplyResult struct {
	Written []string `json:"written"`
	Deleted []string `json:"deleted"`
}

type stagedFile struct {
	path string
	full string
	tmp  string
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
	unlock, err := lockProject(p.Root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	result := &ApplyResult{Written: []string{}, Deleted: []string{}}
	var changes []FileChange
	for _, f := range p.Files {
		switch f.Action {
		case ActionCreate, ActionUpdate, ActionDelete:
		default:
			continue
		}
		full := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		if err := Within(p.Root, full); err != nil {
			return result, err
		}
		if err := checkUnchanged(f, full); err != nil {
			return result, err
		}
		changes = append(changes, f)
	}
	slices.SortStableFunc(changes, func(a, b FileChange) int {
		return boolOrder(a.Owner == OwnerState) - boolOrder(b.Owner == OwnerState)
	})
	var staged []stagedFile
	discard := func() {
		for _, s := range staged {
			os.Remove(s.tmp)
		}
	}
	for _, f := range changes {
		if f.Action == ActionDelete {
			continue
		}
		full := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		tmp, err := stageFile(full, f.content)
		if err != nil {
			discard()
			return result, newError(KindInternal, "write_file", fmt.Sprintf("write %s: %v", f.Path, err))
		}
		staged = append(staged, stagedFile{path: f.Path, full: full, tmp: tmp})
	}
	for i, s := range staged {
		if err := os.Rename(s.tmp, s.full); err != nil {
			staged = staged[i:]
			discard()
			return result, newError(KindInternal, "write_file", fmt.Sprintf("write %s: %v", s.path, err))
		}
		result.Written = append(result.Written, s.path)
	}
	for _, f := range changes {
		if f.Action != ActionDelete {
			continue
		}
		full := filepath.Join(p.Root, filepath.FromSlash(f.Path))
		content, err := os.ReadFile(full)
		if err != nil || !hasHeader(content) {
			continue
		}
		if err := os.Remove(full); err != nil {
			return result, newError(KindInternal, "delete_file", fmt.Sprintf("delete %s: %v", f.Path, err))
		}
		result.Deleted = append(result.Deleted, f.Path)
	}
	return result, nil
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

func checkUnchanged(f FileChange, full string) error {
	current, err := os.ReadFile(full)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return newError(KindInternal, "read_file", err.Error())
	}
	stale := false
	switch f.Action {
	case ActionCreate:
		stale = exists && !bytes.Equal(normalize(current), f.content)
	case ActionUpdate, ActionDelete:
		stale = !exists || !bytes.Equal(normalize(current), f.old)
	}
	if stale {
		e := newError(KindOwnership, "plan_outdated", fmt.Sprintf("%s changed after the plan was computed; run the build again", f.Path))
		e.Details = map[string]string{f.Path: "changed after the plan was computed"}
		return e
	}
	return nil
}

func stageFile(path string, content []byte) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".codebuild-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func lockProject(root string) (func(), error) {
	path := filepath.Join(root, LockFile)
	for range 2 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, werr := f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
			cerr := f.Close()
			if err := errors.Join(werr, cerr); err != nil {
				os.Remove(path)
				return nil, newError(KindInternal, "lock", err.Error())
			}
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, newError(KindInternal, "lock", err.Error())
		}
		info, err := os.Stat(path)
		if err != nil || time.Since(info.ModTime()) < staleLock {
			break
		}
		os.Remove(path)
	}
	owner, _ := os.ReadFile(path)
	e := newError(KindOwnership, "locked", fmt.Sprintf(
		"another codebuild build or MCP generate (process %s) is writing to %s; wait for it to finish, or delete %s if no codebuild is running",
		strings.TrimSpace(string(owner)), root, LockFile))
	e.Details = map[string]string{"lock": path}
	return nil, e
}
