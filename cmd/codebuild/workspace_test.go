package main

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var workspaceModules = []string{"contracts", "databases/shared", "gateway", "services/billing", "services/catalog", "services/identity", "services/inventory"}

// workspaceDatabaseOf names the database each module of the shop workspace
// works on.
var workspaceDatabaseOf = map[string]string{
	"databases/shared":   "shared",
	"services/billing":   "billing",
	"services/catalog":   "shared",
	"services/identity":  "identity",
	"services/inventory": "shared",
}

// shopServices maps every service of the shop workspace to its database.
var shopServices = map[string]string{"billing": "billing", "catalog": "shared", "identity": "identity", "inventory": "shared"}

type process struct {
	name string
	cmd  *exec.Cmd
	logs *bytes.Buffer
}

func (p *process) stop() {
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
}

func TestWorkspaceEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("the end-to-end test downloads modules and builds generated workspaces")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	bin := buildTool(t)
	for _, dialect := range listFromEnv("CODEBUILD_E2E_WORKSPACE_DIALECTS", []string{"sqlite", "postgres", "mysql", "sqlserver"}) {
		t.Run(dialect, func(t *testing.T) {
			workspaceEndToEnd(t, bin, dialect)
		})
	}
}

// workspaceDatabases returns the data source of every database of the shop
// workspace: files for SQLite and, for the server dialects, the disposable
// databases listed in CODEBUILD_TEST_<DIALECT>_WORKSPACE_DSNS as
// billing=<dsn>;identity=<dsn>;shared=<dsn>. Without them it returns nil.
func workspaceDatabases(t *testing.T, dir, dialect string) map[string]string {
	t.Helper()
	if dialect == "sqlite" {
		return map[string]string{"billing": filepath.Join(dir, "billing.db"), "identity": filepath.Join(dir, "identity.db"), "shared": filepath.Join(dir, "shared.db")}
	}
	key := "CODEBUILD_TEST_" + strings.ToUpper(dialect) + "_WORKSPACE_DSNS"
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	dbs := map[string]string{}
	for part := range strings.SplitSeq(raw, ";") {
		if name, dsn, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			dbs[name] = dsn
		}
	}
	for _, name := range []string{"billing", "identity", "shared"} {
		if dbs[name] == "" {
			t.Fatalf("%s lacks the %s database: %q", key, name, raw)
		}
	}
	return dbs
}

// shopWorkspace is a generated shop workspace with its binaries built and its
// databases migrated, ready to run services and gateways.
type shopWorkspace struct {
	t           *testing.T
	dir         string
	dbs         map[string]string
	certs       string
	grpcAddrs   map[string]string
	healthAddrs map[string]string
	peers       []string
	services    map[string]*process
}

func (w *shopWorkspace) exe(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(w.dir, "bin", name)
}

func buildShopWorkspace(t *testing.T, dir string, dbs map[string]string) *shopWorkspace {
	t.Helper()
	w := &shopWorkspace{t: t, dir: dir, dbs: dbs, certs: filepath.Join(dir, "certs"), grpcAddrs: map[string]string{}, healthAddrs: map[string]string{}, services: map[string]*process{}}
	builds := [][3]string{
		{"gateway", "gateway", "./cmd/gateway"},
		{"gateway", "devcerts", "./cmd/devcerts"},
		{"databases/shared", "shared-migrate", "./cmd/migrate"},
		{"services/billing", "billing-migrate", "./cmd/migrate"},
		{"services/identity", "identity-migrate", "./cmd/migrate"},
		{"services/identity", "accounts", "./cmd/accounts"},
	}
	for name := range shopServices {
		builds = append(builds, [3]string{"services/" + name, name, "./cmd/server"})
	}
	for _, b := range builds {
		command(t, filepath.Join(dir, filepath.FromSlash(b[0])), nil, "go", "build", "-o", w.exe(b[1]), b[2])
	}
	command(t, dir, nil, w.exe("devcerts"), "-out", w.certs)
	for name, dsn := range dbs {
		// The module tests may have left rows behind: start from empty tables.
		run(dir, []string{"DATABASE_URL=" + dsn}, "", w.exe(name+"-migrate"), "down", "1")
		command(t, dir, []string{"DATABASE_URL=" + dsn}, w.exe(name+"-migrate"), "up")
		t.Cleanup(func() { run(dir, []string{"DATABASE_URL=" + dsn}, "", w.exe(name+"-migrate"), "down", "1") })
	}
	for name := range shopServices {
		w.grpcAddrs[name] = freeAddr(t)
		w.healthAddrs[name] = freeAddr(t)
	}
	for name, addr := range w.grpcAddrs {
		w.peers = append(w.peers, strings.ToUpper(name)+"_GRPC_ADDR="+addr)
	}
	return w
}

