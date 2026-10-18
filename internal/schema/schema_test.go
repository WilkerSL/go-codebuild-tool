package schema

import (
	"slices"
	"strings"
	"testing"
)

func table(name string, refs ...string) *Table {
	t := &Table{
		Name:       name,
		Columns:    []*Column{{Name: "id", Kind: ColumnInt64, Identity: true}},
		PrimaryKey: &PrimaryKey{Name: ConstraintName("pk", name), Columns: []string{"id"}},
	}
	for _, ref := range refs {
		column := ref + "_id"
		t.Columns = append(t.Columns, &Column{Name: column, Kind: ColumnInt64, Nullable: true})
		t.ForeignKeys = append(t.ForeignKeys, &ForeignKey{
			Name: ConstraintName("fk", name, column), Columns: []string{column},
			RefTable: ref, RefColumns: []string{"id"}, OnDelete: OnDeleteRestrict,
		})
	}
	return t
}

func kinds(changes []Change) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = string(c.Kind) + ":" + c.TableName
	}
	return out
}

func TestCreateOrdersTablesByDependency(t *testing.T) {
	next := &Database{Tables: []*Table{table("a", "b"), table("b", "c"), table("c")}}
	got := kinds(Diff(nil, next, DiffOptions{}).Changes)
	want := []string{"create_table:c", "create_table:b", "create_table:a"}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestCreateBreaksCycles(t *testing.T) {
	next := &Database{Tables: []*Table{table("a", "b"), table("b", "a")}}
	result := Diff(nil, next, DiffOptions{})
	got := kinds(result.Changes)
	want := []string{"create_table:a", "create_table:b", "add_foreign_key:a"}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	if len(result.Changes[0].InlineFKs) != 0 || len(result.Changes[1].InlineFKs) != 1 {
		t.Fatalf("inline foreign keys = %v / %v", result.Changes[0].InlineFKs, result.Changes[1].InlineFKs)
	}
	down := kinds(result.Down())
	if !slices.Equal(down, []string{"drop_foreign_key:a", "drop_table:b", "drop_table:a"}) {
		t.Fatalf("down = %v", down)
	}
}

func TestDropBreaksCycles(t *testing.T) {
	old := &Database{Tables: []*Table{table("a", "b"), table("b", "a")}}
	result := Diff(old, nil, DiffOptions{})
	got := kinds(result.Changes)
	want := []string{"drop_foreign_key:b", "drop_table:a", "drop_table:b"}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	down := result.Down()
	if down[0].Kind != ChangeCreateTable || down[0].TableName != "b" || len(down[0].InlineFKs) != 0 {
		t.Fatalf("down must recreate b without the broken foreign key: %+v", down[0])
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestAlterRecreatesDependents(t *testing.T) {
	old := &Database{Tables: []*Table{table("a")}}
	old.Tables[0].Columns = append(old.Tables[0].Columns, &Column{Name: "code", Kind: ColumnString, Size: 10})
	old.Tables[0].Indexes = []*Index{{Name: "ux_a_code", Columns: []string{"code"}, Unique: true}}
	next := &Database{Tables: []*Table{table("a")}}
	next.Tables[0].Columns = append(next.Tables[0].Columns, &Column{Name: "code", Kind: ColumnString, Size: 20})
	next.Tables[0].Indexes = []*Index{{Name: "ux_a_code", Columns: []string{"code"}, Unique: true}}
	plain := kinds(Diff(old, next, DiffOptions{}).Changes)
	if !slices.Equal(plain, []string{"alter_column:a"}) {
		t.Fatalf("plain = %v", plain)
	}
	recreated := kinds(Diff(old, next, DiffOptions{RecreateDependents: true}).Changes)
	if !slices.Equal(recreated, []string{"drop_index:a", "alter_column:a", "add_index:a"}) {
		t.Fatalf("recreated = %v", recreated)
	}
}

func TestDiffWarnings(t *testing.T) {
	old := &Database{Tables: []*Table{table("a")}}
	old.Tables[0].Columns = append(old.Tables[0].Columns, &Column{Name: "old_name", Kind: ColumnString})
	next := &Database{Tables: []*Table{table("a")}}
	next.Tables[0].Columns = append(next.Tables[0].Columns, &Column{Name: "new_name", Kind: ColumnString})
	result := Diff(old, next, DiffOptions{})
	joined := strings.Join(result.Warnings, "\n")
	for _, want := range []string{"deletes its data", "renamed", "without a default"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %q in:\n%s", want, joined)
		}
	}
}

func TestInverseIsSymmetric(t *testing.T) {
	c := Change{Kind: ChangeAlterColumn, TableName: "a", Column: &Column{Name: "x", Size: 2}, OldColumn: &Column{Name: "x", Size: 1}}
	back := Inverse(Inverse(c))
	if back.Kind != c.Kind || back.Column != c.Column || back.OldColumn != c.OldColumn {
		t.Fatalf("inverse of inverse = %+v", back)
	}
}

func TestConstraintNameLimit(t *testing.T) {
	name := ConstraintName("ux", strings.Repeat("t", 50), strings.Repeat("c", 50))
	if len(name) != MaxIdentifierLength || !strings.HasPrefix(name, "ux_ttt") {
		t.Fatalf("name = %s (%d)", name, len(name))
	}
}

func TestPosAndDiagnosticFormatting(t *testing.T) {
	if (Pos{}).String() != "" || (Pos{File: "f.go"}).String() != "f.go" || (Pos{File: "f.go", Line: 3}).String() != "f.go:3" {
		t.Fatal("unexpected Pos formatting")
	}
	ds := Diagnostics{
		{Pos: Pos{File: "b.go", Line: 1}, Severity: SeverityWarning, Code: "w", Message: "later"},
		{Pos: Pos{File: "a.go", Line: 2}, Severity: SeverityError, Code: "e", Message: "first"},
	}
	ds.Sort()
	if ds[0].File != "a.go" || !ds.HasErrors() || len(ds.Errors()) != 1 || len(ds.Warnings()) != 1 {
		t.Fatalf("diagnostics = %+v", ds)
	}
	if !strings.Contains(ds[1].String(), "warning: later [w]") {
		t.Fatalf("warning format = %s", ds[1].String())
	}
}
