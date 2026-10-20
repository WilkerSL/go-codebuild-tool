# Microservices

> **Experimental.** Microservice workspaces are new in 0.2.0. The layout, the contracts and the generated code may change between minor releases, and no project has run them in production yet. Read the generated code and the limits below before relying on it.

A project whose specs declare `<Name>Service` structs becomes a microservice workspace: one Go module per service exposing gRPC, a generated gateway that is the only REST API, the gRPC contracts, and the files to run everything with Docker Compose.

```go
//go:build ignore

package specs

type BillingService struct {
	_       struct{} `gen:"database=own"`
	Invoice struct {
		_         struct{}      `gen:"softdelete"`
		ID        uuid.UUID     `gen:"pk,auto"`
		OwnerID   uuid.UUID     `gen:"rel=belongs_to:User,ondelete=cascade,owner"`
		Number    string        `gen:"required,unique,max=32"`
		CreatedAt time.Time     `gen:"auto=create"`
		Items     []InvoiceItem `gen:"rel=has_many:InvoiceItem"`
	}
	InvoiceItem struct {
		ID        int64     `gen:"pk,auto"`
		InvoiceID uuid.UUID `gen:"rel=belongs_to:Invoice,ondelete=cascade"`
		ProductID int64     `gen:"rel=belongs_to:Product"`
		Quantity  int32     `gen:"min=1,default=1"`
	}
}
```

Each nested struct is an entity with the usual markers. `testdata/specs/shop` in this repository is a complete example with four services: `identity` (the accounts), `billing` (its own database), and `catalog` and `inventory` (sharing a database).

## Rules

- One file per service, named after it: `BillingService` lives in `specs/billing.go`.
- The first field declares the database: `database=own` gives the service a database of its own, `database=shared` puts it on the one database shared by every `shared` service.
- Once a project declares a service, every entity must be a field of one.
- Exactly one entity declares `auth=`. The gateway authenticates every request through the service that owns it.
- Domain names must stay unique across services, and `many_to_many` relations need both entities in the same database.

## Layout

```
go.mod                      root module: specs/ and the pinned codebuild tool
go.work                     joins every module below
codebuild.json
specs/<service>.go
contracts/                  .proto files, their Go code and codebuild.contracts.json
gateway/                    REST API, auth endpoints, devcerts
services/<service>/         gRPC server, repositories, migrations of an own database
databases/shared/           migrations of the shared database
docker-compose.yml
.github/workflows/          one workflow per module and workspace.yml
```

Each module has its own `go.mod` with `replace` directives for the local modules it imports, so it builds both inside the workspace and alone with `GOWORK=off`, which is how its Dockerfile and its workflow build it.

In a workspace, `./...` stops at nested modules. Use the module path pattern from the root: `go build example.com/shop/...`, `go vet example.com/shop/...`, `go test example.com/shop/...`.

## How a request flows

1. A client calls the gateway, for example `POST /invoices` with `Authorization: Bearer <token>` or the session cookie.
2. The gateway runs the same checks as a monolith: credential extraction, cross-origin protection for cookies, the login rate limiter and the roles of the route. It validates the credential with `AuthService.Authenticate` on the account service.
3. It signs an internal token for the caller and calls `InvoiceService.Create` on the billing service over mutual TLS.
4. The billing service verifies the token, authorizes the call again, validates the request and checks `ProductID` with `ProductService.Exists` on the catalog service, propagating the same token.
5. Errors travel as gRPC status codes with a `common.v1.Problem` detail carrying the HTTP status and the field errors, so the gateway answers with the same RFC 9457 problem details, status codes and `WWW-Authenticate` headers as a monolith.

The REST API is the one a monolith with the same entities would expose: same routes, JSON bodies, filters, pagination and `/auth/*` endpoints.

## Security between services

Services accept only gRPC over mutual TLS. Every module reads `TLS_CA_FILE`, `TLS_CERT_FILE` and `TLS_KEY_FILE`; a client verifies the server name against the host of the address it dials, so the certificate of `billing` must be valid for the host name the others use to reach it.

On top of TLS, every call carries an internal token in the `x-internal-token` metadata: a compact JWT signed with Ed25519 that lives for one minute and holds the caller (`sub`), its role, the authentication method and the credential ID. Only the gateway holds the private key (`INTERNAL_TOKEN_PRIVATE_KEY_FILE`); services verify with the public key (`INTERNAL_TOKEN_PUBLIC_KEY_FILE`) and forward the token to the services they call. Anonymous requests to public routes carry a token without `sub`. A service never trusts a call without a valid token, even from a peer with a valid certificate.

