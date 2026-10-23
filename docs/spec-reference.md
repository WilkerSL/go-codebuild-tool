# Spec reference

This file is generated from the marker registry of codebuild (`internal/spec`). Run `go test ./internal/spec -update` after changing markers.

## File layout

Every domain has one spec file in `specs/<domain>.go`. The file name is the domain name and becomes the Go package `internal/<domain>`, so it must contain only lowercase letters and digits.

```go
//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_         struct{}  `gen:"table=users,softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Email     string    `gen:"required,unique,email,max=254"`
	Role      string    `gen:"enum=admin|operator|viewer,default=viewer"`
	Bio       *string   `gen:"max=500"`
	OrgID     uuid.UUID `gen:"rel=belongs_to:Org"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
}
```

```ebnf
SpecFile   = "//go:build ignore" "package specs" [ Imports ] { AliasDecl } EntityDecl { AliasDecl } .
Imports    = "import" of "time" | "uuid" | "github.com/google/uuid" .
AliasDecl  = "type" Name "=" FieldType .
EntityDecl = "type" EntityName "struct" "{" [ MetaField ] { Field } "}" .
MetaField  = "_" "struct{}" GenTag .
Field      = FieldName FieldType [ GenTag ] .
GenTag     = "`gen:\"" Marker { "," Marker } "\"`" .
Marker     = Key [ "=" Value ] .
RelValue   = ( "belongs_to" | "has_many" | "many_to_many" ) ":" EntityName .
EnumValue  = Item { "|" Item } .
OmitValue  = Target { "|" Target } .
Target     = "create" | "update" | "response" .
AutoValue  = "create" | "update" .
```

Rules:

- The first line is `//go:build ignore`, which keeps specs out of normal builds and out of `go mod tidy` (it reads every other build tag, so a constraint such as `spec` would make it resolve the imports of the specs). gofmt and the type checker keep working.
- The package is `specs` and the only imports are `time`, `uuid` (Go 1.27+) and `github.com/google/uuid` (Go 1.26). Both UUID packages are accepted for every Go version.
- Exactly one entity struct per file. Type aliases such as `type Email = string` are allowed and resolved.
- Fields are exported and not embedded. Tags only use the `gen` key; JSON tags are generated.
- Entity markers go on a single `_ struct{}` field.

## Supported field types

| Go type | Meaning |
|---|---|
| `string` | text, `VARCHAR(max)` when `max=` is set |
| `bool` | boolean |
| `int16`, `int32`, `int`, `int64` | integers (`int` is stored as a 64-bit column) |
| `float32`, `float64` | floating point |
| `[]byte` | binary, base64 in JSON |
| `time.Time` | timestamp, stored in UTC, RFC 3339 in JSON |
| `uuid.UUID` | UUID, string in JSON |
| `*T` of any type above | nullable column; `null` and an absent member are equivalent |
| `[]Entity` | virtual relation field, only with `rel=has_many` or `rel=many_to_many` |

## Naming conventions

| Item | Rule | Example |
|---|---|---|
| Domain and package | spec file name | `specs/users.go` -> `internal/users` |
| Table | snake_case plural of the entity, or `table=` | `OrderItem` -> `order_items` |
| Column | snake_case of the field, or `column=` | `OrgID` -> `org_id` |
| JSON member | lowerCamelCase of the field | `OrgID` -> `orgId` |
| Route | kebab-case of the table | `order_items` -> `/order-items` |
| Constraints | `pk_`, `ux_`, `ix_`, `fk_`, `ck_`, `df_` + table + column, shortened with a hash past 63 characters | `ux_users_email` |

## Entity markers

### `table`

- Syntax: `table=<name>`
- Applies to: entity (`_ struct{}`)
- Effect: Sets the table name. Defaults to the snake_case plural of the entity name (`User` -> `users`, `Category` -> `categories`). Must match `^[a-z_][a-z0-9_]*$` and have at most 63 characters.
- Example: `_ struct{} `gen:"table=people"``
- Common error: `table=People` fails with invalid_value: table names must be lowercase SQL identifiers.

