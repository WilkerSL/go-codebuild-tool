package spec

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

const header = "//go:build ignore\n\npackage specs\n\n"

func repoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func parseFiles(files map[string]string) (*schema.Model, schema.Diagnostics) {
	var sources []Source
	for name, content := range files {
		sources = append(sources, Source{Name: name, Content: []byte(content)})
	}
	return Parse(sources)
}

func TestParseSaaSExample(t *testing.T) {
	sources, err := LoadDir(filepath.Join(repoRoot(t), "testdata"), "specs/saas")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := Parse(sources)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	if got, want := model.EntityNames(), []string{"Audit", "Country", "Org", "Project", "Tag", "User"}; !slices.Equal(got, want) {
		t.Fatalf("entities = %v, want %v", got, want)
	}
	user := model.Entity("User")
	if user.Table != "users" || !user.SoftDelete || user.Domain != "users" || user.File != "specs/saas/users.go" {
		t.Fatalf("unexpected user entity: %+v", user)
	}
	id := user.Field("ID")
	if !id.PK || !id.AutoPK || id.Kind != schema.KindUUID || id.InCreate() || id.InUpdate() {
		t.Fatalf("unexpected ID field: %+v", id)
	}
	email := user.Field("Email")
	if !email.Required || !email.Unique || !email.Email || email.Max != "254" || email.Column != "email" || email.JSON != "email" {
		t.Fatalf("unexpected Email field: %+v", email)
	}
	role := user.Field("Role")
	if !slices.Equal(role.Enum, []string{"admin", "operator", "viewer"}) || !role.HasDefault || role.Default != "viewer" {
		t.Fatalf("unexpected Role field: %+v", role)
	}
	bio := user.Field("Bio")
	if !bio.Nullable || bio.Required {
		t.Fatalf("unexpected Bio field: %+v", bio)
	}
	org := user.Field("OrgID")
	if org.Ref == nil || org.Ref.Entity != "Org" || org.Ref.OnDelete != schema.OnDeleteRestrict || !org.Required || org.JSON != "orgId" || org.Column != "org_id" {
		t.Fatalf("unexpected OrgID field: %+v", org)
	}
	if !user.Field("CreatedAt").AutoCreate || !user.Field("UpdatedAt").AutoUpdate {
		t.Fatal("auto timestamps not detected")
	}
	if len(user.Relations) != 1 || user.Relations[0].Kind != schema.RelationManyToMany || user.Relations[0].Via != "user_tags" {
		t.Fatalf("unexpected user relations: %+v", user.Relations)
	}
	orgEntity := model.Entity("Org")
	if len(orgEntity.Relations) != 2 || orgEntity.Relations[0].FK != "OrgID" || orgEntity.Relations[1].FK != "OrgID" {
		t.Fatalf("has_many fk not resolved: %+v %+v", orgEntity.Relations[0], orgEntity.Relations[1])
	}
	project := model.Entity("Project")
	owner := project.Field("OwnerID")
	if owner.Ref.OnDelete != schema.OnDeleteSetNull || !owner.Nullable || owner.Required {
		t.Fatalf("unexpected OwnerID: %+v", owner)
	}
	if p := project.Field("Priority"); p.Default != "3" || p.Min != "1" || p.Max != "5" || p.Kind != schema.KindInt32 {
		t.Fatalf("unexpected Priority: %+v", p)
	}
	if project.Field("Logo").Kind != schema.KindBytes || project.Field("DueAt").Kind != schema.KindTime {
		t.Fatal("unexpected kinds for Logo or DueAt")
	}
	country := model.Entity("Country")
	if !country.ReadOnly || country.Table != "countries" || !country.Field("Code").Required || country.Field("Code").AutoPK {
		t.Fatalf("unexpected Country: %+v", country)
	}
	audit := model.Entity("Audit")
	if !audit.NoRoutes || audit.Table != "audit_log" || !audit.Field("Payload").OmitResponse {
		t.Fatalf("unexpected Audit: %+v", audit)
	}
	tag := model.Entity("Tag")
	if tag.Field("ID").Kind != schema.KindInt64 || !tag.Field("ID").AutoPK {
		t.Fatalf("unexpected Tag ID: %+v", tag.Field("ID"))
	}
}

