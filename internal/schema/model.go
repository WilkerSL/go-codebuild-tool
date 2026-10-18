package schema

import (
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
)

type Kind int

const (
	KindString Kind = iota + 1
	KindBool
	KindInt16
	KindInt32
	KindInt
	KindInt64
	KindFloat32
	KindFloat64
	KindBytes
	KindTime
	KindUUID
)

func (k Kind) GoType() string {
	switch k {
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindInt16:
		return "int16"
	case KindInt32:
		return "int32"
	case KindInt:
		return "int"
	case KindInt64:
		return "int64"
	case KindFloat32:
		return "float32"
	case KindFloat64:
		return "float64"
	case KindBytes:
		return "[]byte"
	case KindTime:
		return "time.Time"
	case KindUUID:
		return "uuid.UUID"
	}
	return "invalid"
}

func (k Kind) String() string {
	return k.GoType()
}

func (k Kind) ColumnKind() ColumnKind {
	switch k {
	case KindString:
		return ColumnString
	case KindBool:
		return ColumnBool
	case KindInt16:
		return ColumnInt16
	case KindInt32:
		return ColumnInt32
	case KindInt, KindInt64:
		return ColumnInt64
	case KindFloat32:
		return ColumnFloat32
	case KindFloat64:
		return ColumnFloat64
	case KindBytes:
		return ColumnBytes
	case KindTime:
		return ColumnTime
	case KindUUID:
		return ColumnUUID
	}
	return ""
}

func (k Kind) IsInteger() bool {
	return k == KindInt16 || k == KindInt32 || k == KindInt || k == KindInt64
}

func (k Kind) IsFloat() bool {
	return k == KindFloat32 || k == KindFloat64
}

func (k Kind) IsNumeric() bool {
	return k.IsInteger() || k.IsFloat()
}

func (k Kind) BitSize() int {
	switch k {
	case KindInt16:
		return 16
	case KindInt32, KindFloat32:
		return 32
	}
	return 64
}

type Model struct {
	Entities []*Entity
}

func (m *Model) Entity(name string) *Entity {
	for _, e := range m.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (m *Model) EntityNames() []string {
	names := make([]string, len(m.Entities))
	for i, e := range m.Entities {
		names[i] = e.Name
	}
	return names
}

type Entity struct {
	Name       string
	Domain     string
	File       string
	Pos        Pos
	Table      string
	SoftDelete bool
	NoRoutes   bool
	ReadOnly   bool
	UUIDImport string
	Fields     []*Field
	Relations  []*Relation
}

func (e *Entity) PK() *Field {
	for _, f := range e.Fields {
		if f.PK {
			return f
		}
	}
	return nil
}

func (e *Entity) Field(name string) *Field {
	for _, f := range e.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (e *Entity) FieldNames() []string {
	names := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		names[i] = f.Name
	}
	return names
}

func (e *Entity) BelongsTo() []*Field {
	var out []*Field
	for _, f := range e.Fields {
		if f.Ref != nil {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) Var() string {
	return naming.LowerCamel(e.Name)
}

func (e *Entity) Path() string {
	return "/" + naming.Kebab(e.Table)
}

func (e *Entity) PathParam() string {
	return naming.LowerCamel(e.Name) + "Id"
}

func (e *Entity) CreateFields() []*Field {
	var out []*Field
	for _, f := range e.Fields {
		if f.InCreate() {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) UpdateFields() []*Field {
	var out []*Field
	for _, f := range e.Fields {
		if f.InUpdate() {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) FilterFields() []*Field {
	var out []*Field
	for _, f := range e.Fields {
		if f.Filterable() {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) UniqueFields() []*Field {
	var out []*Field
	for _, f := range e.Fields {
		if f.Unique {
			out = append(out, f)
		}
	}
	return out
}

func (e *Entity) AutoCreateField() *Field {
	for _, f := range e.Fields {
		if f.AutoCreate {
			return f
		}
	}
	return nil
}

type OnDelete string

const (
	OnDeleteRestrict OnDelete = "restrict"
	OnDeleteCascade  OnDelete = "cascade"
	OnDeleteSetNull  OnDelete = "setnull"
)

type Reference struct {
	Entity   string
	OnDelete OnDelete
}

type Field struct {
	Name         string
	Kind         Kind
	Nullable     bool
	Column       string
	JSON         string
	Pos          Pos
	PK           bool
	AutoPK       bool
	AutoCreate   bool
	AutoUpdate   bool
	Required     bool
	NotEmpty     bool
	Unique       bool
	Index        bool
	Email        bool
	Min          string
	Max          string
	Enum         []string
	Default      string
	HasDefault   bool
	OmitCreate   bool
	OmitUpdate   bool
	OmitResponse bool
	Ref          *Reference
}

func (f *Field) GoType() string {
	if f.Nullable {
		return "*" + f.Kind.GoType()
	}
	return f.Kind.GoType()
}

func (f *Field) InCreate() bool {
	return !f.OmitCreate && !f.AutoPK && !f.AutoCreate && !f.AutoUpdate
}

func (f *Field) InUpdate() bool {
	return !f.OmitUpdate && !f.PK && !f.AutoCreate && !f.AutoUpdate
}

func (f *Field) Filterable() bool {
	if !f.Index && !f.Unique && f.Ref == nil {
		return false
	}
	switch f.Kind {
	case KindString, KindBool, KindUUID, KindInt16, KindInt32, KindInt, KindInt64:
		return true
	}
	return false
}

func (f *Field) Auto() bool {
	return f.AutoPK || f.AutoCreate || f.AutoUpdate
}

type RelationKind string

const (
	RelationHasMany    RelationKind = "has_many"
	RelationManyToMany RelationKind = "many_to_many"
)

type Relation struct {
	Name   string
	Kind   RelationKind
	Target string
	FK     string
	Via    string
	Pos    Pos
}

func (r *Relation) PathSegment() string {
	return naming.Kebab(r.Name)
}
