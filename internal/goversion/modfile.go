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