### `softdelete`

- Syntax: `softdelete`
- Applies to: entity (`_ struct{}`)
- Effect: Adds a nullable `deleted_at` column. DELETE sets it instead of removing the row, reads ignore deleted rows and unique indexes only consider live rows where the dialect supports partial indexes. Foreign keys pointing at the entity keep their meaning: `restrict` refuses the delete with HTTP 409 while live rows reference it, `setnull` clears the references and `cascade` hides the referencing rows, and their own cascading children, from every query. Creating or updating a row that references a deleted one answers 422.
- Example: `_ struct{} `gen:"softdelete"``
- Common error: Declaring a `DeletedAt` field together with softdelete fails with duplicate_column: the column is added automatically.

### `noroutes`

- Syntax: `noroutes`
- Applies to: entity (`_ struct{}`)
- Effect: Generates model, repository and service but no HTTP handler, routes or wiring. Relation routes that would live in this domain are not generated either.
- Example: `_ struct{} `gen:"noroutes"``
- Common error: `noroutes=true` fails with invalid_value: the marker takes no value.

### `readonly`

- Syntax: `readonly`
- Applies to: entity (`_ struct{}`)
- Effect: Only read operations are generated: list, get and relation listings. The repository has no create, update or delete methods. The table is still created by migrations.
- Example: `_ struct{} `gen:"table=countries,readonly"``
- Common error: Putting `readonly` on a regular field fails with marker_scope: it is an entity marker.

### `auth`

- Syntax: `auth=<method>|<method>`
- Applies to: entity (`_ struct{}`) of the account entity; one entity per project
- Effect: Turns the entity into the account entity and generates authentication. Methods: `session` (opaque tokens stored as SHA-256 hashes in `auth_sessions`) or `jwt` (HS256 access tokens plus rotating refresh tokens in `auth_refresh_tokens`), optionally combined with `apikey` (keys stored as hashes in `auth_api_keys`). The entity needs one `identity` and one `password` field. Once a project has an account entity, every route requires authentication unless the entity says `public`.
- Example: `_ struct{} `gen:"auth=session|apikey,register,roles=admin"``
- Common error: `auth=session|jwt` fails with invalid_value: choose one login method, session or jwt.

### `register`

- Syntax: `register`
- Applies to: the entity with `auth=`
- Effect: Generates `POST /auth/register`, which creates an account from the create fields plus `password`. The `role` field is not accepted there and always takes its default. The endpoint is rate limited per client IP.
- Example: `_ struct{} `gen:"auth=jwt,register"``
- Common error: `register` with a `role` field that has no `default=` fails with invalid_combination.

### `public`

- Syntax: `public | public=read|create|update|delete`
- Applies to: entities of a project with an account entity
- Effect: Opens the listed operations to anonymous callers; without a value every operation is public. `read` covers list, get and relation listings. Everything else keeps requiring authentication.
- Example: `_ struct{} `gen:"public=read,writeroles=admin"``
- Common error: `public` on an entity with an `owner` field fails with invalid_combination: owned rows always need a caller.

### `roles`

- Syntax: `roles=<role>|<role>`
- Applies to: entities of a project whose account entity has a `role` field
- Effect: Restricts every protected operation to callers with one of the roles. Callers without it get HTTP 403. Roles must be items of the `role` enum.
- Example: `_ struct{} `gen:"roles=admin"``
- Common error: `roles=root` fails with invalid_value when root is not an item of the role enum.

### `writeroles`

- Syntax: `writeroles=<role>|<role>`
- Applies to: entities of a project whose account entity has a `role` field
- Effect: Roles required for create, update, delete, link and unlink. Overrides `roles` for those operations, so `roles=admin|member,writeroles=admin` lets members read and only admins write. A link writes the join table of both entities, so the write roles of either side apply to the link routes of both.
- Example: `_ struct{} `gen:"public=read,writeroles=admin"``
- Common error: `writeroles` on a readonly entity fails with invalid_combination: it has no write operations.

## Field markers

### `pk`

