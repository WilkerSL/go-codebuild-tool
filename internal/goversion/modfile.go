package goversion

import (
	"fmt"

	"golang.org/x/mod/modfile"
)

type ModFile struct {
	Module    string
	Go        string
	Toolchain string
	File      *modfile.File
}

const implicitGoVersion = "1.16"

func ParseModFile(name string, data []byte) (*ModFile, error) {
	f, err := modfile.Parse(name, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	mf := &ModFile{File: f, Go: implicitGoVersion}
	if f.Module != nil {
		mf.Module = f.Module.Mod.Path
	}
	if f.Go != nil {
		mf.Go = f.Go.Version
	}
	if f.Toolchain != nil {
		mf.Toolchain = f.Toolchain.Name
	}
	return mf, nil
}

func (m *ModFile) Target() (Target, error) {
	t, err := Resolve(m.Go)
	if err != nil {
		return Target{}, err
	}
	t.Toolchain = m.Toolchain
	return t, nil
}

func (m *ModFile) RequiredToolchain() string {
	return Max("go"+m.Go, m.Toolchain)
}

type WorkFile struct {
	Go        string
	Toolchain string
	Use       []string
}

func ParseWorkFile(name string, data []byte) (*WorkFile, error) {
	f, err := modfile.ParseWork(name, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	wf := &WorkFile{Go: implicitGoVersion}
	if f.Go != nil {
		wf.Go = f.Go.Version
	}
	if f.Toolchain != nil {
		wf.Toolchain = f.Toolchain.Name
	}
	for _, u := range f.Use {
		wf.Use = append(wf.Use, u.Path)
	}
	return wf, nil
}

func (w *WorkFile) RequiredToolchain() string {
	return Max("go"+w.Go, w.Toolchain)
}
