package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// prometheusOperator is the release whose CRDs the Kubernetes test installs,
// so the PodMonitors and the PrometheusRule of the observability component
// apply as they would on a cluster with kube-prometheus-stack.
const prometheusOperator = "v0.94.2"

// fakePanel stands in for Jaeger behind the operations proxy of the API and
// records what the proxy forwards.
type fakePanel struct {
	url      string
	mu       sync.Mutex
	hits     int
	problems []string
}

func newFakePanel(t *testing.T) *fakePanel {
	p := &fakePanel{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.hits++
		if r.Header.Get("Authorization") != "" {
			p.problems = append(p.problems, "the Authorization header reached the panel")
		}
		if cookie := r.Header.Get("Cookie"); cookie != "" {
			p.problems = append(p.problems, "cookies reached the panel: "+cookie)
		}
		if r.Header.Get("X-Webauth-User") == "" {
			p.problems = append(p.problems, "the panel did not get the user")
		}
		if !strings.HasPrefix(r.URL.Path, "/_ops/jaeger/") {
			p.problems = append(p.problems, "the panel got "+r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"services":["api"]}`)
	}))
	t.Cleanup(srv.Close)
	p.url = srv.URL
	return p
}

func (p *fakePanel) check(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hits == 0 {
		t.Fatal("no request reached the operations panel")
	}
	if len(p.problems) > 0 {
		t.Fatalf("operations proxy: %s", strings.Join(p.problems, "; "))
	}
}

// opsAccounts registers a member and an admin through the API and returns
// clients signed in as each of them.
func opsAccounts(t *testing.T, c client, promote func(email string)) (member, admin client) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	emails := []string{"ops-member-" + suffix + "@example.com", "ops-admin-" + suffix + "@example.com"}
	for _, email := range emails {
		c.expect(http.MethodPost, "/auth/register", map[string]any{"email": email, "name": "Ops", "password": "ops password 1"}, http.StatusCreated)
	}
	promote(emails[1])
	login := func(email string) client {
		tokens := c.expect(http.MethodPost, "/auth/login", map[string]any{"email": email, "password": "ops password 1"}, http.StatusOK)
		return c.as(tokens["accessToken"].(string))
	}
	return login(emails[0]), login(emails[1])
}

// opsCheck walks the operations panels behind the API or the gateway: anyone
// gets the sign-in page, anonymous calls are refused and anonymous browsers
// are sent to that page, members are forbidden, and admins reach path once
// ready accepts its answer.
func opsCheck(t *testing.T, c, member, admin client, path string, ready func(body []byte) bool) {
	t.Helper()
	direct := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(as client, target string, header http.Header) (reply, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, c.base+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		if as.token != "" {
			req.Header.Set("Authorization", "Bearer "+as.token)
		}
		for k, v := range header {
			req.Header[k] = v
		}
		resp, err := direct.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return reply{StatusCode: resp.StatusCode, Header: resp.Header}, body
	}
	resp, body := get(c, "/_ops/", nil)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") || resp.Header.Get("Content-Security-Policy") == "" || !bytes.Contains(body, []byte(`href="/_ops/jaeger/"`)) {
		t.Fatalf("GET /_ops/ = %d %v\n%s", resp.StatusCode, resp.Header, body)
	}
	if resp, body := get(c, path, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous GET %s = %d %s, want 401", path, resp.StatusCode, body)
	}
	if resp, _ := get(c, "/_ops/jaeger/", http.Header{"Accept": {"text/html,application/xhtml+xml"}}); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/_ops/" {
		t.Fatalf("anonymous browser on /_ops/jaeger/ = %d %q, want a redirect to /_ops/", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, body := get(member, path, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET %s as a member = %d %s, want 403", path, resp.StatusCode, body)
	}
	opsWait(t, c, admin, path, ready)
}

// opsWait polls path as admin until the panel answers what ready accepts:
// collectors and scrapers take a few rounds to see the first requests.
func opsWait(t *testing.T, c, admin client, path string, ready func(body []byte) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+admin.token)
		status, body := 0, []byte(nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			body, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			status = resp.StatusCode
		}
		if status == http.StatusOK && ready(body) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s as an admin = %d %s", path, status, body)
		}
		time.Sleep(2 * time.Second)
	}
}

func contains(s string) func([]byte) bool {
	return func(body []byte) bool { return bytes.Contains(body, []byte(s)) }
}