- Syntax: `pk`
- Applies to: `uuid.UUID`, `int`, `int32`, `int64`, `string` (not pointers)
- Effect: Marks the primary key. Every entity has exactly one. A primary key without `auto` must be sent by the client on create.
- Example: `ID uuid.UUID `gen:"pk,auto"``
- Common error: An entity without pk fails with missing_pk; `ID *uuid.UUID` fails with invalid_combination because primary keys cannot be nullable.

### `auto`

- Syntax: `auto | auto=create | auto=update`
- Applies to: `auto`: pk of type `uuid.UUID` or integer; `auto=create` and `auto=update`: `time.Time`
- Effect: `auto` on a UUID pk generates a version 7 UUID in Go; on an integer pk it uses the dialect identity column. `auto=create` is filled by the database clock on insert and orders lists newest first, backed by the index `lx_<table>` on the creation time and the primary key; `auto=update` is filled on insert and on every update. Auto fields never appear in request bodies.
- Example: `CreatedAt time.Time `gen:"auto=create"``
- Common error: `auto` on a non-pk field fails with invalid_combination and suggests `auto=create`.

### `required`

- Syntax: `required`
- Applies to: non-pointer fields
- Effect: The JSON member must be present in create and update requests. Non-pointer `belongs_to` fields and client-provided primary keys are required implicitly.
- Example: `Name string `gen:"required"``
- Common error: `Bio *string `gen:"required"`` fails with invalid_combination: remove the pointer or the marker.

### `notempty`

- Syntax: `notempty`
- Applies to: `string`, `[]byte` and their pointers
- Effect: Rejects empty values. Strings are trimmed with `strings.TrimSpace` before the check.
- Example: `Name string `gen:"required,notempty"``
- Common error: `notempty` on an `int` fails with invalid_combination; use `min=` for numbers.

### `unique`

- Syntax: `unique`
- Applies to: `string`, integers, `uuid.UUID`
- Effect: Creates the unique index `ux_<table>_<column>`. Violations become HTTP 409 with the field name. The field also becomes an equality filter on the list endpoint.
- Example: `Email string `gen:"required,unique,email,max=254"``
- Common error: A string without `max=` fails with dialect_limit on sqlserver and mysql, which cannot index unbounded text.

### `index`

- Syntax: `index`
- Applies to: `string`, integers, `bool`, `uuid.UUID`, `time.Time`
- Effect: Creates the index `ix_<table>_<column>`. Indexed fields of type string, integer, bool and uuid become equality filters on the list endpoint.
- Example: `Status string `gen:"enum=open|closed,default=open,index"``
- Common error: `unique,index` fails with invalid_combination: a unique index already indexes the column.

### `min`

- Syntax: `min=<n>`
- Applies to: `string` (characters), `[]byte` (bytes), integers and floats (value)
- Effect: Lower bound validated in Go. For strings the length is counted in runes.
- Example: `Name string `gen:"min=2,max=120"``
- Common error: `min=10,max=5` fails with invalid_value: min cannot be greater than max.

### `max`

- Syntax: `max=<n>`
- Applies to: `string` (characters), `[]byte` (bytes), integers and floats (value)
- Effect: Upper bound validated in Go. On strings and byte slices it also sets the column size (`VARCHAR(n)`, `NVARCHAR(n)`, `VARBINARY(n)`).
- Example: `Title string `gen:"max=200"``
- Common error: `max=abc` fails with invalid_value: the bound must be a number.

### `email`

- Syntax: `email`
- Applies to: `string` and `*string`
- Effect: Validates the value with `net/mail.ParseAddress` and rejects display names such as `Ann <ann@example.com>`.
- Example: `Email string `gen:"required,email"``
- Common error: `email,enum=a|b` fails with invalid_combination.

### `enum`

- Syntax: `enum=<a>|<b>|<c>`
- Applies to: `string` and `*string`
- Effect: Restricts the value to the listed items, validated in Go and enforced with the CHECK constraint `ck_<table>_<column>`. Constants such as `RoleAdmin` are generated. Without `max=` the column size is the smallest of 32, 64, 128 or 255 that fits every item.
- Example: `Role string `gen:"enum=admin|operator|viewer,default=viewer"``
- Common error: `enum=admin|admin` fails with invalid_value: items must be unique.

