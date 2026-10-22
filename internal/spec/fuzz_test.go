package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, set := range []string{"saas", "secure", "shop"} {
		dir := filepath.Join(repoRoot(f), "testdata", "specs", set)
		entries, err := os.ReadDir(dir)
		if err != nil {
			f.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				f.Fatal(err)
			}
			f.Add(e.Name(), data)
		}
	}
	f.Add("broken.go", []byte(header+"type X struct {\n\tID int64 `gen:\"pk,auto,max=\"`\n"))
	f.Add("svc.go", []byte(header+"type SvcService struct {\n\t_ struct{} `gen:\"database=own\"`\n\tA struct{ ID int64 `gen:\"pk\"` }\n}\n"))
	f.Fuzz(func(t *testing.T, name string, content []byte) {
		name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
		if !strings.HasSuffix(name, ".go") || len(name) > 64 {
			name = "fuzz.go"
		}
		model, diags := Parse([]Source{{Name: "specs/" + name, Content: content}})
		if model == nil {
			t.Fatal("Parse returned a nil model")
		}
		for _, d := range diags {
			if d.Code == "" || d.Message == "" {
				t.Fatalf("diagnostic without code or message: %+v", d)
			}
		}
	})
}

func FuzzParseStructTag(f *testing.F) {
	for _, seed := range []string{
		`gen:"pk,auto"`,
		`gen:"rel=belongs_to:User,ondelete=cascade,owner" json:"x"`,
		`gen:"enum=a|b|c,default=a"`,
		`gen:"`,
		`gen:"\"quoted\""`,
		"  gen:\"max=10\"  ",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, tag string) {
		entries, _, err := parseStructTag(tag)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.offset < 0 || e.offset > len(tag) {
				t.Fatalf("offset %d outside the tag of length %d", e.offset, len(tag))
			}
			for _, m := range splitMarkers(e.value) {
				if m.offset < 0 || m.offset > len(e.value) {
					t.Fatalf("marker offset %d outside the value of length %d", m.offset, len(e.value))
				}
			}
		}
	})
}
