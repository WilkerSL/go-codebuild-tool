package goversion

import (
	"fmt"
	"go/version"
	"strings"
)

type Capabilities struct {
	Release string `json:"release"`
	StdUUID bool   `json:"stdUUID"`
	JSONv2  bool   `json:"jsonV2"`
}

var known = []Capabilities{
	{Release: "1.26"},
	{Release: "1.27", StdUUID: true, JSONv2: true},
}

const MinimumRelease = "1.26"

func Known() []Capabilities {
	return append([]Capabilities(nil), known...)
}

func Latest() Capabilities {
	return known[len(known)-1]
}

func LatestVersion() string {
	return Latest().Release + ".0"
}

func (c Capabilities) Enabled() []string {
	var names []string
	if c.StdUUID {
		names = append(names, "StdUUID")
	}
	if c.JSONv2 {
		names = append(names, "JSONv2")
	}
	return names
}

func (c Capabilities) String() string {
	enabled := c.Enabled()
	if len(enabled) == 0 {
		return "go" + c.Release + " (baseline)"
	}
	return "go" + c.Release + " (" + strings.Join(enabled, ", ") + ")"
}

func (c Capabilities) UUIDImport() string {
	if c.StdUUID {
		return "uuid"
	}
	return "github.com/google/uuid"
}

func (c Capabilities) JSONImport() string {
	if c.JSONv2 {
		return "encoding/json/v2"
	}
	return "encoding/json"
}

type Status string

const (
	StatusSupported    Status = "supported"
	StatusBelowMinimum Status = "below_minimum"
	StatusAboveKnown   Status = "above_known"
)

type Target struct {
	GoVersion string       `json:"goVersion"`
	Toolchain string       `json:"toolchain,omitempty"`
	Status    Status       `json:"status"`
	Caps      Capabilities `json:"capabilities"`
	Warnings  []string     `json:"warnings,omitempty"`
}

func Resolve(goVersion string) (Target, error) {
	v := "go" + goVersion
	if !version.IsValid(v) {
		return Target{}, fmt.Errorf("invalid Go version %q", goVersion)
	}
	lang := version.Lang(v)
	t := Target{GoVersion: goVersion, Status: StatusSupported}
	if version.Compare(lang, "go"+MinimumRelease) < 0 {
		t.Status = StatusBelowMinimum
		return t, nil
	}
	latest := Latest()
	if version.Compare(lang, "go"+latest.Release) > 0 {
		t.Status = StatusAboveKnown
		t.Caps = latest
		t.Warnings = append(t.Warnings, fmt.Sprintf(
			"go %s is newer than the latest version known by codebuild (go %s); generating with the go %s capability set",
			goVersion, latest.Release, latest.Release))
		return t, nil
	}
	for _, c := range known {
		if version.Compare(lang, "go"+c.Release) >= 0 {
			t.Caps = c
		}
	}
	return t, nil
}

func Supported(goVersion string) bool {
	t, err := Resolve(goVersion)
	return err == nil && t.Status != StatusBelowMinimum
}

func Max(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if version.Compare(a, b) >= 0 {
		return a
	}
	return b
}

func UpgradeTarget(local string) string {
	v := strings.TrimPrefix(local, "go")
	if t, err := Resolve(v); err == nil && t.Status == StatusSupported {
		return v
	}
	return LatestVersion()
}