func (w *shopWorkspace) cert(name string) []string {
	return []string{
		"TLS_CA_FILE=" + filepath.Join(w.certs, "ca.pem"),
		"TLS_CERT_FILE=" + filepath.Join(w.certs, name+".pem"),
		"TLS_KEY_FILE=" + filepath.Join(w.certs, name+"-key.pem"),
		"LOG_LEVEL=warn",
		"GRPC_BREAKER_COOLDOWN=1s",
	}
}

func (w *shopWorkspace) start(name, binary string, env []string) *process {
	w.t.Helper()
	cmd := exec.Command(w.exe(binary))
	cmd.Dir = w.dir
	cmd.Env = append(os.Environ(), env...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		w.t.Fatal(err)
	}
	p := &process{name: name, cmd: cmd, logs: &logs}
	w.t.Cleanup(p.stop)
	return p
}

// startService starts one service and waits until it is ready.
func (w *shopWorkspace) startService(name string, extra ...string) {
	w.t.Helper()
	env := append(w.cert(name), "DATABASE_URL="+w.dbs[shopServices[name]], "GRPC_ADDR="+w.grpcAddrs[name], "HEALTH_ADDR="+w.healthAddrs[name],
		"INTERNAL_TOKEN_PUBLIC_KEY_FILE="+filepath.Join(w.certs, "internal-token.pub.pem"), "AUTH_COOKIE_SECURE=false")
	p := w.start(name, name, append(append(env, w.peers...), extra...))
	w.services[name] = p
	waitReady(w.t, "http://"+w.healthAddrs[name], p.logs.String)
}

// startGateway starts a gateway and returns a client for it once it is ready.
func (w *shopWorkspace) startGateway(name string, extra ...string) (client, *process) {
	w.t.Helper()
	addr := freeAddr(w.t)
	env := append(w.cert("gateway"), "HTTP_ADDR="+addr, "INTERNAL_TOKEN_PRIVATE_KEY_FILE="+filepath.Join(w.certs, "internal-token.pem"), "AUTH_COOKIE_SECURE=false")
	p := w.start(name, "gateway", append(append(env, w.peers...), extra...))
	c := client{t: w.t, base: "http://" + addr}
	waitReady(w.t, c.base, p.logs.String)
	return c, p
}