### `default`

- Syntax: `default=<value>`
- Applies to: `string`, `bool`, integers, floats and their pointers
- Effect: Applied when the JSON member is absent (or null for pointers) and declared as the column DEFAULT. The value must satisfy the other markers of the field and cannot contain commas.
- Example: `Active bool `gen:"default=true"``
- Common error: `required,default=x` fails with invalid_combination: a required member is never absent.

### `omit`

- Syntax: `omit=create|update|response`
- Applies to: any column field
- Effect: Removes the field from the create request, the update request and/or the JSON response (`json:"-"`). Combine targets with `|`.
- Example: `PasswordHash string `gen:"omit=response"``
- Common error: `required,omit=create` fails with invalid_combination.

### `column`

- Syntax: `column=<name>`
- Applies to: any column field
- Effect: Overrides the column name. Defaults to the snake_case field name (`OrgID` -> `org_id`).
- Example: `Email string `gen:"column=email_address"``
- Common error: Two fields mapped to the same column fail with duplicate_column.

### `rel`

- Syntax: `rel=belongs_to:<Entity> | rel=has_many:<Entity> | rel=many_to_many:<Entity>`
- Applies to: `belongs_to`: field with the type of the target primary key (pointer for an optional reference); `has_many` and `many_to_many`: `[]<Entity>`
- Effect: `belongs_to` creates the foreign key `fk_<table>_<column>`, the index `ix_<table>_<column>`, the index `lx_<table>_<column>` in list order (the column, then the creation time and the primary key) so the lists of one parent or owner read a page in order, and a list filter. `has_many` generates `GET /<parents>/{parentId}/<field>` in the child domain. `many_to_many` requires `via=` and generates list, link and unlink routes in the target domain.
- Example: `OrgID uuid.UUID `gen:"rel=belongs_to:Org"``
- Common error: `rel=belongs_to:Orgs` fails with unknown_entity and suggests `Org`.

### `ondelete`

- Syntax: `ondelete=restrict|cascade|setnull`
- Applies to: fields with `rel=belongs_to`
- Effect: Foreign key action when the referenced row is deleted. Defaults to `restrict`, which turns the parent delete into HTTP 409. `setnull` requires a pointer field.
- Example: `OrgID uuid.UUID `gen:"rel=belongs_to:Org,ondelete=cascade"``
- Common error: `ondelete=setnull` on a non-pointer field fails with invalid_combination.

### `fk`

- Syntax: `fk=<Field>`
- Applies to: fields with `rel=has_many`
- Effect: Chooses which `belongs_to` field of the child entity backs the relation. Needed only when the child references the parent more than once.
- Example: `Owned []Project `gen:"rel=has_many:Project,fk=OwnerID"``
- Common error: A child with two references to the parent and no fk= fails with relation_ambiguous.

### `identity`

- Syntax: `identity`
- Applies to: `string` field of the account entity, together with `unique`
- Effect: The login name sent to `POST /auth/login` under the field's JSON name. Email identities are lowercased on create, update and login.
- Example: `Email string `gen:"required,unique,email,max=254,identity"``
- Common error: `identity` without `unique` fails with invalid_combination.

### `password`

- Syntax: `password`
- Applies to: `string` field of the account entity, conventionally named `PasswordHash`
- Effect: Stores a PBKDF2-SHA256 hash (600,000 iterations, random salt). Requests send the plain text as `password` (the field name without the `Hash` suffix) on create and register only; responses never include it. `min=` sets the minimum length (default and lowest value 8); the maximum is 1024 characters. Passwords change through `PUT /auth/password`.
- Example: `PasswordHash string `gen:"password,min=12"``
- Common error: `password,max=64` fails with invalid_combination: the column size is fixed for the hash.

### `role`

