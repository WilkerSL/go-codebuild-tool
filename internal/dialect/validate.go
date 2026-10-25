package dialect

import (
	"fmt"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

const (
	CodeDialectLimit   = "dialect_limit"
	CodeDialectCascade = "dialect_cascade"
)

func indexed(f *schema.Field) bool {
	return f.PK || f.Unique || f.Index || f.Ref != nil
}

func Validate(d Dialect, m *schema.Model) schema.Diagnostics {
	var diags schema.Diagnostics
	limits := d.Limits()
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if f.Kind == schema.KindString && !f.Decimal() && indexed(f) {
				limit := limits.IndexedString
				if f.PK {
					limit = limits.PrimaryKeyString
				}
				size := schema.ColumnSize(f)
				if limit > 0 && (size == 0 || size > limit) {
					diags = append(diags, schema.Diagnostic{
						Pos:      f.Pos,
						Severity: schema.SeverityError,
						Code:     CodeDialectLimit,
						Message: fmt.Sprintf("%s.%s is indexed but its length is unbounded or above %d characters, which %s cannot index; add max=%d or less",
							e.Name, f.Name, limit, d.Name(), min(255, limit)),
					})
				}
			}
			if f.Unique && e.SoftDelete && !limits.PartialIndexes {
				diags = append(diags, schema.Diagnostic{
					Pos:      f.Pos,
					Severity: schema.SeverityWarning,
					Code:     CodeDialectLimit,
					Message:  fmt.Sprintf("%s has no partial indexes, so soft-deleted %s rows still block their %s values", d.Name(), e.Name, f.Name),
				})
			}
		}
	}
	if limits.SingleCascade {
		diags = append(diags, cascadePaths(d, m)...)
	}
	return diags
}

type cascadeEdge struct {
	from, to string
	pos      schema.Pos
	label    string
}

func cascadePaths(d Dialect, m *schema.Model) schema.Diagnostics {
	edges := map[string][]cascadeEdge{}
	add := func(e cascadeEdge) {
		edges[e.from] = append(edges[e.from], e)
	}
	joins := map[string]bool{}
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if f.Ref == nil || f.Ref.OnDelete == schema.OnDeleteRestrict || f.External(e) {
				continue
			}
			target := m.Entity(f.Ref.Entity)
			if target == nil {
				continue
			}
			add(cascadeEdge{from: target.Table, to: e.Table, pos: f.Pos, label: e.Name + "." + f.Name})
		}
		for _, r := range e.Relations {
			if r.Kind != schema.RelationManyToMany || r.Via == "" || joins[r.Via] {
				continue
			}
			target := m.Entity(r.Target)
			if target == nil {
				continue
			}
			joins[r.Via] = true
			add(cascadeEdge{from: e.Table, to: r.Via, pos: r.Pos, label: r.Via})
			add(cascadeEdge{from: target.Table, to: r.Via, pos: r.Pos, label: r.Via})
		}
	}
	var diags schema.Diagnostics
	reported := map[string]bool{}
	report := func(e cascadeEdge, format string, args ...any) {
		key := e.pos.String() + e.label
		if reported[key] {
			return
		}
		reported[key] = true
		diags = append(diags, schema.Diagnostic{
			Pos:      e.pos,
			Severity: schema.SeverityError,
			Code:     CodeDialectCascade,
			Message:  fmt.Sprintf(format, args...),
		})
	}
	for _, origin := range m.Entities {
		counts := map[string]int{}
		onStack := map[string]bool{}
		var visit func(node string)
		visit = func(node string) {
			onStack[node] = true
			for _, e := range edges[node] {
				if onStack[e.to] {
					report(e, "%s creates a cascading cycle that %s rejects; use ondelete=restrict", e.label, d.Name())
					continue
				}
				counts[e.to]++
				if counts[e.to] == 2 {
					report(e, "deleting %s reaches %s through more than one cascading path, which %s rejects; use ondelete=restrict on %s", origin.Table, e.to, d.Name(), e.label)
				}
				visit(e.to)
			}
			onStack[node] = false
		}
		visit(origin.Table)
	}
	return diags
}
