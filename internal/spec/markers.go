package spec

import (
	"slices"
)

type markerScope int

const (
	scopeEntity markerScope = iota + 1
	scopeField
)

type valueMode int

const (
	valueNone valueMode = iota + 1
	valueRequired
	valueOptional
)

type MarkerDoc struct {
	Name        string
	Scope       string
	Syntax      string
	AppliesTo   string
	Description string
	Example     string
	CommonError string
}

type markerDef struct {
	name  string
	scope markerScope
	value valueMode
	doc   MarkerDoc
}

var markerDefs = []markerDef{
	{
		name: "table", scope: scopeEntity, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "table=<name>",
			AppliesTo:   "entity (`_ struct{}`)",
			Description: "Sets the table name. Defaults to the snake_case plural of the entity name (`User` -> `users`, `Category` -> `categories`). Must match `^[a-z_][a-z0-9_]*$` and have at most 63 characters.",
			Example:     "_ struct{} `gen:\"table=people\"`",
			CommonError: "`table=People` fails with invalid_value: table names must be lowercase SQL identifiers.",
		},
	},
	{
		name: "softdelete", scope: scopeEntity, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "softdelete",
			AppliesTo:   "entity (`_ struct{}`)",
			Description: "Adds a nullable `deleted_at` column. DELETE sets it instead of removing the row, reads ignore deleted rows and unique indexes only consider live rows where the dialect supports partial indexes.",
			Example:     "_ struct{} `gen:\"softdelete\"`",
			CommonError: "Declaring a `DeletedAt` field together with softdelete fails with duplicate_column: the column is added automatically.",
		},
	},
	{
		name: "noroutes", scope: scopeEntity, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "noroutes",
			AppliesTo:   "entity (`_ struct{}`)",
			Description: "Generates model, repository and service but no HTTP handler, routes or wiring. Relation routes that would live in this domain are not generated either.",
			Example:     "_ struct{} `gen:\"noroutes\"`",
			CommonError: "`noroutes=true` fails with invalid_value: the marker takes no value.",
		},
	},
	{
		name: "readonly", scope: scopeEntity, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "readonly",
			AppliesTo:   "entity (`_ struct{}`)",
			Description: "Only read operations are generated: list, get and relation listings. The repository has no create, update or delete methods. The table is still created by migrations.",
			Example:     "_ struct{} `gen:\"table=countries,readonly\"`",
			CommonError: "Putting `readonly` on a regular field fails with marker_scope: it is an entity marker.",
		},
	},
	{
		name: "pk", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "pk",
			AppliesTo:   "`uuid.UUID`, `int`, `int32`, `int64`, `string` (not pointers)",
			Description: "Marks the primary key. Every entity has exactly one. A primary key without `auto` must be sent by the client on create.",
			Example:     "ID uuid.UUID `gen:\"pk,auto\"`",
			CommonError: "An entity without pk fails with missing_pk; `ID *uuid.UUID` fails with invalid_combination because primary keys cannot be nullable.",
		},
	},
	{
		name: "auto", scope: scopeField, value: valueOptional,
		doc: MarkerDoc{
			Syntax:      "auto | auto=create | auto=update",
			AppliesTo:   "`auto`: pk of type `uuid.UUID` or integer; `auto=create` and `auto=update`: `time.Time`",
			Description: "`auto` on a UUID pk generates a version 7 UUID in Go; on an integer pk it uses the dialect identity column. `auto=create` is filled by the database clock on insert, `auto=update` on insert and on every update. Auto fields never appear in request bodies.",
			Example:     "CreatedAt time.Time `gen:\"auto=create\"`",
			CommonError: "`auto` on a non-pk field fails with invalid_combination and suggests `auto=create`.",
		},
	},
	{
		name: "required", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "required",
			AppliesTo:   "non-pointer fields",
			Description: "The JSON member must be present in create and update requests. Non-pointer `belongs_to` fields and client-provided primary keys are required implicitly.",
			Example:     "Name string `gen:\"required\"`",
			CommonError: "`Bio *string `gen:\"required\"`` fails with invalid_combination: remove the pointer or the marker.",
		},
	},
	{
		name: "notempty", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "notempty",
			AppliesTo:   "`string`, `[]byte` and their pointers",
			Description: "Rejects empty values. Strings are trimmed with `strings.TrimSpace` before the check.",
			Example:     "Name string `gen:\"required,notempty\"`",
			CommonError: "`notempty` on an `int` fails with invalid_combination; use `min=` for numbers.",
		},
	},
	{
		name: "unique", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "unique",
			AppliesTo:   "`string`, integers, `uuid.UUID`",
			Description: "Creates the unique index `ux_<table>_<column>`. Violations become HTTP 409 with the field name. The field also becomes an equality filter on the list endpoint.",
			Example:     "Email string `gen:\"required,unique,email,max=254\"`",
			CommonError: "A string without `max=` fails with dialect_limit on sqlserver and mysql, which cannot index unbounded text.",
		},
	},
	{
		name: "index", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "index",
			AppliesTo:   "`string`, integers, `bool`, `uuid.UUID`, `time.Time`",
			Description: "Creates the index `ix_<table>_<column>`. Indexed fields of type string, integer, bool and uuid become equality filters on the list endpoint.",
			Example:     "Status string `gen:\"enum=open|closed,default=open,index\"`",
			CommonError: "`unique,index` fails with invalid_combination: a unique index already indexes the column.",
		},
	},
	{
		name: "min", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "min=<n>",
			AppliesTo:   "`string` (characters), `[]byte` (bytes), integers and floats (value)",
			Description: "Lower bound validated in Go. For strings the length is counted in runes.",
			Example:     "Name string `gen:\"min=2,max=120\"`",
			CommonError: "`min=10,max=5` fails with invalid_value: min cannot be greater than max.",
		},
	},
	{
		name: "max", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "max=<n>",
			AppliesTo:   "`string` (characters), `[]byte` (bytes), integers and floats (value)",
			Description: "Upper bound validated in Go. On strings and byte slices it also sets the column size (`VARCHAR(n)`, `NVARCHAR(n)`, `VARBINARY(n)`).",
			Example:     "Title string `gen:\"max=200\"`",
			CommonError: "`max=abc` fails with invalid_value: the bound must be a number.",
		},
	},
	{
		name: "email", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "email",
			AppliesTo:   "`string` and `*string`",
			Description: "Validates the value with `net/mail.ParseAddress` and rejects display names such as `Ann <ann@example.com>`.",
			Example:     "Email string `gen:\"required,email\"`",
			CommonError: "`email,enum=a|b` fails with invalid_combination.",
		},
	},
	{
		name: "enum", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "enum=<a>|<b>|<c>",
			AppliesTo:   "`string` and `*string`",
			Description: "Restricts the value to the listed items, validated in Go and enforced with the CHECK constraint `ck_<table>_<column>`. Constants such as `RoleAdmin` are generated. Without `max=` the column size is the smallest of 32, 64, 128 or 255 that fits every item.",
			Example:     "Role string `gen:\"enum=admin|operator|viewer,default=viewer\"`",
			CommonError: "`enum=admin|admin` fails with invalid_value: items must be unique.",
		},
	},
	{
		name: "default", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "default=<value>",
			AppliesTo:   "`string`, `bool`, integers, floats and their pointers",
			Description: "Applied when the JSON member is absent (or null for pointers) and declared as the column DEFAULT. The value must satisfy the other markers of the field and cannot contain commas.",
			Example:     "Active bool `gen:\"default=true\"`",
			CommonError: "`required,default=x` fails with invalid_combination: a required member is never absent.",
		},
	},
	{
		name: "omit", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "omit=create|update|response",
			AppliesTo:   "any column field",
			Description: "Removes the field from the create request, the update request and/or the JSON response (`json:\"-\"`). Combine targets with `|`.",
			Example:     "PasswordHash string `gen:\"omit=response\"`",
			CommonError: "`required,omit=create` fails with invalid_combination.",
		},
	},
	{
		name: "column", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "column=<name>",
			AppliesTo:   "any column field",
			Description: "Overrides the column name. Defaults to the snake_case field name (`OrgID` -> `org_id`).",
			Example:     "Email string `gen:\"column=email_address\"`",
			CommonError: "Two fields mapped to the same column fail with duplicate_column.",
		},
	},
	{
		name: "rel", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "rel=belongs_to:<Entity> | rel=has_many:<Entity> | rel=many_to_many:<Entity>",
			AppliesTo:   "`belongs_to`: field with the type of the target primary key (pointer for an optional reference); `has_many` and `many_to_many`: `[]<Entity>`",
			Description: "`belongs_to` creates the foreign key `fk_<table>_<column>`, an index and a list filter. `has_many` generates `GET /<parents>/{parentId}/<field>` in the child domain. `many_to_many` requires `via=` and generates list, link and unlink routes in the target domain.",
			Example:     "OrgID uuid.UUID `gen:\"rel=belongs_to:Org\"`",
			CommonError: "`rel=belongs_to:Orgs` fails with unknown_entity and suggests `Org`.",
		},
	},
	{
		name: "ondelete", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "ondelete=restrict|cascade|setnull",
			AppliesTo:   "fields with `rel=belongs_to`",
			Description: "Foreign key action when the referenced row is deleted. Defaults to `restrict`, which turns the parent delete into HTTP 409. `setnull` requires a pointer field.",
			Example:     "OrgID uuid.UUID `gen:\"rel=belongs_to:Org,ondelete=cascade\"`",
			CommonError: "`ondelete=setnull` on a non-pointer field fails with invalid_combination.",
		},
	},
	{
		name: "fk", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "fk=<Field>",
			AppliesTo:   "fields with `rel=has_many`",
			Description: "Chooses which `belongs_to` field of the child entity backs the relation. Needed only when the child references the parent more than once.",
			Example:     "Owned []Project `gen:\"rel=has_many:Project,fk=OwnerID\"`",
			CommonError: "A child with two references to the parent and no fk= fails with relation_ambiguous.",
		},
	},
	{
		name: "via", scope: scopeField, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "via=<join_table>",
			AppliesTo:   "fields with `rel=many_to_many`",
			Description: "Names the join table. It gets the columns `<entity>_id` of both sides, a composite primary key and cascading foreign keys. Both sides of a relation must use the same table.",
			Example:     "Tags []Tag `gen:\"rel=many_to_many:Tag,via=user_tags\"`",
			CommonError: "`rel=many_to_many:Tag` without via= fails with invalid_value.",
		},
	},
}

func init() {
	for i := range markerDefs {
		markerDefs[i].doc.Name = markerDefs[i].name
		if markerDefs[i].scope == scopeEntity {
			markerDefs[i].doc.Scope = "entity"
		} else {
			markerDefs[i].doc.Scope = "field"
		}
	}
}

func lookupMarker(name string) (markerDef, bool) {
	for _, d := range markerDefs {
		if d.name == name {
			return d, true
		}
	}
	return markerDef{}, false
}

func markerNames(scope markerScope) []string {
	var names []string
	for _, d := range markerDefs {
		if d.scope == scope {
			names = append(names, d.name)
		}
	}
	slices.Sort(names)
	return names
}

func Markers() []MarkerDoc {
	docs := make([]MarkerDoc, len(markerDefs))
	for i, d := range markerDefs {
		docs[i] = d.doc
	}
	return docs
}
