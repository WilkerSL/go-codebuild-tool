package gen

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

func saasOptions(t *testing.T, version, dialectName string) Options {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	sources, err := spec.LoadDir(root, "specs/saas")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	target, err := goversion.Resolve(version)
	if err != nil {
		t.Fatal(err)
	}
	d, err := dialect.Get(dialectName)
	if err != nil {
		t.Fatal(err)
	}
	return Options{
		Module:        "example.com/saas",
		GoVersion:     version,
		Caps:          target.Caps,
		Dialect:       d,
		Model:         model,
		Pagination:    Pagination{DefaultLimit: 20, MaxLimit: 100},
		HasMigrations: true,
	}
}

func TestGenerateAllCombinations(t *testing.T) {
	for _, version := range []string{"1.26.0", "1.27.0"} {
		for _, name := range dialect.Names() {
			t.Run(version+"/"+name, func(t *testing.T) {
				files, err := Generate(saasOptions(t, version, name))
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range files {
					isGenerated := strings.HasSuffix(f.Path, "_gen.go") || strings.HasSuffix(f.Path, "_gen_test.go")
					if isGenerated != (f.Owner == OwnerGenerated) {
						t.Errorf("%s has owner %s", f.Path, f.Owner)
					}
					if isGenerated && !bytes.HasPrefix(f.Content, []byte(Header+"\n")) {
						t.Errorf("%s lacks the generated header", f.Path)
					}
					if strings.HasSuffix(f.Path, ".go") && bytes.Contains(f.Content, []byte("\n//")) && !bytes.Contains(f.Content, []byte("//go:")) {
						t.Errorf("%s contains a comment", f.Path)
					}
				}
			})
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, err := Generate(saasOptions(t, "1.27.0", "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(saasOptions(t, "1.27.0", "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("file count differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
			t.Fatalf("output differs at %s", a[i].Path)
		}
	}
}

func TestFormatGoPrunesImports(t *testing.T) {
	src := []byte("package x\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n\t\"github.com/google/uuid\"\n\t_ \"embed\"\n)\n\nfunc F() string { return strings.ToUpper(\"a\") }\n")
	out, err := FormatGo(src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if strings.Contains(got, "\"fmt\"") || strings.Contains(got, "uuid") || !strings.Contains(got, "\"strings\"") || !strings.Contains(got, "_ \"embed\"") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}
