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
	"slices"
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

const e2eSecret = "e2e-secret-with-at-least-32-bytes-of-entropy"

type e2eProject struct {
	name   string
	specs  string
	method string
	smoke  func(t *testing.T, c client, dir string, env []string, method string)
}

var e2eProjects = []e2eProject{
	{name: "saas", specs: "saas", smoke: saasSmoke},
	{name: "secure", specs: "secure", method: "session", smoke: secureSmoke},
	{name: "secure-jwt", specs: "secure", method: "jwt", smoke: secureSmoke},
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func run(dir string, env []string, stdin string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func command(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, err := run(dir, env, "", name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
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

func listFromEnv(key string, all []string) []string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ",")
	}
	return all
}

func TestEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("the end-to-end test downloads modules and builds generated projects")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	bin := buildTool(t)
	var names []string
	for _, p := range e2eProjects {
		names = append(names, p.name)
	}
	projects := listFromEnv("CODEBUILD_E2E_PROJECTS", names)
	for _, project := range e2eProjects {
		if !slices.Contains(projects, project.name) {
			continue
		}
		for _, dialect := range listFromEnv("CODEBUILD_E2E_DIALECTS", []string{"postgres", "sqlserver", "mysql", "sqlite"}) {
			t.Run(project.name+"/"+dialect, func(t *testing.T) {
				endToEnd(t, bin, project, dialect)
			})
		}
	}
}

