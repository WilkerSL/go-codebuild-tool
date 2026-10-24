package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func hasPrefix(files map[string][]byte, prefix string) bool {
	for name := range files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func TestMinimalStarterLeavesPartsOut(t *testing.T) {
	dir := t.TempDir()
	copySpecSet(t, dir, "secure")
	build(t, Options{Dir: dir, Starter: true, Minimal: true, Module: "example.com/secure", Dialect: "postgres", Toolchain: fakeToolchain("go1.27.0")})
	files := treeFiles(t, dir)
	for _, prefix := range []string{"deploy/", ".github/", "internal/platform/ops/"} {
		if hasPrefix(files, prefix) {
			t.Errorf("a minimal project contains %s", prefix)
		}
	}
	for _, want := range []string{"Dockerfile", "openapi.json", "internal/platform/telemetry/telemetry_gen.go"} {
		if files[want] == nil {
			t.Errorf("a minimal project lacks %s", want)
		}
	}
	cfg := string(files[ConfigFile])
	if !strings.Contains(cfg, `"kubernetes": false`) || !strings.Contains(cfg, `"observability": false`) || !strings.Contains(cfg, `"ci": false`) {
		t.Errorf("%s does not record the parts left out:\n%s", ConfigFile, cfg)
	}
	agents := string(files["AGENTS.md"])
	if strings.Contains(agents, "deploy/kubernetes") || strings.Contains(agents, ".github/workflows") {
		t.Errorf("AGENTS.md describes parts that were left out:\n%s", agents)
	}
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if plan.HasChanges() {
		t.Errorf("a second build of a minimal project changes files: %+v", plan.Files)
	}
}

func TestMinimalNeedsANewProject(t *testing.T) {
	dir := starter(t, "1.27.0", "sqlite")
	_, err := BuildPlan(t.Context(), Options{Dir: dir, Minimal: true, Toolchain: fakeToolchain("go1.27.0")})
	if e, ok := err.(*Error); !ok || e.Code != "minimal_existing" {
		t.Fatalf("plan with --minimal on an existing project = %v", err)
	}
}

func TestTurningPartsOffMakesTheirFilesStale(t *testing.T) {
	dir := workspaceStarter(t, "1.27.0", "sqlite")
	path := filepath.Join(dir, ConfigFile)
	cfg := readTestFile(t, path)
	cfg = strings.Replace(cfg, `"kubernetes": true`, `"kubernetes": false`, 1)
	cfg = strings.Replace(cfg, `"observability": true`, `"observability": false`, 1)
	writeTestFile(t, path, cfg)
	plan := build(t, Options{Dir: dir, Prune: true, Toolchain: fakeToolchain("go1.27.0")})
	deleted := map[string]bool{}
	for _, f := range plan.Files {
		if f.Action == ActionDelete {
			deleted[f.Path] = true
		}
	}
	for _, want := range []string{"deploy/kubernetes/base/gateway.yaml", "deploy/kubernetes/components/observability/jaeger.yaml", "deploy/observability/prometheus.yml", "gateway/internal/platform/ops/ops_gen.go"} {
		if !deleted[want] {
			t.Errorf("%s was not deleted after turning its part off", want)
		}
	}
	files := treeFiles(t, dir)
	if files["deploy/kubernetes/overlays/production/kustomization.yaml"] == nil {
		t.Error("the user overlay was deleted")
	}
	compose := string(files["docker-compose.yml"])
	if strings.Contains(compose, "jaeger") || strings.Contains(compose, "OPS_JAEGER_URL") {
		t.Errorf("docker-compose.yml still runs the observability stack:\n%s", compose)
	}
	routes := string(files["gateway/internal/app/routes_gen.go"])
	if strings.Contains(routes, "ops.") {
		t.Error("the gateway still registers the operations panels")
	}
}