// useLocalDriver lets every module with a database use the driver variant
// named in CODEBUILD_TEST_<DIALECT>_IMPORT, such as SQL Server over shared
// memory on a development machine.
func useLocalDriver(t *testing.T, dir, dialect string) {
	t.Helper()
	imp := os.Getenv("CODEBUILD_TEST_" + strings.ToUpper(dialect) + "_IMPORT")
	if imp == "" {
		return
	}
	for module := range workspaceDatabaseOf {
		hook := "package database\n\nimport _ " + strconv.Quote(imp) + "\n"
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(module), "internal", "platform", "database", "driver_local.go"), []byte(hook), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func workspaceEndToEnd(t *testing.T, bin, dialect string) {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "shop"), filepath.Join(dir, "specs"))
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/shop", "--dialect", dialect)
	for _, m := range workspaceModules {
		command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "mod", "tidy")
	}
	useLocalDriver(t, dir, dialect)
	command(t, dir, nil, "go", "build", "example.com/shop/...")
	command(t, dir, nil, "go", "vet", "example.com/shop/...")
	if out := command(t, dir, nil, "gofmt", "-l", "."); strings.TrimSpace(out) != "" {
		t.Fatalf("gofmt reports unformatted files:\n%s", out)
	}
	dbs := workspaceDatabases(t, dir, dialect)
	if dialect == "sqlite" || dbs == nil {
		command(t, dir, nil, "go", "test", "-count=1", "example.com/shop/...")
	} else {
		// Every module tests against its own database, as it runs.
		for _, m := range workspaceModules {
			var env []string
			if db, ok := workspaceDatabaseOf[m]; ok {
				env = []string{"TEST_DATABASE_URL=" + dbs[db]}
			}
			command(t, filepath.Join(dir, filepath.FromSlash(m)), env, "go", "test", "-count=1", "./...")
		}
	}
	if out := command(t, dir, nil, bin, "plan", "--exit-code"); !strings.Contains(out, "0 created, 0 updated") {
		t.Fatalf("second plan is not empty:\n%s", out)
	}
	if dbs == nil {
		t.Logf("set CODEBUILD_TEST_%s_WORKSPACE_DSNS to run the %s workspace", strings.ToUpper(dialect), dialect)
		return
	}
	w := buildShopWorkspace(t, dir, dbs)
	for name := range shopServices {
		w.startService(name)
	}
	c, gateway := w.startGateway("gateway", "AUTH_LOGIN_LIMIT=5", "AUTH_LIMIT_STORE=database")
	shopSmoke(t, c, func(email string) {
		command(t, dir, []string{"DATABASE_URL=" + dbs["identity"]}, w.exe("accounts"), "set-role", email, "admin")
	})
	openAPICheck(t, c, dir)
	if resp, _ := c.send(http.MethodGet, "/categories", nil, http.Header{"X-Request-Id": {"workspace-check"}}); resp.Header.Get("X-Request-ID") != "workspace-check" {
		t.Fatalf("the gateway did not keep the request id: %q", resp.Header.Get("X-Request-ID"))
	}
	// A second gateway shares the login counters kept by the identity service.
	second, other := w.startGateway("gateway-2", "AUTH_LOGIN_LIMIT=5", "AUTH_LIMIT_STORE=database")
	if resp, _ := second.send(http.MethodPost, "/auth/login", map[string]any{"email": "bob@example.com", "password": "supersecret2"}, nil); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("login on a second gateway = %d, want the shared limit", resp.StatusCode)
	}
	other.stop()
	failureCheck(t, w, c)
	w.services["identity"].stop()
	if resp, _ := c.as("ses_unknown").send(http.MethodGet, "/auth/me", nil, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/auth/me with the identity service down = %d, want 503", resp.StatusCode)
	}
	if dialect == "sqlite" && os.Getenv("CODEBUILD_E2E_DOCKER") == "1" {
		gateway.stop()
		for _, p := range w.services {
			p.stop()
		}
		composeCheck(t, dir)
	}
}

// failureCheck stops the catalog service: reads from it, and writes that check
// a product in it, answer 503 instead of failing or hanging, and calls work
// again once it is back.
func failureCheck(t *testing.T, w *shopWorkspace, c client) {
	t.Helper()
	tokens := c.expect(http.MethodPost, "/auth/login", map[string]any{"email": "ana@example.com", "password": "supersecret1"}, http.StatusOK)
	admin := c.as(tokens["accessToken"].(string))
	invoice := admin.expect(http.MethodPost, "/invoices", map[string]any{"number": "INV-DOWN"}, http.StatusCreated)
	w.services["catalog"].stop()
	if resp, out := c.send(http.MethodGet, "/products", nil, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/products with the catalog down = %d %v, want 503", resp.StatusCode, out)
	}
	resp, out := admin.send(http.MethodPost, "/invoice-items", map[string]any{"invoiceId": invoice["id"], "productId": 1, "quantity": 1, "price": 1}, nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("an invoice item checked against the catalog while it is down = %d %v, want 503", resp.StatusCode, out)
	}
	w.startService("catalog")
	waitFor(t, func() bool {
		resp, _ := c.send(http.MethodGet, "/products", nil, nil)
		return resp.StatusCode == http.StatusOK
	})
}

