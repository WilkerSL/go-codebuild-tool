package goversion

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		in      string
		status  Status
		release string
		stdUUID bool
		jsonV2  bool
		warns   bool
	}{
		{in: "1.26.0", status: StatusSupported, release: "1.26"},
		{in: "1.26", status: StatusSupported, release: "1.26"},
		{in: "1.26rc1", status: StatusSupported, release: "1.26"},
		{in: "1.26.4", status: StatusSupported, release: "1.26"},
		{in: "1.27.0", status: StatusSupported, release: "1.27", stdUUID: true, jsonV2: true},
		{in: "1.27.3", status: StatusSupported, release: "1.27", stdUUID: true, jsonV2: true},
		{in: "1.28.0", status: StatusAboveKnown, release: "1.27", stdUUID: true, jsonV2: true, warns: true},
		{in: "1.30", status: StatusAboveKnown, release: "1.27", stdUUID: true, jsonV2: true, warns: true},
		{in: "1.25.9", status: StatusBelowMinimum},
		{in: "1.21", status: StatusBelowMinimum},
		{in: "1.16", status: StatusBelowMinimum},
	}
	for _, tt := range tests {
		got, err := Resolve(tt.in)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tt.in, err)
		}
		if got.Status != tt.status || got.Caps.Release != tt.release || got.Caps.StdUUID != tt.stdUUID || got.Caps.JSONv2 != tt.jsonV2 {
			t.Errorf("Resolve(%q) = %+v", tt.in, got)
		}
		if (len(got.Warnings) > 0) != tt.warns {
			t.Errorf("Resolve(%q) warnings = %v, want warnings=%v", tt.in, got.Warnings, tt.warns)
		}
	}
	if _, err := Resolve("abc"); err == nil {
		t.Error("Resolve(abc) should fail")
	}
}

func TestCapabilitiesImports(t *testing.T) {
	old, _ := Resolve("1.26.0")
	if old.Caps.UUIDImport() != "github.com/google/uuid" || old.Caps.JSONImport() != "encoding/json" {
		t.Errorf("unexpected imports for 1.26: %s %s", old.Caps.UUIDImport(), old.Caps.JSONImport())
	}
	cur, _ := Resolve("1.27.0")
	if cur.Caps.UUIDImport() != "uuid" || cur.Caps.JSONImport() != "encoding/json/v2" {
		t.Errorf("unexpected imports for 1.27: %s %s", cur.Caps.UUIDImport(), cur.Caps.JSONImport())
	}
	if LatestVersion() != "1.27.0" {
		t.Errorf("LatestVersion() = %s", LatestVersion())
	}
}

func TestParseModFile(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		goVersion string
		toolchain string
		required  string
		status    Status
	}{
		{
			name:      "go directive only",
			data:      "module example.com/app\n\ngo 1.26.0\n",
			goVersion: "1.26.0",
			required:  "go1.26.0",
			status:    StatusSupported,
		},
		{
			name:      "with toolchain",
			data:      "module example.com/app\n\ngo 1.26.0\n\ntoolchain go1.27.1\n",
			goVersion: "1.26.0",
			toolchain: "go1.27.1",
			required:  "go1.27.1",
			status:    StatusSupported,
		},
		{
			name:      "toolchain older than go",
			data:      "module example.com/app\n\ngo 1.27.2\n\ntoolchain go1.27.0\n",
			goVersion: "1.27.2",
			toolchain: "go1.27.0",
			required:  "go1.27.2",
			status:    StatusSupported,
		},
		{
			name:      "below minimum",
			data:      "module example.com/app\n\ngo 1.24\n",
			goVersion: "1.24",
			required:  "go1.24",
			status:    StatusBelowMinimum,
		},
		{
			name:      "missing go directive",
			data:      "module example.com/app\n",
			goVersion: "1.16",
			required:  "go1.16",
			status:    StatusBelowMinimum,
		},
		{
			name:      "above latest known",
			data:      "module example.com/app\n\ngo 1.29.0\n",
			goVersion: "1.29.0",
			required:  "go1.29.0",
			status:    StatusAboveKnown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf, err := ParseModFile("go.mod", []byte(tt.data))
			if err != nil {
				t.Fatal(err)
			}
			if mf.Module != "example.com/app" || mf.Go != tt.goVersion || mf.Toolchain != tt.toolchain {
				t.Fatalf("unexpected modfile: %+v", mf)
			}
			if got := mf.RequiredToolchain(); got != tt.required {
				t.Errorf("RequiredToolchain() = %s, want %s", got, tt.required)
			}
			target, err := mf.Target()
			if err != nil {
				t.Fatal(err)
			}
			if target.Status != tt.status || target.Toolchain != tt.toolchain {
				t.Errorf("Target() = %+v", target)
			}
		})
	}
	if _, err := ParseModFile("go.mod", []byte("module\n")); err == nil {
		t.Error("expected parse error")
	}
}

