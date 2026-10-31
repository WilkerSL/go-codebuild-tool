package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func noteSpec(marker string) string {
	tag := ""
	if marker != "" {
		tag = "\t_    struct{} `gen:\"" + marker + "\"`\n"
	}
	return "//go:build ignore\n\npackage specs\n\ntype Note struct {\n" + tag + "\tID   int64  `gen:\"pk,auto\"`\n\tText string `gen:\"required\"`\n}\n"
}

// The golden projects all have routes that write; these are the projects at
// the other end, which the wiring must not assume anything about.
func TestEndToEndSmallProjects(t *testing.T) {
	if testing.Short() {
		t.Skip("the end-to-end test downloads modules and builds generated projects")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	bin := buildTool(t)
	projects := []struct {
		name string
		spec string
		args []string
	}{
		{name: "no-specs"},
		{name: "no-specs-minimal", args: []string{"--minimal"}},
		{name: "noroutes", spec: noteSpec("noroutes")},
		{name: "readonly", spec: noteSpec("readonly")},
		{name: "routes", spec: noteSpec("")},
	}
	for _, p := range projects {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			if p.spec != "" {
				if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "specs", "notes.go"), []byte(p.spec), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"build", "--starter", "--module", "example.com/small", "--dialect", "sqlite"}, p.args...)
			command(t, dir, nil, bin, args...)
			command(t, dir, nil, "go", "mod", "tidy")
			command(t, dir, nil, "go", "build", "./...")
			command(t, dir, nil, "go", "vet", "./...")
			command(t, dir, nil, "go", "test", "-count=1", "./...")
		})
	}
}

// TestKnownVulnerabilities scans what a new project actually depends on, which
// the pinned versions decide. New advisories appear without any commit, so CI
// also runs it on a schedule.
func TestKnownVulnerabilities(t *testing.T) {
	if os.Getenv("CODEBUILD_CHECK_VULNS") != "1" {
		t.Skip("set CODEBUILD_CHECK_VULNS=1 to run govulncheck on generated projects and workspaces")
	}
	const govulncheck = "golang.org/x/vuln/cmd/govulncheck@latest"
	bin := buildTool(t)
	for _, dialect := range listFromEnv("CODEBUILD_E2E_DIALECTS", []string{"postgres", "sqlserver", "mysql", "sqlite"}) {
		t.Run("project/"+dialect, func(t *testing.T) {
			dir := t.TempDir()
			copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "saas"), filepath.Join(dir, "specs"))
			command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/saas", "--dialect", dialect)
			command(t, dir, nil, "go", "mod", "tidy")
			command(t, dir, nil, "go", "run", govulncheck, "./...")
		})
		t.Run("workspace/"+dialect, func(t *testing.T) {
			dir := t.TempDir()
			copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "shop"), filepath.Join(dir, "specs"))
			command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/shop", "--dialect", dialect)
			for _, m := range workspaceModules {
				command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "mod", "tidy")
			}
			for _, m := range workspaceModules {
				command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "run", govulncheck, "./...")
			}
		})
	}
}
