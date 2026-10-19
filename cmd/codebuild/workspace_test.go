package main

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var workspaceModules = []string{"contracts", "databases/shared", "gateway", "services/billing", "services/catalog", "services/identity", "services/inventory"}

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

func workspaceEndToEnd(t *testing.T, bin, dialect string) {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "shop"), filepath.Join(dir, "specs"))
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/shop", "--dialect", dialect)
	for _, m := range workspaceModules {
		command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "mod", "tidy")
	}
	command(t, dir, nil, "go", "build", "example.com/shop/...")
	command(t, dir, nil, "go", "vet", "example.com/shop/...")
	if out := command(t, dir, nil, "gofmt", "-l", "."); strings.TrimSpace(out) != "" {
		t.Fatalf("gofmt reports unformatted files:\n%s", out)
	}
	command(t, dir, nil, "go", "test", "-count=1", "example.com/shop/...")
	if out := command(t, dir, nil, bin, "plan", "--exit-code"); !strings.Contains(out, "0 created, 0 updated") {
		t.Fatalf("second plan is not empty:\n%s", out)
	}
	if dialect != "sqlite" {
		t.Logf("the running workspace is only exercised with sqlite")
		return
	}
	binDir := filepath.Join(dir, "bin")
	exe := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(binDir, name)
	}
	builds := [][3]string{
		{"gateway", "gateway", "./cmd/gateway"},
		{"gateway", "devcerts", "./cmd/devcerts"},
		{"databases/shared", "shared-migrate", "./cmd/migrate"},
		{"services/billing", "billing-migrate", "./cmd/migrate"},
		{"services/identity", "identity-migrate", "./cmd/migrate"},
		{"services/identity", "accounts", "./cmd/accounts"},
	}
	for _, s := range []string{"billing", "catalog", "identity", "inventory"} {
		builds = append(builds, [3]string{"services/" + s, s, "./cmd/server"})
	}
	for _, b := range builds {
		command(t, filepath.Join(dir, filepath.FromSlash(b[0])), nil, "go", "build", "-o", exe(b[1]), b[2])
	}
	certs := filepath.Join(dir, "certs")
	command(t, dir, nil, exe("devcerts"), "-out", certs)
	dbs := map[string]string{"billing": filepath.Join(dir, "billing.db"), "identity": filepath.Join(dir, "identity.db"), "shared": filepath.Join(dir, "shared.db")}
	for name, path := range dbs {
		command(t, dir, []string{"DATABASE_URL=" + path}, exe(name+"-migrate"), "up")
	}
	cert := func(name string) []string {
		return []string{
			"TLS_CA_FILE=" + filepath.Join(certs, "ca.pem"),
			"TLS_CERT_FILE=" + filepath.Join(certs, name+".pem"),
			"TLS_KEY_FILE=" + filepath.Join(certs, name+"-key.pem"),
			"LOG_LEVEL=warn",
		}
	}
	services := map[string]string{"billing": dbs["billing"], "catalog": dbs["shared"], "identity": dbs["identity"], "inventory": dbs["shared"]}
	grpcAddrs := map[string]string{}
	healthAddrs := map[string]string{}
	for name := range services {
		grpcAddrs[name] = freeAddr(t)
		healthAddrs[name] = freeAddr(t)
	}
	var peers []string
	for name, addr := range grpcAddrs {
		peers = append(peers, strings.ToUpper(name)+"_GRPC_ADDR="+addr)
	}
	start := func(name, binary string, env []string) *process {
		cmd := exec.Command(exe(binary))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		var logs bytes.Buffer
		cmd.Stdout, cmd.Stderr = &logs, &logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		p := &process{name: name, cmd: cmd, logs: &logs}
		t.Cleanup(p.stop)
		return p
	}
	running := map[string]*process{}
	for name, db := range services {
		env := append(cert(name), "DATABASE_URL="+db, "GRPC_ADDR="+grpcAddrs[name], "HEALTH_ADDR="+healthAddrs[name],
			"INTERNAL_TOKEN_PUBLIC_KEY_FILE="+filepath.Join(certs, "internal-token.pub.pem"), "AUTH_COOKIE_SECURE=false")
		running[name] = start(name, name, append(env, peers...))
	}
	for name, p := range running {
		waitReady(t, "http://"+healthAddrs[name], p.logs.String)
	}
	httpAddr := freeAddr(t)
	gatewayEnv := append(cert("gateway"), "HTTP_ADDR="+httpAddr, "INTERNAL_TOKEN_PRIVATE_KEY_FILE="+filepath.Join(certs, "internal-token.pem"), "AUTH_COOKIE_SECURE=false", "AUTH_LOGIN_LIMIT=5")
	gateway := start("gateway", "gateway", append(gatewayEnv, peers...))
	c := client{t: t, base: "http://" + httpAddr}
	waitReady(t, c.base, gateway.logs.String)
	shopSmoke(t, c, func(email string) {
		command(t, dir, []string{"DATABASE_URL=" + dbs["identity"]}, exe("accounts"), "set-role", email, "admin")
	})
	running["identity"].stop()
	if resp, _ := c.as("ses_unknown").send(http.MethodGet, "/auth/me", nil, nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/auth/me with the identity service down = %d, want 503", resp.StatusCode)
	}
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
	admin.expect(http.MethodPost, "/stocks", map[string]any{"productId": productID, "quantity": 10}, http.StatusCreated)
	missing := admin.expect(http.MethodPost, "/stocks", map[string]any{"productId": 999999, "quantity": 1}, http.StatusUnprocessableEntity)
	if got := strings.Join(fieldErrors(missing), ","); got != "productId" {
		t.Fatalf("stock with a missing product = %v", missing)
	}
	invoice := admin.expect(http.MethodPost, "/invoices", map[string]any{"number": "INV-1"}, http.StatusCreated)
	if invoice["ownerId"] != ana["id"] || invoice["status"] != "draft" || invoice["notes"] != nil {
		t.Fatalf("invoice = %v", invoice)
	}
	invoiceID := invoice["id"].(string)
	admin.expect(http.MethodPost, "/invoice-items", map[string]any{"invoiceId": invoiceID, "productId": productID, "quantity": 2, "price": 2.5}, http.StatusCreated)
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
	if page := bob.expect(http.MethodGet, "/invoices", nil, http.StatusOK); page["total"].(float64) != 0 {
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