- Syntax: `role`
- Applies to: `string` field with `enum=` on the account entity
- Effect: The role checked by `roles=` and `writeroles=`. Write operations on the account entity must then be restricted with `roles=` or `writeroles=`, otherwise any user could promote themselves.
- Example: `Role string `gen:"enum=admin|member,default=member,role"``
- Common error: `role` without `enum=` fails with invalid_combination.

### `owner`

- Syntax: `owner`
- Applies to: non-pointer `rel=belongs_to:<account entity>` field
- Effect: Scopes every query of the entity to the authenticated caller: the field is filled from the caller on create, never accepted in requests, and rows of other owners answer 404. Repository and service methods take the owner as an explicit parameter.
- Example: `OwnerID uuid.UUID `gen:"rel=belongs_to:User,ondelete=cascade,owner"``
- Common error: `owner` on a reference to an entity without `auth=` fails with invalid_combination.

### `via`

- Syntax: `via=<join_table>`
- Applies to: fields with `rel=many_to_many`
- Effect: Names the join table. It gets the columns `<entity>_id` of both sides, a composite primary key and cascading foreign keys. Both sides of a relation must use the same table.
- Example: `Tags []Tag `gen:"rel=many_to_many:Tag,via=user_tags"``
- Common error: `rel=many_to_many:Tag` without via= fails with invalid_value.

## Microservices

A spec file may declare one `XxxService` struct instead of an entity. Each field written as `Invoice struct{ ... }` is an entity of that service, with the same markers as a top-level entity; its domain is the snake_case plural of its name without underscores (`InvoiceItem` -> `invoiceitems`). The file is named after the service (`BillingService` -> `specs/billing.go`), and once a project declares a service every entity must live in one. Each service becomes its own Go module exposing gRPC, and a generated gateway is the only REST API. A workspace needs one account entity (`auth=`): the gateway authenticates every request through the service that owns it. A `belongs_to` reference to an entity stored in another database has no foreign key; the service checks it with the gRPC `Exists` call of the owning service and answers 422 when it is missing. `many_to_many` relations need both entities in the same database. `docs/microservices.md` describes the generated workspace.

```go
type BillingService struct {
	_       struct{} `gen:"database=own"`
	Invoice struct {
		ID     uuid.UUID `gen:"pk,auto"`
		UserID uuid.UUID `gen:"rel=belongs_to:User"`
	}
}
```

## Service markers

### `database`

- Syntax: `database=own|shared`
- Applies to: service (`_ struct{}` inside a `XxxService` struct); required
- Effect: Chooses the database of a microservice. `own` gives the service its own database, migrations and snapshot; references to entities of other services become IDs without foreign keys, checked with a gRPC call to the owning service. `shared` puts the service in the database shared by every `shared` service, where foreign keys between them keep working.
- Example: `_ struct{} `gen:"database=own"``
- Common error: A service without `database=` fails with invalid_value: the choice is explicit.

## Request semantics

- Request bodies must be sent with `Content-Type: application/json` (or another `+json` media type); other content types answer HTTP 415.
- Create (`POST`) and update (`PUT`, full replacement) requests reject unknown members with HTTP 400.
- On create, an absent member takes the `default=` value, or the zero value (an empty value for `[]byte`). Pointer fields treat `null` like an absent member.
- `PUT` replaces the whole resource, so every non-pointer member is required: leaving one out answers 422 instead of resetting it to its default. Pointer members left out become `null`.
- `required` members must be present. Non-pointer `belongs_to` fields and client-provided primary keys are required implicitly.
- Validation rules apply to every value of non-pointer fields and to non-nil values of pointer fields. Use a pointer for optional values.
- Fields with `auto`, `auto=create`, `auto=update` or the primary key never appear in update requests.
- Validation failures return HTTP 422 with RFC 9457 problem details and an `errors` array of `{field, message}`.
- Lists are ordered newest first by the `auto=create` field with the primary key breaking ties, or by the primary key when there is none. A list response is `{items, limit, offset, total, nextCursor}`; `nextCursor` is `null` on the last page.
- Lists page by `offset` or by `cursor`. Passing the `nextCursor` of a page as `?cursor=` returns the rows after it without counting them, so the response has no `total`; it stays correct while rows are added and is faster on large tables. A cursor cannot be combined with `offset`, and a malformed one answers 400.

