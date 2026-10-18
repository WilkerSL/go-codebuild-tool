package project

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

type DomainInfo struct {
	Domain   string `json:"domain"`
	Entity   string `json:"entity"`
	Table    string `json:"table"`
	File     string `json:"file"`
	Path     string `json:"path"`
	Routes   bool   `json:"routes"`
	ReadOnly bool   `json:"readOnly"`
}

type Info struct {
	Root         string                 `json:"root"`
	Module       string                 `json:"module"`
	Dialect      string                 `json:"dialect"`
	Configured   bool                   `json:"configured"`
	GoVersion    string                 `json:"goVersion"`
	Toolchain    string                 `json:"toolchain,omitempty"`
	Status       goversion.Status       `json:"status"`
	Capabilities goversion.Capabilities `json:"capabilities"`
	Domains      []DomainInfo           `json:"domains"`
	Diagnostics  schema.Diagnostics     `json:"diagnostics,omitempty"`
}

type project struct {
	root   string
	mod    *goversion.ModFile
	cfg    Config
	exists bool
	target goversion.Target
	d      dialect.Dialect
}

func open(dir string) (*project, error) {
	p := &planner{opts: Options{Dir: dir}, plan: &Plan{}}
	if err := p.locate(context.Background()); err != nil {
		return nil, err
	}
	if err := p.loadConfig(context.Background()); err != nil {
		return nil, err
	}
	target, err := p.mod.Target()
	if err != nil {
		return nil, newError(KindConfig, "invalid_go_version", err.Error())
	}
	if target.Status == goversion.StatusBelowMinimum {
		latest, _ := goversion.Resolve(goversion.LatestVersion())
		target.Caps = latest.Caps
	}
	return &project{root: p.root, mod: p.mod, cfg: p.cfg, exists: p.cfgExisted, target: target, d: p.dialect}, nil
}

func Inspect(dir string) (*Info, error) {
	pr, err := open(dir)
	if err != nil {
		return nil, err
	}
	sources, err := spec.LoadDir(pr.root, SpecsDir)
	if err != nil {
		return nil, newError(KindInternal, "read_specs", err.Error())
	}
	model, diags := analyze(sources, pr.d, pr.target.Caps, pr.cfg)
	info := &Info{
		Root:         pr.root,
		Module:       pr.mod.Module,
		Dialect:      pr.cfg.Dialect,
		Configured:   pr.exists,
		GoVersion:    pr.mod.Go,
		Toolchain:    pr.mod.Toolchain,
		Status:       pr.target.Status,
		Capabilities: pr.target.Caps,
		Domains:      []DomainInfo{},
		Diagnostics:  diags,
	}
	for _, e := range model.Entities {
		info.Domains = append(info.Domains, DomainInfo{
			Domain:   e.Domain,
			Entity:   e.Name,
			Table:    e.Table,
			File:     e.File,
			Path:     e.Path(),
			Routes:   !e.NoRoutes,
			ReadOnly: e.ReadOnly,
		})
	}
	return info, nil
}

func ValidateSpec(dir, file string, content []byte) (schema.Diagnostics, error) {
	pr, err := open(dir)
	if err != nil {
		return nil, err
	}
	name, err := specName(file)
	if err != nil {
		return nil, err
	}
	sources, err := spec.LoadDir(pr.root, SpecsDir)
	if err != nil {
		return nil, newError(KindInternal, "read_specs", err.Error())
	}
	replaced := false
	for i, s := range sources {
		if s.Name == name {
			sources[i].Content = content
			replaced = true
		}
	}
	if !replaced {
		sources = append(sources, spec.Source{Name: name, Content: content})
	}
	_, diags := analyze(sources, pr.d, pr.target.Caps, pr.cfg)
	return diags, nil
}

func specName(file string) (string, error) {
	name := path.Clean(filepath.ToSlash(file))
	if !strings.Contains(name, "/") {
		name = SpecsDir + "/" + name
	}
	if path.Dir(name) != SpecsDir || !strings.HasSuffix(name, ".go") {
		return "", newError(KindUsage, "invalid_spec_file", fmt.Sprintf("spec files live in %s/<domain>.go, got %q", SpecsDir, file))
	}
	return name, nil
}