func TestParseAcceptsAliasesAndBothUUIDImports(t *testing.T) {
	model, diags := parseFiles(map[string]string{
		"specs/orgs.go":  header + "import \"uuid\"\n\ntype Name = string\n\ntype Org struct {\n\tID   uuid.UUID `gen:\"pk,auto\"`\n\tName Name      `gen:\"required\"`\n\tNote *Name\n}\n",
		"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype ID = uuid.UUID\n\ntype User struct {\n\tID    ID `gen:\"pk,auto\"`\n\tOrgID ID `gen:\"rel=belongs_to:Org\"`\n}\n",
	})
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	org := model.Entity("Org")
	if org.Field("Name").Kind != schema.KindString || !org.Field("Note").Nullable || org.UUIDImport != "uuid" {
		t.Fatalf("aliases not resolved: %+v", org.Fields)
	}
	user := model.Entity("User")
	if user.Field("ID").Kind != schema.KindUUID || user.UUIDImport != "github.com/google/uuid" {
		t.Fatalf("uuid alias not resolved: %+v", user.Field("ID"))
	}
}

type wantDiag struct {
	file       string
	at         string
	code       string
	suggestion string
	contains   string
	warning    bool
}

func TestParseDiagnostics(t *testing.T) {
	const orgSpec = header + "import \"github.com/google/uuid\"\n\ntype Org struct {\n\tID uuid.UUID `gen:\"pk,auto\"`\n}\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []wantDiag
	}{
		{
			name:  "unknown marker suggests closest",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"requird,max=10\"`\n}\n"},
			want:  []wantDiag{{at: "requird", code: CodeUnknownMarker, suggestion: "required"}},
		},
		{
			name:  "missing primary key suggests ID",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64\n\tName string\n}\n"},
			want:  []wantDiag{{at: "Item struct", code: CodeMissingPK, suggestion: "ID"}},
		},
		{
			name: "unknown relation target suggests entity",
			files: map[string]string{
				"specs/orgs.go":  orgSpec,
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID    uuid.UUID `gen:\"pk,auto\"`\n\tOrgID uuid.UUID `gen:\"rel=belongs_to:Orgs\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "rel=belongs_to:Orgs", code: CodeUnknownEntity, suggestion: "Org"}},
		},
		{
			name:  "required on pointer",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID  int64   `gen:\"pk,auto\"`\n\tBio *string `gen:\"required\"`\n}\n"},
			want:  []wantDiag{{at: "required", code: CodeInvalidCombination, contains: "remove the *"}},
		},
		{
			name:  "auto on regular field suggests auto=create",
			files: map[string]string{"specs/items.go": header + "import \"time\"\n\ntype Item struct {\n\tID int64     `gen:\"pk,auto\"`\n\tAt time.Time `gen:\"auto\"`\n}\n"},
			want:  []wantDiag{{at: "auto\"`\n}", code: CodeInvalidCombination, suggestion: "auto=create"}},
		},
		{
			name:  "missing build tag",
			files: map[string]string{"specs/items.go": "package specs\n\ntype Item struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "package", code: CodeBuildTag}},
		},
		{
			name:  "constraint read by go mod tidy",
			files: map[string]string{"specs/items.go": "//go:build spec\n\npackage specs\n\ntype Item struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "//go:build spec", code: CodeBuildTag, suggestion: "//go:build ignore", contains: "go mod tidy"}},
		},
		{
			name:  "unsupported import",
			files: map[string]string{"specs/items.go": header + "import \"encoding/json\"\n\ntype Item struct {\n\tID  int64 `gen:\"pk,auto\"`\n\tRaw json.RawMessage\n}\n"},
			want: []wantDiag{
				{at: "\"encoding/json\"", code: CodeUnsupportedImport},
				{at: "json.RawMessage", code: CodeUnsupportedType},
			},
		},
		{
			name:  "unsupported type",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64 `gen:\"pk,auto\"`\n\tCount uint\n}\n"},
			want:  []wantDiag{{at: "uint", code: CodeUnsupportedType, contains: "unsupported type uint"}},
		},
		{
			name:  "entity marker on field",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"table=x\"`\n}\n"},
			want:  []wantDiag{{at: "table=x", code: CodeMarkerScope}},
		},
		{
			name:  "duplicate marker",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"max=5,max=6\"`\n}\n"},
			want:  []wantDiag{{at: "max=6", code: CodeDuplicateMarker}},
		},
		{
			name:  "default outside enum",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tRole string `gen:\"enum=admin|viewer,default=veiwer\"`\n}\n"},
			want:  []wantDiag{{at: "default=veiwer", code: CodeInvalidValue, suggestion: "viewer"}},
		},
		{
			name:  "min greater than max",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"min=10,max=5\"`\n}\n"},
			want:  []wantDiag{{at: "min=10", code: CodeInvalidValue, contains: "cannot be greater"}},
		},
		{
			name: "has_many without back reference",
			files: map[string]string{
				"specs/orgs.go":  header + "import \"github.com/google/uuid\"\n\ntype Org struct {\n\tID    uuid.UUID `gen:\"pk,auto\"`\n\tUsers []User   `gen:\"rel=has_many:User\"`\n}\n",
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID uuid.UUID `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/orgs.go", at: "rel=has_many:User", code: CodeRelationTypeMismatch, contains: "has no field with rel=belongs_to:Org"}},
		},
		{
			name: "ambiguous has_many",
			files: map[string]string{
				"specs/users.go":    header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID       uuid.UUID `gen:\"pk,auto\"`\n\tProjects []Project `gen:\"rel=has_many:Project\"`\n}\n",
				"specs/projects.go": header + "import \"github.com/google/uuid\"\n\ntype Project struct {\n\tID         uuid.UUID  `gen:\"pk,auto\"`\n\tOwnerID    uuid.UUID  `gen:\"rel=belongs_to:User\"`\n\tReviewerID *uuid.UUID `gen:\"rel=belongs_to:User\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "rel=has_many:Project", code: CodeRelationAmbiguous, suggestion: "fk=OwnerID"}},
		},
		{
			name: "has_many with unknown fk",
			files: map[string]string{
				"specs/users.go":    header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID       uuid.UUID `gen:\"pk,auto\"`\n\tProjects []Project `gen:\"rel=has_many:Project,fk=OwnerId\"`\n}\n",
				"specs/projects.go": header + "import \"github.com/google/uuid\"\n\ntype Project struct {\n\tID      uuid.UUID `gen:\"pk,auto\"`\n\tOwnerID uuid.UUID `gen:\"rel=belongs_to:User\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "fk=OwnerId", code: CodeUnknownField, suggestion: "OwnerID"}},
		},
		{
			name: "many_to_many without via",
			files: map[string]string{
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID   uuid.UUID `gen:\"pk,auto\"`\n\tTags []Tag     `gen:\"rel=many_to_many:Tag\"`\n}\n",
				"specs/tags.go":  header + "type Tag struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "rel=many_to_many:Tag", code: CodeInvalidValue, contains: "requires via"}},
		},
		{
			name: "many_to_many sides disagree on via",
			files: map[string]string{
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID   uuid.UUID `gen:\"pk,auto\"`\n\tTags []Tag     `gen:\"rel=many_to_many:Tag,via=user_tags\"`\n}\n",
				"specs/tags.go":  header + "type Tag struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tUsers []User `gen:\"rel=many_to_many:User,via=tag_users\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "via=user_tags", code: CodeInvalidCombination, suggestion: "tag_users"}},
		},
		{
			name: "non-nullable reference cycle",
			files: map[string]string{
				"specs/as.go": header + "type A struct {\n\tID  int64 `gen:\"pk,auto\"`\n\tBID int64 `gen:\"rel=belongs_to:B\"`\n}\n",
				"specs/bs.go": header + "type B struct {\n\tID  int64 `gen:\"pk,auto\"`\n\tAID int64 `gen:\"rel=belongs_to:A\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/bs.go", at: "rel=belongs_to:A", code: CodeRelationCycle, contains: "A -> B -> A"}},
		},
		{
			name: "reference type mismatch",
			files: map[string]string{
				"specs/orgs.go":  orgSpec,
				"specs/users.go": header + "type User struct {\n\tID    int64 `gen:\"pk,auto\"`\n\tOrgID int64 `gen:\"rel=belongs_to:Org\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "rel=belongs_to:Org", code: CodeRelationTypeMismatch, contains: "uuid.UUID"}},
		},
		{
			name:  "two entities in one file",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n\ntype Other struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "Other struct", code: CodeEntityCount, contains: "specs/others.go"}},
		},
		{
			name:  "json tag key",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `json:\"name\"`\n}\n"},
			want:  []wantDiag{{at: "json:", code: CodeUnsupportedTagKey, suggestion: "gen"}},
		},
		{
			name:  "invalid domain name",
			files: map[string]string{"specs/order_items.go": header + "type OrderItem struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "//go:build", code: CodeDomainName, suggestion: "orderitems"}},
		},
		{
			name:  "unique on bool",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID     int64 `gen:\"pk,auto\"`\n\tActive bool  `gen:\"unique\"`\n}\n"},
			want:  []wantDiag{{at: "unique", code: CodeInvalidCombination}},
		},
		{
			name: "setnull on non-pointer",
			files: map[string]string{
				"specs/orgs.go":  orgSpec,
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\tID    uuid.UUID `gen:\"pk,auto\"`\n\tOrgID uuid.UUID `gen:\"rel=belongs_to:Org,ondelete=setnull\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "ondelete=setnull", code: CodeInvalidCombination, contains: "*uuid.UUID"}},
		},
		{
			name:  "syntax error",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID int64 `gen:\"pk,auto\"`\n"},
			want:  []wantDiag{{code: CodeSyntax}},
		},
		{
			name:  "non-struct type",
			files: map[string]string{"specs/items.go": header + "type Role string\n\ntype Item struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "Role string", code: CodeUnsupportedDecl, contains: "type Role = string"}},
		},
		{
			name:  "unexported field",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64 `gen:\"pk,auto\"`\n\tname string\n}\n"},
			want:  []wantDiag{{at: "name string", code: CodeUnexportedField, suggestion: "Name"}},
		},
		{
			name:  "duplicate column",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tName  string `gen:\"column=title\"`\n\tTitle string\n}\n"},
			want:  []wantDiag{{at: "Title string", code: CodeDuplicateColumn}},
		},
		{
			name:  "unknown omit target",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"omit=respones\"`\n}\n"},
			want:  []wantDiag{{at: "omit=respones", code: CodeInvalidValue, suggestion: "response"}},
		},
		{
			name:  "required with default",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tName string `gen:\"required,default=x\"`\n}\n"},
			want:  []wantDiag{{at: "default=x", code: CodeInvalidCombination}},
		},
		{
			name:  "softdelete with explicit deleted_at",
			files: map[string]string{"specs/items.go": header + "import \"time\"\n\ntype Item struct {\n\t_         struct{} `gen:\"softdelete\"`\n\tID        int64    `gen:\"pk,auto\"`\n\tDeletedAt *time.Time\n}\n"},
			want:  []wantDiag{{at: "DeletedAt", code: CodeDuplicateColumn}},
		},
		{
			name: "relation route warning for noroutes target",
			files: map[string]string{
				"specs/orgs.go":  header + "import \"github.com/google/uuid\"\n\ntype Org struct {\n\tID    uuid.UUID `gen:\"pk,auto\"`\n\tUsers []User   `gen:\"rel=has_many:User\"`\n}\n",
				"specs/users.go": header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\t_     struct{}  `gen:\"noroutes\"`\n\tID    uuid.UUID `gen:\"pk,auto\"`\n\tOrgID uuid.UUID `gen:\"rel=belongs_to:Org\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/orgs.go", at: "rel=has_many:User", code: CodeRelationRoutes, warning: true}},
		},
		{
			name:  "reserved domain",
			files: map[string]string{"specs/http.go": header + "type Item struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "//go:build", code: CodeDomainName, contains: "reserved"}},
		},
		{
			name:  "string pk cannot be auto",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tCode string `gen:\"pk,auto\"`\n}\n"},
			want:  []wantDiag{{at: "auto\"`", code: CodeInvalidCombination, contains: "sent by the client"}},
		},
		{
			name:  "decimal needs a string",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64   `gen:\"pk,auto\"`\n\tPrice float64 `gen:\"decimal=12:2\"`\n}\n"},
			want:  []wantDiag{{at: "decimal=12:2", code: CodeInvalidCombination, contains: "only valid on string"}},
		},
		{
			name:  "decimal precision out of range",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tPrice string `gen:\"decimal=40:2\"`\n}\n"},
			want:  []wantDiag{{at: "decimal=40:2", code: CodeInvalidValue, contains: "between 1 and 38"}},
		},
		{
			name:  "decimal scale above precision",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tPrice string `gen:\"decimal=4:6\"`\n}\n"},
			want:  []wantDiag{{at: "decimal=4:6", code: CodeInvalidValue, contains: "between 0 and the precision"}},
		},
		{
			name:  "decimal with enum",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tPrice string `gen:\"decimal=8:2,enum=a|b\"`\n}\n"},
			want:  []wantDiag{{at: "enum=a|b", code: CodeInvalidCombination, contains: "holds a number"}},
		},
		{
			name:  "decimal bound outside the digits",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tPrice string `gen:\"decimal=4:2,max=100\"`\n}\n"},
			want:  []wantDiag{{at: "max=100", code: CodeInvalidValue, contains: "decimal=4:2"}},
		},
		{
			name:  "decimal default that is not a number",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tPrice string `gen:\"decimal=8:2,default=free\"`\n}\n"},
			want:  []wantDiag{{at: "default=free", code: CodeInvalidValue, contains: "not a decimal number"}},
		},
		{
			name:  "empty marker",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID int64 `gen:\"pk,auto,\"`\n}\n"},
			want:  []wantDiag{{at: "\"`\n}", code: CodeInvalidValue, contains: "empty marker"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := parseFiles(tt.files)
			for _, want := range tt.want {
				file := want.file
				if file == "" {
					for name := range tt.files {
						file = name
					}
				}
				if !containsDiag(diags, tt.files[file], file, want) {
					t.Errorf("missing diagnostic %+v in:\n%s", want, diags.Error())
				}
			}
			if !diags.HasErrors() && !slices.ContainsFunc(tt.want, func(w wantDiag) bool { return w.warning }) {
				t.Errorf("expected errors, got none")
			}
		})
	}
}