Inside a service, `grpcx.Authorize(ctx, roles...)` and `auth.Current(ctx)` behave as in a monolith, so the `owner`, `roles` and `public` markers keep their meaning.

`go run ./gateway/cmd/devcerts` writes a development CA, a certificate per module (valid for the module name, `localhost`, `127.0.0.1` and `::1`) and the internal token key pair into `./certs`. Use your own PKI and key management in production; rotating the internal key means deploying the new public key to the services before the new private key to the gateway.

## References across services

| Reference | Database | Behavior |
|---|---|---|
| Same service | same | Foreign key, as in a monolith. |
| Another service on the same database | shared | Foreign key, as in a monolith. |
| Another service on another database | different | No foreign key. Create and update call `Exists` on the owning service and answer 422 with the field error when the row is missing. |

`ondelete=` rules and soft-delete visibility only apply inside one database. Deleting a user in the identity database does not delete the invoices that reference it in the billing database; clean them up in a service of yours if you need that.

## Contracts

`contracts/` holds `<service>/v1/<service>.proto`, the protobuf Go code and the gRPC stubs, all generated by codebuild without `protoc`, plus `common/v1/common.proto`. Each entity has a `<Entity>Service` with `Get`, `List`, `Create`, `Update`, `Delete`, `Exists` and the relation methods; the account service also exposes `AuthService`.

`contracts/codebuild.contracts.json` records the number of every field. A new field takes the next free number, a removed field becomes `reserved`, and renaming the Go field counts as removing it and adding a new one. Commit the file and never edit it: it keeps old clients compatible.

Other clients can compile the `.proto` files with their own tools; they still need the internal token and a certificate from your CA.

## Databases and migrations

Every database has its own migrations and snapshot: `services/<service>/migrations/` for a service with `database=own`, `databases/shared/migrations/` for the shared database. `codebuild plan` lists the new migration of each database, and each database has its own migrate command:

```bash
DATABASE_URL=... go run ./services/billing/cmd/migrate up
DATABASE_URL=... go run ./databases/shared/cmd/migrate up
```

Repository tests run against `TEST_DATABASE_URL` like in a monolith. Give each module its own disposable database: the migrations of every database start at `0001`, so two modules cannot share a test database.

## Running

```bash
docker compose up --build
```

Compose generates development certificates in a volume, starts one database server per database (one SQL Server instance with one database each for `sqlserver`, volumes for SQLite), applies the migrations, starts every service once its migrations ran and the gateway once every service is healthy, and publishes the gateway on port 8080. `docker-compose.yml` is generated and rewritten when services change; put your changes in `docker-compose.override.yml`, which Compose merges automatically.

| Variable | Module | Purpose |
|---|---|---|
| `GRPC_ADDR` | service | gRPC listen address, default `:9000`. |
| `HEALTH_ADDR` | service | `/livez` and `/readyz`, default `:8081`; `server healthcheck` probes it. |
| `DATABASE_URL` | service, migrate | Connection string of the database of the service. |
| `<SERVICE>_GRPC_ADDR` | gateway, service | `host:port` of a service: the gateway needs every service, a service needs the services it calls for `Exists`. |
| `HTTP_ADDR` | gateway | REST listen address, default `:8080`; `gateway healthcheck` probes it. |
| `TLS_CA_FILE`, `TLS_CERT_FILE`, `TLS_KEY_FILE` | all | Mutual TLS. |
| `INTERNAL_TOKEN_PRIVATE_KEY_FILE` | gateway | Ed25519 key that signs internal tokens. |
| `INTERNAL_TOKEN_PUBLIC_KEY_FILE` | service | Key that verifies them. |
| `AUTH_*` | gateway, account service | The gateway reads the cookie, origin and login limit settings; the account service reads the session and JWT settings. See [authentication.md](authentication.md). |

Every outgoing gRPC call without a deadline gets 10 seconds. A service that is down answers 503 at the gateway, a timeout 504.

## Changing the layout

- Adding a service creates its module, Dockerfile, workflow and migrations, adds it to `go.work` and regenerates `docker-compose.yml` and the gateway.
- Removing a service marks its generated files stale; `codebuild build --prune` deletes them. Delete the directory, its `use` line in `go.work` and its workflow yourself; codebuild warns about them.
- codebuild does not convert a monolith into a workspace or back. Create the workspace in a new directory with `codebuild build --starter --module <path>`, copy `specs/` and move your service code into `services/<service>/internal/<domain>/`.

## Limits

- No distributed transactions: a request that changes rows in two services is two independent calls.
- Unary calls only; no streaming RPCs.
- The gateway asks the account service on every authenticated request, so revocations take effect immediately and the account service is on the path of every protected request.
- The login rate limiter lives in each gateway instance.