func composeCheck(t *testing.T, dir string) {
	project := []string{"compose", "--project-name", "codebuild-e2e-shop", "--profile", "observability"}
	compose := func(args ...string) string {
		t.Helper()
		return command(t, dir, nil, "docker", append(slices.Clone(project), args...)...)
	}
	t.Cleanup(func() {
		run(dir, nil, "", "docker", append(slices.Clone(project), "down", "--volumes", "--rmi", "all")...)
	})
	override := "services:\n  gateway:\n    environment:\n      AUTH_LOGIN_LIMIT: \"5\"\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := compose("config"); !strings.Contains(out, "AUTH_LOGIN_LIMIT") {
		t.Fatalf("docker compose ignores docker-compose.override.yml:\n%s", out)
	}
	compose("up", "--detach", "--build")
	c := client{t: t, base: "http://localhost:8080"}
	waitReady(t, c.base, func() string {
		out, _ := run(dir, nil, "", "docker", append(slices.Clone(project), "logs")...)
		return out
	})
	shopSmoke(t, c, func(email string) {
		compose("exec", "-T", "identity", "/app/accounts", "set-role", email, "admin")
	})
	member, admin := opsAccounts(t, c, func(email string) {
		compose("exec", "-T", "identity", "/app/accounts", "set-role", email, "admin")
	})
	opsCheck(t, c, member, admin, "/_ops/jaeger/api/v3/services", contains(`"gateway"`))
	opsWait(t, c, admin, "/_ops/prometheus/api/v1/targets?state=active", func(body []byte) bool {
		return bytes.Count(body, []byte(`"health":"up"`)) == 6
	})
	opsWait(t, c, admin, "/_ops/grafana/api/search?query=shop", contains(`"uid":"shop"`))
	// The Monitor tab of Jaeger: span metrics that Prometheus scraped from Jaeger.
	opsWait(t, c, admin, "/_ops/jaeger/api/metrics/calls?service=gateway", contains(`"gaugeValue"`))
}

func fieldErrors(out map[string]any) []string {
	var fields []string
	list, _ := out["errors"].([]any)
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			fields = append(fields, m["field"].(string))
		}
	}
	return fields
}