func containsDiag(diags schema.Diagnostics, content, file string, want wantDiag) bool {
	line, col := 0, 0
	if want.at != "" {
		idx := strings.Index(content, want.at)
		if idx < 0 {
			return false
		}
		line = strings.Count(content[:idx], "\n") + 1
		col = idx - strings.LastIndex(content[:idx], "\n")
	}
	for _, d := range diags {
		if d.Code != want.code || d.File != file {
			continue
		}
		if want.warning != (d.Severity == schema.SeverityWarning) {
			continue
		}
		if line != 0 && (d.Line != line || d.Column != col) {
			continue
		}
		if want.suggestion != "" && d.Suggestion != want.suggestion {
			continue
		}
		if want.contains != "" && !strings.Contains(d.Message, want.contains) {
			continue
		}
		return true
	}
	return false
}

func TestDiagnosticString(t *testing.T) {
	d := schema.Diagnostic{
		Pos:        schema.Pos{File: "specs/users.go", Line: 7, Column: 31},
		Severity:   schema.SeverityError,
		Code:       CodeUnknownMarker,
		Message:    `unknown marker "requird"`,
		Suggestion: "required",
	}
	want := `specs/users.go:7:31: unknown marker "requird" (did you mean "required"?) [unknown_marker]`
	if got := d.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseIsDeterministic(t *testing.T) {
	sources, err := LoadDir(filepath.Join(repoRoot(t), "testdata"), "specs/saas")
	if err != nil {
		t.Fatal(err)
	}
	reversed := slices.Clone(sources)
	slices.Reverse(reversed)
	a, _ := Parse(sources)
	b, _ := Parse(reversed)
	if !slices.Equal(a.EntityNames(), b.EntityNames()) {
		t.Fatalf("entity order differs: %v vs %v", a.EntityNames(), b.EntityNames())
	}
}

func TestParseDecimal(t *testing.T) {
	src := header + "type Item struct {\n\tID       int64   `gen:\"pk,auto\"`\n\tPrice    string  `gen:\"decimal=12:2,min=0,max=+1000.5,default=7.1,unique\"`\n\tQuantity *string `gen:\"decimal=9,index\"`\n}\n"
	m, diags := parseFiles(map[string]string{"specs/items.go": src})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	e := m.Entity("Item")
	price, quantity := e.Field("Price"), e.Field("Quantity")
	if price.Precision != 12 || price.Scale != 2 || price.Min != "0.00" || price.Max != "1000.50" || price.Default != "7.10" {
		t.Errorf("price = %+v", price)
	}
	if quantity.Precision != 9 || quantity.Scale != 0 || quantity.Filterable() {
		t.Errorf("quantity = %+v, filterable %v", quantity, quantity.Filterable())
	}
	c := schema.FieldColumn(price)
	if c.Kind != schema.ColumnDecimal || c.Size != 12 || c.Scale != 2 {
		t.Errorf("column = %+v", c)
	}
}
