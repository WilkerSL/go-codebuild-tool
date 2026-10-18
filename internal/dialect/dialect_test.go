package dialect

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func loadSaaS(t *testing.T) []spec.Source {
	t.Helper()
	sources, err := spec.LoadDir(filepath.Join(repoRoot(t), "testdata"), "specs/saas")
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func parse(t *testing.T, sources []spec.Source) *schema.Model {
	t.Helper()
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatalf("spec errors:\n%s", diags.Error())
	}
	return model
}

func golden(t *testing.T, path string, got string) {
	t.Helper()
	full := filepath.Join(repoRoot(t), "testdata", "golden", "sql", path)
	if *update {
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read golden %s: %v (run go test ./internal/dialect -update)", path, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden file; run go test ./internal/dialect -update and review the diff\n--- got ---\n%s", path, got)
	}
}

func replaceSource(sources []spec.Source, name, content string) []spec.Source {
	out := make([]spec.Source, 0, len(sources)+1)
	replaced := false
	for _, s := range sources {
		if s.Name == name {
			replaced = true
			if content == "" {
				continue
			}
			s = spec.Source{Name: name, Content: []byte(content)}
		}
		out = append(out, s)
	}
	if !replaced && content != "" {
		out = append(out, spec.Source{Name: name, Content: []byte(content)})
	}
	return out
}

func changedSaaS(t *testing.T) []spec.Source {
	sources := loadSaaS(t)
	sources = replaceSource(sources, "specs/saas/users.go", `//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_         struct{}  `+"`gen:\"table=users,softdelete\"`"+`
	ID        uuid.UUID `+"`gen:\"pk,auto\"`"+`
	Name      string    `+"`gen:\"required,notempty,max=200\"`"+`
	Email     string    `+"`gen:\"required,unique,email,max=254\"`"+`
	Role      string    `+"`gen:\"enum=owner|admin|operator|viewer,default=viewer\"`"+`
	Phone     *string   `+"`gen:\"max=20,index\"`"+`
	OrgID     uuid.UUID `+"`gen:\"rel=belongs_to:Org\"`"+`
	CreatedAt time.Time `+"`gen:\"auto=create\"`"+`
	UpdatedAt time.Time `+"`gen:\"auto=update\"`"+`
	Tags      []Tag     `+"`gen:\"rel=many_to_many:Tag,via=user_tags\"`"+`
}
`)
	sources = replaceSource(sources, "specs/saas/tags.go", `//go:build spec

package specs

type Tag struct {
	_     struct{} `+"`gen:\"softdelete\"`"+`
	ID    int64    `+"`gen:\"pk,auto\"`"+`
	Name  string   `+"`gen:\"required,notempty,unique,max=50\"`"+`
	Color *string  `+"`gen:\"min=4,max=7\"`"+`
	Users []User   `+"`gen:\"rel=many_to_many:User,via=user_tags\"`"+`
}
`)
	sources = replaceSource(sources, "specs/saas/countries.go", "")
	sources = replaceSource(sources, "specs/saas/comments.go", `//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID        uuid.UUID `+"`gen:\"pk,auto\"`"+`
	ProjectID uuid.UUID `+"`gen:\"rel=belongs_to:Project,ondelete=cascade\"`"+`
	Body      string    `+"`gen:\"required,notempty,max=2000\"`"+`
	CreatedAt time.Time `+"`gen:\"auto=create\"`"+`
}
`)
	return sources
}

func renderQuery(b *strings.Builder, name string, q Query) {
	fmt.Fprintf(b, "-- %s args:", name)
	for _, a := range q.Args {
		if a.Field != nil {
			fmt.Fprintf(b, " %s(%s)", a.Kind, a.Field.Name)
		} else {
			fmt.Fprintf(b, " %s", a.Kind)
		}
	}
	b.WriteString("\n" + q.SQL + ";\n\n")
}

