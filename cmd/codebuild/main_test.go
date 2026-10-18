package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

var e2eDSNEnv = map[string]string{
	"postgres":  "CODEBUILD_TEST_POSTGRES_DSN",
	"sqlserver": "CODEBUILD_TEST_SQLSERVER_DSN",
	"mysql":     "CODEBUILD_TEST_MYSQL_DSN",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func command(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func buildTool(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "codebuild")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	command(t, repoRoot(t), nil, "go", "build", "-o", bin, "./cmd/codebuild")
	return bin
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func dialectsUnderTest() []string {
	if v := os.Getenv("CODEBUILD_E2E_DIALECTS"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"postgres", "sqlserver", "mysql", "sqlite"}
}

func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("the end-to-end test downloads modules and builds generated projects")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	bin := buildTool(t)
	for _, name := range dialectsUnderTest() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "saas"), filepath.Join(dir, "specs"))
			command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/saas", "--dialect", name)
			command(t, dir, nil, "go", "mod", "tidy")
			command(t, dir, nil, "go", "build", "./...")
			command(t, dir, nil, "go", "vet", "./...")
			if out := command(t, dir, nil, "gofmt", "-l", "."); strings.TrimSpace(out) != "" {
				t.Fatalf("gofmt reports unformatted files:\n%s", out)
			}
			if imp := os.Getenv("CODEBUILD_TEST_" + strings.ToUpper(name) + "_IMPORT"); imp != "" {
				hook := "package database\n\nimport _ " + strconv.Quote(imp) + "\n"
				if err := os.WriteFile(filepath.Join(dir, "internal", "platform", "database", "driver_local.go"), []byte(hook), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dsn := os.Getenv(e2eDSNEnv[name])
			var env []string
			if dsn != "" {
				env = append(env, "TEST_DATABASE_URL="+dsn)
			}
			command(t, dir, env, "go", "test", "-count=1", "./...")
			if out := command(t, dir, nil, bin, "plan", "--exit-code"); !strings.Contains(out, "0 created, 0 updated") {
				t.Fatalf("second plan is not empty:\n%s", out)
			}
			if name == "sqlite" {
				dsn = filepath.Join(dir, "app.db")
			}
			if dsn == "" {
				t.Logf("skipping the HTTP smoke test: set %s to run it", e2eDSNEnv[name])
				return
			}
			smokeTest(t, dir, dsn)
		})
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

type client struct {
	t    *testing.T
	base string
}

func (c client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	data, _ := io.ReadAll(resp.Body)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			c.t.Fatalf("%s %s returned invalid JSON: %s", method, path, data)
		}
	}
	return resp.StatusCode, out
}

func (c client) expect(method, path string, body any, status int) map[string]any {
	c.t.Helper()
	got, out := c.do(method, path, body)
	if got != status {
		c.t.Fatalf("%s %s = %d, want %d: %v", method, path, got, status, out)
	}
	return out
}

