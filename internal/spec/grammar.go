package spec

import (
	"fmt"
	"strings"
)

type DiagnosticDoc struct {
	Code    string
	Meaning string
}

var diagnosticDocs = []DiagnosticDoc{
	{CodeSyntax, "The file is not valid Go."},
	{CodeBuildTag, "The file does not start with `//go:build ignore`, or uses a constraint such as `//go:build spec` that `go mod tidy` still reads."},
	{CodePackageName, "The package clause is not `package specs`."},
	{CodeUnsupportedImport, "An import other than `time`, `uuid` or `github.com/google/uuid`."},
	{CodeUnsupportedDecl, "A declaration other than the entity struct or a type alias."},
	{CodeTypeError, "The Go type checker rejected the file (undefined names, unused imports)."},
	{CodeEntityCount, "The file declares zero or more than one entity."},
	{CodeDomainName, "The file name is not a valid, unreserved Go package name."},
	{CodeReservedName, "An entity or field name collides with generated code."},
	{CodeUnexportedField, "A field is not exported."},
	{CodeEmbeddedField, "A field is embedded."},
	{CodeInvalidTag, "The struct tag is malformed or `_ struct{}` is repeated."},
	{CodeUnsupportedTagKey, "A tag key other than `gen` is used."},
	{CodeUnsupportedType, "The field type is not supported."},
	{CodeUnknownMarker, "The marker does not exist; the closest marker is suggested."},
	{CodeMarkerScope, "An entity marker is used on a field or the other way around."},
	{CodeDuplicateMarker, "A marker is repeated on the same field."},
	{CodeInvalidValue, "A marker value is malformed or out of range."},
	{CodeInvalidCombination, "Markers or types that cannot be combined."},
	{CodeMissingPK, "The entity has no primary key."},
	{CodeMultiplePK, "The entity has more than one primary key."},
	{CodeUnknownEntity, "A relation targets an entity that does not exist."},
	{CodeUnknownField, "`fk=` names a field that does not exist."},
	{CodeRelationTypeMismatch, "A relation field has the wrong type or no matching back reference."},
	{CodeRelationAmbiguous, "A has_many relation matches several belongs_to fields; add `fk=`."},
	{CodeRelationCycle, "Non-nullable belongs_to references form a cycle."},
	{CodeRelationRoutes, "Warning: relation routes are skipped because the target domain has noroutes."},
	{CodeDuplicateColumn, "Two fields map to the same column, or a field collides with `deleted_at`."},
	{CodeDuplicateJSON, "Two fields map to the same JSON member."},
	{CodeDuplicateTable, "Two entities or join tables share a table name."},
	{CodeNameCollision, "A generated identifier or query parameter would collide."},
	{"dialect_limit", "The configured SQL dialect cannot represent the field (for example an unbounded indexed string on sqlserver or mysql)."},
	{"dialect_cascade", "Warning on sqlserver: several cascading paths reach the same table, which SQL Server rejects."},
	{"toolchain_feature", "Warning: the spec imports the standard library `uuid` package while the project targets Go 1.26."},
}

func Diagnostics() []DiagnosticDoc {
	return diagnosticDocs
}

