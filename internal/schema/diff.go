package schema

import (
	"fmt"
	"slices"
	"strings"
)

type ChangeKind string

const (
	ChangeCreateTable    ChangeKind = "create_table"
	ChangeDropTable      ChangeKind = "drop_table"
	ChangeAddColumn      ChangeKind = "add_column"
	ChangeDropColumn     ChangeKind = "drop_column"
	ChangeAlterColumn    ChangeKind = "alter_column"
	ChangeAddIndex       ChangeKind = "add_index"
	ChangeDropIndex      ChangeKind = "drop_index"
	ChangeAddForeignKey  ChangeKind = "add_foreign_key"
	ChangeDropForeignKey ChangeKind = "drop_foreign_key"
	ChangeAddCheck       ChangeKind = "add_check"
	ChangeDropCheck      ChangeKind = "drop_check"
)

type Change struct {
	Kind       ChangeKind
	TableName  string
	Table      *Table
	InlineFKs  []*ForeignKey
	Column     *Column
	OldColumn  *Column
	Index      *Index
	ForeignKey *ForeignKey
	Check      *Check
	NewTable   bool
}

type DiffOptions struct {
	RecreateDependents bool
}

type DiffResult struct {
	Changes  []Change
	Warnings []string
}

func (r DiffResult) Empty() bool {
	return len(r.Changes) == 0
}

func (r DiffResult) Down() []Change {
	down := make([]Change, 0, len(r.Changes))
	for i := len(r.Changes) - 1; i >= 0; i-- {
		down = append(down, Inverse(r.Changes[i]))
	}
	return down
}

func Inverse(c Change) Change {
	inv := c
	switch c.Kind {
	case ChangeCreateTable:
		inv.Kind = ChangeDropTable
	case ChangeDropTable:
		inv.Kind = ChangeCreateTable
	case ChangeAddColumn:
		inv.Kind = ChangeDropColumn
	case ChangeDropColumn:
		inv.Kind = ChangeAddColumn
	case ChangeAlterColumn:
		inv.Column, inv.OldColumn = c.OldColumn, c.Column
	case ChangeAddIndex:
		inv.Kind = ChangeDropIndex
	case ChangeDropIndex:
		inv.Kind = ChangeAddIndex
	case ChangeAddForeignKey:
		inv.Kind = ChangeDropForeignKey
	case ChangeDropForeignKey:
		inv.Kind = ChangeAddForeignKey
	case ChangeAddCheck:
		inv.Kind = ChangeDropCheck
	case ChangeDropCheck:
		inv.Kind = ChangeAddCheck
	}
	return inv
}

type tablePair struct {
	old *Table
	new *Table
}

type diffState struct {
	opts         DiffOptions
	warnings     []string
	dropFKs      []Change
	dropChecks   []Change
	dropIndexes  []Change
	dropColumns  []Change
	dropTables   []Change
	createTables []Change
	addColumns   []Change
	alterColumns []Change
	addIndexes   []Change
	addChecks    []Change
	addFKs       []Change
}

func Diff(oldDB, newDB *Database, opts DiffOptions) DiffResult {
	if oldDB == nil {
		oldDB = &Database{}
	}
	if newDB == nil {
		newDB = &Database{}
	}
	s := &diffState{opts: opts}
	var created, dropped []*Table
	var kept []tablePair
	for _, t := range newDB.Tables {
		if o := oldDB.Table(t.Name); o != nil {
			kept = append(kept, tablePair{old: o, new: t})
		} else {
			created = append(created, t)
		}
	}
	for _, t := range oldDB.Tables {
		if newDB.Table(t.Name) == nil {
			dropped = append(dropped, t)
		}
	}
	for _, p := range kept {
		s.diffTable(p.old, p.new)
	}
	existing := map[string]bool{}
	for _, p := range kept {
		existing[p.new.Name] = true
	}
	s.planDrops(dropped)
	s.planCreates(created, existing)
	var changes []Change
	for _, group := range [][]Change{
		s.dropFKs, s.dropChecks, s.dropIndexes, s.dropColumns, s.dropTables,
		s.createTables, s.addColumns, s.alterColumns, s.addIndexes, s.addChecks, s.addFKs,
	} {
		changes = append(changes, group...)
	}
	return DiffResult{Changes: changes, Warnings: s.warnings}
}

func (s *diffState) warnf(format string, args ...any) {
	s.warnings = append(s.warnings, fmt.Sprintf(format, args...))
}

