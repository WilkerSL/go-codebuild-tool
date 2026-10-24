# Running in production

This guide covers what a generated project needs around it before it serves real users. It applies to monoliths and, where noted, to microservice workspaces.

Read it with two facts in mind. The security review of the generated code was done internally by the maintainers; it was not an independent audit. And no generated project is known to run in production yet. If your project handles sensitive data, such as health, financial or authentication data of other systems, have the generated code and your deployment reviewed by an external security team before going live.

## TLS and the reverse proxy

The API and the gateway speak plain HTTP on `HTTP_ADDR`. Terminate TLS in front of them: a Kubernetes Ingress or Gateway, a cloud load balancer, or a reverse proxy such as nginx, Caddy or Envoy. Then:

- Keep `AUTH_COOKIE_SECURE=true` (the default), so cookies never travel over plain HTTP.
- Set `AUTH_TRUST_FORWARDED_FOR=true` only when the proxy is the only way in and it appends the client address to `X-Forwarded-For`; the rate limiter then counts clients by the last hop of that header. Without it every client looks like the proxy.
- Send `Strict-Transport-Security` from the proxy.
- Expose `METRICS_ADDR` only inside the cluster or the private network.

Inside a workspace, services talk to each other over mutual TLS with certificates from your own CA; see [microservices.md](microservices.md#your-own-pki).

## Secrets and key rotation

Generated code reads every secret from the environment and never logs it. Keep them in a secret manager (Kubernetes Secrets, Vault, a cloud secret store), never in the image or in `codebuild.json`.

| Secret | Used by | Rotation |
|---|---|---|
| `DATABASE_URL` | API, services, migrations | Create the new database user, update the secret, roll out, drop the old user. |
| `AUTH_JWT_SECRET` | API or account service (`jwt`) | Move the current value to `AUTH_JWT_PREVIOUS_SECRET`, set a new one (`openssl rand -base64 48`), roll out, and remove the previous secret once `AUTH_ACCESS_TTL` has passed. Refresh tokens are not affected. |
| TLS certificates and CA | workspace modules | Renew in place; modules reload the files every 30 seconds. To change the CA, bundle old and new CA first. |
| Internal token keys | gateway and services | Append the new public key, switch the gateway's private key, then remove the old public key, waiting a few minutes between the steps; modules reread the files every 30 seconds. See [microservices.md](microservices.md#your-own-pki). |

Sessions, refresh tokens and API keys are stored as SHA-256 hashes, so a database dump does not reveal usable credentials; password hashes use PBKDF2 with 600,000 iterations.

## Database

### Migrations in deployments

Run the migrations as a separate step before the new version starts, never from the API process:

```bash
DATABASE_URL=... /app/migrate up        # in the image
DATABASE_URL=... go run ./cmd/migrate up
```

The migrator takes a database lock on PostgreSQL, MySQL and SQL Server, so two runs never apply the same migration twice (with SQLite, run one at a time), and refuses migrations older than the applied version that never ran. While it runs, a statement waits at most `MIGRATE_LOCK_TIMEOUT` (10 seconds) for a table lock and then fails, instead of queueing every query on that table behind a long transaction; run the migration again later, or raise the timeout (`0` waits forever). The timeout bounds the wait, not the work: an index built on a large table still locks it while it is built, so schedule those migrations, or build the index concurrently yourself in a migration of your own. Generated migrations only add or drop what the specs changed; read every new migration before deploying it, because codebuild warns about destructive changes and possible renames but cannot know your data. Prefer changes that keep the previous version of the code working (add a column, deploy, backfill, then make it required), so a rollout can be rolled back.

### Backups

codebuild does not back up anything. Use the tools of your database: point-in-time recovery or `pg_dump` for PostgreSQL, binary logs and `mysqldump` or a physical backup for MySQL, full and log backups for SQL Server, and `sqlite3 app.db ".backup backup.db"` (or Litestream) for SQLite. Test restores regularly. In a workspace every database has its own backups, and a restore of one database does not roll back the others.

### Pool sizing

| Variable | Default | Guidance |
|---|---|---|
| `DB_MAX_OPEN_CONNS` | `25` | Keep `replicas × DB_MAX_OPEN_CONNS` below the database's connection limit, leaving room for migrations and administration. |
| `DB_MAX_IDLE_CONNS` | `25` | Equal to the open limit avoids reconnecting under steady load. |
| `DB_CONN_MAX_LIFETIME` | `30m` | Shorter than the idle timeout of any proxy (PgBouncer, RDS Proxy) between the API and the database. |

Count every process that connects to the same database: in a workspace the services sharing a database add up, so the Kubernetes manifests give each service replica a pool of 10. `db_client_connection_waits_total` growing means requests queue for a connection: raise the pool if the database has headroom, otherwise add database capacity.

## Timeouts and shutdown

| Variable | Default | Meaning |
|---|---|---|
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | Time to read request headers. |
| `HTTP_READ_TIMEOUT` | `15s` | Time to read the whole request. |
| `HTTP_WRITE_TIMEOUT` | `15s` | Time to write the response. |
| `HTTP_IDLE_TIMEOUT` | `60s` | Keep-alive connections. |
| `HTTP_REQUEST_TIMEOUT` | `12s` | Workspace gateway: time budget of a request and every backend call it makes; shorter than `HTTP_WRITE_TIMEOUT`. |
| `SHUTDOWN_DELAY` | `0s` | Keep serving after `/readyz` turns 503 on SIGTERM, so load balancers stop sending traffic first; the Kubernetes manifests use `5s`. |
| `SHUTDOWN_TIMEOUT` | `20s` | Time for in-flight requests to finish. |
| `GRPC_CALL_TIMEOUT` | `10s` | Deadline of calls between modules of a workspace. |

Keep the proxy's timeouts a little longer than the API's, and the orchestrator's grace period longer than `SHUTDOWN_DELAY + SHUTDOWN_TIMEOUT`.

## CORS

Leave `CORS_ALLOWED_ORIGINS` empty unless a browser front end on another origin calls the API, then list its exact origins. Avoid `*` with an authenticated API. See [observability.md](observability.md#cors).

## Health checks

`GET /livez` answers 200 while the process runs; use it for liveness. `GET /readyz` checks the database and answers 503 while shutting down; use it for readiness and load balancer health checks. Workspace services serve both on `HEALTH_ADDR`.

## Metrics, traces and alerts

Set `METRICS_ADDR` and scrape `/metrics` with Prometheus, and set `OTEL_EXPORTER_OTLP_ENDPOINT` to send traces to a collector; see [observability.md](observability.md). Alerts worth starting with:

| Alert | Expression (PromQL) |
|---|---|
| Error rate above 1% | `sum(rate(http_server_requests_total{status=~"5.."}[5m])) / sum(rate(http_server_requests_total[5m])) > 0.01` |
| Slow requests | `histogram_quantile(0.99, sum by (le, route) (rate(http_server_request_duration_seconds_bucket[5m]))) > 1` |
| Database pool exhausted | `rate(db_client_connection_waits_total[5m]) > 0` |
| Login abuse | `sum(rate(http_server_requests_total{route="/auth/login",status="429"}[5m])) > 1` |
| Backend unavailable (workspace) | `max by (target) (grpc_client_circuit_open) == 1` |
| Certificate expires within 14 days (workspace) | `min(tls_certificate_expiry_timestamp_seconds) - time() < 14 * 86400` |
| Certificate files stopped loading (workspace) | `increase(tls_reload_errors_total[10m]) > 0` |
| Instance down | `up == 0` |
| Traces lost | `rate(telemetry_spans_dropped_total[5m]) > 0` |

## Rate limits

The generated limiter covers login, registration and password changes only. Behind more than one replica set `AUTH_LIMIT_STORE=database`, so the replicas share the counters (the Kubernetes manifests do). Limit everything else, and absorb floods, at the proxy or the load balancer. See [authentication.md](authentication.md#rate-limiting).

## Retries and idempotency

Networks lose answers. A client that repeats a create after a timeout or a dropped connection should send an `Idempotency-Key` header, such as a UUID per logical request: the API keeps the successful answer for 24 hours in the `idempotency_keys` table and returns it to a repeat with `Idempotent-Replayed: true`, instead of creating a second row. The same key with another body answers 422, and a repeat while the first request runs answers 409. `PUT` is idempotent by itself and a repeated `DELETE` answers 404. Expired keys are deleted in the background, in batches.

## Kubernetes

`deploy/kubernetes/base` is a generated kustomize base; `deploy/kubernetes/overlays/production` is yours. The base contains, for the API or for every service and the gateway:

- a Deployment with two replicas (one with SQLite), a rolling update that never drops below the desired count, startup, liveness (`/livez`) and readiness (`/readyz`) probes, CPU and memory requests and a memory limit, `GOMEMLIMIT` set below that limit so the garbage collector works harder before the container is killed, soft spreading of the replicas over nodes and zones, a termination grace period that covers `SHUTDOWN_DELAY` and `SHUTDOWN_TIMEOUT`, and a container that runs as the distroless `nonroot` user with a read-only root file system, no privilege escalation, no capabilities, the `RuntimeDefault` seccomp profile and no service account token;
- a Service and a PodDisruptionBudget;
- a ConfigMap generated by kustomize, so changing it rolls the pods;
- a migration Job per database, labelled `app.kubernetes.io/component=migration`.
- in a workspace, headless Services for the services, so gRPC clients balance over every ready replica, and a NetworkPolicy that admits only the pods of the workspace to their gRPC ports (it needs a network plugin that enforces policies; the health and metrics ports stay open to probes and Prometheus).

The manifests reference Secrets by name and never contain a value. Create them first (`<project>` is the last element of the module path):

| Secret | Keys | For |
|---|---|---|
| `<project>-database` (monolith) or `<project>-<database>-database` (workspace) | `url` | `DATABASE_URL` |
| `<project>-auth` | `jwt-secret`, optionally `jwt-previous-secret` | `jwt` authentication |
| `<project>-<module>-tls` | `ca.crt`, `tls.crt`, `tls.key` | workspace mutual TLS, the layout of cert-manager Certificates |
| `<project>-internal-token` | `private.pem` (gateway), `public.pem` (services) | workspace internal tokens |

Then deploy in two steps, so the new code never runs against an old schema:

```bash
kubectl delete job -l app.kubernetes.io/component=migration --ignore-not-found
kubectl apply -k deploy/kubernetes/overlays/production -l app.kubernetes.io/component=migration
kubectl wait --for=condition=complete job -l app.kubernetes.io/component=migration --timeout=10m
kubectl apply -k deploy/kubernetes/overlays/production
```

Jobs cannot be updated in place, hence the delete; finished Jobs are also removed an hour after they complete. In the overlay, set the image names and tags (a version or digest, not `latest`), the namespace, replicas and any patch: resources (change `GOMEMLIMIT` with the memory limit, about 90% of it), node selectors, an Ingress, a HorizontalPodAutoscaler (then drop `replicas` from the patch). Tools that apply manifests on their own, such as Argo CD or Flux, must run the migration Jobs first too: give them a sync hook or wave in the overlay. The workspace manifests of the `shop` example are deployed to a kind cluster by the `Kubernetes` workflow of this repository, which also rotates every certificate without restarting the gateway.

## Load test results

`TestLoad` (run with `CODEBUILD_E2E_LOAD=1`, or the manual `Load test` workflow) generates the `saas` project and the `secure` project with `jwt`, runs them against PostgreSQL 17 and measures each scenario with concurrent clients after seeding 50,000 projects. With `CODEBUILD_TEST_POSTGRES_WORKSPACE_DSNS` it also runs the `shop` workspace and measures it through its gateway, so every request crosses the gateway, mutual TLS and gRPC, and authenticated ones also ask the identity service. The numbers below come from one run of the workflow on a GitHub Actions runner where the load generator, the APIs, the services and PostgreSQL competed for the same four CPUs, so read them as an order of magnitude and as a comparison between scenarios, not as a capacity promise. Run the test on your own hardware.

Hardware: AMD EPYC 9V74, 4 logical CPUs, linux/amd64, go1.27.3. Load generator, API and PostgreSQL 17 on the same machine, codebuild 0.5.0, October 2026.
Settings: 32 concurrent clients, 20s per scenario, 50000 seeded projects (5,000 categories in the workspace).

Monoliths:

| Scenario | Requests | Throughput (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errors |
|---|---:|---:|---:|---:|---:|---:|
| create project (seeding) | 50000 | 5077 | 5.3 | 10.7 | 31.0 | 0 |
| create project | 118776 | 5937 | 5.1 | 9.4 | 12.1 | 0 |
| get project | 229350 | 11462 | 2.5 | 5.6 | 7.6 | 0 |
| list projects, first page | 2036 | 101 | 308.5 | 492.5 | 573.0 | 0 |
| list projects, offset in the last tenth | 416 | 20 | 1529.6 | 2142.0 | 2414.8 | 0 |
| list projects, cursor | 65276 | 3262 | 9.0 | 19.1 | 24.4 | 0 |
| list projects of an org, cursor | 58767 | 2937 | 10.4 | 19.2 | 23.9 | 0 |
| login | 502 | 24 | 1307.0 | 1686.3 | 1804.1 | 0 |
| refresh | 75611 | 3779 | 8.2 | 13.2 | 16.2 | 0 |
| list own notes (JWT) | 113434 | 5671 | 5.3 | 9.9 | 13.0 | 0 |

Workspace, through the gateway:

| Scenario | Requests | Throughput (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errors |
|---|---:|---:|---:|---:|---:|---:|
| create category (seeding) | 5000 | 2577 | 11.5 | 18.4 | 24.1 | 0 |
| create category | 55679 | 2783 | 11.2 | 17.8 | 20.9 | 0 |
| get category | 115926 | 5795 | 5.2 | 9.2 | 11.5 | 0 |
| list categories, cursor | 92654 | 4632 | 6.6 | 11.5 | 14.3 | 0 |
| list own invoices (session) | 48521 | 2425 | 12.7 | 20.4 | 24.7 | 0 |

What the numbers say:

- Cursor pages read a page from an index in list order, so they stay fast on large tables: about 3,000 requests per second here, against about 40 before the list indexes of 0.4.0 on the same scenario. Use `?cursor=` for anything that scrolls.
- Offset pages deep in a large table stay slow (about 20 requests per second here): they count every matching row and skip the offset. The table holds about 170,000 rows by then, because the create scenario runs first, so the first page of an offset list, which also pays for the count, falls to about 100 requests per second. Prefer cursors, or keep offsets for small tables.
- The workspace answers a read in about twice the time of the monolith (5.2 against 2.5 ms at the median) and serves about half as many on the same four CPUs, which also run the gateway and four services: the price of the gateway hop, mutual TLS and gRPC. An authenticated read adds the call to the identity service, which checks the session in its database, and roughly halves the throughput again. Creates go through the idempotency check and the same hops.
- Logins are deliberately expensive: PBKDF2 with 600,000 iterations costs a sixth to a third of a CPU second depending on the CPU, so four CPUs serve between 10 and 25 logins per second, and 32 concurrent clients wait over a second for theirs. Size login capacity on CPUs, and keep the rate limiter on. Refreshes and authenticated reads cost no hashing.
- No request failed in any scenario.

## Remaining limits

- The security reviews were internal, not an independent audit, and no generated project is known to run in production yet.
- The rate limiter only covers the authentication endpoints.
- Offset pagination counts every matching row, and a list without `?cursor=` is an offset list, so its first page pays for the count too; the load test shows it slowing down past 100,000 rows. Use `?cursor=` for large tables and deep pages.
- Idempotency keys cover creates; other writes are idempotent by nature or answer an error when repeated.
- `jwt` access tokens stay valid until they expire on routes without roles unless `AUTH_JWT_CONFIRM=true`.
- SQLite runs one replica; use PostgreSQL, MySQL or SQL Server for more.
- codebuild does not provide backups, a web application firewall, secret management or an Ingress; bring your own.
- Microservice workspaces have no distributed transactions; see [microservices.md](microservices.md#limits) for their other limits.