func Reference() string {
	var b strings.Builder
	w := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
	}
	w("# Spec reference\n\n")
	w("This file is generated from the marker registry of codebuild (`internal/spec`). Run `go test ./internal/spec -update` after changing markers.\n\n")
	w("## File layout\n\n")
	w("Every domain has one spec file in `specs/<domain>.go`. The file name is the domain name and becomes the Go package `internal/<domain>`, so it must contain only lowercase letters and digits.\n\n")
	w("```go\n%s```\n\n", exampleSpec)
	w("```ebnf\n%s```\n\n", grammarEBNF)
	w("Rules:\n\n")
	w("- The first line is `//go:build ignore`, which keeps specs out of normal builds and out of `go mod tidy` (it reads every other build tag, so a constraint such as `spec` would make it resolve the imports of the specs). gofmt and the type checker keep working.\n")
	w("- The package is `specs` and the only imports are `time`, `uuid` (Go 1.27+) and `github.com/google/uuid` (Go 1.26). Both UUID packages are accepted for every Go version.\n")
	w("- Exactly one entity struct per file. Type aliases such as `type Email = string` are allowed and resolved.\n")
	w("- Fields are exported and not embedded. Tags only use the `gen` key; JSON tags are generated.\n")
	w("- Entity markers go on a single `_ struct{}` field.\n\n")
	w("## Supported field types\n\n")
	w("| Go type | Meaning |\n|---|---|\n")
	w("| `string` | text, `VARCHAR(max)` when `max=` is set |\n")
	w("| `bool` | boolean |\n")
	w("| `int16`, `int32`, `int`, `int64` | integers (`int` is stored as a 64-bit column) |\n")
	w("| `float32`, `float64` | floating point; use `string` with `decimal=` for money |\n")
	w("| `[]byte` | binary, base64 in JSON |\n")
	w("| `time.Time` | timestamp, stored in UTC, RFC 3339 in JSON |\n")
	w("| `uuid.UUID` | UUID, string in JSON |\n")
	w("| `*T` of any type above | nullable column; `null` and an absent member are equivalent |\n")
	w("| `[]Entity` | virtual relation field, only with `rel=has_many` or `rel=many_to_many` |\n\n")
	w("## Naming conventions\n\n")
	w("| Item | Rule | Example |\n|---|---|---|\n")
	w("| Domain and package | spec file name | `specs/users.go` -> `internal/users` |\n")
	w("| Table | snake_case plural of the entity, or `table=` | `OrderItem` -> `order_items` |\n")
	w("| Column | snake_case of the field, or `column=` | `OrgID` -> `org_id` |\n")
	w("| JSON member | lowerCamelCase of the field | `OrgID` -> `orgId` |\n")
	w("| Route | kebab-case of the table | `order_items` -> `/order-items` |\n")
	w("| Constraints | `pk_`, `ux_`, `ix_`, `fk_`, `ck_`, `df_` + table + column, shortened with a hash past 63 characters | `ux_users_email` |\n\n")
	for _, scope := range []string{"entity", "field", "service"} {
		switch scope {
		case "entity":
			w("## Entity markers\n\n")
		case "field":
			w("## Field markers\n\n")
		default:
			w("## Microservices\n\n")
			w("A spec file may declare one `XxxService` struct instead of an entity. Each field written as `Invoice struct{ ... }` is an entity of that service, with the same markers as a top-level entity; its domain is the snake_case plural of its name without underscores (`InvoiceItem` -> `invoiceitems`). The file is named after the service (`BillingService` -> `specs/billing.go`), and once a project declares a service every entity must live in one. Each service becomes its own Go module exposing gRPC, and a generated gateway is the only REST API. A workspace needs one account entity (`auth=`): the gateway authenticates every request through the service that owns it. A `belongs_to` reference to an entity stored in another database has no foreign key; the service checks it with the gRPC `Exists` call of the owning service and answers 422 when it is missing. `many_to_many` relations need both entities in the same database. `docs/microservices.md` describes the generated workspace.\n\n")
			w("```go\ntype BillingService struct {\n\t_       struct{} `gen:\"database=own\"`\n\tInvoice struct {\n\t\tID     uuid.UUID `gen:\"pk,auto\"`\n\t\tUserID uuid.UUID `gen:\"rel=belongs_to:User\"`\n\t}\n}\n```\n\n")
			w("## Service markers\n\n")
		}
		for _, m := range Markers() {
			if m.Scope != scope {
				continue
			}
			w("### `%s`\n\n", m.Name)
			w("- Syntax: `%s`\n", m.Syntax)
			w("- Applies to: %s\n", m.AppliesTo)
			w("- Effect: %s\n", m.Description)
			w("- Example: `%s`\n", m.Example)
			w("- Common error: %s\n\n", m.CommonError)
		}
	}
	w("## Request semantics\n\n")
	w("- Request bodies must be sent with `Content-Type: application/json` (or another `+json` media type); other content types answer HTTP 415.\n")
	w("- Create (`POST`), update (`PUT`, full replacement) and patch (`PATCH`) requests reject unknown members with HTTP 400.\n")
	w("- On create, an absent member takes the `default=` value, or the zero value (an empty value for `[]byte`). Pointer fields treat `null` like an absent member.\n")
	w("- `PUT` replaces the whole resource, so every non-pointer member is required: leaving one out answers 422 instead of resetting it to its default. Pointer members left out become `null`.\n")
	w("- `PATCH` takes a JSON merge patch (RFC 7396), sent as `application/merge-patch+json` or `application/json`: members that are present replace the stored values, `null` clears a pointer member (and answers 422 on any other), and absent members keep their values. It reads the row and writes it back through the same validation and service `Update` as `PUT`, so two concurrent patches of different members of one row can overwrite each other.\n")
	w("- `required` members must be present. Non-pointer `belongs_to` fields and client-provided primary keys are required implicitly.\n")
	w("- Validation rules apply to every value of non-pointer fields and to non-nil values of pointer fields. Use a pointer for optional values.\n")
	w("- Fields with `auto`, `auto=create`, `auto=update` or the primary key never appear in update requests.\n")
	w("- Validation failures return HTTP 422 with RFC 9457 problem details and an `errors` array of `{field, message}`.\n")
	w("- Lists are ordered newest first by the `auto=create` field with the primary key breaking ties, or by the primary key when there is none. A list response is `{items, limit, offset, total, nextCursor}`; `nextCursor` is `null` on the last page.\n")
	w("- Lists page by `offset` or by `cursor`. Passing the `nextCursor` of a page as `?cursor=` returns the rows after it without counting them, so the response has no `total`; it stays correct while rows are added and is faster on large tables. A cursor cannot be combined with `offset`, and a malformed one answers 400.\n\n")
	w("## Generated routes\n\n")
	w("| Method and path | Purpose | Notes |\n|---|---|---|\n")
	w("| `GET /<table>` | list | `limit` with `offset` or `cursor`, and equality filters on indexed, unique and belongs_to fields |\n")
	w("| `POST /<table>` | create | 201 with `Location`; skipped for `readonly` |\n")
	w("| `GET /<table>/{id}` | get | 404 when missing or soft deleted |\n")
	w("| `PUT /<table>/{id}` | full update | skipped for `readonly` |\n")
	w("| `PATCH /<table>/{id}` | partial update | JSON merge patch; skipped for `readonly` |\n")
	w("| `DELETE /<table>/{id}` | delete | soft delete with `softdelete`; 409 when referenced |\n")
	w("| `GET /<parents>/{parentId}/<field>` | has_many | generated in the child domain |\n")
	w("| `GET /<sources>/{sourceId}/<field>` | many_to_many list | generated in the target domain |\n")
	w("| `PUT` and `DELETE /<sources>/{sourceId}/<field>/{targetId}` | many_to_many link and unlink | 204, idempotent |\n")
	w("| `GET /openapi.json` | OpenAPI 3.1 description of every route | the same document as `openapi.json` at the project root, regenerated on every build |\n\n")
	w("## Diagnostic codes\n\n")
	w("Diagnostics are printed as `file:line:column: message (did you mean \"x\"?) [code]` and as JSON objects with `file`, `line`, `column`, `severity`, `code`, `message` and `suggestion` when `--json` is used.\n\n")
	w("| Code | Meaning |\n|---|---|\n")
	for _, d := range diagnosticDocs {
		w("| `%s` | %s |\n", d.Code, d.Meaning)
	}
	return b.String()
}

