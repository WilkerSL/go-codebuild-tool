package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKubernetesEndToEnd deploys the generated manifests of a monolith and of
// a workspace to the cluster of the current kubectl context, usually a kind
// cluster created by the kubernetes workflow. It needs docker, kind and
// kubectl, and only runs with CODEBUILD_E2E_KUBERNETES=1.
func TestKubernetesEndToEnd(t *testing.T) {
	if os.Getenv("CODEBUILD_E2E_KUBERNETES") != "1" {
		t.Skip("set CODEBUILD_E2E_KUBERNETES=1 to deploy to the cluster of the current kubectl context")
	}
	for _, tool := range []string{"go", "docker", "kind", "kubectl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s is required: %v", tool, err)
		}
	}
	// A cluster with kube-prometheus-stack has these CRDs; the observability
	// component of the overlay needs them, not the operator itself.
	for _, crd := range []string{"podmonitors", "prometheusrules"} {
		command(t, "", nil, "kubectl", "apply", "--server-side", "-f", "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/"+prometheusOperator+"/example/prometheus-operator-crd/monitoring.coreos.com_"+crd+".yaml")
	}
	bin := buildTool(t)
	t.Run("monolith", func(t *testing.T) { kubernetesMonolith(t, bin) })
	t.Run("workspace", func(t *testing.T) { kubernetesWorkspace(t, bin) })
}

func kindCluster() string {
	if name := os.Getenv("CODEBUILD_E2E_KIND_CLUSTER"); name != "" {
		return name
	}
	return "kind"
}

type cluster struct {
	t         *testing.T
	namespace string
}

func newCluster(t *testing.T, namespace string) cluster {
	c := cluster{t: t, namespace: namespace}
	command(t, "", nil, "kubectl", "create", "namespace", namespace)
	t.Cleanup(func() {
		if t.Failed() {
			out, _ := run("", nil, "", "kubectl", "-n", namespace, "get", "pods,jobs,events", "-o", "wide")
			t.Logf("cluster state:\n%s", out)
			logs, _ := run("", nil, "", "kubectl", "-n", namespace, "logs", "--all-containers", "--prefix", "--max-log-requests", "20", "-l", "app.kubernetes.io/name", "--tail", "50")
			t.Logf("logs:\n%s", logs)
		}
		run("", nil, "", "kubectl", "delete", "namespace", namespace, "--wait=false")
	})
	return c
}

func (c cluster) kubectl(args ...string) string {
	c.t.Helper()
	return command(c.t, "", nil, "kubectl", append([]string{"-n", c.namespace}, args...)...)
}

