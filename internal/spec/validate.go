package spec

import (
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

func (p *parser) validateModel(m *schema.Model) {
	names := m.EntityNames()
	p.checkTables(m)
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if f.Ref != nil {
				p.checkBelongsTo(m, e, f, names)
			}
		}
		for _, r := range e.Relations {
			switch r.Kind {
			case schema.RelationHasMany:
				p.checkHasMany(m, e, r, names)
			case schema.RelationManyToMany:
				p.checkManyToMany(m, e, r, names)
			}
		}
	}
	p.checkJoinTables(m)
	p.checkCycles(m)
	p.checkAuthModel(m)
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if f.Ref != nil {
				f.Ref.Target = m.Entity(f.Ref.Entity)
			}
		}
	}
}

func (p *parser) checkAuthModel(m *schema.Model) {
	var auth *schema.Entity
	for _, e := range m.Entities {
		if !e.IsAuth() {
			continue
		}
		if auth != nil {
			p.errorf(p.entityPos(e, "auth"), CodeInvalidCombination, "", "%s and %s both declare auth=; a project has one account entity", auth.Name, e.Name)
			continue
		}
		auth = e
	}
	for _, e := range m.Entities {
		if slices.Contains(schema.AuthTables, e.Table) {
			p.errorf(p.entityPos(e, "table"), CodeDuplicateTable, "", "table %s is reserved for the authentication tables; choose another table name", e.Table)
		}
		for _, f := range e.Fields {
			if !f.Owner || f.Ref == nil {
				continue
			}
			if auth == nil || f.Ref.Entity != auth.Name {
				p.errorf(p.fieldPos(f, "owner"), CodeInvalidCombination, "", "owner %s.%s must reference the account entity (the entity with auth=)", e.Name, f.Name)
			}
		}
		if auth == nil {
			for _, key := range []string{"public", "roles", "writeroles"} {
				if set, ok := p.entityMarkers[e]; ok && set.has(key) {
					p.errorf(set.get(key).pos, CodeInvalidCombination, "", "%s needs an account entity; declare auth=session on the entity that holds the accounts", key)
				}
			}
			continue
		}
		var roleItems []string
		if role := auth.Role(); role != nil {
			roleItems = role.Enum
		}
		for _, key := range []string{"roles", "writeroles"} {
			roles := e.Roles
			if key == "writeroles" {
				roles = e.WriteRoles
			}
			for _, r := range roles {
				switch {
				case auth.Role() == nil:
					p.errorf(p.entityPos(e, key), CodeInvalidCombination, "", "%s needs a field marked role on the account entity %s", key, auth.Name)
				case !slices.Contains(roleItems, r):
					p.errorf(p.entityPos(e, key), CodeInvalidValue, naming.Suggest(r, roleItems), "role %q is not one of the roles of %s.%s: %s", r, auth.Name, auth.Role().Name, strings.Join(roleItems, ", "))
				default:
					continue
				}
				break
			}
		}
	}
	if auth == nil {
		return
	}
	for _, source := range m.Entities {
		for _, r := range source.Relations {
			target := m.Entity(r.Target)
			if target == nil || target.NoRoutes {
				continue
			}
			if _, _, ok := schema.RelationPolicies(source, target); !ok {
				p.errorf(p.relationPos(r, "rel"), CodeInvalidCombination, "",
					"no role can use the routes of %s.%s: %s and %s require roles with nothing in common", source.Name, r.Name, source.Name, target.Name)
			}
		}
	}
}

func (p *parser) checkTables(m *schema.Model) {
	owners := map[string]*schema.Entity{}
	for _, e := range m.Entities {
		if other, ok := owners[e.Table]; ok {
			pos := e.Pos
			if set, ok := p.entityMarkers[e]; ok && set.has("table") {
				pos = set.get("table").pos
			}
			p.errorf(pos, CodeDuplicateTable, "", "entities %s and %s both use table %s", other.Name, e.Name, e.Table)
			continue
		}
		owners[e.Table] = e
	}
}

func (p *parser) relPos(f *schema.Field) schema.Pos {
	if set, ok := p.fieldMarkers[f]; ok && set.has("rel") {
		return set.get("rel").pos
	}
	return f.Pos
}

func (p *parser) relationPos(r *schema.Relation, key string) schema.Pos {
	if set, ok := p.relationMarkers[r]; ok && set.has(key) {
		return set.get(key).pos
	}
	return r.Pos
}

func (p *parser) checkBelongsTo(m *schema.Model, e *schema.Entity, f *schema.Field, names []string) {
	target := m.Entity(f.Ref.Entity)
	if target == nil {
		p.errorf(p.relPos(f), CodeUnknownEntity, naming.Suggest(f.Ref.Entity, names), "rel=belongs_to:%s references an unknown entity", f.Ref.Entity)
		return
	}
	pk := target.PK()
	if pk == nil {
		return
	}
	if pk.Kind != f.Kind {
		p.errorf(p.relPos(f), CodeRelationTypeMismatch, "", "%s.%s has type %s but %s.%s is %s; references must use the primary key type",
			e.Name, f.Name, f.Kind.GoType(), target.Name, pk.Name, pk.Kind.GoType())
	}
}