func (s *diffState) diffTable(o, n *Table) {
	if !samePrimaryKey(o.PrimaryKey, n.PrimaryKey) {
		s.warnf("the primary key of %s changed; codebuild does not generate primary key migrations, edit the new migration manually", n.Name)
	}
	altered := map[string]bool{}
	var added, removed []*Column
	for _, nc := range n.Columns {
		oc := o.Column(nc.Name)
		switch {
		case oc == nil:
			added = append(added, nc)
			s.addColumns = append(s.addColumns, Change{Kind: ChangeAddColumn, TableName: n.Name, Column: nc})
			if !nc.Nullable && nc.Default == nil && !nc.Identity {
				s.warnf("adding NOT NULL column %s.%s without a default fails if the table already has rows", n.Name, nc.Name)
			}
		case sameColumn(oc, nc):
		case isPrimaryKeyColumn(o, nc.Name):
			s.warnf("changing primary key column %s.%s is not supported; edit the new migration manually", n.Name, nc.Name)
		case oc.Identity != nc.Identity:
			s.warnf("changing the identity of %s.%s is not supported; edit the new migration manually", n.Name, nc.Name)
		default:
			altered[nc.Name] = true
			s.alterColumns = append(s.alterColumns, Change{Kind: ChangeAlterColumn, TableName: n.Name, Column: nc, OldColumn: oc})
			if oc.Nullable && !nc.Nullable {
				s.warnf("making %s.%s NOT NULL fails if it contains NULL values", n.Name, nc.Name)
			}
			if oc.Kind != nc.Kind {
				s.warnf("changing the type of %s.%s from %s to %s may fail or lose data", n.Name, nc.Name, oc.Kind, nc.Kind)
			} else if nc.Size != 0 && (oc.Size == 0 || nc.Size < oc.Size) {
				s.warnf("shrinking %s.%s may fail or truncate existing values", n.Name, nc.Name)
			} else if nc.Scale < oc.Scale || nc.Size-nc.Scale < oc.Size-oc.Scale {
				s.warnf("narrowing %s.%s may fail or round existing values", n.Name, nc.Name)
			}
		}
	}
	for _, oc := range o.Columns {
		if n.Column(oc.Name) == nil {
			removed = append(removed, oc)
			s.dropColumns = append(s.dropColumns, Change{Kind: ChangeDropColumn, TableName: n.Name, Column: oc})
			s.warnf("dropping column %s.%s deletes its data", n.Name, oc.Name)
		}
	}
	for _, r := range removed {
		for _, a := range added {
			if r.Kind == a.Kind {
				s.warnf("if %s.%s was renamed to %s, edit the new migration to rename the column and keep its data", n.Name, r.Name, a.Name)
			}
		}
	}
	touches := func(columns []string) bool {
		if !s.opts.RecreateDependents {
			return false
		}
		return slices.ContainsFunc(columns, func(c string) bool { return altered[c] })
	}
	for _, oi := range o.Indexes {
		ni := n.Index(oi.Name)
		if ni == nil || !sameIndex(oi, ni) || touches(oi.Columns) {
			s.dropIndexes = append(s.dropIndexes, Change{Kind: ChangeDropIndex, TableName: n.Name, Index: oi})
		}
	}
	for _, ni := range n.Indexes {
		oi := o.Index(ni.Name)
		if oi == nil || !sameIndex(oi, ni) || touches(ni.Columns) {
			s.addIndexes = append(s.addIndexes, Change{Kind: ChangeAddIndex, TableName: n.Name, Index: ni})
		}
	}
	for _, ofk := range o.ForeignKeys {
		nfk := n.ForeignKey(ofk.Name)
		if nfk == nil || !sameForeignKey(ofk, nfk) || touches(ofk.Columns) {
			s.dropFKs = append(s.dropFKs, Change{Kind: ChangeDropForeignKey, TableName: n.Name, ForeignKey: ofk})
		}
	}
	for _, nfk := range n.ForeignKeys {
		ofk := o.ForeignKey(nfk.Name)
		if ofk == nil || !sameForeignKey(ofk, nfk) || touches(nfk.Columns) {
			s.addFKs = append(s.addFKs, Change{Kind: ChangeAddForeignKey, TableName: n.Name, ForeignKey: nfk})
		}
	}
	for _, oc := range o.Checks {
		nc := n.Check(oc.Name)
		if nc == nil || !sameCheck(oc, nc) || touches([]string{oc.Column}) {
			s.dropChecks = append(s.dropChecks, Change{Kind: ChangeDropCheck, TableName: n.Name, Check: oc})
		}
	}
	for _, nc := range n.Checks {
		oc := o.Check(nc.Name)
		if oc == nil || !sameCheck(oc, nc) || touches([]string{nc.Column}) {
			s.addChecks = append(s.addChecks, Change{Kind: ChangeAddCheck, TableName: n.Name, Check: nc})
		}
	}
}