const exampleSpec = `//go:build ignore

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_         struct{}  ` + "`" + `gen:"table=users,softdelete"` + "`" + `
	ID        uuid.UUID ` + "`" + `gen:"pk,auto"` + "`" + `
	Name      string    ` + "`" + `gen:"required,notempty,max=120"` + "`" + `
	Email     string    ` + "`" + `gen:"required,unique,email,max=254"` + "`" + `
	Role      string    ` + "`" + `gen:"enum=admin|operator|viewer,default=viewer"` + "`" + `
	Bio       *string   ` + "`" + `gen:"max=500"` + "`" + `
	OrgID     uuid.UUID ` + "`" + `gen:"rel=belongs_to:Org"` + "`" + `
	CreatedAt time.Time ` + "`" + `gen:"auto=create"` + "`" + `
	UpdatedAt time.Time ` + "`" + `gen:"auto=update"` + "`" + `
}
`

const grammarEBNF = `SpecFile   = "//go:build ignore" "package specs" [ Imports ] { AliasDecl } EntityDecl { AliasDecl } .
Imports    = "import" of "time" | "uuid" | "github.com/google/uuid" .
AliasDecl  = "type" Name "=" FieldType .
EntityDecl = "type" EntityName "struct" "{" [ MetaField ] { Field } "}" .
MetaField  = "_" "struct{}" GenTag .
Field      = FieldName FieldType [ GenTag ] .
GenTag     = "` + "`" + `gen:\"" Marker { "," Marker } "\"` + "`" + `" .
Marker     = Key [ "=" Value ] .
RelValue   = ( "belongs_to" | "has_many" | "many_to_many" ) ":" EntityName .
EnumValue  = Item { "|" Item } .
OmitValue  = Target { "|" Target } .
Target     = "create" | "update" | "response" .
AutoValue  = "create" | "update" .
`
