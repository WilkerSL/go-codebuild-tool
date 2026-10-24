package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoad measures generated monoliths on PostgreSQL, and the shop workspace
// when CODEBUILD_TEST_POSTGRES_WORKSPACE_DSNS names its databases: throughput,
// latency percentiles and errors of the common requests, after seeding enough
// rows that list queries cannot hide behind a small table. It only runs with
// CODEBUILD_E2E_LOAD=1 and CODEBUILD_TEST_POSTGRES_DSN; CODEBUILD_E2E_LOAD_OUT
// receives the results as a Markdown table.
func TestLoad(t *testing.T) {
	if os.Getenv("CODEBUILD_E2E_LOAD") != "1" {
		t.Skip("set CODEBUILD_E2E_LOAD=1 to run the load test")
	}
	dsn := os.Getenv("CODEBUILD_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("the load test needs CODEBUILD_TEST_POSTGRES_DSN")
	}
	cfg := loadConfig{
		duration:    envDuration(t, "CODEBUILD_E2E_LOAD_DURATION", 20*time.Second),
		concurrency: envInt(t, "CODEBUILD_E2E_LOAD_CONCURRENCY", 32),
		rows:        envInt(t, "CODEBUILD_E2E_LOAD_ROWS", 50_000),
	}
	bin := buildTool(t)
	var results []loadResult
	t.Run("saas", func(t *testing.T) { results = append(results, loadSaaS(t, bin, dsn, cfg)...) })
	t.Run("secure-jwt", func(t *testing.T) { results = append(results, loadAuth(t, bin, dsn, cfg)...) })
	if dbs := workspaceDatabases(t, "", "postgres"); dbs != nil {
		t.Run("workspace", func(t *testing.T) { results = append(results, loadWorkspace(t, bin, dbs, cfg)...) })
	}
	report := loadReport(cfg, results)
	t.Log("\n" + report)
	if out := os.Getenv("CODEBUILD_E2E_LOAD_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(report), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range results {
		if r.errors > 0 {
			t.Errorf("%s: %d of %d requests failed, first: %s", r.name, r.errors, r.requests, r.firstError)
		}
	}
}

type loadConfig struct {
	duration    time.Duration
	concurrency int
	rows        int
}

type loadResult struct {
	name       string
	requests   int
	errors     int
	firstError string
	elapsed    time.Duration
	latencies  []time.Duration
}

func envDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		return d
	}
	return fallback
}

func envInt(t *testing.T, key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		return n
	}
	return fallback
}

type loadClient struct {
	base   string
	client *http.Client
}

func newLoadClient(base string, concurrency int) *loadClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = concurrency * 2
	transport.MaxIdleConnsPerHost = concurrency * 2
	return &loadClient{base: base, client: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
}

// do sends one request and decodes the JSON answer into out when it is not nil.
func (c *loadClient) do(method, path, token string, body any, want int, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != want {
		return fmt.Errorf("%s %s = %d: %.200s", method, path, resp.StatusCode, data)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// measure runs request from cfg.concurrency workers for cfg.duration, or
// count times in total when count is positive.
func measure(name string, cfg loadConfig, count int, request func(worker, i int) error) loadResult {
	var (
		mu      sync.Mutex
		result  = loadResult{name: name}
		next    atomic.Int64
		wg      sync.WaitGroup
		started = time.Now()
	)
	deadline := started.Add(cfg.duration)
	for worker := range cfg.concurrency {
		wg.Go(func() {
			var local []time.Duration
			var errs int
			var first string
			for {
				i := int(next.Add(1)) - 1
				if count > 0 && i >= count || count <= 0 && time.Now().After(deadline) {
					break
				}
				start := time.Now()
				err := request(worker, i)
				local = append(local, time.Since(start))
				if err != nil {
					errs++
					if first == "" {
						first = err.Error()
					}
				}
			}
			mu.Lock()
			result.latencies = append(result.latencies, local...)
			result.errors += errs
			if result.firstError == "" {
				result.firstError = first
			}
			mu.Unlock()
		})
	}
	wg.Wait()
	result.elapsed = time.Since(started)
	result.requests = len(result.latencies)
	return result
}

func (r loadResult) percentile(p float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}
	sorted := slices.Clone(r.latencies)
	slices.Sort(sorted)
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*p))]
}

func ms(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64)
}

func cpuModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown CPU"
	}
	for line := range strings.Lines(string(data)) {
		if name, ok := strings.CutPrefix(line, "model name"); ok {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), ":"))
		}
	}
	return "unknown CPU"
}