// planCreates orders new tables so each one is created after the tables it references.
// When tables reference each other in a cycle, one of the foreign keys is added
// afterwards with its own statement.
func (s *diffState) planCreates(created []*Table, existing map[string]bool) {
	available := map[string]bool{}
	for name := range existing {
		available[name] = true
	}
	remaining := slices.Clone(created)
	for len(remaining) > 0 {
		idx := slices.IndexFunc(remaining, func(t *Table) bool {
			for _, fk := range t.ForeignKeys {
				if fk.RefTable != t.Name && !available[fk.RefTable] {
					return false
				}
			}
			return true
		})
		if idx < 0 {
			idx = 0
		}
		t := remaining[idx]
		remaining = slices.Delete(remaining, idx, idx+1)
		var inline []*ForeignKey
		for _, fk := range t.ForeignKeys {
			if fk.RefTable == t.Name || available[fk.RefTable] {
				inline = append(inline, fk)
			} else {
				s.addFKs = append(s.addFKs, Change{Kind: ChangeAddForeignKey, TableName: t.Name, ForeignKey: fk, NewTable: true})
			}
		}
		available[t.Name] = true
		s.createTables = append(s.createTables, Change{Kind: ChangeCreateTable, TableName: t.Name, Table: t, InlineFKs: inline})
		for _, idx := range t.Indexes {
			s.addIndexes = append(s.addIndexes, Change{Kind: ChangeAddIndex, TableName: t.Name, Index: idx, NewTable: true})
		}
	}
}

func (s *diffState) planDrops(dropped []*Table) {
	broken := map[*ForeignKey]bool{}
	remaining := slices.Clone(dropped)
	referencedBy := func(target *Table) []*Table {
		var refs []*Table
		for _, t := range remaining {
			if t == target {
				continue
			}
			for _, fk := range t.ForeignKeys {
				if fk.RefTable == target.Name && !broken[fk] {
					refs = append(refs, t)
					break
				}
			}
		}
		return refs
	}
	for len(remaining) > 0 {
		idx := slices.IndexFunc(remaining, func(t *Table) bool { return len(referencedBy(t)) == 0 })
		if idx < 0 {
			idx = 0
			target := remaining[0]
			for _, t := range remaining[1:] {
				for _, fk := range t.ForeignKeys {
					if fk.RefTable == target.Name && !broken[fk] {
						broken[fk] = true
						s.dropFKs = append(s.dropFKs, Change{Kind: ChangeDropForeignKey, TableName: t.Name, ForeignKey: fk})
					}
				}
			}
		}
		t := remaining[idx]
		remaining = slices.Delete(remaining, idx, idx+1)
		s.warnf("dropping table %s deletes its data", t.Name)
		s.dropTables = append(s.dropTables, Change{Kind: ChangeDropTable, TableName: t.Name, Table: t})
	}
	for i, c := range s.dropTables {
		var inline []*ForeignKey
		for _, fk := range c.Table.ForeignKeys {
			if !broken[fk] {
				inline = append(inline, fk)
			}
		}
		s.dropTables[i].InlineFKs = inline
	}
}

func samePrimaryKey(a, b *PrimaryKey) bool {
	if a == nil || b == nil {
		return a == b
	}
	return slices.Equal(a.Columns, b.Columns)
}

func isPrimaryKeyColumn(t *Table, column string) bool {
	return t.PrimaryKey != nil && slices.Contains(t.PrimaryKey.Columns, column)
}

func sameColumn(a, b *Column) bool {
	if a.Kind != b.Kind || a.Size != b.Size || a.Scale != b.Scale || a.Nullable != b.Nullable || a.Identity != b.Identity {
		return false
	}
	if a.Default == nil || b.Default == nil {
		return a.Default == b.Default
	}
	return *a.Default == *b.Default
}

func sameIndex(a, b *Index) bool {
	return slices.Equal(a.Columns, b.Columns) && a.Unique == b.Unique && a.NotDeleted == b.NotDeleted
}

func sameForeignKey(a, b *ForeignKey) bool {
	return slices.Equal(a.Columns, b.Columns) && a.RefTable == b.RefTable && slices.Equal(a.RefColumns, b.RefColumns) && a.OnDelete == b.OnDelete
}

func sameCheck(a, b *Check) bool {
	x, y := slices.Clone(a.Values), slices.Clone(b.Values)
	slices.Sort(x)
	slices.Sort(y)
	return a.Column == b.Column && slices.Equal(x, y)
}

func MigrationName(oldDB *Database, changes []Change) string {
	if oldDB == nil || len(oldDB.Tables) == 0 {
		return "init"
	}
	kinds := map[ChangeKind]bool{}
	tables := map[string]bool{}
	for _, c := range changes {
		kinds[c.Kind] = true
		tables[c.TableName] = true
	}
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	onlyKinds := func(allowed ...ChangeKind) bool {
		for k := range kinds {
			if !slices.Contains(allowed, k) {
				return false
			}
		}
		return true
	}
	join := func(prefix string) string {
		name := prefix + "_" + strings.Join(names, "_")
		if len(names) > 2 || len(name) > 48 {
			return prefix + "_tables"
		}
		return name
	}
	switch {
	case onlyKinds(ChangeCreateTable, ChangeAddIndex, ChangeAddForeignKey):
		created := 0
		for _, c := range changes {
			if c.Kind == ChangeCreateTable {
				created++
			}
		}
		if created == len(names) {
			return join("create")
		}
	case onlyKinds(ChangeDropTable, ChangeDropForeignKey):
		return join("drop")
	}
	if len(names) == 1 {
		return "alter_" + names[0]
	}
	return "update_schema"
}
