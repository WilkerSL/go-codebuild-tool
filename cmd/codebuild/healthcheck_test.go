package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGeneratedPostgresHealthChecksUseTCP(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "golden")
	found := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(d.Name(), ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range strings.Lines(string(data)) {
			if strings.Contains(line, "pg_isready") {
				found++
				if !strings.Contains(line, "-h 127.0.0.1") {
					t.Errorf("%s: %s", path, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Fatal("no golden file has a pg_isready health check")
	}
}

// The postgres image runs its init scripts on a server that only opens the
// socket. A slow script holds that server up, so a health check that passes
// during it lets the migrations start and fail with connection refused.
func TestEndToEndPostgresHealthCheckWaitsForTCP(t *testing.T) {
	if os.Getenv("CODEBUILD_E2E_DOCKER") != "1" {
		t.Skip("set CODEBUILD_E2E_DOCKER=1 to run the generated health check against the postgres image")
	}
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "golden", "shop", "go1.27", "postgres", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`"CMD-SHELL", "(pg_isready[^"]*)"`).FindSubmatch(data)
	if match == nil {
		t.Fatal("the generated docker-compose.yml has no pg_isready health check")
	}
	health := string(match[1])
	name := "codebuild-e2e-health"
	run("", nil, "", "docker", "rm", "-f", name)
	t.Cleanup(func() { run("", nil, "", "docker", "rm", "-f", name) })
	command(t, "", nil, "docker", "run", "-d", "--name", name,
		"-e", "POSTGRES_USER=app", "-e", "POSTGRES_PASSWORD=app", "-e", "POSTGRES_DB=app",
		"--entrypoint", "sh", "postgres:17",
		"-c", "echo 'sleep 8' > /docker-entrypoint-initdb.d/slow.sh; exec docker-entrypoint.sh postgres")
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, err := run("", nil, "", "docker", "exec", name, "sh", "-c", health); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%q never passed", health)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if out, err := run("", nil, "", "docker", "exec", name, "bash", "-c", "exec 3<>/dev/tcp/127.0.0.1/5432"); err != nil {
		t.Fatalf("%q passed while port 5432 was closed: %v\n%s", health, err, out)
	}
}