func endToEnd(t *testing.T, bin string, project e2eProject, dialect string) {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", project.specs), filepath.Join(dir, "specs"))
	if project.method == "jwt" {
		spec := filepath.Join(dir, "specs", "users.go")
		data, err := os.ReadFile(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(spec, bytes.Replace(data, []byte("auth=session|apikey"), []byte("auth=jwt|apikey"), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/"+project.specs, "--dialect", dialect)
	command(t, dir, nil, "go", "mod", "tidy")
	command(t, dir, nil, "go", "build", "./...")
	command(t, dir, nil, "go", "vet", "./...")
	if out := command(t, dir, nil, "gofmt", "-l", "."); strings.TrimSpace(out) != "" {
		t.Fatalf("gofmt reports unformatted files:\n%s", out)
	}
	if imp := os.Getenv("CODEBUILD_TEST_" + strings.ToUpper(dialect) + "_IMPORT"); imp != "" {
		hook := "package database\n\nimport _ " + strconv.Quote(imp) + "\n"
		if err := os.WriteFile(filepath.Join(dir, "internal", "platform", "database", "driver_local.go"), []byte(hook), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dsn := os.Getenv(e2eDSNEnv[dialect])
	var env []string
	if dsn != "" {
		env = append(env, "TEST_DATABASE_URL="+dsn)
	}
	command(t, dir, env, "go", "test", "-count=1", "./...")
	if out := command(t, dir, nil, bin, "plan", "--exit-code"); !strings.Contains(out, "0 created, 0 updated") {
		t.Fatalf("second plan is not empty:\n%s", out)
	}
	if os.Getenv("CODEBUILD_E2E_DOCKER") == "1" && dialect == "sqlite" {
		dockerCheck(t, dir, project, dialect)
	}
	if dialect == "sqlite" {
		dsn = filepath.Join(dir, "app.db")
	}
	if dsn == "" {
		t.Logf("skipping the HTTP smoke test: set %s to run it", e2eDSNEnv[dialect])
		return
	}
	env = []string{"DATABASE_URL=" + dsn, "AUTH_COOKIE_SECURE=false", "AUTH_LOGIN_LIMIT=5", "AUTH_JWT_SECRET=" + e2eSecret, "AUTH_REFRESH_REUSE_GRACE=1ms"}
	command(t, dir, env, "go", "run", "./cmd/migrate", "up")
	c, stop := startAPI(t, dir, env)
	project.smoke(t, c, dir, env, project.method)
	stop()
	if out := command(t, dir, env, "go", "run", "./cmd/migrate", "down", "1"); !strings.Contains(out, "reverted 1") {
		t.Fatalf("migrate down: %s", out)
	}
	if out := command(t, dir, env, "go", "run", "./cmd/migrate", "version"); strings.TrimSpace(out) != "0" {
		t.Fatalf("version after down: %s", out)
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

func waitReady(t *testing.T, base string, logs func() string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := http.Get(base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("api did not become healthy:\n%s", logs())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func startAPI(t *testing.T, dir string, env []string) (client, func()) {
	t.Helper()
	api := filepath.Join(dir, "api")
	if runtime.GOOS == "windows" {
		api += ".exe"
	}
	command(t, dir, nil, "go", "build", "-o", api, "./cmd/api")
	addr := freeAddr(t)
	cmd := exec.Command(api)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append(slices.Clone(env), "HTTP_ADDR="+addr, "LOG_LEVEL=warn")...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			cmd.Process.Kill()
			cmd.Wait()
		}
	}
	t.Cleanup(stop)
	c := client{t: t, base: "http://" + addr}
	waitReady(t, c.base, logs.String)
	return c, stop
}

func dockerCheck(t *testing.T, dir string, project e2eProject, dialect string) {
	image := "codebuild-e2e-" + project.name + "-" + dialect
	command(t, dir, nil, "docker", "build", "-t", image, ".")
	t.Cleanup(func() { run(dir, nil, "", "docker", "rmi", "-f", image) })
	volume := image + "-data"
	command(t, dir, nil, "docker", "volume", "create", volume)
	t.Cleanup(func() { run(dir, nil, "", "docker", "volume", "rm", "-f", volume) })
	command(t, dir, nil, "docker", "run", "--rm", "-v", volume+":/data", "--entrypoint", "/app/migrate", image, "up")
	addr := freeAddr(t)
	id := strings.TrimSpace(command(t, dir, nil, "docker", "run", "-d", "-v", volume+":/data", "-p", addr+":8080",
		"-e", "AUTH_JWT_SECRET="+e2eSecret, image))
	t.Cleanup(func() { run(dir, nil, "", "docker", "rm", "-f", id) })
	base := "http://" + addr
	waitReady(t, base, func() string {
		out, _ := run(dir, nil, "", "docker", "logs", id)
		return out
	})
	resp, err := http.Get(base + "/livez")
	if err != nil {
		t.Fatalf("container liveness: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("container liveness = %d", resp.StatusCode)
	}
}

type client struct {
	t     *testing.T
	base  string
	token string
}

func (c client) as(token string) client {
	c.token = token
	return c
}

type reply struct {
	StatusCode int
	Header     http.Header
}

func (c client) send(method, path string, body any, header http.Header) (reply, map[string]any) {
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range header {
		req.Header[k] = v
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
	return reply{StatusCode: resp.StatusCode, Header: resp.Header}, out
}

func (c client) expect(method, path string, body any, status int) map[string]any {
	c.t.Helper()
	resp, out := c.send(method, path, body, nil)
	if resp.StatusCode != status {
		c.t.Fatalf("%s %s = %d, want %d: %v", method, path, resp.StatusCode, status, out)
	}
	return out
}

func saasSmoke(t *testing.T, c client, dir string, env []string, method string) {
	c.expect("GET", "/livez", nil, http.StatusOK)
	resp, _ := c.send("GET", "/countries", nil, http.Header{"X-Request-Id": {"e2e-check"}})
	if resp.Header.Get("X-Request-ID") != "e2e-check" {
		t.Fatalf("request id was not propagated")
	}
	suffix := fmt.Sprint(time.Now().UnixNano())
	org := c.expect("POST", "/orgs", map[string]any{"name": "Acme", "slug": "acme-" + suffix}, http.StatusCreated)
	if org["locale"] != "en-US" {
		t.Fatalf("text default not applied: %v", org)
	}
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
	partial := c.expect("PUT", "/users/"+userID, map[string]any{"name": "Ann B", "email": email, "orgId": orgID}, http.StatusUnprocessableEntity)
	if errs := partial["errors"].([]any); len(errs) != 1 || errs[0].(map[string]any)["field"] != "role" {
		t.Fatalf("partial update = %v", partial)
	}
	updated := c.expect("PUT", "/users/"+userID, map[string]any{"name": "Ann B", "email": email, "role": "admin", "orgId": orgID}, http.StatusOK)
	if updated["name"] != "Ann B" || updated["role"] != "admin" {
		t.Fatalf("update = %v", updated)
	}
	project := c.expect("POST", "/projects", map[string]any{"orgId": orgID, "name": "Apollo", "budget": 10.5, "logo": "aGk="}, http.StatusCreated)
	if project["priority"].(float64) != 3 || project["logo"] != "aGk=" {
		t.Fatalf("project = %v", project)
	}
	c.expect("POST", "/projects", map[string]any{"orgId": orgID, "name": "Hermes"}, http.StatusCreated)
	c.expect("DELETE", "/orgs/"+orgID, nil, http.StatusConflict)
	c.expect("DELETE", "/users/"+userID+"/tags/"+tagID, nil, http.StatusNoContent)
	c.expect("DELETE", "/users/"+userID, nil, http.StatusNoContent)
	c.expect("GET", "/users/"+userID, nil, http.StatusNotFound)
	c.expect("DELETE", "/orgs/"+orgID, nil, http.StatusNoContent)
	c.expect("GET", "/projects/"+project["id"].(string), nil, http.StatusNotFound)
	if page := c.expect("GET", "/projects?orgId="+orgID, nil, http.StatusOK); page["total"].(float64) != 0 {
		t.Fatalf("projects of a deleted org are still listed: %v", page)
	}
	c.expect("POST", "/projects", map[string]any{"orgId": orgID, "name": "Late"}, http.StatusUnprocessableEntity)
	c.expect("GET", "/users/not-a-uuid", nil, http.StatusBadRequest)
	c.expect("GET", "/countries", nil, http.StatusOK)
	c.expect("DELETE", "/tags/"+tagID, nil, http.StatusNoContent)
}

func secureSmoke(t *testing.T, c client, dir string, env []string, method string) {
	resp, _ := c.send("GET", "/notes", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("anonymous GET /notes = %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	c.expect("GET", "/labels", nil, http.StatusOK)
	c.expect("POST", "/labels", map[string]any{"name": "x"}, http.StatusUnauthorized)

	suffix := fmt.Sprint(time.Now().UnixNano())
	annEmail, bobEmail := "ann-"+suffix+"@example.com", "bob-"+suffix+"@example.com"
	ann := c.expect("POST", "/auth/register", map[string]any{"email": strings.ToUpper(annEmail), "name": "Ann", "password": "ann password 1"}, http.StatusCreated)
	if ann["email"] != annEmail || ann["role"] != "member" || ann["passwordHash"] != nil || ann["password"] != nil {
		t.Fatalf("register = %v", ann)
	}
	c.expect("POST", "/auth/register", map[string]any{"email": annEmail, "name": "Ann", "password": "ann password 1"}, http.StatusConflict)
	c.expect("POST", "/auth/register", map[string]any{"email": "eve-" + suffix + "@example.com", "name": "Eve", "password": "eve password 1", "role": "admin"}, http.StatusBadRequest)
	c.expect("POST", "/auth/register", map[string]any{"email": "eve-" + suffix + "@example.com", "name": "Eve", "password": "short"}, http.StatusUnprocessableEntity)
	c.expect("POST", "/auth/register", map[string]any{"email": bobEmail, "name": "Bob", "password": "bob password 1"}, http.StatusCreated)

	login := func(email, password string) map[string]any {
		t.Helper()
		resp, out := c.send("POST", "/auth/login", map[string]any{"email": email, "password": password}, nil)
		if resp.StatusCode != http.StatusOK || out["tokenType"] != "Bearer" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("login %s = %d %v", email, resp.StatusCode, out)
		}
		if !strings.Contains(strings.Join(resp.Header.Values("Set-Cookie"), ";"), "HttpOnly") {
			t.Fatalf("login did not set an HttpOnly cookie: %v", resp.Header.Values("Set-Cookie"))
		}
		if method == "jwt" && out["refreshToken"] == nil {
			t.Fatalf("login without refresh token: %v", out)
		}
		return out
	}
	c.expect("POST", "/auth/login", map[string]any{"email": annEmail, "password": "wrong password"}, http.StatusUnauthorized)
	c.expect("POST", "/auth/login", map[string]any{"email": "nobody-" + annEmail, "password": "wrong password"}, http.StatusUnauthorized)
	annTokens := login(annEmail, "ann password 1")
	annToken := annTokens["accessToken"].(string)
	asAnn := c.as(annToken)
	if me := asAnn.expect("GET", "/auth/me", nil, http.StatusOK); me["id"] != ann["id"] {
		t.Fatalf("me = %v", me)
	}
	bobToken := login(bobEmail, "bob password 1")["accessToken"].(string)
	asBob := c.as(bobToken)

	folder := asAnn.expect("POST", "/folders", map[string]any{"name": "Work"}, http.StatusCreated)
	folderID := fmt.Sprint(int64(folder["id"].(float64)))
	asAnn.expect("POST", "/notes", map[string]any{"title": "Plan", "folderId": folder["id"], "ownerId": "ignored"}, http.StatusBadRequest)
	note := asAnn.expect("POST", "/notes", map[string]any{"title": "Plan", "folderId": folder["id"]}, http.StatusCreated)
	if note["ownerId"] != ann["id"] {
		t.Fatalf("note owner = %v", note)
	}
	noteID := note["id"].(string)
	if page := asAnn.expect("GET", "/notes", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("ann notes = %v", page)
	}
	if page := asAnn.expect("GET", "/folders/"+folderID+"/notes", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("folder notes = %v", page)
	}
	asBob.expect("GET", "/notes/"+noteID, nil, http.StatusNotFound)
	asBob.expect("PUT", "/notes/"+noteID, map[string]any{"title": "Mine"}, http.StatusUnprocessableEntity)
	asBob.expect("PUT", "/notes/"+noteID, map[string]any{"title": "Mine", "pinned": true}, http.StatusNotFound)
	asBob.expect("DELETE", "/notes/"+noteID, nil, http.StatusNotFound)
	asBob.expect("GET", "/folders/"+folderID+"/notes", nil, http.StatusNotFound)
	if page := asBob.expect("GET", "/notes", nil, http.StatusOK); page["total"].(float64) != 0 {
		t.Fatalf("bob sees notes of ann: %v", page)
	}
	asBob.expect("POST", "/notes", map[string]any{"title": "Steal", "folderId": folder["id"]}, http.StatusUnprocessableEntity)

	asAnn.expect("POST", "/labels", map[string]any{"name": "urgent"}, http.StatusForbidden)
	asAnn.expect("GET", "/users", nil, http.StatusForbidden)
	command(t, dir, env, "go", "run", "./cmd/accounts", "set-role", annEmail, "admin")
	annToken = login(annEmail, "ann password 1")["accessToken"].(string)
	asAnn = c.as(annToken)
	label := asAnn.expect("POST", "/labels", map[string]any{"name": "urgent-" + suffix[len(suffix)-8:]}, http.StatusCreated)
	labelID := fmt.Sprint(int64(label["id"].(float64)))
	asAnn.expect("GET", "/users", nil, http.StatusOK)
	asBob.expect("GET", "/users", nil, http.StatusForbidden)
	asAnn.expect("PUT", "/notes/"+noteID+"/labels/"+labelID, nil, http.StatusNoContent)
	asBob.expect("PUT", "/notes/"+noteID+"/labels/"+labelID, nil, http.StatusNotFound)
	if page := asAnn.expect("GET", "/notes/"+noteID+"/labels", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("note labels = %v", page)
	}
	if page := asBob.expect("GET", "/labels/"+labelID+"/notes", nil, http.StatusOK); page["total"].(float64) != 0 {
		t.Fatalf("bob sees labelled notes of ann: %v", page)
	}

	key := asAnn.expect("POST", "/auth/api-keys", map[string]any{"name": "ci"}, http.StatusCreated)
	asKey := c.as(key["key"].(string))
	asKey.expect("GET", "/auth/me", nil, http.StatusOK)
	asKey.expect("POST", "/auth/api-keys", map[string]any{"name": "nested"}, http.StatusForbidden)
	if keys := asAnn.expect("GET", "/auth/api-keys", nil, http.StatusOK); len(keys["items"].([]any)) != 1 {
		t.Fatalf("api keys = %v", keys)
	}
	asAnn.expect("DELETE", "/auth/api-keys/"+key["id"].(string), nil, http.StatusNoContent)
	asKey.expect("GET", "/auth/me", nil, http.StatusUnauthorized)

	cookieName := "session"
	if method == "jwt" {
		cookieName = "access_token"
	}
	cookie := http.Header{"Cookie": {cookieName + "=" + annToken}}
	if resp, _ := c.send("GET", "/auth/me", nil, cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("cookie authentication = %d", resp.StatusCode)
	}
	crossSite := http.Header{"Cookie": cookie["Cookie"], "Sec-Fetch-Site": {"cross-site"}}
	if resp, _ := c.send("POST", "/notes", map[string]any{"title": "csrf"}, crossSite); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site cookie request = %d", resp.StatusCode)
	}

	asAnn.expect("PUT", "/auth/password", map[string]any{"currentPassword": "wrong", "newPassword": "ann password 2"}, http.StatusUnprocessableEntity)
	asAnn.expect("PUT", "/auth/password", map[string]any{"currentPassword": "ann password 1", "newPassword": "ann password 2"}, http.StatusNoContent)
	c.expect("POST", "/auth/login", map[string]any{"email": annEmail, "password": "ann password 1"}, http.StatusUnauthorized)
	fresh := login(annEmail, "ann password 2")
	if method == "jwt" {
		refreshed := c.expect("POST", "/auth/refresh", map[string]any{"refreshToken": fresh["refreshToken"]}, http.StatusOK)
		time.Sleep(20 * time.Millisecond)
		c.expect("POST", "/auth/refresh", map[string]any{"refreshToken": fresh["refreshToken"]}, http.StatusUnauthorized)
		c.expect("POST", "/auth/refresh", map[string]any{"refreshToken": refreshed["refreshToken"]}, http.StatusUnauthorized)
		fresh = login(annEmail, "ann password 2")
	}
	asFresh := c.as(fresh["accessToken"].(string))
	asFresh.expect("POST", "/auth/logout", nil, http.StatusNoContent)
	if method == "session" {
		asFresh.expect("GET", "/auth/me", nil, http.StatusUnauthorized)
	} else {
		c.expect("POST", "/auth/refresh", map[string]any{"refreshToken": fresh["refreshToken"]}, http.StatusUnauthorized)
	}

	var limited reply
	for range 6 {
		limited, _ = c.send("POST", "/auth/login", map[string]any{"email": bobEmail, "password": "wrong password"}, nil)
	}
	if limited.StatusCode != http.StatusTooManyRequests || limited.Header.Get("Retry-After") == "" {
		t.Fatalf("login rate limit = %d %q", limited.StatusCode, limited.Header.Get("Retry-After"))
	}
	asBob.expect("GET", "/auth/me", nil, http.StatusOK)
	asAnn.expect("DELETE", "/notes/"+noteID+"/labels/"+labelID, nil, http.StatusNoContent)
}
