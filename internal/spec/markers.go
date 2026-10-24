package spec

import (
	"slices"
)

type markerScope int

const (
	scopeEntity markerScope = iota + 1
	scopeField
	scopeService
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
		name: "database", scope: scopeService, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "database=own|shared",
			AppliesTo:   "service (`_ struct{}` inside a `XxxService` struct); required",
			Description: "Chooses the database of a microservice. `own` gives the service its own database, migrations and snapshot; references to entities of other services become IDs without foreign keys, checked with a gRPC call to the owning service. `shared` puts the service in the database shared by every `shared` service, where foreign keys between them keep working.",
			Example:     "_ struct{} `gen:\"database=own\"`",
			CommonError: "A service without `database=` fails with invalid_value: the choice is explicit.",
		},
	},
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
			Description: "Adds a nullable `deleted_at` column. DELETE sets it instead of removing the row, reads ignore deleted rows and unique indexes only consider live rows where the dialect supports partial indexes. Foreign keys pointing at the entity keep their meaning: `restrict` refuses the delete with HTTP 409 while live rows reference it, `setnull` clears the references and `cascade` hides the referencing rows, and their own cascading children, from every query. Creating or updating a row that references a deleted one answers 422.",
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
		name: "auth", scope: scopeEntity, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "auth=<method>|<method>",
			AppliesTo:   "entity (`_ struct{}`) of the account entity; one entity per project",
			Description: "Turns the entity into the account entity and generates authentication. Methods: `session` (opaque tokens stored as SHA-256 hashes in `auth_sessions`) or `jwt` (HS256 access tokens plus rotating refresh tokens in `auth_refresh_tokens`), optionally combined with `apikey` (keys stored as hashes in `auth_api_keys`). The entity needs one `identity` and one `password` field. Once a project has an account entity, every route requires authentication unless the entity says `public`.",
			Example:     "_ struct{} `gen:\"auth=session|apikey,register,roles=admin\"`",
			CommonError: "`auth=session|jwt` fails with invalid_value: choose one login method, session or jwt.",
		},
	},
	{
		name: "register", scope: scopeEntity, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "register",
			AppliesTo:   "the entity with `auth=`",
			Description: "Generates `POST /auth/register`, which creates an account from the create fields plus `password`. The `role` field is not accepted there and always takes its default. The endpoint is rate limited per client IP.",
			Example:     "_ struct{} `gen:\"auth=jwt,register\"`",
			CommonError: "`register` with a `role` field that has no `default=` fails with invalid_combination.",
		},
	},
	{
		name: "public", scope: scopeEntity, value: valueOptional,
		doc: MarkerDoc{
			Syntax:      "public | public=read|create|update|delete",
			AppliesTo:   "entities of a project with an account entity",
			Description: "Opens the listed operations to anonymous callers; without a value every operation is public. `read` covers list, get and relation listings. Everything else keeps requiring authentication.",
			Example:     "_ struct{} `gen:\"public=read,writeroles=admin\"`",
			CommonError: "`public` on an entity with an `owner` field fails with invalid_combination: owned rows always need a caller.",
		},
	},
	{
		name: "roles", scope: scopeEntity, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "roles=<role>|<role>",
			AppliesTo:   "entities of a project whose account entity has a `role` field",
			Description: "Restricts every protected operation to callers with one of the roles. Callers without it get HTTP 403. Roles must be items of the `role` enum.",
			Example:     "_ struct{} `gen:\"roles=admin\"`",
			CommonError: "`roles=root` fails with invalid_value when root is not an item of the role enum.",
		},
	},
	{
		name: "writeroles", scope: scopeEntity, value: valueRequired,
		doc: MarkerDoc{
			Syntax:      "writeroles=<role>|<role>",
			AppliesTo:   "entities of a project whose account entity has a `role` field",
			Description: "Roles required for create, update, delete, link and unlink. Overrides `roles` for those operations, so `roles=admin|member,writeroles=admin` lets members read and only admins write. A link writes the join table of both entities, so the write roles of either side apply to the link routes of both.",
			Example:     "_ struct{} `gen:\"public=read,writeroles=admin\"`",
			CommonError: "`writeroles` on a readonly entity fails with invalid_combination: it has no write operations.",
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
			Description: "`auto` on a UUID pk generates a version 7 UUID in Go; on an integer pk it uses the dialect identity column. `auto=create` is filled by the database clock on insert and orders lists newest first, backed by the index `lx_<table>` on the creation time and the primary key; `auto=update` is filled on insert and on every update. Auto fields never appear in request bodies.",
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
			Description: "`belongs_to` creates the foreign key `fk_<table>_<column>`, the index `ix_<table>_<column>`, the index `lx_<table>_<column>` in list order (the column, then the creation time and the primary key) so the lists of one parent or owner read a page in order, and a list filter. `has_many` generates `GET /<parents>/{parentId}/<field>` in the child domain. `many_to_many` requires `via=` and generates list, link and unlink routes in the target domain.",
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
		name: "identity", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "identity",
			AppliesTo:   "`string` field of the account entity, together with `unique`",
			Description: "The login name sent to `POST /auth/login` under the field's JSON name. Email identities are lowercased on create, update and login.",
			Example:     "Email string `gen:\"required,unique,email,max=254,identity\"`",
			CommonError: "`identity` without `unique` fails with invalid_combination.",
		},
	},
	{
		name: "password", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "password",
			AppliesTo:   "`string` field of the account entity, conventionally named `PasswordHash`",
			Description: "Stores a PBKDF2-SHA256 hash (600,000 iterations, random salt). Requests send the plain text as `password` (the field name without the `Hash` suffix) on create and register only; responses never include it. `min=` sets the minimum length (default and lowest value 8); the maximum is 1024 characters. Passwords change through `PUT /auth/password`.",
			Example:     "PasswordHash string `gen:\"password,min=12\"`",
			CommonError: "`password,max=64` fails with invalid_combination: the column size is fixed for the hash.",
		},
	},
	{
		name: "role", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "role",
			AppliesTo:   "`string` field with `enum=` on the account entity",
			Description: "The role checked by `roles=` and `writeroles=`. Write operations on the account entity must then be restricted with `roles=` or `writeroles=`, otherwise any user could promote themselves.",
			Example:     "Role string `gen:\"enum=admin|member,default=member,role\"`",
			CommonError: "`role` without `enum=` fails with invalid_combination.",
		},
	},
	{
		name: "owner", scope: scopeField, value: valueNone,
		doc: MarkerDoc{
			Syntax:      "owner",
			AppliesTo:   "non-pointer `rel=belongs_to:<account entity>` field",
			Description: "Scopes every query of the entity to the authenticated caller: the field is filled from the caller on create, never accepted in requests, and rows of other owners answer 404. Repository and service methods take the owner as an explicit parameter. Entities without an owner field that reference an owned entity through a required `belongs_to` in the same database inherit the scope on all their routes: an order item belongs to the customer of its order. They cannot be `public`.",
			Example:     "OwnerID uuid.UUID `gen:\"rel=belongs_to:User,ondelete=cascade,owner\"`",
			CommonError: "`owner` on a reference to an entity without `auth=` fails with invalid_combination.",
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
		switch markerDefs[i].scope {
		case scopeEntity:
			markerDefs[i].doc.Scope = "entity"
		case scopeService:
			markerDefs[i].doc.Scope = "service"
		default:
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