func TestGoldenSQL(t *testing.T) {
	model := parse(t, loadSaaS(t))
	changed := parse(t, changedSaaS(t))
	for _, name := range Names() {
		d, _ := Get(name)
		t.Run(name, func(t *testing.T) {
			db := schema.Build(model)
			initial := schema.Diff(nil, db, schema.DiffOptions{RecreateDependents: d.RecreateDependents()})
			golden(t, name+"/0001_init.up.sql", d.Migration(initial.Changes, nil, db))
			golden(t, name+"/0001_init.down.sql", d.Migration(initial.Down(), db, nil))

			next := schema.Build(changed)
			delta := schema.Diff(db, next, schema.DiffOptions{RecreateDependents: d.RecreateDependents()})
			golden(t, name+"/0002_update_schema.up.sql", d.Migration(delta.Changes, db, next))
			golden(t, name+"/0002_update_schema.down.sql", d.Migration(delta.Down(), next, db))
			golden(t, name+"/0002_update_schema.warnings.txt", strings.Join(delta.Warnings, "\n")+"\n")

			for _, e := range model.Entities {
				var b strings.Builder
				q := BuildEntityQueries(d, e)
				renderQuery(&b, "get", q.Get)
				renderQuery(&b, "exists", q.Exists)
				renderQuery(&b, "list", q.List)
				renderQuery(&b, "count", q.Count)
				if q.Create != nil {
					renderQuery(&b, "create", *q.Create)
				}
				if q.Update != nil {
					renderQuery(&b, "update", *q.Update)
				}
				if q.Delete != nil {
					renderQuery(&b, "delete", *q.Delete)
				}
				for _, parent := range model.Entities {
					for _, r := range parent.Relations {
						if r.Target != e.Name {
							continue
						}
						switch r.Kind {
						case schema.RelationHasMany:
							rq := BuildHasManyQueries(d, parent, e, e.Field(r.FK))
							renderQuery(&b, "exists "+parent.Name, rq.SourceExists)
							renderQuery(&b, "list "+parent.Name+"."+r.Name, rq.List)
							renderQuery(&b, "count "+parent.Name+"."+r.Name, rq.Count)
						case schema.RelationManyToMany:
							rq := BuildManyToManyQueries(d, parent, e, r.Via, !e.ReadOnly)
							renderQuery(&b, "exists "+parent.Name, rq.SourceExists)
							renderQuery(&b, "list "+parent.Name+"."+r.Name, rq.List)
							renderQuery(&b, "count "+parent.Name+"."+r.Name, rq.Count)
							renderQuery(&b, "link "+parent.Name+"."+r.Name, *rq.Link)
							renderQuery(&b, "unlink "+parent.Name+"."+r.Name, *rq.Unlink)
						}
					}
				}
				golden(t, name+"/queries_"+e.Domain+".sql", b.String())
			}
		})
	}
}

func TestPlaceholderReuse(t *testing.T) {
	model := parse(t, loadSaaS(t))
	user := model.Entity("User")
	pg := BuildEntityQueries(Postgres{}, user)
	if !strings.Contains(pg.List.SQL, `($1::text IS NULL OR "email" = $1)`) {
		t.Errorf("postgres list does not reuse placeholders:\n%s", pg.List.SQL)
	}
	my := BuildEntityQueries(MySQL{}, user)
	if !strings.Contains(my.List.SQL, "(? IS NULL OR `email` = ?)") {
		t.Errorf("mysql list does not repeat placeholders:\n%s", my.List.SQL)
	}
	if got, want := len(my.List.Args), 2*len(user.FilterFields())+2; got != want {
		t.Errorf("mysql list args = %d, want %d", got, want)
	}
	if got, want := len(pg.List.Args), len(user.FilterFields())+2; got != want {
		t.Errorf("postgres list args = %d, want %d", got, want)
	}
	ms := BuildEntityQueries(SQLServer{}, user)
	if !strings.Contains(ms.List.SQL, "OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY\nOPTION (RECOMPILE)") || ms.List.Args[2].Kind != ArgOffset {
		t.Errorf("sqlserver pagination is wrong:\n%s %+v", ms.List.SQL, ms.List.Args)
	}
}

