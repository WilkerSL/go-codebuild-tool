package schema

type ColumnKind string

const (
	ColumnString  ColumnKind = "string"
	ColumnBool    ColumnKind = "bool"
	ColumnInt16   ColumnKind = "int16"
	ColumnInt32   ColumnKind = "int32"
	ColumnInt64   ColumnKind = "int64"
	ColumnFloat32 ColumnKind = "float32"
	ColumnFloat64 ColumnKind = "float64"
	ColumnBytes   ColumnKind = "bytes"
	ColumnTime    ColumnKind = "time"
	ColumnUUID    ColumnKind = "uuid"
)

type DefaultKind string

const (
	DefaultLiteral DefaultKind = "literal"
	DefaultNow     DefaultKind = "now"
)

type Default struct {
	Kind  DefaultKind `json:"kind"`
	Value string      `json:"value,omitempty"`
}

type Column struct {
	Name     string     `json:"name"`
	Kind     ColumnKind `json:"kind"`
	Size     int        `json:"size,omitempty"`
	Nullable bool       `json:"nullable,omitempty"`
	Identity bool       `json:"identity,omitempty"`
	Default  *Default   `json:"default,omitempty"`
}

type PrimaryKey struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

type Index struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	Unique     bool     `json:"unique,omitempty"`
	NotDeleted bool     `json:"notDeleted,omitempty"`
}

type ForeignKey struct {
	Name       string   `json:"name"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"refTable"`
	RefColumns []string `json:"refColumns"`
	OnDelete   OnDelete `json:"onDelete"`
}

type Check struct {
	Name   string   `json:"name"`
	Column string   `json:"column"`
	Values []string `json:"values"`
}

type Table struct {
	Name        string        `json:"name"`
	Columns     []*Column     `json:"columns"`
	PrimaryKey  *PrimaryKey   `json:"primaryKey"`
	Indexes     []*Index      `json:"indexes,omitempty"`
	ForeignKeys []*ForeignKey `json:"foreignKeys,omitempty"`
	Checks      []*Check      `json:"checks,omitempty"`
}

func (t *Table) Column(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (t *Table) Index(name string) *Index {
	for _, i := range t.Indexes {
		if i.Name == name {
			return i
		}
	}
	return nil
}

func (t *Table) ForeignKey(name string) *ForeignKey {
	for _, fk := range t.ForeignKeys {
		if fk.Name == name {
			return fk
		}
	}
	return nil
}

func (t *Table) Check(name string) *Check {
	for _, c := range t.Checks {
		if c.Name == name {
			return c
		}
	}
	return nil
}

type Database struct {
	Tables []*Table `json:"tables"`
}

func (d *Database) Table(name string) *Table {
	if d == nil {
		return nil
	}
	for _, t := range d.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}