## Generated routes

| Method and path | Purpose | Notes |
|---|---|---|
| `GET /<table>` | list | `limit` with `offset` or `cursor`, and equality filters on indexed, unique and belongs_to fields |
| `POST /<table>` | create | 201 with `Location`; skipped for `readonly` |
| `GET /<table>/{id}` | get | 404 when missing or soft deleted |
| `PUT /<table>/{id}` | full update | skipped for `readonly` |
| `DELETE /<table>/{id}` | delete | soft delete with `softdelete`; 409 when referenced |
| `GET /<parents>/{parentId}/<field>` | has_many | generated in the child domain |
| `GET /<sources>/{sourceId}/<field>` | many_to_many list | generated in the target domain |
| `PUT` and `DELETE /<sources>/{sourceId}/<field>/{targetId}` | many_to_many link and unlink | 204, idempotent |
| `GET /openapi.json` | OpenAPI 3.1 description of every route | the same document as `openapi.json` at the project root, regenerated on every build |

## Diagnostic codes

Diagnostics are printed as `file:line:column: message (did you mean "x"?) [code]` and as JSON objects with `file`, `line`, `column`, `severity`, `code`, `message` and `suggestion` when `--json` is used.

| Code | Meaning |
|---|---|
| `syntax` | The file is not valid Go. |
| `build_tag` | The file does not start with `//go:build ignore`, or uses a constraint such as `//go:build spec` that `go mod tidy` still reads. |
| `package_name` | The package clause is not `package specs`. |
| `unsupported_import` | An import other than `time`, `uuid` or `github.com/google/uuid`. |
| `unsupported_decl` | A declaration other than the entity struct or a type alias. |
| `type_error` | The Go type checker rejected the file (undefined names, unused imports). |
| `entity_count` | The file declares zero or more than one entity. |
| `domain_name` | The file name is not a valid, unreserved Go package name. |
| `reserved_name` | An entity or field name collides with generated code. |
| `unexported_field` | A field is not exported. |
| `embedded_field` | A field is embedded. |
| `invalid_tag` | The struct tag is malformed or `_ struct{}` is repeated. |
| `unsupported_tag_key` | A tag key other than `gen` is used. |
| `unsupported_type` | The field type is not supported. |
| `unknown_marker` | The marker does not exist; the closest marker is suggested. |
| `marker_scope` | An entity marker is used on a field or the other way around. |
| `duplicate_marker` | A marker is repeated on the same field. |
| `invalid_value` | A marker value is malformed or out of range. |
| `invalid_combination` | Markers or types that cannot be combined. |
| `missing_pk` | The entity has no primary key. |
| `multiple_pk` | The entity has more than one primary key. |
| `unknown_entity` | A relation targets an entity that does not exist. |
| `unknown_field` | `fk=` names a field that does not exist. |
| `relation_type_mismatch` | A relation field has the wrong type or no matching back reference. |
| `relation_ambiguous` | A has_many relation matches several belongs_to fields; add `fk=`. |
| `relation_cycle` | Non-nullable belongs_to references form a cycle. |
| `relation_routes` | Warning: relation routes are skipped because the target domain has noroutes. |
| `duplicate_column` | Two fields map to the same column, or a field collides with `deleted_at`. |
| `duplicate_json` | Two fields map to the same JSON member. |
| `duplicate_table` | Two entities or join tables share a table name. |
| `name_collision` | A generated identifier or query parameter would collide. |
| `dialect_limit` | The configured SQL dialect cannot represent the field (for example an unbounded indexed string on sqlserver or mysql). |
| `dialect_cascade` | Warning on sqlserver: several cascading paths reach the same table, which SQL Server rejects. |
| `toolchain_feature` | Warning: the spec imports the standard library `uuid` package while the project targets Go 1.26. |