func TestToolchainCheck(t *testing.T) {
	tests := []struct {
		name     string
		tc       Toolchain
		required string
		ok       bool
		fatal    bool
		contains string
	}{
		{name: "local newer", tc: Toolchain{Local: "go1.27.0", Setting: "auto"}, required: "go1.26.0", ok: true},
		{name: "local equal", tc: Toolchain{Local: "go1.27.0", Setting: "local"}, required: "go1.27.0", ok: true},
		{name: "local mode older", tc: Toolchain{Local: "go1.26.2", Setting: "local"}, required: "go1.27.0", fatal: true, contains: "prevents switching"},
		{name: "auto older", tc: Toolchain{Local: "go1.26.2", Setting: "auto"}, required: "go1.27.0", contains: "download go1.27.0"},
		{name: "path older", tc: Toolchain{Local: "go1.26.2", Setting: "path"}, required: "go1.27.0", contains: "look up in PATH"},
		{name: "pinned older", tc: Toolchain{Local: "go1.27.0", Setting: "go1.26.1"}, required: "go1.27.0", fatal: true, contains: "pins an older"},
		{name: "pinned newer", tc: Toolchain{Local: "go1.26.0", Setting: "go1.27.1"}, required: "go1.27.0", ok: true},
		{name: "pinned auto older", tc: Toolchain{Local: "go1.26.0", Setting: "go1.26.1+auto"}, required: "go1.27.0", contains: "download"},
		{name: "pinned auto satisfies", tc: Toolchain{Local: "go1.26.0", Setting: "go1.27.1+auto"}, required: "go1.27.0", ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tc.Check(tt.required)
			if got.OK != tt.ok || got.Fatal != tt.fatal || !strings.Contains(got.Message, tt.contains) {
				t.Errorf("Check(%s) = %+v", tt.required, got)
			}
		})
	}
}

func TestExperimentWarning(t *testing.T) {
	latest := Latest()
	if w := (Toolchain{Experiment: "nojsonv2"}).ExperimentWarning(latest); !strings.Contains(w, "nojsonv2") {
		t.Errorf("expected warning, got %q", w)
	}
	if w := (Toolchain{Experiment: "nojsonv2"}).ExperimentWarning(Known()[0]); w != "" {
		t.Errorf("unexpected warning for go 1.26: %q", w)
	}
	if w := (Toolchain{Experiment: "arenas"}).ExperimentWarning(latest); w != "" {
		t.Errorf("unexpected warning: %q", w)
	}
}

func TestNormalizeGoVersion(t *testing.T) {
	tests := map[string]string{
		"go1.27.0":                      "1.27.0",
		"go1.26.3 X:nojsonv2":           "1.26.3",
		"go1.28rc1":                     "1.28rc1",
		"devel go1.28-abc123 Mon Jan 1": "1.28",
	}
	for in, want := range tests {
		got, err := NormalizeGoVersion(in)
		if err != nil || got != want {
			t.Errorf("NormalizeGoVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := NormalizeGoVersion("unknown"); err == nil {
		t.Error("expected error")
	}
}

func TestDetectToolchain(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	tc, err := DetectToolchain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tc.Local, "go1.") || tc.Setting == "" {
		t.Errorf("unexpected toolchain: %+v", tc)
	}
}