func (c cluster) apply(manifest string) {
	c.t.Helper()
	cmd := exec.Command("kubectl", "-n", c.namespace, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if out, err := cmd.CombinedOutput(); err != nil {
		c.t.Fatalf("kubectl apply: %v\n%s", err, out)
	}
}

func (c cluster) secret(name string, files map[string]string) {
	c.t.Helper()
	args := []string{"create", "secret", "generic", name, "--dry-run=client", "-o", "yaml"}
	for key, value := range files {
		if strings.HasPrefix(value, "file:") {
			args = append(args, "--from-file="+key+"="+strings.TrimPrefix(value, "file:"))
		} else {
			args = append(args, "--from-literal="+key+"="+value)
		}
	}
	c.apply(c.kubectl(args...))
}

func (c cluster) postgres(name string) {
	c.t.Helper()
	c.apply(fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
spec:
  selector:
    matchLabels: {app: %[1]s}
  template:
    metadata:
      labels: {app: %[1]s}
    spec:
      containers:
        - name: postgres
          image: postgres:17
          env:
            - {name: POSTGRES_USER, value: app}
            - {name: POSTGRES_PASSWORD, value: app}
            - {name: POSTGRES_DB, value: app}
          ports: [{containerPort: 5432}]
          readinessProbe:
            exec: {command: ["pg_isready", "-U", "app"]}
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
spec:
  selector: {app: %[1]s}
  ports: [{port: 5432}]
`, name))
}

func (c cluster) deploy(dir, overlay string) {
	c.t.Helper()
	c.kubectl("rollout", "status", "deployment", "--timeout=180s")
	out := command(c.t, "", nil, "kubectl", "kustomize", overlay)
	c.apply(out)
	c.kubectl("wait", "--for=condition=complete", "job", "-l", "app.kubernetes.io/component=migration", "--timeout=300s")
	c.kubectl("rollout", "status", "deployment", "-l", "app.kubernetes.io/managed-by=codebuild", "--timeout=300s")
}

func (c cluster) forward(target string, port int) (client, func()) {
	c.t.Helper()
	addr := freeAddr(c.t)
	local := addr[strings.LastIndex(addr, ":")+1:]
	cmd := exec.Command("kubectl", "-n", c.namespace, "port-forward", target, fmt.Sprintf("%s:%d", local, port))
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() {
		if c.t.Failed() {
			c.t.Logf("port-forward %s:\n%s", target, logs.String())
		}
	})
	stop := func() {
		cmd.Process.Kill()
		cmd.Wait()
	}
	c.t.Cleanup(stop)
	cl := client{t: c.t, base: "http://127.0.0.1:" + local}
	waitReady(c.t, cl.base, logs.String)
	return cl, stop
}

func (c cluster) restarts() string {
	c.t.Helper()
	return c.kubectl("get", "pods", "-l", "app.kubernetes.io/name,app.kubernetes.io/component!=migration", "-o", "jsonpath={range .items[*]}{.metadata.name}={.status.containerStatuses[0].restartCount} {end}")
}

func buildImages(t *testing.T, dir string, images map[string]string) {
	t.Helper()
	for image, dockerfile := range images {
		command(t, dir, nil, "docker", "build", "-q", "-t", image+":e2e", "-f", dockerfile, ".")
		command(t, dir, nil, "kind", "load", "docker-image", "--name", kindCluster(), image+":e2e")
	}
}

func useLocalImages(t *testing.T, overlay, namespace string) {
	t.Helper()
	path := filepath.Join(overlay, "kustomization.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "registry.example.com/", "")
	text = strings.ReplaceAll(text, "newTag: latest", "newTag: e2e")
	text = strings.Replace(text, "namespace: ", "namespace: "+namespace+"\n# was: ", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// limitLogins sets the limit of five failed logins that shopSmoke expects, as
// the Compose test does with an override, through the user-owned overlay.
func limitLogins(t *testing.T, overlay string) {
	t.Helper()
	path := filepath.Join(overlay, "kustomization.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const merge = "  - name: gateway-config\n    behavior: merge\n    literals:\n"
	if !strings.Contains(string(data), merge) {
		t.Fatalf("overlay does not merge gateway-config:\n%s", data)
	}
	text := strings.Replace(string(data), merge, merge+"      - AUTH_LOGIN_LIMIT=5\n", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func kubernetesMonolith(t *testing.T, bin string) {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "secure"), filepath.Join(dir, "specs"))
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/secure", "--dialect", "postgres")
	command(t, dir, nil, "go", "mod", "tidy")
	buildImages(t, dir, map[string]string{"secure-api": "Dockerfile"})
	c := newCluster(t, "codebuild-e2e-secure")
	c.postgres("postgres")
	c.secret("secure-database", map[string]string{"url": "postgres://app:app@postgres:5432/app?sslmode=disable"})
	overlay := filepath.Join(dir, "deploy", "kubernetes", "overlays", "production")
	useLocalImages(t, overlay, c.namespace)
	c.deploy(dir, overlay)
	pods := strings.Fields(c.kubectl("get", "pods", "-l", "app.kubernetes.io/name=api", "-o", "jsonpath={.items[*].metadata.name}"))
	if len(pods) != 2 {
		t.Fatalf("api pods = %v, want 2 replicas", pods)
	}
	first, _ := c.forward("pod/"+pods[0], 8080)
	second, _ := c.forward("pod/"+pods[1], 8080)
	email := "kube-" + fmt.Sprint(time.Now().UnixNano()) + "@example.com"
	first.expect(http.MethodPost, "/auth/register", map[string]any{"email": email, "name": "Kube", "password": "kube password 1"}, http.StatusCreated)
	tokens := first.expect(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": "kube password 1"}, http.StatusOK)
	second.as(tokens["accessToken"].(string)).expect(http.MethodGet, "/auth/me", nil, http.StatusOK)
	member, admin := opsAccounts(t, first, func(email string) {
		c.kubectl("exec", "deploy/api", "--", "/app/accounts", "set-role", email, "admin")
	})
	opsCheck(t, first, member, admin, "/_ops/jaeger/api/v3/services", contains(`"api"`))
	for range 10 {
		first.expect(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": "wrong password"}, http.StatusUnauthorized)
	}
	// AUTH_LIMIT_STORE=database: the second replica sees the attempts made on the first.
	second.expect(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": "wrong password"}, http.StatusTooManyRequests)
	if out := c.restarts(); strings.Contains(out, "=1") || strings.Contains(out, "=2") {
		t.Fatalf("pods restarted: %s", out)
	}
}

func kubernetesWorkspace(t *testing.T, bin string) {
	dir := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "testdata", "specs", "shop"), filepath.Join(dir, "specs"))
	command(t, dir, nil, bin, "build", "--starter", "--module", "example.com/shop", "--dialect", "postgres")
	for _, m := range workspaceModules {
		command(t, filepath.Join(dir, filepath.FromSlash(m)), nil, "go", "mod", "tidy")
	}
	buildImages(t, dir, map[string]string{
		"shop-gateway":        "gateway/Dockerfile",
		"shop-billing":        "services/billing/Dockerfile",
		"shop-catalog":        "services/catalog/Dockerfile",
		"shop-identity":       "services/identity/Dockerfile",
		"shop-inventory":      "services/inventory/Dockerfile",
		"shop-shared-migrate": "databases/shared/Dockerfile",
	})
	c := newCluster(t, "codebuild-e2e-shop")
	databases := []string{"billing", "identity", "shared"}
	for _, db := range databases {
		c.postgres(db + "-db")
		c.secret("shop-"+db+"-database", map[string]string{"url": "postgres://app:app@" + db + "-db:5432/app?sslmode=disable"})
	}
	modules := []string{"gateway", "billing", "catalog", "identity", "inventory"}
	certs := func(out string) {
		command(t, dir, nil, "go", "run", "./gateway/cmd/devcerts", "-out", out, "-force")
		for _, m := range modules {
			c.secret("shop-"+m+"-tls", map[string]string{"ca.crt": "file:" + filepath.Join(out, "ca.pem"), "tls.crt": "file:" + filepath.Join(out, m+".pem"), "tls.key": "file:" + filepath.Join(out, m+"-key.pem")})
		}
	}
	first := filepath.Join(dir, "certs")
	certs(first)
	c.secret("shop-internal-token", map[string]string{"private.pem": "file:" + filepath.Join(first, "internal-token.pem"), "public.pem": "file:" + filepath.Join(first, "internal-token.pub.pem")})
	overlay := filepath.Join(dir, "deploy", "kubernetes", "overlays", "production")
	useLocalImages(t, overlay, c.namespace)
	limitLogins(t, overlay)
	c.deploy(dir, overlay)
	gateway, _ := c.forward("svc/gateway", 8080)
	shopSmoke(t, gateway, func(email string) {
		c.kubectl("exec", "deploy/identity", "--", "/app/accounts", "set-role", email, "admin")
	})
	member, admin := opsAccounts(t, gateway, func(email string) {
		c.kubectl("exec", "deploy/identity", "--", "/app/accounts", "set-role", email, "admin")
	})
	opsCheck(t, gateway, member, admin, "/_ops/jaeger/api/v3/services", contains(`"gateway"`))

	// Rotate every certificate to a new CA. Running pods only pick it up by
	// reloading the mounted secrets: after the rotation the catalog restarts
	// with the new certificates only, and the gateway, which keeps running,
	// must present and trust them on its next handshake.
	gatewayPods := c.kubectl("get", "pods", "-l", "app.kubernetes.io/name=gateway", "-o", "jsonpath={.items[*].metadata.name}")
	certs(filepath.Join(dir, "certs-rotated"))
	time.Sleep(150 * time.Second)
	c.kubectl("rollout", "restart", "deployment/catalog")
	c.kubectl("rollout", "status", "deployment/catalog", "--timeout=180s")
	// A fresh port-forward: kubectl may have dropped the first one while the
	// gateway closed its idle connection during the wait.
	gateway, _ = c.forward("svc/gateway", 8080)
	for range 5 {
		gateway.expect(http.MethodGet, "/categories", nil, http.StatusOK)
	}
	if now := c.kubectl("get", "pods", "-l", "app.kubernetes.io/name=gateway", "-o", "jsonpath={.items[*].metadata.name}"); now != gatewayPods {
		t.Fatalf("gateway pods changed during the rotation: %s, then %s", gatewayPods, now)
	}
	if out := c.restarts(); strings.Contains(out, "=1") || strings.Contains(out, "=2") {
		t.Fatalf("pods restarted: %s", out)
	}
}
