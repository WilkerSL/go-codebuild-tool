# Microservices

Microservice workspaces left the experimental stage in 0.4.0. Their end-to-end tests build every module, test it against its own database and run the services and the gateway over mutual TLS with each dialect (SQLite also under Docker Compose), stop a service to check that the gateway answers 503 and recovers, and deploy them with PostgreSQL to a Kubernetes cluster, where they also rotate every certificate without restarting the gateway. A load test measures them through the gateway. No generated workspace is known to run in production yet, so read the generated code, [production.md](production.md) and the limits below before relying on it.

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

Failures between modules are not the client's fault and never look like one: a rejected internal token, a module calling a method it may not call, or a peer that fails answer 500 with a generic message and an error in the gateway log, while a backend that is down answers 503 and one that is too slow 504. A gateway request has one time budget for all its calls, `HTTP_REQUEST_TIMEOUT` (12 seconds), shorter than `HTTP_WRITE_TIMEOUT`, so a slow request gets its 504 instead of a dropped connection.

The REST API is the one a monolith with the same entities would expose: same routes, JSON bodies, filters, pagination and `/auth/*` endpoints. The gateway serves its OpenAPI description at `GET /openapi.json`, and the same document is written to `openapi.json` at the workspace root.

## Security between services

Services accept only gRPC over mutual TLS. Every module reads `TLS_CA_FILE`, `TLS_CERT_FILE` and `TLS_KEY_FILE`; a client verifies the server name against the host of the address it dials, so the certificate of `billing` must be valid for the host name the others use to reach it.

On top of TLS, every call carries an internal token in the `x-internal-token` metadata: a compact JWT signed with Ed25519 that lives for one minute and holds the caller (`sub`), its role, the authentication method and the credential ID. Only the gateway holds the private key (`INTERNAL_TOKEN_PRIVATE_KEY_FILE`); services verify with the public key (`INTERNAL_TOKEN_PUBLIC_KEY_FILE`) and forward the token to the services they call. Anonymous requests to public routes carry a token without `sub`. A service never trusts a call without a valid token, even from a peer with a valid certificate.

Inside a service, `grpcx.Authorize(ctx, roles...)` and `auth.Current(ctx)` behave as in a monolith, so the `owner`, `roles` and `public` markers keep their meaning.

Because services forward the tokens they receive, a compromised service could replay a user's token against another service while it is valid. The client certificate limits what such a replay can reach:

- Every service accepts methods with side effects (`Create`, `Update`, `Delete`, links and the methods of your own `.proto` files) only from the gateway, whose certificate name (common name or DNS name) it reads from `TLS_GATEWAY_NAME` (`gateway` by default). Other modules may only call the generated reads, `Get`, `List…` and `Exists`, which is all the generated code needs. If code of yours in one service writes through another, list the caller's certificate name in `TLS_TRUSTED_CALLERS` of the service it calls, separated by commas.
- The account service answers every `AuthService` call only to the gateway.
- It checks every JWT against its database when the gateway authenticates a request, so logout, revocation, deleted accounts and role changes apply to the next request.

Streaming methods you add to your own `.proto` files go through the same token check, caller check and panic recovery as unary ones.

`go run ./gateway/cmd/devcerts` writes a development CA, a certificate per module (valid for the module name, `localhost`, `127.0.0.1` and `::1`) and the internal token key pair into `./certs`. It is for development only.

### Your own PKI

In production, issue the certificates from your own CA, for example with cert-manager. Each module needs:

- a certificate valid for the host name the other modules dial (the Service name in Kubernetes, such as `billing`), with both the server and client authentication usages;
- for the gateway, a certificate whose common name or DNS name matches `TLS_GATEWAY_NAME` on the account service (`gateway` by default);
- the CA bundle that signed the certificates of its peers.

Every module rereads `TLS_CA_FILE`, `TLS_CERT_FILE` and `TLS_KEY_FILE` every 30 seconds and uses the new files for the next handshakes, so renewing a certificate needs no restart; connections already open keep working until they are closed. If a file cannot be read or parsed, the module keeps the last working certificate. To move to a new CA, put both CAs in every bundle, renew every certificate from the new CA, then remove the old CA from the bundles. The Kubernetes manifests mount the Secret `<project>-<module>-tls` with the keys `ca.crt`, `tls.crt` and `tls.key`, the layout cert-manager writes.

