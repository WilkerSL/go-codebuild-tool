package spec

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

const credentials = "\tEmail        string `gen:\"required,unique,email,max=254,identity\"`\n\tPasswordHash string `gen:\"password\"`\n"

func accountSpec(meta, fields string) string {
	return header + "import \"github.com/google/uuid\"\n\ntype User struct {\n\t_  struct{}  `gen:\"" + meta + "\"`\n\tID uuid.UUID `gen:\"pk,auto\"`\n" + fields + "}\n"
}

func ownedSpec(name, meta, fields string) string {
	return header + "import \"github.com/google/uuid\"\n\ntype " + name + " struct {\n\t_       struct{}  `gen:\"" + meta + "\"`\n\tID      int64     `gen:\"pk,auto\"`\n" + fields + "}\n"
}

func TestParseSecureExample(t *testing.T) {
	sources, err := LoadDir(filepath.Join(repoRoot(t), "testdata"), "specs/secure")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := Parse(sources)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	user := model.AuthEntity()
	if user == nil || user.Name != "User" || !slices.Equal(user.Auth, []schema.Method{schema.MethodSession, schema.MethodAPIKey}) || !user.Register {
		t.Fatalf("account entity = %+v", user)
	}
	if user.LoginMethod() != schema.MethodSession || !slices.Equal(user.Roles, []string{"admin"}) {
		t.Fatalf("login method %s, roles %v", user.LoginMethod(), user.Roles)
	}
	email, password, role := user.Identity(), user.Password(), user.Role()
	if email == nil || email.Name != "Email" || !email.Required || !email.NotEmpty {
		t.Fatalf("identity = %+v", email)
	}
	if password == nil || password.InputName() != "Password" || password.InputJSON() != "password" || password.Min != "10" || !password.OmitResponse || password.InUpdate() || !password.InCreate() {
		t.Fatalf("password = %+v", password)
	}
	if schema.ColumnSize(password) != schema.PasswordSize {
		t.Fatalf("password column size = %d", schema.ColumnSize(password))
	}
	if role == nil || role.Name != "Role" {
		t.Fatalf("role = %+v", role)
	}
	note := model.Entity("Note")
	owner := note.Owner()
	if owner == nil || owner.Name != "OwnerID" || owner.InCreate() || owner.InUpdate() || owner.Filterable() {
		t.Fatalf("owner = %+v", owner)
	}
	label := model.Entity("Label")
	if !label.Public.Read || label.Public.Create || !slices.Equal(label.WriteRoles, []string{"admin"}) {
		t.Fatalf("label access = %+v %v", label.Public, label.WriteRoles)
	}
	if p := label.Policy(schema.OpRead); !p.Public {
		t.Fatalf("label read policy = %+v", p)
	}
	if p := label.Policy(schema.OpCreate); p.Public || !slices.Equal(p.Roles, []string{"admin"}) {
		t.Fatalf("label create policy = %+v", p)
	}
	list, link, ok := schema.RelationPolicies(label, note, schema.RelationManyToMany)
	if !ok || list.Public || len(list.Roles) != 0 || !slices.Equal(link.Roles, []string{"admin"}) {
		t.Fatalf("Label.Notes policies = %+v %+v %v", list, link, ok)
	}
	db := schema.Build(model)
	for _, table := range []string{schema.SessionsTable, schema.APIKeysTable} {
		if db.Table(table) == nil {
			t.Errorf("missing table %s", table)
		}
	}
	if db.Table(schema.RefreshTokensTable) != nil {
		t.Error("session projects do not need refresh tokens")
	}
	if fk := db.Table(schema.SessionsTable).ForeignKeys[0]; fk.RefTable != "users" || fk.OnDelete != schema.OnDeleteCascade || fk.Columns[0] != "user_id" {
		t.Fatalf("session foreign key = %+v", fk)
	}
}