func TestConstraintName(t *testing.T) {
	if got := schema.ConstraintName("ux", "users", "email"); got != "ux_users_email" {
		t.Errorf("ConstraintName = %s", got)
	}
	long := schema.ConstraintName("fk", strings.Repeat("a", 40), strings.Repeat("b", 40))
	if len(long) != schema.MaxIdentifierLength {
		t.Errorf("long name has length %d: %s", len(long), long)
	}
	if long != schema.ConstraintName("fk", strings.Repeat("a", 40), strings.Repeat("b", 40)) {
		t.Error("ConstraintName is not deterministic")
	}
}

func TestValidate(t *testing.T) {
	const header = "//go:build spec\n\npackage specs\n\n"
	model, diags := spec.Parse([]spec.Source{
		{Name: "specs/orgs.go", Content: []byte(header + "type Org struct {\n\t_    struct{} `gen:\"softdelete\"`\n\tID   int64    `gen:\"pk,auto\"`\n\tSlug string   `gen:\"unique\"`\n}\n")},
		{Name: "specs/teams.go", Content: []byte(header + "type Team struct {\n\tID    int64 `gen:\"pk,auto\"`\n\tOrgID int64 `gen:\"rel=belongs_to:Org,ondelete=cascade\"`\n}\n")},
		{Name: "specs/members.go", Content: []byte(header + "type Member struct {\n\tID     int64 `gen:\"pk,auto\"`\n\tOrgID  int64 `gen:\"rel=belongs_to:Org,ondelete=cascade\"`\n\tTeamID int64 `gen:\"rel=belongs_to:Team,ondelete=cascade\"`\n}\n")},
	})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	ms := Validate(SQLServer{}, model)
	if !hasCode(ms, CodeDialectLimit, schema.SeverityError) || !hasCode(ms, CodeDialectCascade, schema.SeverityError) {
		t.Errorf("sqlserver diagnostics missing:\n%s", ms.Error())
	}
	my := Validate(MySQL{}, model)
	if !hasCode(my, CodeDialectLimit, schema.SeverityError) || !hasCode(my, CodeDialectLimit, schema.SeverityWarning) {
		t.Errorf("mysql diagnostics missing:\n%s", my.Error())
	}
	if pg := Validate(Postgres{}, model); len(pg) != 0 {
		t.Errorf("unexpected postgres diagnostics:\n%s", pg.Error())
	}
	if saas := Validate(SQLServer{}, parse(t, loadSaaS(t))); saas.HasErrors() {
		t.Errorf("saas example must be valid on sqlserver:\n%s", saas.Error())
	}
}

func hasCode(diags schema.Diagnostics, code string, severity schema.Severity) bool {
	for _, d := range diags {
		if d.Code == code && d.Severity == severity {
			return true
		}
	}
	return false
}

func TestMigrationName(t *testing.T) {
	model := parse(t, loadSaaS(t))
	db := schema.Build(model)
	if got := schema.MigrationName(nil, schema.Diff(nil, db, schema.DiffOptions{}).Changes); got != "init" {
		t.Errorf("initial migration name = %s", got)
	}
	next := schema.Build(parse(t, changedSaaS(t)))
	if got := schema.MigrationName(db, schema.Diff(db, next, schema.DiffOptions{}).Changes); got != "update_schema" {
		t.Errorf("update migration name = %s", got)
	}
	withComments := parse(t, replaceSource(loadSaaS(t), "specs/saas/comments.go", "//go:build spec\n\npackage specs\n\ntype Comment struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"))
	if got := schema.MigrationName(db, schema.Diff(db, schema.Build(withComments), schema.DiffOptions{}).Changes); got != "create_comments" {
		t.Errorf("create migration name = %s", got)
	}
	if !schema.Diff(db, schema.Build(model), schema.DiffOptions{}).Empty() {
		t.Error("diff of identical schemas must be empty")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	db := schema.Build(parse(t, loadSaaS(t)))
	data, err := schema.NewSnapshot("postgres", db).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := schema.UnmarshalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Diff(db, back.Database(), schema.DiffOptions{}).Empty() {
		t.Error("snapshot round trip changed the schema")
	}
	again, _ := back.Marshal()
	if string(again) != string(data) {
		t.Error("snapshot marshaling is not stable")
	}
	if _, err := schema.UnmarshalSnapshot([]byte(`{"version":99,"dialect":"postgres","tables":[]}`)); err == nil {
		t.Error("expected version error")
	}
}
