package goversion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/version"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Toolchain struct {
	Local      string `json:"local"`
	Setting    string `json:"setting"`
	Experiment string `json:"experiment,omitempty"`
}

var versionPattern = regexp.MustCompile(`go1\.\d+(?:\.\d+)?(?:(?:rc|beta)\d+)?`)

func NormalizeGoVersion(goversion string) (string, error) {
	match := versionPattern.FindString(goversion)
	if match == "" || !version.IsValid(match) {
		return "", fmt.Errorf("cannot parse Go version %q", goversion)
	}
	return strings.TrimPrefix(match, "go"), nil
}

func DetectToolchain(ctx context.Context) (Toolchain, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOENV", "GOROOT", "GOEXPERIMENT")
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Toolchain{}, fmt.Errorf("run go env: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var env struct {
		GOVERSION    string
		GOENV        string
		GOROOT       string
		GOEXPERIMENT string
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return Toolchain{}, fmt.Errorf("decode go env output: %w", err)
	}
	local, err := NormalizeGoVersion(env.GOVERSION)
	if err != nil {
		return Toolchain{}, err
	}
	setting := os.Getenv("GOTOOLCHAIN")
	if setting == "" {
		setting = readEnvFile(env.GOENV, "GOTOOLCHAIN")
	}
	if setting == "" && env.GOROOT != "" {
		setting = readEnvFile(filepath.Join(env.GOROOT, "go.env"), "GOTOOLCHAIN")
	}
	if setting == "" {
		setting = "local"
	}
	return Toolchain{Local: "go" + local, Setting: setting, Experiment: env.GOEXPERIMENT}, nil
}

func readEnvFile(path, key string) string {
	if path == "" || path == "off" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	value := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			value = strings.TrimSpace(v)
		}
	}
	return value
}

type ToolchainCheck struct {
	OK       bool   `json:"ok"`
	Fatal    bool   `json:"fatal"`
	Required string `json:"required"`
	Message  string `json:"message,omitempty"`
}

func (tc Toolchain) Check(required string) ToolchainCheck {
	result := ToolchainCheck{OK: true, Required: required}
	if required == "" || tc.Local == "" {
		return result
	}
	name, mode, _ := strings.Cut(tc.Setting, "+")
	pinned := ""
	switch name {
	case "auto", "path":
		mode = name
	case "local":
		mode = "local"
	default:
		if version.IsValid(name) {
			pinned = name
		}
		if mode == "" {
			mode = "pinned"
		}
	}
	switch mode {
	case "local":
		if version.Compare(tc.Local, required) < 0 {
			result.OK, result.Fatal = false, true
			result.Message = fmt.Sprintf("the project requires %s but the local toolchain is %s and GOTOOLCHAIN=%s prevents switching; install %s or allow switching with GOTOOLCHAIN=auto", required, tc.Local, tc.Setting, required)
		}
	case "pinned":
		if pinned != "" && version.Compare(pinned, required) < 0 {
			result.OK, result.Fatal = false, true
			result.Message = fmt.Sprintf("the project requires %s but GOTOOLCHAIN=%s pins an older toolchain", required, tc.Setting)
		}
	default:
		effective := Max(tc.Local, pinned)
		if version.Compare(effective, required) < 0 {
			result.OK = false
			action := "download"
			if mode == "path" {
				action = "look up in PATH"
			}
			result.Message = fmt.Sprintf("the local toolchain is %s; the go command will %s %s because GOTOOLCHAIN=%s", tc.Local, action, required, tc.Setting)
		}
	}
	return result
}

func (tc Toolchain) ExperimentWarning(c Capabilities) string {
	if !c.JSONv2 {
		return ""
	}
	for _, exp := range strings.Split(tc.Experiment, ",") {
		if strings.TrimSpace(exp) == "nojsonv2" {
			return "GOEXPERIMENT contains nojsonv2, which removes encoding/json/v2; the generated code for go " + c.Release + " needs it"
		}
	}
	return ""
}