Modules report a certificate that stops loading with a warning in the log and `tls_reload_errors_total`, and export when the certificate in use expires as `tls_certificate_expiry_timestamp_seconds`; alert on both (see [production.md](production.md#metrics-traces-and-alerts)). Connections end after five minutes, so a rotated certificate is in use everywhere within minutes.

`INTERNAL_TOKEN_PUBLIC_KEY_FILE` may hold several PEM public keys, and services accept a token signed by any of them. Services and the gateway reread the internal token keys every 30 seconds too, so they rotate without restarts, in three steps that each wait until the files reached every pod (a minute or two in Kubernetes):

1. Append the new public key to `public.pem`; services now accept both keys.
2. Replace `private.pem` with the new private key; the gateway signs with it.
3. A few minutes later, once no token signed by the old key is alive (they live one minute), remove the old public key.

The Kubernetes manifests mount both files from the Secret `<project>-internal-token`, so make each step a separate update of that Secret. Workspaces created before 0.5.0 read the keys once at startup: switch `gateway/cmd/gateway/main.go` to `auth.WatchPrivateKey` and each `cmd/server/main.go` to `auth.WatchPublicKeys` to get the reload, or roll the pods after each step.

## References across services

| Reference | Database | Behavior |
|---|---|---|
| Same service | same | Foreign key, as in a monolith. |
| Another service on the same database | shared | Foreign key, as in a monolith. |
| Another service on another database | different | No foreign key. Create and update call `Exists` on the owning service and answer 422 with the field error when the row is missing. |

`ondelete=` rules and soft-delete visibility only apply inside one database. Deleting a user in the identity database does not delete the invoices that reference it in the billing database; clean them up in a service of yours if you need that.

## Contracts

`contracts/` holds `<service>/v1/<service>.proto`, the protobuf Go code and the gRPC stubs, all generated by codebuild without `protoc`, plus `common/v1/common.proto`. Each entity has a `<Entity>Service` with `Get`, `List`, `Create`, `Update`, `Delete`, `Exists` and the relation methods; the account service also exposes `AuthService`.

`contracts/codebuild.contracts.json` records the number and the wire type of every field. A new field takes the next free number, a removed field becomes `reserved`, and renaming the Go field counts as removing it and adding a new one. A field whose wire type changes, say from a string to an integer, is treated the same way: it gets a new number and its old number is reserved, so clients built before the change skip it instead of failing to parse every message, and `codebuild plan` warns about it. Commit the file, never edit it by hand, and review its diff in pull requests: an existing number must never change.

During a rolling deployment old and new modules run side by side, so make every change compatible in both directions:

- Deploy the services before the gateway. A new gateway that calls a method an older service lacks answers 503 until that service is updated, and a service ignores fields it does not know, so it keeps working with the old gateway.
- Add a field as optional, deploy everywhere, and only then make it required.

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
| `TLS_GATEWAY_NAME` | service | Certificate name of the gateway, the only client allowed to call methods with side effects and `AuthService`; default `gateway`. |
| `TLS_TRUSTED_CALLERS` | service | Certificate names of other modules that may also call methods with side effects, separated by commas; empty by default. |
| `HTTP_REQUEST_TIMEOUT` | gateway | Time budget of a request and all its backend calls, default `12s`; shorter than `HTTP_WRITE_TIMEOUT`. |
| `INTERNAL_TOKEN_PRIVATE_KEY_FILE` | gateway | Ed25519 key that signs internal tokens. |
| `INTERNAL_TOKEN_PUBLIC_KEY_FILE` | service | One or more PEM public keys that verify them. |
| `AUTH_*` | gateway, account service | The gateway reads the cookie, origin and login limit settings (`AUTH_LIMIT_STORE=database` keeps the counters in the account service's database); the account service reads the session and JWT settings. See [authentication.md](authentication.md). |
| `GRPC_*` | gateway, service | Timeouts, retries and circuit breakers of outgoing calls; see below. |
| `METRICS_ADDR`, `OTEL_*`, `CORS_*` | gateway, service | Metrics, tracing and, at the gateway, CORS. See [observability.md](observability.md). |

## Calls between modules

A client resolves the host of `<SERVICE>_GRPC_ADDR` and spreads its calls round robin over every address it gets, asking each replica for its gRPC health; a replica that drains on shutdown or fails is skipped. In Kubernetes the generated Services of the services are headless, so the name resolves to every ready replica; behind one virtual IP a client would keep a single connection to a single pod. Clients ping idle connections every 30 seconds, so a connection to a node that vanished is noticed in seconds, and reconnect with a backoff of at most 5 seconds. Servers end every connection after five minutes, with 30 seconds for running calls to finish, so clients resolve the name again and reach replicas added since. A client resolves again whenever a replica goes away, at most every 2 seconds instead of the 30 of gRPC, so a rolling update of a service is followed within seconds.

Every outgoing gRPC call without a deadline gets `GRPC_CALL_TIMEOUT` (10 seconds); at the gateway the request budget sets it. The generated calls that only read (`Get`, `List…`, `Exists`, `Authenticate`, `Me`, `ListAPIKeys`) are retried when the backend answers `Unavailable`, usually on another replica, with exponential backoff starting at `GRPC_RETRY_BACKOFF` (50 ms, doubling up to 1 second) and full jitter, up to `GRPC_RETRY_ATTEMPTS` attempts (3) and never past the deadline. Writes and the methods of your own `.proto` files are never retried, because the backend may have applied them; a client of the REST API can repeat a create safely with an `Idempotency-Key` (see below).

Each client connection has a circuit breaker: after `GRPC_BREAKER_FAILURES` consecutive calls (5) end, after their retries, with `Unavailable` or `DeadlineExceeded`, calls to that backend fail at once for `GRPC_BREAKER_COOLDOWN` (10 seconds); then one call probes the backend and closes the breaker if it gets an answer. A deadline does not count when the call started with less than a tenth of `GRPC_CALL_TIMEOUT` left, because the caller had used up the time. `GRPC_BREAKER_FAILURES=0` disables it. `/metrics` exports `grpc_client_retries_total`, `grpc_client_circuit_open` and `grpc_client_circuit_rejected_total` per backend.

Messages between modules are limited to 4 MiB, the gRPC default; raise `GRPC_MAX_MESSAGE_BYTES` on the gateway and the services if pages of large rows exceed it. A message over the limit answers 502.

| Variable | Default | Meaning |
|---|---|---|
| `GRPC_CALL_TIMEOUT` | `10s` | Deadline of calls that have none. |
| `GRPC_RETRY_ATTEMPTS` | `3` | Attempts of a read call, 1 to 100; `1` disables retries. |
| `GRPC_RETRY_BACKOFF` | `50ms` | First backoff; it doubles up to 1 second. |
| `GRPC_BREAKER_FAILURES` | `5` | Consecutive failures that open the breaker; `0` disables it. |
| `GRPC_BREAKER_COOLDOWN` | `10s` | How long an open breaker rejects calls before a probe. |
| `GRPC_MAX_MESSAGE_BYTES` | `4194304` | Largest message a client sends or receives, 1 MiB to 1 GiB. |

## Retrying creates

A client that sends `POST` with an `Idempotency-Key` header, for example a UUID, can repeat the request after a timeout or a dropped connection: the gateway forwards the key, and the service that creates the row keeps its answer for 24 hours in the `idempotency_keys` table of its database. A repeat with the same key and body gets that answer again, with `Idempotent-Replayed: true`, instead of a second row; the same key with another body answers 422, and a repeat while the first request still runs answers 409. Only successful answers are kept, so a rejected request can be fixed and sent again with the same key. Keys are scoped by the caller and the route. Monoliths do the same in their HTTP handlers.

## Kubernetes

Every build writes a kustomize base to `deploy/kubernetes/base`: a Deployment, a Service and a PodDisruptionBudget per service and for the gateway, a migration Job per database, a ConfigMap per module generated by kustomize, so a configuration change rolls the pods, and a NetworkPolicy that lets only the pods of the workspace reach the gRPC ports of the services. The Services of the services are headless; Kubernetes cannot turn an existing Service headless, so when you upgrade a workspace deployed before 0.5.0, delete those Services once before applying the new base. Each service replica gets a connection pool of 10, so the two replicas of each of the services that share a database stay well under its connection limit. The files are generated and rewritten; customize them in the overlay `deploy/kubernetes/overlays/production`, created once and yours, which sets the images, the namespace, the replicas and any patch. See [production.md](production.md#kubernetes) for the Secrets to create and the order of a deployment.

```bash
kubectl apply -k deploy/kubernetes/overlays/production
```

## Changing the layout

- Adding a service creates its module, Dockerfile, workflow and migrations, adds it to `go.work` and regenerates `docker-compose.yml` and the gateway.
- Removing a service marks its generated files stale; `codebuild build --prune` deletes them. Delete the directory, its `use` line in `go.work` and its workflow yourself; codebuild warns about them.
- codebuild does not convert a monolith into a workspace or back. Create the workspace in a new directory with `codebuild build --starter --module <path>`, copy `specs/` and move your service code into `services/<service>/internal/<domain>/`.

## Limits

Each limit below is a design decision or a known gap, with what to do about it.

- No distributed transactions: a request that changes rows in two services is two independent calls. This is deliberate: two-phase commit across databases would couple every service to every other. Keep invariants inside one service, or write a saga or an outbox in your own service code.
- The generated contracts use unary calls only. The gateway exposes a REST API of bounded pages, which unary calls serve. Streaming methods you add to your own `.proto` files get the authentication, the caller check and panic recovery, but no per-call metrics or spans.
- The gateway asks the account service on every authenticated request. That is what makes logout and revocation immediate everywhere, so it stays: scale the account service with the gateway, and round robin, retries and the circuit breaker keep the loss of one replica from failing requests.
- The gateway never retries writes, because a lost answer does not tell whether the backend applied them. Clients repeat creates safely with `Idempotency-Key`; a repeated `PUT` writes the same values again and a repeated `DELETE` answers 404.
- A reference to a row of another database is checked with `Exists` on every create and update, so while the service that owns that row is down, those writes answer 503.
- `ondelete=` and soft-delete visibility do not cross databases (see above).
- The internal token is not bound to one service: a compromised module with a valid certificate could replay a token it received against the reads of another service during its one-minute lifetime. Writes need the gateway's certificate, or a name in `TLS_TRUSTED_CALLERS`.
- With SQLite every database is a file on a `ReadWriteOnce` volume, so the Kubernetes manifests run one replica of each service and services sharing the `shared` database must run on the same node. Use PostgreSQL, MySQL or SQL Server for more replicas.
