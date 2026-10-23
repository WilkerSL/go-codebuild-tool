package schema_test

import (
	"bytes"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

const header = "//go:build ignore\n\npackage specs\n\nimport (\n\t\"time\"\n\n\t\"github.com/google/uuid\"\n)\n\n"

var notesSpecs = map[string]string{
	"specs/users.go": header + "type User struct {\n" +
		"\t_            struct{}  `gen:\"auth=jwt|apikey,register,roles=admin,softdelete\"`\n" +
		"\tID           uuid.UUID `gen:\"pk,auto\"`\n" +
		"\tEmail        string    `gen:\"required,unique,email,max=254,identity\"`\n" +
		"\tPasswordHash string    `gen:\"password,min=10\"`\n" +
		"\tRole         string    `gen:\"enum=admin|member,default=member,role\"`\n" +
		"\tCreatedAt    time.Time `gen:\"auto=create\"`\n" +
		"}\n",
	"specs/notes.go": header + "type Note struct {\n" +
		"\tID        int64      `gen:\"pk,auto\"`\n" +
		"\tOwnerID   uuid.UUID  `gen:\"rel=belongs_to:User,ondelete=cascade,owner\"`\n" +
		"\tTitle     string     `gen:\"required,max=200,index\"`\n" +
		"\tBody      []byte\n" +
		"\tArchiveAt *time.Time\n" +
		"\tTags      []Tag      `gen:\"rel=many_to_many:Tag,via=note_tags\"`\n" +
		"\tUpdatedAt time.Time  `gen:\"auto=update\"`\n" +
		"}\n",
	"specs/tags.go": header + "type Tag struct {\n" +
		"\t_    struct{} `gen:\"public=read,writeroles=admin\"`\n" +
		"\tID   int64    `gen:\"pk,auto\"`\n" +
		"\tName string   `gen:\"required,unique,max=40\"`\n" +
		"}\n",
}

func parse(t *testing.T, files map[string]string) *schema.Model {
	t.Helper()
	var sources []spec.Source
	for name, content := range files {
		sources = append(sources, spec.Source{Name: name, Content: []byte(strings.Replace(content, "import (\n\t\"time\"\n\n\t\"github.com/google/uuid\"\n)\n\n", imports(content), 1))})
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatalf("parse:\n%s", diags.Error())
	}
	return model
}

func imports(content string) string {
	var used []string
	if strings.Contains(content, "time.") {
		used = append(used, "\t\"time\"\n")
	}
	if strings.Contains(content, "uuid.") {
		if len(used) > 0 {
			used = append(used, "\n")
		}
		used = append(used, "\t\"github.com/google/uuid\"\n")
	}
	if len(used) == 0 {
		return ""
	}
	return "import (\n" + strings.Join(used, "") + ")\n\n"
}

func tableNames(db *schema.Database) []string {
	names := make([]string, len(db.Tables))
	for i, t := range db.Tables {
		names[i] = t.Name
	}
	return names
}

func TestBuildTables(t *testing.T) {
	db := schema.Build(parse(t, notesSpecs))
	want := []string{schema.APIKeysTable, schema.RefreshTokensTable, "note_tags", "notes", "tags", "users"}
	if got := tableNames(db); !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	users := db.Table("users")
	if c := users.Column(schema.DeletedAtColumn); c == nil || c.Kind != schema.ColumnTime || !c.Nullable {
		t.Errorf("users.deleted_at = %+v", c)
	}
	if ix := users.Index("ux_users_email"); ix == nil || !ix.Unique || !ix.NotDeleted {
		t.Errorf("ux_users_email = %+v", ix)
	}
	if c := users.Column("password_hash"); c == nil || c.Size != schema.PasswordSize {
		t.Errorf("users.password_hash = %+v", c)
	}
	if c := users.Column("role"); c == nil || c.Size != 32 || c.Default == nil || *c.Default != (schema.Default{Kind: schema.DefaultLiteral, Value: "member"}) {
		t.Errorf("users.role = %+v", c)
	}
	if ck := users.Check("ck_users_role"); ck == nil || !slices.Equal(ck.Values, []string{"admin", "member"}) {
		t.Errorf("ck_users_role = %+v", ck)
	}
	if c := users.Column("created_at"); c == nil || c.Default == nil || c.Default.Kind != schema.DefaultNow {
		t.Errorf("users.created_at = %+v", c)
	}
	if c := users.Column("id"); c == nil || c.Kind != schema.ColumnUUID || c.Identity {
		t.Errorf("users.id = %+v", c)
	}
	notes := db.Table("notes")
	if c := notes.Column("id"); c == nil || !c.Identity {
		t.Errorf("notes.id = %+v", c)
	}
	if fk := notes.ForeignKey("fk_notes_owner_id"); fk == nil || fk.RefTable != "users" || fk.OnDelete != schema.OnDeleteCascade {
		t.Errorf("fk_notes_owner_id = %+v", fk)
	}
	if ix := notes.Index("ix_notes_title"); ix == nil || ix.Unique {
		t.Errorf("ix_notes_title = %+v", ix)
	}
	if c := notes.Column("body"); c == nil || c.Kind != schema.ColumnBytes || c.Size != 0 || c.Nullable {
		t.Errorf("notes.body = %+v", c)
	}
	if c := notes.Column("archive_at"); c == nil || !c.Nullable {
		t.Errorf("notes.archive_at = %+v", c)
	}
	if notes.Column(schema.DeletedAtColumn) != nil {
		t.Error("notes has deleted_at without softdelete")
	}
	join := db.Table("note_tags")
	if !slices.Equal(join.PrimaryKey.Columns, []string{"note_id", "tag_id"}) || len(join.ForeignKeys) != 2 {
		t.Fatalf("note_tags = %+v", join)
	}
	for _, fk := range join.ForeignKeys {
		if fk.OnDelete != schema.OnDeleteCascade {
			t.Errorf("%s on delete = %s", fk.Name, fk.OnDelete)
		}
	}
	if join.Index("ix_note_tags_tag_id") == nil {
		t.Error("note_tags has no index on tag_id")
	}
	refresh := db.Table(schema.RefreshTokensTable)
	if c := refresh.Column("user_id"); c == nil || c.Kind != schema.ColumnUUID {
		t.Errorf("auth_refresh_tokens.user_id = %+v", c)
	}
	if fk := refresh.ForeignKey("fk_auth_refresh_tokens_user_id"); fk == nil || fk.RefTable != "users" || fk.OnDelete != schema.OnDeleteCascade {
		t.Errorf("refresh foreign key = %+v", fk)
	}
	if db.Table(schema.SessionsTable) != nil {
		t.Error("sessions table without the session method")
	}
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestBuildDatabasesSplitsServices(t *testing.T) {
	sources, err := spec.LoadDir(filepath.Join(repoRoot(), "testdata"), "specs/shop")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	dbs := schema.BuildDatabases(model)
	tables := map[string][]string{}
	for name, db := range dbs {
		tables[name] = tableNames(db)
	}
	want := map[string][]string{
		"billing":  {"invoice_items", "invoices"},
		"identity": {schema.APIKeysTable, schema.SessionsTable, "users"},
		"shared":   {"categories", "products", "stocks"},
	}
	for name, names := range want {
		if !slices.Equal(tables[name], names) {
			t.Errorf("%s tables = %v, want %v", name, tables[name], names)
		}
	}
	if len(dbs) != len(want) {
		t.Errorf("databases = %v", tables)
	}
	billing := dbs["billing"]
	if fks := billing.Table("invoices").ForeignKeys; len(fks) != 0 {
		t.Errorf("invoices reference users in another database but have foreign keys %+v", fks)
	}
	if ix := billing.Table("invoices").Index("ix_invoices_owner_id"); ix == nil {
		t.Error("references to other databases keep their index")
	}
	items := billing.Table("invoice_items")
	if len(items.ForeignKeys) != 1 || items.ForeignKeys[0].RefTable != "invoices" {
		t.Errorf("invoice_items foreign keys = %+v", items.ForeignKeys)
	}
	if fk := dbs["shared"].Table("stocks").ForeignKey("fk_stocks_product_id"); fk == nil || fk.RefTable != "products" {
		t.Errorf("services sharing a database keep their foreign keys: %+v", fk)
	}
	monolith := schema.Build(model)
	if len(monolith.Tables) != 8 {
		t.Errorf("Build merges every database: %v", tableNames(monolith))
	}
}

func simpleTable(name string, columns ...string) *schema.Table {
	t := &schema.Table{
		Name:       name,
		Columns:    []*schema.Column{{Name: "id", Kind: schema.ColumnInt64, Identity: true}},
		PrimaryKey: &schema.PrimaryKey{Name: schema.ConstraintName("pk", name), Columns: []string{"id"}},
	}
	for _, c := range columns {
		t.Columns = append(t.Columns, &schema.Column{Name: c, Kind: schema.ColumnString, Nullable: true})
	}
	return t
}

func TestMigrationName(t *testing.T) {
	notes := simpleTable("notes")
	tests := []struct {
		name     string
		old, new []*schema.Table
		want     string
	}{
		{name: "first migration", old: nil, new: []*schema.Table{notes}, want: "init"},
		{name: "one new table", old: []*schema.Table{notes}, new: []*schema.Table{notes, simpleTable("tags")}, want: "create_tags"},
		{name: "two new tables", old: []*schema.Table{notes}, new: []*schema.Table{notes, simpleTable("a"), simpleTable("b")}, want: "create_a_b"},
		{name: "many new tables", old: []*schema.Table{notes}, new: []*schema.Table{notes, simpleTable("a"), simpleTable("b"), simpleTable("c")}, want: "create_tables"},
		{name: "dropped table", old: []*schema.Table{notes, simpleTable("tags")}, new: []*schema.Table{notes}, want: "drop_tags"},
		{name: "altered table", old: []*schema.Table{notes}, new: []*schema.Table{simpleTable("notes", "title")}, want: "alter_notes"},
		{name: "mixed changes", old: []*schema.Table{notes}, new: []*schema.Table{simpleTable("notes", "title"), simpleTable("tags")}, want: "update_schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var old *schema.Database
			if tt.old != nil {
				old = &schema.Database{Tables: tt.old}
			}
			diff := schema.Diff(old, &schema.Database{Tables: tt.new}, schema.DiffOptions{})
			if diff.Empty() {
				t.Fatal("diff is empty")
			}
			if got := schema.MigrationName(old, diff.Changes); got != tt.want {
				t.Fatalf("MigrationName = %q, want %q", got, tt.want)
			}
		})
	}
	same := &schema.Database{Tables: []*schema.Table{notes}}
	if !schema.Diff(same, same, schema.DiffOptions{}).Empty() {
		t.Fatal("a database compared with itself has changes")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	empty, err := schema.NewSnapshot("postgres", &schema.Database{}, nil).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(empty, []byte(`"tables": []`)) || !bytes.Contains(empty, []byte(`"migrations": []`)) {
		t.Fatalf("empty snapshot:\n%s", empty)
	}
	db := schema.Build(parse(t, notesSpecs))
	data, err := schema.NewSnapshot("sqlite", db, []string{"0001_init"}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	snap, err := schema.UnmarshalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Dialect != "sqlite" || !slices.Equal(snap.Migrations, []string{"0001_init"}) {
		t.Fatalf("snapshot = %+v", snap)
	}
	if !schema.Diff(snap.Database(), db, schema.DiffOptions{}).Empty() {
		t.Fatal("the decoded snapshot differs from the database it was built from")
	}
	again, err := snap.Marshal()
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("snapshot encoding is not stable: %v", err)
	}
	if _, err := schema.UnmarshalSnapshot([]byte(`{"version":1,"dialect":"sqlite","tables":[],"extra":true}`)); err == nil {
		t.Error("unknown members are accepted")
	}
	if _, err := schema.UnmarshalSnapshot([]byte(`{"version":2,"dialect":"sqlite","tables":[]}`)); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("future version = %v", err)
	}
}

func TestPolicies(t *testing.T) {
	admin := schema.Policy{Roles: []string{"admin"}}
	member := schema.Policy{Roles: []string{"member"}}
	both := schema.Policy{Roles: []string{"admin", "member"}}
	public := schema.Policy{Public: true}
	anyone := schema.Policy{}
	tests := []struct {
		a, b schema.Policy
		want []string
		ok   bool
	}{
		{a: public, b: admin, want: []string{"admin"}, ok: true},
		{a: admin, b: public, want: []string{"admin"}, ok: true},
		{a: anyone, b: member, want: []string{"member"}, ok: true},
		{a: both, b: anyone, want: []string{"admin", "member"}, ok: true},
		{a: both, b: admin, want: []string{"admin"}, ok: true},
		{a: admin, b: member, want: nil, ok: false},
	}
	for _, tt := range tests {
		got, ok := schema.Combine(tt.a, tt.b)
		if ok != tt.ok || !slices.Equal(got.Roles, tt.want) {
			t.Errorf("Combine(%+v, %+v) = %+v, %v", tt.a, tt.b, got, ok)
		}
	}
	if got, _ := schema.Combine(public, public); !got.Public {
		t.Error("two public policies are not public")
	}
	model := parse(t, notesSpecs)
	tag, note, user := model.Entity("Tag"), model.Entity("Note"), model.Entity("User")
	if p := tag.Policy(schema.OpRead); !p.Public {
		t.Errorf("tag read = %+v", p)
	}
	if p := tag.Policy(schema.OpCreate); p.Public || !slices.Equal(p.Roles, []string{"admin"}) {
		t.Errorf("tag create = %+v", p)
	}
	if p := user.Policy(schema.OpDelete); !slices.Equal(p.Roles, []string{"admin"}) {
		t.Errorf("user delete = %+v", p)
	}
	list, link, ok := schema.RelationPolicies(note, tag, schema.RelationManyToMany)
	if !ok || list.Public || len(list.Roles) != 0 || !slices.Equal(link.Roles, []string{"admin"}) {
		t.Errorf("RelationPolicies(note, tag) = %+v %+v %v", list, link, ok)
	}
	if _, back, _ := schema.RelationPolicies(tag, note, schema.RelationManyToMany); !slices.Equal(back.Roles, link.Roles) {
		t.Errorf("the two directions of a link disagree: %+v and %+v", link, back)
	}
	if _, hasMany, ok := schema.RelationPolicies(user, note, schema.RelationHasMany); !ok || hasMany.Public || len(hasMany.Roles) != 0 {
		t.Errorf("has-many relations have no link policy, got %+v %v", hasMany, ok)
	}
	var ops schema.Operations
	if ops.Any() {
		t.Fatal("zero operations are set")
	}
	for _, op := range schema.AllOperations {
		ops.Set(op)
		if !ops.Has(op) {
			t.Errorf("%s was not set", op)
		}
	}
	if ops.Has("archive") {
		t.Error("unknown operations are reported as set")
	}
}

func TestEntityAndFieldHelpers(t *testing.T) {
	model := parse(t, notesSpecs)
	user, note := model.Entity("User"), model.Entity("Note")
	if model.AuthEntity() != user || !user.IsAuth() || note.IsAuth() {
		t.Fatal("User is the account entity")
	}
	if user.LoginMethod() != schema.MethodJWT || !user.HasMethod(schema.MethodAPIKey) || user.HasMethod(schema.MethodSession) {
		t.Errorf("methods = %v", user.Auth)
	}
	if user.Identity().Name != "Email" || user.Password().Name != "PasswordHash" || user.Role().Name != "Role" || user.Owner() != nil {
		t.Error("account field lookups")
	}
	password := user.Password()
	if password.InputName() != "Password" || password.InputJSON() != "password" || password.InUpdate() || password.Filterable() {
		t.Errorf("password field = %s %s", password.InputName(), password.InputJSON())
	}
	if email := user.Field("Email"); email.InputJSON() != "email" || !email.Filterable() || email.Auto() {
		t.Error("email field")
	}
	owner := note.Owner()
	if owner == nil || owner.InCreate() || owner.InUpdate() || owner.Filterable() {
		t.Fatalf("owner field = %+v", owner)
	}
	if id := note.PK(); id.InCreate() || id.InUpdate() || !id.Auto() || id.GoType() != "int64" {
		t.Error("auto primary key")
	}
	if f := note.Field("ArchiveAt"); f.GoType() != "*time.Time" || !f.InCreate() || !f.InUpdate() || f.Filterable() {
		t.Errorf("ArchiveAt = %s", f.GoType())
	}
	if f := note.Field("UpdatedAt"); f.InCreate() || f.InUpdate() || !f.Auto() {
		t.Error("auto=update field")
	}
	if names := note.FieldNames(); !slices.Equal(names, []string{"ID", "OwnerID", "Title", "Body", "ArchiveAt", "UpdatedAt"}) {
		t.Errorf("field names = %v", names)
	}
	fieldNames := func(fields []*schema.Field) []string {
		var out []string
		for _, f := range fields {
			out = append(out, f.Name)
		}
		return out
	}
	if got := fieldNames(note.CreateFields()); !slices.Equal(got, []string{"Title", "Body", "ArchiveAt"}) {
		t.Errorf("create fields = %v", got)
	}
	if got := fieldNames(note.UpdateFields()); !slices.Equal(got, []string{"Title", "Body", "ArchiveAt"}) {
		t.Errorf("update fields = %v", got)
	}
	if got := fieldNames(note.FilterFields()); !slices.Equal(got, []string{"Title"}) {
		t.Errorf("filter fields = %v", got)
	}
	if got := fieldNames(note.BelongsTo()); !slices.Equal(got, []string{"OwnerID"}) {
		t.Errorf("belongs_to fields = %v", got)
	}
	if got := fieldNames(user.UniqueFields()); !slices.Equal(got, []string{"Email"}) {
		t.Errorf("unique fields = %v", got)
	}
	if user.AutoCreateField().Name != "CreatedAt" || note.AutoCreateField() != nil {
		t.Error("auto=create lookup")
	}
	if note.Path() != "/notes" || note.PathParam() != "noteId" || note.Var() != "note" || note.Relations[0].PathSegment() != "tags" {
		t.Errorf("paths = %s %s %s", note.Path(), note.PathParam(), note.Relations[0].PathSegment())
	}
	if model.Entity("Missing") != nil || note.Field("Missing") != nil {
		t.Error("lookups of unknown names")
	}
	if names := model.EntityNames(); !slices.Equal(names, []string{"Note", "Tag", "User"}) {
		t.Errorf("entity names = %v", names)
	}
	if schema.JoinColumn(note) != "note_id" || schema.AuthUserColumn(user) != "user_id" {
		t.Error("join columns")
	}
	if a, b := schema.JoinPair(model.Entity("Tag"), note); a != note || b.Name != "Tag" {
		t.Error("join pair order")
	}
}

func TestDiagnosticsError(t *testing.T) {
	ds := schema.Diagnostics{
		{Pos: schema.Pos{File: "specs/b.go", Line: 2}, Severity: schema.SeverityError, Code: "x", Message: "second"},
		{Pos: schema.Pos{File: "specs/a.go", Line: 9, Column: 3}, Severity: schema.SeverityWarning, Code: "y", Message: "first"},
	}
	ds.Sort()
	lines := strings.Split(ds.Error(), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "specs/a.go:9:3") || !strings.HasPrefix(lines[1], "specs/b.go:2") {
		t.Fatalf("Error() = %q", ds.Error())
	}
	if (schema.Pos{}).IsValid() || !(schema.Pos{File: "specs/a.go"}).IsValid() {
		t.Error("IsValid")
	}
}