func loadReport(cfg loadConfig, results []loadResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hardware: %s, %d logical CPUs, %s/%s, %s. Load generator, API and PostgreSQL on the same machine.\n", cpuModel(), runtime.NumCPU(), runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(&b, "Settings: %d concurrent clients, %s per scenario, %d seeded projects.\n\n", cfg.concurrency, cfg.duration, cfg.rows)
	b.WriteString("| Scenario | Requests | Throughput (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errors |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %d | %.0f | %s | %s | %s | %d |\n", r.name, r.requests, float64(r.requests)/r.elapsed.Seconds(), ms(r.percentile(0.50)), ms(r.percentile(0.95)), ms(r.percentile(0.99)), r.errors)
	}
	return b.String()
}

func startLoadProject(t *testing.T, bin, dsn, specs, method string, env []string) string {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", specs), filepath.Join(dir, "specs"))
	if method == "jwt" {
		spec := filepath.Join(dir, "specs", "users.go")
		data, err := os.ReadFile(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(spec, bytes.Replace(data, []byte("auth=session|apikey"), []byte("auth=jwt|apikey"), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/"+specs, "--dialect", "postgres")
	command(t, dir, nil, "go", "mod", "tidy")
	env = append([]string{"DATABASE_URL=" + dsn, "LOG_LEVEL=error"}, env...)
	command(t, dir, env, "go", "run", "./cmd/migrate", "up")
	t.Cleanup(func() { run(dir, env, "", "go", "run", "./cmd/migrate", "down", "1") })
	c, _ := startAPI(t, dir, env)
	return c.base
}

func loadSaaS(t *testing.T, bin, dsn string, cfg loadConfig) []loadResult {
	base := startLoadProject(t, bin, dsn, "saas", "", nil)
	c := newLoadClient(base, cfg.concurrency)
	var org struct{ ID string }
	if err := c.do("POST", "/orgs", "", map[string]any{"name": "Load", "slug": "load"}, http.StatusCreated, &org); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, cfg.rows)
	create := func(_, i int) error {
		var p struct{ ID string }
		err := c.do("POST", "/projects", "", map[string]any{"orgId": org.ID, "name": "Project " + strconv.Itoa(i), "budget": float64(i)}, http.StatusCreated, &p)
		if i < len(ids) {
			ids[i] = p.ID
		}
		return err
	}
	results := []loadResult{measure("create project (seeding)", cfg, cfg.rows, create)}
	results = append(results, measure("create project", cfg, 0, func(w, i int) error { return create(w, cfg.rows+i) }))
	results = append(results, measure("get project", cfg, 0, func(int, int) error {
		return c.do("GET", "/projects/"+ids[rand.IntN(len(ids))], "", nil, http.StatusOK, nil)
	}))
	results = append(results, measure("list projects, first page", cfg, 0, func(int, int) error {
		return c.do("GET", "/projects?limit=20", "", nil, http.StatusOK, nil)
	}))
	results = append(results, measure("list projects, offset in the last tenth", cfg, 0, func(int, int) error {
		offset := cfg.rows - cfg.rows/10 + rand.IntN(cfg.rows/10)
		return c.do("GET", "/projects?limit=20&offset="+strconv.Itoa(offset), "", nil, http.StatusOK, nil)
	}))
	var cursors []string
	var page struct{ NextCursor *string }
	path := "/projects?limit=20&cursor="
	if err := c.do("GET", "/projects?limit=100", "", nil, http.StatusOK, &page); err != nil {
		t.Fatal(err)
	}
	for cursor := page.NextCursor; cursor != nil && len(cursors) < 500; {
		cursors = append(cursors, *cursor)
		var next struct{ NextCursor *string }
		if err := c.do("GET", path+*cursor+"&limit=100", "", nil, http.StatusOK, &next); err != nil {
			t.Fatal(err)
		}
		cursor = next.NextCursor
	}
	results = append(results, measure("list projects, cursor", cfg, 0, func(int, int) error {
		return c.do("GET", path+cursors[rand.IntN(len(cursors))], "", nil, http.StatusOK, nil)
	}))
	results = append(results, measure("list projects of an org, cursor", cfg, 0, func(int, int) error {
		return c.do("GET", "/orgs/"+org.ID+"/projects?limit=20&cursor="+cursors[rand.IntN(len(cursors))], "", nil, http.StatusOK, nil)
	}))
	return results
}

func loadAuth(t *testing.T, bin, dsn string, cfg loadConfig) []loadResult {
	base := startLoadProject(t, bin, dsn, "secure", "jwt", []string{"AUTH_JWT_SECRET=" + e2eSecret, "AUTH_LOGIN_LIMIT=0", "AUTH_COOKIE_SECURE=false"})
	c := newLoadClient(base, cfg.concurrency)
	users := cfg.concurrency * 2
	for i := range users {
		body := map[string]any{"email": fmt.Sprintf("load-%d@example.com", i), "name": "Load", "password": "load password 1"}
		if err := c.do("POST", "/auth/register", "", body, http.StatusCreated, nil); err != nil {
			t.Fatal(err)
		}
	}
	type tokens struct{ AccessToken, RefreshToken string }
	login := func(i int) (tokens, error) {
		var out tokens
		err := c.do("POST", "/auth/login", "", map[string]any{"email": fmt.Sprintf("load-%d@example.com", i%users), "password": "load password 1"}, http.StatusOK, &out)
		return out, err
	}
	results := []loadResult{measure("login", cfg, 0, func(_, i int) error {
		_, err := login(i)
		return err
	})}
	current := make([]tokens, cfg.concurrency)
	for w := range current {
		var err error
		if current[w], err = login(w); err != nil {
			t.Fatal(err)
		}
	}
	results = append(results, measure("refresh", cfg, 0, func(w, _ int) error {
		var out tokens
		if err := c.do("POST", "/auth/refresh", "", map[string]any{"refreshToken": current[w].RefreshToken}, http.StatusOK, &out); err != nil {
			return err
		}
		current[w] = out
		return nil
	}))
	for w := range current {
		if err := c.do("POST", "/notes", current[w].AccessToken, map[string]any{"title": "load"}, http.StatusCreated, nil); err != nil {
			t.Fatal(err)
		}
	}
	results = append(results, measure("list own notes (JWT)", cfg, 0, func(w, _ int) error {
		return c.do("GET", "/notes?limit=20", current[w].AccessToken, nil, http.StatusOK, nil)
	}))
	return results
}

// loadWorkspace measures the shop workspace through its gateway: every
// request crosses the gateway, mutual TLS and gRPC, and authenticated ones
// also ask the identity service, so the numbers show what the hops cost next
// to the monolith.
func loadWorkspace(t *testing.T, bin string, dbs map[string]string, cfg loadConfig) []loadResult {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "shop"), filepath.Join(dir, "specs"))
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/shop", "--dialect", "postgres")
	for _, m := range workspaceModules {
		command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "mod", "tidy")
	}
	w := buildShopWorkspace(t, dir, dbs)
	for name := range shopServices {
		w.startService(name, "LOG_LEVEL=error")
	}
	gateway, _ := w.startGateway("gateway", "LOG_LEVEL=error", "AUTH_LOGIN_LIMIT=0")
	c := newLoadClient(gateway.base, cfg.concurrency)
	email := fmt.Sprintf("load-%d@example.com", time.Now().UnixNano())
	if err := c.do("POST", "/auth/register", "", map[string]any{"email": email, "name": "Load", "password": "load password 1"}, http.StatusCreated, nil); err != nil {
		t.Fatal(err)
	}
	command(t, dir, []string{"DATABASE_URL=" + dbs["identity"]}, w.exe("accounts"), "set-role", email, "admin")
	var tokens struct{ AccessToken string }
	if err := c.do("POST", "/auth/login", "", map[string]any{"email": email, "password": "load password 1"}, http.StatusOK, &tokens); err != nil {
		t.Fatal(err)
	}
	admin := tokens.AccessToken
	rows := min(cfg.rows, 5000)
	ids := make([]string, rows)
	create := func(_, i int) error {
		var category struct{ ID int64 }
		err := c.do("POST", "/categories", admin, map[string]any{"name": fmt.Sprintf("Category %d-%d", i, time.Now().UnixNano())}, http.StatusCreated, &category)
		if i < len(ids) {
			ids[i] = strconv.FormatInt(category.ID, 10)
		}
		return err
	}
	results := []loadResult{measure("workspace: create category (seeding)", cfg, rows, create)}
	results = append(results, measure("workspace: get category", cfg, 0, func(int, int) error {
		return c.do("GET", "/categories/"+ids[rand.IntN(len(ids))], "", nil, http.StatusOK, nil)
	}))
	var page struct{ NextCursor *string }
	if err := c.do("GET", "/categories?limit=20", "", nil, http.StatusOK, &page); err != nil || page.NextCursor == nil {
		t.Fatalf("first page: %v", err)
	}
	results = append(results, measure("workspace: list categories, cursor", cfg, 0, func(int, int) error {
		return c.do("GET", "/categories?limit=20&cursor="+*page.NextCursor, "", nil, http.StatusOK, nil)
	}))
	if err := c.do("POST", "/invoices", admin, map[string]any{"number": "LOAD-1"}, http.StatusCreated, nil); err != nil {
		t.Fatal(err)
	}
	results = append(results, measure("workspace: list own invoices (session)", cfg, 0, func(int, int) error {
		return c.do("GET", "/invoices?limit=20", admin, nil, http.StatusOK, nil)
	}))
	results = append(results, measure("workspace: create category", cfg, 0, func(w, i int) error { return create(w, rows+i) }))
	return results
}
