package spec

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/spec-reference.md")

func TestReferenceMatchesDocs(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "spec-reference.md")
	got := Reference()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run go test ./internal/spec -update)", path, err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Fatalf("%s is out of date; run go test ./internal/spec -update", path)
	}
}

func TestReferenceDocumentsEveryMarker(t *testing.T) {
	ref := Reference()
	for _, m := range Markers() {
		if !strings.Contains(ref, "### `"+m.Name+"`") {
			t.Errorf("marker %s is not documented", m.Name)
		}
		if m.Example == "" || m.CommonError == "" || m.Description == "" {
			t.Errorf("marker %s lacks documentation fields", m.Name)
		}
	}
}

func TestExampleSpecParses(t *testing.T) {
	_, diags := parseFiles(map[string]string{
		"specs/users.go": exampleSpec,
		"specs/orgs.go":  header + "import \"github.com/google/uuid\"\n\ntype Org struct {\n\tID uuid.UUID `gen:\"pk,auto\"`\n}\n",
	})
	if len(diags) > 0 {
		t.Fatalf("example spec has diagnostics:\n%s", diags.Error())
	}
}