func (p *parser) checkHasMany(m *schema.Model, e *schema.Entity, r *schema.Relation, names []string) {
	target := m.Entity(r.Target)
	if target == nil {
		p.errorf(p.relationPos(r, "rel"), CodeUnknownEntity, naming.Suggest(r.Target, names), "rel=has_many:%s references an unknown entity", r.Target)
		return
	}
	var candidates []*schema.Field
	for _, f := range target.Fields {
		if f.Ref != nil && f.Ref.Entity == e.Name {
			candidates = append(candidates, f)
		}
	}
	if r.FK != "" {
		f := target.Field(r.FK)
		switch {
		case f == nil:
			p.errorf(p.relationPos(r, "fk"), CodeUnknownField, naming.Suggest(r.FK, target.FieldNames()), "fk=%s does not name a field of %s", r.FK, target.Name)
		case f.Ref == nil || f.Ref.Entity != e.Name:
			p.errorf(p.relationPos(r, "fk"), CodeRelationTypeMismatch, "", "%s.%s is not declared with rel=belongs_to:%s", target.Name, f.Name, e.Name)
		}
		return
	}
	switch len(candidates) {
	case 0:
		p.errorf(p.relationPos(r, "rel"), CodeRelationTypeMismatch, "",
			"%s has no field with rel=belongs_to:%s; add %sID to %s to back the has_many relation %s", target.Name, e.Name, e.Name, target.Name, r.Name)
	case 1:
		r.FK = candidates[0].Name
	default:
		fields := make([]string, len(candidates))
		for i, f := range candidates {
			fields[i] = f.Name
		}
		p.errorf(p.relationPos(r, "rel"), CodeRelationAmbiguous, "fk="+fields[0],
			"%s references %s through %s; add fk=<Field> to choose one", target.Name, e.Name, strings.Join(fields, " and "))
	}
	if target.NoRoutes {
		p.warnf(p.relationPos(r, "rel"), CodeRelationRoutes, "route for %s.%s is generated in the %s domain, which has noroutes; it will not be generated", e.Name, r.Name, target.Domain)
	}
}

func (p *parser) checkManyToMany(m *schema.Model, e *schema.Entity, r *schema.Relation, names []string) {
	target := m.Entity(r.Target)
	if target == nil {
		p.errorf(p.relationPos(r, "rel"), CodeUnknownEntity, naming.Suggest(r.Target, names), "rel=many_to_many:%s references an unknown entity", r.Target)
		return
	}
	if target == e {
		p.errorf(p.relationPos(r, "rel"), CodeInvalidCombination, "", "self-referencing many_to_many is not supported in this version")
		return
	}
	if target.NoRoutes {
		p.warnf(p.relationPos(r, "rel"), CodeRelationRoutes, "routes for %s.%s are generated in the %s domain, which has noroutes; they will not be generated", e.Name, r.Name, target.Domain)
	}
}

type joinUse struct {
	pair     [2]string
	relation *schema.Relation
	source   *schema.Entity
}

func (p *parser) checkJoinTables(m *schema.Model) {
	tables := map[string]bool{}
	for _, e := range m.Entities {
		tables[e.Table] = true
	}
	byVia := map[string]joinUse{}
	byPair := map[[2]string]joinUse{}
	for _, e := range m.Entities {
		for _, r := range e.Relations {
			if r.Kind != schema.RelationManyToMany || r.Via == "" || m.Entity(r.Target) == nil || r.Target == e.Name {
				continue
			}
			pair := [2]string{e.Name, r.Target}
			slices.Sort(pair[:])
			use := joinUse{pair: pair, relation: r, source: e}
			if tables[r.Via] {
				p.errorf(p.relationPos(r, "via"), CodeDuplicateTable, "", "join table %s collides with an entity table", r.Via)
				continue
			}
			if prev, ok := byVia[r.Via]; ok && prev.pair != pair {
				p.errorf(p.relationPos(r, "via"), CodeDuplicateTable, "",
					"join table %s is already used by %s.%s for %s and %s", r.Via, prev.source.Name, prev.relation.Name, prev.pair[0], prev.pair[1])
				continue
			}
			if prev, ok := byPair[pair]; ok && prev.relation.Via != r.Via {
				p.errorf(p.relationPos(r, "via"), CodeInvalidCombination, prev.relation.Via,
					"%s.%s uses via=%s but %s.%s uses via=%s for the same entities; both sides must share one join table",
					e.Name, r.Name, r.Via, prev.source.Name, prev.relation.Name, prev.relation.Via)
				continue
			}
			if _, ok := byPair[pair]; ok && byPair[pair].source == e {
				p.errorf(p.relationPos(r, "rel"), CodeInvalidCombination, "", "%s declares more than one many_to_many relation with %s", e.Name, r.Target)
				continue
			}
			byVia[r.Via] = use
			byPair[pair] = use
		}
	}
}

func (p *parser) checkCycles(m *schema.Model) {
	edges := map[string][]*schema.Field{}
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if f.Ref != nil && !f.Nullable && m.Entity(f.Ref.Entity) != nil {
				edges[e.Name] = append(edges[e.Name], f)
			}
		}
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	reported := map[string]bool{}
	var stack []string
	var visit func(name string)
	visit = func(name string) {
		state[name] = visiting
		stack = append(stack, name)
		for _, f := range edges[name] {
			next := f.Ref.Entity
			switch state[next] {
			case unvisited:
				visit(next)
			case visiting:
				start := slices.Index(stack, next)
				cycle := append(slices.Clone(stack[start:]), next)
				key := strings.Join(cycle, ">")
				if !reported[key] {
					reported[key] = true
					p.errorf(p.relPos(f), CodeRelationCycle, "",
						"non-nullable belongs_to cycle %s; no row could ever be inserted, make one of the references a pointer", strings.Join(cycle, " -> "))
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = done
	}
	for _, e := range m.Entities {
		if state[e.Name] == unvisited {
			visit(e.Name)
		}
	}
}