func TestAuthDiagnostics(t *testing.T) {
	const orgSpec = header + "import \"github.com/google/uuid\"\n\ntype Org struct {\n\tID uuid.UUID `gen:\"pk,auto\"`\n}\n"
	const roleField = "\tRole         string `gen:\"enum=admin|member,default=member,role\"`\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []wantDiag
	}{
		{
			name:  "two login methods",
			files: map[string]string{"specs/users.go": accountSpec("auth=session|jwt", credentials)},
			want:  []wantDiag{{at: "auth=session|jwt", code: CodeInvalidValue, contains: "choose one login method"}},
		},
		{
			name:  "api keys without a login method",
			files: map[string]string{"specs/users.go": accountSpec("auth=apikey", credentials)},
			want:  []wantDiag{{at: "auth=apikey", code: CodeInvalidValue, suggestion: "session|apikey"}},
		},
		{
			name:  "unknown method",
			files: map[string]string{"specs/users.go": accountSpec("auth=sesion", credentials)},
			want:  []wantDiag{{at: "auth=sesion", code: CodeInvalidValue, suggestion: "session"}},
		},
		{
			name:  "missing password",
			files: map[string]string{"specs/users.go": accountSpec("auth=session", "\tEmail string `gen:\"required,unique,max=254,identity\"`\n")},
			want:  []wantDiag{{at: "auth=session", code: CodeInvalidCombination, contains: "needs a field marked password"}},
		},
		{
			name:  "identity without unique",
			files: map[string]string{"specs/users.go": accountSpec("auth=session", "\tEmail string `gen:\"max=254,identity\"`\n\tPasswordHash string `gen:\"password\"`\n")},
			want:  []wantDiag{{at: "identity", code: CodeInvalidCombination, suggestion: "unique"}},
		},
		{
			name:  "password with max",
			files: map[string]string{"specs/users.go": accountSpec("auth=session", "\tEmail string `gen:\"unique,max=254,identity\"`\n\tPasswordHash string `gen:\"password,max=64\"`\n")},
			want:  []wantDiag{{at: "max=64", code: CodeInvalidCombination}},
		},
		{
			name:  "short password minimum",
			files: map[string]string{"specs/users.go": accountSpec("auth=session", "\tEmail string `gen:\"unique,max=254,identity\"`\n\tPasswordHash string `gen:\"password,min=6\"`\n")},
			want:  []wantDiag{{at: "min=6", code: CodeInvalidValue, suggestion: "8"}},
		},
		{
			name:  "role without enum",
			files: map[string]string{"specs/users.go": accountSpec("auth=session,roles=admin", credentials+"\tRole string `gen:\"role\"`\n")},
			want:  []wantDiag{{at: "role\"", code: CodeInvalidCombination, contains: "needs enum="}},
		},
		{
			name:  "role field with open account routes",
			files: map[string]string{"specs/users.go": accountSpec("auth=session", credentials+roleField)},
			want:  []wantDiag{{at: "role\"", code: CodeInvalidCombination, suggestion: "writeroles=admin"}},
		},
		{
			name:  "register needs a default role",
			files: map[string]string{"specs/users.go": accountSpec("auth=session,register,roles=admin", credentials+"\tRole string `gen:\"enum=admin|member,role\"`\n")},
			want:  []wantDiag{{at: "role\"", code: CodeInvalidCombination, suggestion: "default=admin"}},
		},
		{
			name:  "account entity cannot be public",
			files: map[string]string{"specs/users.go": accountSpec("auth=session,public=read", credentials)},
			want:  []wantDiag{{at: "public=read", code: CodeInvalidCombination, suggestion: "register"}},
		},
		{
			name:  "identity outside the account entity",
			files: map[string]string{"specs/items.go": header + "type Item struct {\n\tID   int64  `gen:\"pk,auto\"`\n\tCode string `gen:\"unique,max=20,identity\"`\n}\n"},
			want:  []wantDiag{{at: "identity", code: CodeInvalidCombination, contains: "only valid on the entity that declares auth="}},
		},
		{
			name: "two account entities",
			files: map[string]string{
				"specs/users.go":  accountSpec("auth=session", credentials),
				"specs/admins.go": header + "import \"github.com/google/uuid\"\n\ntype Admin struct {\n\t_  struct{}  `gen:\"auth=jwt\"`\n\tID uuid.UUID `gen:\"pk,auto\"`\n" + credentials + "}\n",
			},
			want: []wantDiag{{file: "specs/users.go", at: "auth=session", code: CodeInvalidCombination, contains: "one account entity"}},
		},
		{
			name: "access markers without an account entity",
			files: map[string]string{
				"specs/items.go": header + "type Item struct {\n\t_  struct{} `gen:\"public=read\"`\n\tID int64    `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{at: "public=read", code: CodeInvalidCombination, contains: "needs an account entity"}},
		},
		{
			name: "owner must reference the account entity",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session", credentials),
				"specs/orgs.go":  orgSpec,
				"specs/notes.go": ownedSpec("Note", "softdelete", "\tOrgID   uuid.UUID `gen:\"rel=belongs_to:Org,owner\"`\n"),
			},
			want: []wantDiag{{file: "specs/notes.go", at: "owner\"", code: CodeInvalidCombination, contains: "must reference the account entity"}},
		},
		{
			name: "owned rows cannot be public",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session", credentials),
				"specs/notes.go": ownedSpec("Note", "public=read", "\tOwnerID uuid.UUID `gen:\"rel=belongs_to:User,owner\"`\n"),
			},
			want: []wantDiag{{file: "specs/notes.go", at: "public=read", code: CodeInvalidCombination}},
		},
		{
			name: "nullable owner",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session", credentials),
				"specs/notes.go": ownedSpec("Note", "softdelete", "\tOwnerID *uuid.UUID `gen:\"rel=belongs_to:User,owner\"`\n"),
			},
			want: []wantDiag{{file: "specs/notes.go", at: "owner\"", code: CodeInvalidCombination, contains: "cannot be nullable"}},
		},
		{
			name: "unknown role",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session,roles=admin", credentials+roleField),
				"specs/items.go": header + "type Item struct {\n\t_  struct{} `gen:\"roles=admn\"`\n\tID int64    `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/items.go", at: "roles=admn", code: CodeInvalidValue, suggestion: "admin"}},
		},
		{
			name: "roles without a role field",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session", credentials),
				"specs/items.go": header + "type Item struct {\n\t_  struct{} `gen:\"writeroles=admin\"`\n\tID int64    `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/items.go", at: "writeroles=admin", code: CodeInvalidCombination, contains: "needs a field marked role"}},
		},
		{
			name: "writeroles on a readonly entity",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session,roles=admin", credentials+roleField),
				"specs/items.go": header + "type Item struct {\n\t_  struct{} `gen:\"readonly,writeroles=admin\"`\n\tID int64    `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/items.go", at: "writeroles=admin", code: CodeInvalidCombination}},
		},
		{
			name: "relation roles with nothing in common",
			files: map[string]string{
				"specs/users.go":   accountSpec("auth=session,roles=admin", credentials+roleField),
				"specs/teams.go":   header + "type Team struct {\n\t_       struct{} `gen:\"roles=admin\"`\n\tID      int64    `gen:\"pk,auto\"`\n\tMembers []Member `gen:\"rel=has_many:Member\"`\n}\n",
				"specs/members.go": header + "type Member struct {\n\t_      struct{} `gen:\"roles=member\"`\n\tID     int64    `gen:\"pk,auto\"`\n\tTeamID int64    `gen:\"rel=belongs_to:Team\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/teams.go", at: "rel=has_many:Member", code: CodeInvalidCombination, contains: "no role can use"}},
		},
		{
			name: "reserved authentication table",
			files: map[string]string{
				"specs/users.go": accountSpec("auth=session", credentials),
				"specs/items.go": header + "type Item struct {\n\t_  struct{} `gen:\"table=auth_sessions\"`\n\tID int64    `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/items.go", at: "table=auth_sessions", code: CodeDuplicateTable}},
		},
		{
			name:  "auth domain is reserved",
			files: map[string]string{"specs/auth.go": accountSpec("auth=session", credentials)},
			want:  []wantDiag{{code: CodeDomainName}},
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
		})
	}
}