func smokeTest(t *testing.T, dir, dsn string) {
	env := []string{"DATABASE_URL=" + dsn}
	command(t, dir, env, "go", "run", "./cmd/migrate", "up")
	api := filepath.Join(dir, "api")
	if runtime.GOOS == "windows" {
		api += ".exe"
	}
	command(t, dir, nil, "go", "build", "-o", api, "./cmd/api")
	addr := freeAddr(t)
	cmd := exec.Command(api)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append(env, "HTTP_ADDR="+addr, "LOG_LEVEL=warn")...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	c := client{t: t, base: "http://" + addr}
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get(c.base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("api did not become healthy:\n%s", logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	suffix := fmt.Sprint(time.Now().UnixNano())
	org := c.expect("POST", "/orgs", map[string]any{"name": "Acme", "slug": "acme-" + suffix}, http.StatusCreated)
	orgID := org["id"].(string)
	email := "ann-" + suffix + "@example.com"
	user := c.expect("POST", "/users", map[string]any{"name": "Ann", "email": email, "orgId": orgID}, http.StatusCreated)
	userID := user["id"].(string)
	if user["role"] != "viewer" || user["bio"] != nil {
		t.Fatalf("defaults not applied: %v", user)
	}
	conflict := c.expect("POST", "/users", map[string]any{"name": "Bob", "email": email, "orgId": orgID}, http.StatusConflict)
	if errs := conflict["errors"].([]any); errs[0].(map[string]any)["field"] != "email" {
		t.Fatalf("conflict = %v", conflict)
	}
	invalid := c.expect("POST", "/users", map[string]any{"name": " ", "email": "nope", "role": "root", "orgId": orgID}, http.StatusUnprocessableEntity)
	if len(invalid["errors"].([]any)) != 3 {
		t.Fatalf("validation = %v", invalid)
	}
	c.expect("POST", "/users", map[string]any{"nickname": "x"}, http.StatusBadRequest)
	missing := c.expect("POST", "/users", map[string]any{"name": "Cy", "email": "cy-" + suffix + "@example.com", "orgId": "01a14fe9-2c69-7eb3-8cb7-000000000000"}, http.StatusUnprocessableEntity)
	if errs := missing["errors"].([]any); errs[0].(map[string]any)["field"] != "orgId" {
		t.Fatalf("missing reference = %v", missing)
	}
	page := c.expect("GET", "/users?email="+email, nil, http.StatusOK)
	if page["total"].(float64) != 1 {
		t.Fatalf("filtered list = %v", page)
	}
	c.expect("GET", "/users?limit=1000", nil, http.StatusBadRequest)
	nested := c.expect("GET", "/orgs/"+orgID+"/users", nil, http.StatusOK)
	if nested["total"].(float64) != 1 {
		t.Fatalf("nested list = %v", nested)
	}
	tag := c.expect("POST", "/tags", map[string]any{"name": "vip-" + suffix[len(suffix)-8:]}, http.StatusCreated)
	tagID := fmt.Sprint(int64(tag["id"].(float64)))
	c.expect("PUT", "/users/"+userID+"/tags/"+tagID, nil, http.StatusNoContent)
	c.expect("PUT", "/users/"+userID+"/tags/"+tagID, nil, http.StatusNoContent)
	if tags := c.expect("GET", "/users/"+userID+"/tags", nil, http.StatusOK); tags["total"].(float64) != 1 {
		t.Fatalf("linked tags = %v", tags)
	}
	updated := c.expect("PUT", "/users/"+userID, map[string]any{"name": "Ann B", "email": email, "role": "admin", "orgId": orgID}, http.StatusOK)
	if updated["name"] != "Ann B" || updated["role"] != "admin" {
		t.Fatalf("update = %v", updated)
	}
	project := c.expect("POST", "/projects", map[string]any{"orgId": orgID, "name": "Apollo", "budget": 10.5, "logo": "aGk="}, http.StatusCreated)
	if project["priority"].(float64) != 3 || project["logo"] != "aGk=" {
		t.Fatalf("project = %v", project)
	}
	c.expect("DELETE", "/orgs/"+orgID, nil, http.StatusNoContent)
	c.expect("DELETE", "/users/"+userID+"/tags/"+tagID, nil, http.StatusNoContent)
	c.expect("DELETE", "/users/"+userID, nil, http.StatusNoContent)
	c.expect("GET", "/users/"+userID, nil, http.StatusNotFound)
	c.expect("GET", "/users/not-a-uuid", nil, http.StatusBadRequest)
	c.expect("GET", "/countries", nil, http.StatusOK)
	c.expect("DELETE", "/tags/"+tagID, nil, http.StatusNoContent)
	cmd.Process.Kill()
	cmd.Wait()
	if out := command(t, dir, env, "go", "run", "./cmd/migrate", "down", "1"); !strings.Contains(out, "reverted 1") {
		t.Fatalf("migrate down: %s", out)
	}
	if out := command(t, dir, env, "go", "run", "./cmd/migrate", "version"); strings.TrimSpace(out) != "0" {
		t.Fatalf("version after down: %s", out)
	}
}