func shopSmoke(t *testing.T, c client, promote func(email string)) {
	c.expect(http.MethodGet, "/products", nil, http.StatusOK)
	resp, _ := c.send(http.MethodGet, "/invoices", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("Www-Authenticate") != "Bearer" {
		t.Fatalf("anonymous /invoices = %d %q", resp.StatusCode, resp.Header.Get("Www-Authenticate"))
	}
	resp, ana := c.send(http.MethodPost, "/auth/register", map[string]any{"email": "Ana@Example.com", "name": "Ana", "password": "supersecret1"}, nil)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get("Location") != "/users/"+ana["id"].(string) || ana["email"] != "ana@example.com" || ana["role"] != "member" {
		t.Fatalf("register = %d %v %v", resp.StatusCode, resp.Header.Get("Location"), ana)
	}
	invalid := c.expect(http.MethodPost, "/auth/register", map[string]any{"email": "nope", "name": " ", "password": "short"}, http.StatusUnprocessableEntity)
	if got := strings.Join(fieldErrors(invalid), ","); got != "email,name,password" {
		t.Fatalf("register errors = %s", got)
	}
	login := func(email, password string) client {
		t.Helper()
		tokens := c.expect(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": password}, http.StatusOK)
		return c.as(tokens["accessToken"].(string))
	}
	user := login("ana@example.com", "supersecret1")
	if me := user.expect(http.MethodGet, "/auth/me", nil, http.StatusOK); me["id"] != ana["id"] {
		t.Fatalf("me = %v", me)
	}
	user.expect(http.MethodPost, "/products", map[string]any{"name": "Pen", "price": 2.5}, http.StatusForbidden)
	promote("ana@example.com")
	admin := user
	category := admin.expect(http.MethodPost, "/categories", map[string]any{"name": "Office"}, http.StatusCreated)
	product := admin.expect(http.MethodPost, "/products", map[string]any{"name": "Pen", "price": 2.5, "categoryId": category["id"]}, http.StatusCreated)
	categoryID := int64(category["id"].(float64))
	productID := int64(product["id"].(float64))
	if page := c.expect(http.MethodGet, "/categories/"+strconv.FormatInt(categoryID, 10)+"/products", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("category products = %v", page)
	}
	admin.expect(http.MethodPost, "/categories", map[string]any{"name": "Paper"}, http.StatusCreated)
	if ids := walkCursor(t, c, "/categories"); len(ids) != 2 {
		t.Fatalf("cursor pagination through the gateway saw %d categories", len(ids))
	}
	admin.expect(http.MethodPost, "/stocks", map[string]any{"productId": productID, "quantity": 10}, http.StatusCreated)
	idempotencyCheck(t, admin, "/categories", map[string]any{"name": "Idempotent"}, map[string]any{"name": "Other"})
	missing := admin.expect(http.MethodPost, "/stocks", map[string]any{"productId": 999999, "quantity": 1}, http.StatusUnprocessableEntity)
	if got := strings.Join(fieldErrors(missing), ","); got != "productId" {
		t.Fatalf("stock with a missing product = %v", missing)
	}
	invoice := admin.expect(http.MethodPost, "/invoices", map[string]any{"number": "INV-1"}, http.StatusCreated)
	if invoice["ownerId"] != ana["id"] || invoice["status"] != "draft" || invoice["notes"] != nil {
		t.Fatalf("invoice = %v", invoice)
	}
	invoiceID := invoice["id"].(string)
	item := admin.expect(http.MethodPost, "/invoice-items", map[string]any{"invoiceId": invoiceID, "productId": productID, "quantity": 2, "price": 2.5}, http.StatusCreated)
	itemPath := "/invoice-items/" + strconv.FormatInt(int64(item["id"].(float64)), 10)
	remote := admin.expect(http.MethodPost, "/invoice-items", map[string]any{"invoiceId": invoiceID, "productId": 424242, "quantity": 2, "price": 2.5}, http.StatusUnprocessableEntity)
	if got := strings.Join(fieldErrors(remote), ","); got != "productId" {
		t.Fatalf("invoice item with a product of another database = %v", remote)
	}
	if page := admin.expect(http.MethodGet, "/invoices/"+invoiceID+"/items", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("invoice items = %v", page)
	}
	admin.expect(http.MethodGet, "/invoices/not-a-uuid", nil, http.StatusBadRequest)
	admin.expect(http.MethodGet, "/invoice-items?productId=abc", nil, http.StatusBadRequest)
	key := admin.expect(http.MethodPost, "/auth/api-keys", map[string]any{"name": "ci", "expiresAt": "2030-01-01T00:00:00Z"}, http.StatusCreated)
	if keys := admin.expect(http.MethodGet, "/auth/api-keys", nil, http.StatusOK); len(keys["items"].([]any)) != 1 {
		t.Fatalf("api keys = %v", keys)
	}
	c.as(key["key"].(string)).expect(http.MethodGet, "/invoices", nil, http.StatusOK)
	c.expect(http.MethodPost, "/auth/register", map[string]any{"email": "bob@example.com", "name": "Bob", "password": "supersecret2"}, http.StatusCreated)
	bob := login("bob@example.com", "supersecret2")
	bob.expect(http.MethodGet, "/invoices/"+invoiceID, nil, http.StatusNotFound)
	// Invoice items belong to the owner of their invoice, on their own routes too.
	bob.expect(http.MethodGet, itemPath, nil, http.StatusNotFound)
	bob.expect(http.MethodPut, itemPath, map[string]any{"invoiceId": invoiceID, "productId": productID, "quantity": 9, "price": 0.1}, http.StatusUnprocessableEntity)
	bobInvoice := bob.expect(http.MethodPost, "/invoices", map[string]any{"number": "INV-BOB"}, http.StatusCreated)
	bob.expect(http.MethodPut, itemPath, map[string]any{"invoiceId": bobInvoice["id"], "productId": productID, "quantity": 9, "price": 0.1}, http.StatusNotFound)
	bob.expect(http.MethodDelete, itemPath, nil, http.StatusNotFound)
	bob.expect(http.MethodPost, "/invoice-items", map[string]any{"invoiceId": invoiceID, "productId": productID, "quantity": 1, "price": 1}, http.StatusUnprocessableEntity)
	if page := bob.expect(http.MethodGet, "/invoice-items", nil, http.StatusOK); page["total"].(float64) != 0 {
		t.Fatalf("bob sees invoice items of another owner: %v", page)
	}
	admin.expect(http.MethodGet, itemPath, nil, http.StatusOK)
	if page := bob.expect(http.MethodGet, "/invoices", nil, http.StatusOK); page["total"].(float64) != 1 {
		t.Fatalf("bob sees invoices of another owner: %v", page)
	}
	bob.expect(http.MethodPost, "/auth/logout", nil, http.StatusNoContent)
	bob.expect(http.MethodGet, "/auth/me", nil, http.StatusUnauthorized)
	for range 5 {
		c.expect(http.MethodPost, "/auth/login", map[string]any{"email": "bob@example.com", "password": "wrong-password"}, http.StatusUnauthorized)
	}
	resp, _ = c.send(http.MethodPost, "/auth/login", map[string]any{"email": "bob@example.com", "password": "wrong-password"}, nil)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("sixth failed login = %d %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}
