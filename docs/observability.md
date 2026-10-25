# Observability and CORS

Generated projects can expose Prometheus metrics, export OpenTelemetry traces and answer cross-origin requests. All three are off until you set an environment variable, and all of them use the standard library only: there is no Prometheus client or OpenTelemetry SDK in `go.mod`.

The code lives in `internal/platform/telemetry` and `internal/platform/httpx/cors_gen.go`, both generated. A `main.go` created by codebuild 0.3.0 or later wires them; see [Wiring an older project](#wiring-an-older-project) for projects created before.

codebuild also generates the infrastructure that reads them: Jaeger with its Monitor tab, Prometheus with alert rules and Grafana with a dashboard, for Docker Compose and for Kubernetes, behind the login of the project when accounts have roles. See [The observability stack](#the-observability-stack).

## Metrics

Set `METRICS_ADDR` (for example `:9090`) and the process serves `GET /metrics` in the Prometheus text format on that address, separate from the API so it is not exposed with it.

| Metric | Type | Labels | Where |
|---|---|---|---|
| `http_server_requests_total` | counter | `method`, `route`, `status` | API and gateway |
| `http_server_request_duration_seconds` | histogram | `method`, `route` | API and gateway |
| `http_server_requests_in_flight` | gauge | | API and gateway |
| `grpc_server_handled_total` | counter | `method`, `code` | services |
| `grpc_server_handling_seconds` | histogram | `method` | services |
| `grpc_client_retries_total`, `grpc_client_circuit_rejected_total` | counter | `target` | gateway and services |
| `grpc_client_circuit_open` | gauge | `target` | gateway and services |
| `tls_certificate_expiry_timestamp_seconds` | gauge | | gateway and services |
| `tls_reload_errors_total` | counter | | gateway and services |
| `db_client_connections_open`, `_in_use`, `_idle`, `_max` | gauge | | processes with a database |
| `db_client_connection_waits_total`, `db_client_connection_wait_seconds_total` | counter | | processes with a database |
| `go_goroutines`, `go_heap_objects_bytes`, `go_gc_cycles_total`, `process_start_time_seconds` | | | every process |
| `telemetry_spans_dropped_total` | counter | | every process |

`route` is the `net/http` pattern that matched, such as `/notes/{id}`, never the raw path, so the number of series stays bounded. Unknown HTTP methods are counted as `_OTHER`, and unmatched requests have an empty `route`.

## Traces

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to the base URL of an OpenTelemetry collector, for example `http://otel-collector:4318`, and every HTTP request and gRPC call becomes a span exported over OTLP/HTTP with JSON encoding to `<endpoint>/v1/traces`. The collector, Jaeger, Grafana Tempo and most hosted backends accept that protocol.

- The API and the gateway continue the trace of an incoming W3C `traceparent` header and create a server span named after the route, for example `GET /notes/{id}`. Liveness and readiness probes are not traced.
- The gateway and the services send `traceparent` with every gRPC call and create a client span for it; the called service continues the trace with a server span. A request through the gateway is therefore one trace across every service it touches.
- Logs written with the request context carry `trace_id` and `span_id` next to `request_id`, so a log line leads to its trace.
- Spans are batched in memory, at most 2,048 waiting, and sent every five seconds or every 512 spans. When the collector is down they are dropped and counted in `telemetry_spans_dropped_total`; the API never waits for the collector. Shutdown flushes what is left.

Every query of the generated repositories and of the account code becomes a client span inside the request span, named after the operation and the table (`SELECT notes`, `INSERT users`), with `db.system.name`, `db.operation.name`, `db.collection.name` and the parameterized SQL in `db.query.text`. Parameter values are never recorded, and a failed query only marks the span as an error, because some drivers put row values in their messages. Queries in your own code are traced when they go through `database.Traced(db)`, which wraps a `*sql.DB` or `*sql.Tx`; migrations and health checks are not traced.

| Variable | Default | Meaning |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Base URL of the collector; `/v1/traces` is appended. Empty disables tracing. |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | empty | Full URL for traces, used as is; wins over the base URL. |
| `OTEL_EXPORTER_OTLP_HEADERS` | empty | `key=value` pairs separated by commas, values URL-encoded, sent with every export (for example an API key of a hosted backend). `OTEL_EXPORTER_OTLP_TRACES_HEADERS` adds more. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/json` | Only `http/json` is supported; any other value is a configuration error. |
| `OTEL_SERVICE_NAME` | `api`, `gateway` or the service name | `service.name` of the spans. |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` | `always_on`, `always_off`, `traceidratio` or their `parentbased_` forms. |
| `OTEL_TRACES_SAMPLER_ARG` | `1` | Ratio between 0 and 1 for the `traceidratio` samplers. |
| `OTEL_SDK_DISABLED` | `false` | `true` turns tracing off whatever the other variables say. |

The variables follow the OpenTelemetry specification, so the same settings work for services written with the official SDKs.

## The observability stack

Every build also writes the stack that reads those metrics and traces, so a project only has to run it:

| Path | What |
|---|---|
| `deploy/observability/jaeger.yaml`, `jaeger-ui.json` | Jaeger 2 (workspaces): receives the spans over OTLP, keeps them in memory, turns them into span metrics for the Monitor tab and reads those back from Prometheus. |
| `deploy/observability/prometheus.yml`, `alerts.yml` | Prometheus (workspaces): scrapes `/metrics` of every module and the span metrics of Jaeger, with the [alert rules](#alerts). |
| `deploy/observability/grafana/` | Grafana provisioning (workspaces): the Prometheus and Jaeger data sources and a [dashboard](#dashboard) of the project. |
| `deploy/kubernetes/components/observability/` | A kustomize component for Kubernetes, monoliths and workspaces alike; see [below](#kubernetes). |

All of them are regenerated on every build: the YAML files carry the generated header, and the JSON files open with an `"x-codebuild"` member, as `openapi.json` does. The three panels are served under `/_ops/jaeger/`, `/_ops/grafana/` and `/_ops/prometheus/`, so the API or the gateway can put them behind the login of the project.

### Docker Compose

The `docker-compose.yml` of a workspace sets `METRICS_ADDR` and `OTEL_EXPORTER_OTLP_ENDPOINT` on every module and declares Jaeger, Prometheus and Grafana under the `observability` profile:

```bash
docker compose --profile observability up --build
```

Without the profile the stack does not start; the modules then log once that the collector does not answer and count the lost spans in `telemetry_spans_dropped_total`.

| Panel | Accounts with roles | No roles or no authentication |
|---|---|---|
| Jaeger | http://localhost:8080/_ops/jaeger/ | http://127.0.0.1:16686/_ops/jaeger/ |
| Grafana | http://localhost:8080/_ops/grafana/ | http://127.0.0.1:3000/_ops/grafana/ |
| Prometheus | http://localhost:8080/_ops/prometheus/ | http://127.0.0.1:9091/_ops/prometheus/ |

### Who can open the panels

Traces show paths, queries and identifiers, and Prometheus and Grafana answer any query, so the panels are never public.

- When the account entity has a `role` field, the API of a monolith or the gateway of a workspace serves `/_ops/`: a sign-in page and a reverse proxy to every panel whose `OPS_*_URL` is set. Only accounts with a role listed in `OPS_ROLES` get through; other accounts get 403, anonymous calls 401, and a browser without a session is sent to the sign-in page. The session is the usual one, so the authentication cookie of the API works and so does `Authorization: Bearer`. Compose publishes no port of the stack in this case: the gateway is the only way in.
- The proxy drops the `Authorization` header and the authentication cookies, so a panel never sees a credential of the project. It also removes any `X-Webauth-*` header the client sent and sets `X-Webauth-User` to the account ID, which Grafana trusts as its [auth proxy](https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/auth-proxy/) user, created on first visit with the Editor role. Grafana is only reachable through the gateway, so nobody else can set that header.
- Requests to `/_ops/` are counted in the metrics under their route but never traced, and the alerts and the dashboard leave them out, so browsing the panels does not show up as traffic of the product.
- Without a `role` field any registered account could get in, so the API serves no `/_ops/` route. Compose then publishes the panels on 127.0.0.1 only and Grafana admits anonymous visitors as admins, which suits a development machine; on a shared host reach them through SSH or a VPN.

| Variable | Default | Meaning |
|---|---|---|
| `OPS_JAEGER_URL`, `OPS_GRAFANA_URL`, `OPS_PROMETHEUS_URL` | empty | Address of the panel without a path, such as `http://jaeger:16686`. The proxy keeps the `/_ops/<panel>/` path, so the panel must be served under it, as the generated stack is. Empty leaves the panel out; with none set there is no `/_ops/` route. |
| `OPS_ROLES` | `admin`, when it is one of the roles | Roles that may open the panels, separated by commas. Required when a panel is set and the roles have no `admin`. |

### Kubernetes

`deploy/kubernetes/components/observability` is a kustomize component, and the overlay that codebuild creates includes it and points every module at it with `OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318`, plus `OPS_JAEGER_URL=http://jaeger:16686` on the API or the gateway when the panels are on. It contains:

- a Jaeger Deployment and Service, with a NetworkPolicy that accepts spans only from the pods of the project and queries only from the API or the gateway;
- a PodMonitor for the `metrics` port of every module, which adds the `service` and `project` labels the alerts and the dashboard use, and one for the span metrics of Jaeger;
- a PrometheusRule with the [alerts](#alerts);
- the dashboard in the ConfigMap `<project>-dashboard`, labelled `grafana_dashboard: "1"` for the Grafana sidecar of kube-prometheus-stack, which only looks in its own namespace unless its `searchNamespace` says otherwise.

PodMonitor and PrometheusRule need the Prometheus Operator, which kube-prometheus-stack installs; on a cluster without it, remove the component and the `OTEL_EXPORTER_OTLP_ENDPOINT` lines from the overlay and send the spans to your own collector. kube-prometheus-stack only selects monitors and rules that carry its release label by default: add the label in the overlay or set `podMonitorSelectorNilUsesHelmValues` and `ruleSelectorNilUsesHelmValues` to `false`. Jaeger reads the span metrics back from `PROMETHEUS_URL`, `http://prometheus-operated.monitoring.svc:9090` by default. Kustomize runs the generators of the overlay before its components, so change it, or any other file of the component, with a patch rather than a `configMapGenerator` merge:

```yaml
patches:
  - path: deployments.yaml
  - patch: |-
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: jaeger-env
      data:
        PROMETHEUS_URL: http://prometheus.observability.svc:9090
```

Jaeger keeps up to 100,000 traces in memory and loses them when it restarts. For lasting storage, patch the `config.yaml` key of the `jaeger-config` ConfigMap the same way, with a configuration that uses Badger on a volume, Elasticsearch, OpenSearch or Cassandra. Only Jaeger is proxied by default: the Prometheus and Grafana of a cluster keep their own access control.

### Alerts

The rules are the same in `alerts.yml` and in the PrometheusRule, and only look at the series of the project (`project="<project>"`):

| Alert | Fires when | Severity |
|---|---|---|
| `TargetDown` | a module does not answer the scrape for 2 minutes | critical |
| `HTTPServerErrors` | more than 5% of the HTTP requests of a module answer 5xx for 5 minutes | critical |
| `HTTPLatency` | the 95th percentile of HTTP requests stays above 1 second for 10 minutes | warning |
| `GRPCServerErrors` | more than 5% of the gRPC calls to a service fail with `Unknown`, `Internal`, `Unavailable`, `DataLoss` or `DeadlineExceeded` for 5 minutes (workspaces) | critical |
| `GRPCLatency` | the 95th percentile of gRPC calls stays above 1 second for 10 minutes (workspaces) | warning |
| `CircuitOpen` | a module stopped calling a backend because its circuit breaker opened (workspaces) | warning |
| `CertificateExpiresSoon` | the certificate in use expires within 7 days (workspaces) | warning |
| `CertificateReloadFails` | new certificate files failed to load in the last 15 minutes (workspaces) | warning |
| `DatabaseConnectionWaits` | queries of a module spend more than 10% of the time waiting for a pool connection, for 10 minutes | warning |
| `Restarts` | a process restarted more than twice in 15 minutes | warning |
| `SpansDropped` | a module dropped spans in the last 15 minutes | warning |

Compose has no Alertmanager: the alerts show on the Alerts page of Prometheus. In a cluster, the Alertmanager of the Prometheus Operator routes them.

### Dashboard

The Grafana dashboard, named after the project, shows per module the rate, errors and 95th percentile of HTTP requests and of gRPC calls, the slowest routes, retries and open circuits, database connections in use and the time spent waiting for one, heap, goroutines and dropped spans. It reads its data source from a variable, so the same file works with the provisioned Prometheus of Compose and with the one of a cluster.

## CORS

Browsers only let a page call an API on another origin when the API allows it. Set `CORS_ALLOWED_ORIGINS` and the API, or the gateway of a workspace, answers preflight requests and adds the CORS headers for the listed origins.

| Variable | Default | Meaning |
|---|---|---|
| `CORS_ALLOWED_ORIGINS` | empty | Comma-separated origins such as `https://app.example.com`, subdomain patterns such as `https://*.example.com`, or `*` for any origin. Empty disables CORS. |
| `CORS_ALLOW_CREDENTIALS` | `false` | Lets the browser send cookies cross-origin. Requires explicit origins, not `*`. |
| `CORS_MAX_AGE` | `10m` | How long the browser may cache a preflight answer. |

Allowed methods are `GET`, `POST`, `PUT`, `PATCH` and `DELETE`; allowed request headers are `Authorization`, `Content-Type`, `X-Request-ID`, `traceparent` and `tracestate`; `Location`, `Retry-After`, `WWW-Authenticate` and `X-Request-ID` are exposed to scripts. Requests from origins that are not listed get no CORS headers, so the browser blocks them.

A front end on another origin that sends `Authorization: Bearer` only needs `CORS_ALLOWED_ORIGINS`. One that relies on the authentication cookie needs more, because the cookie is `SameSite=Strict` and cookie requests pass `http.CrossOriginProtection`: the two origins must share a site (for example `app.example.com` and `api.example.com`), and the origin must also be listed in `AUTH_TRUSTED_ORIGINS` with `CORS_ALLOW_CREDENTIALS=true`.

## Wiring an older project

`main.go` belongs to you and codebuild never rewrites it, so a project created before 0.3.0 gets the generated packages but keeps metrics, tracing and CORS off. `codebuild plan` warns about it. Add this to `cmd/api/main.go` (the gateway uses `"gateway"` as the service name and wraps `auth.Anonymous(signer, logger, mux)` instead of `mux`):

```go
telemetryConfig, err := telemetry.LoadConfig(os.Getenv, "api")
if err != nil {
	return err
}
cors, err := httpx.LoadCORS(os.Getenv)
if err != nil {
	return err
}
tel, err := telemetry.Start(telemetryConfig, logger)
if err != nil {
	return err
}

// after opening the database
telemetry.ObserveDB(db)

// instead of httpx.RequestID(httpx.Recover(logger, httpx.AccessLog(logger, mux)))
var handler http.Handler = httpx.CORS(cors, mux)
handler = httpx.AccessLog(logger, handler)
handler = httpx.Recover(logger, handler)
handler = telemetry.HTTP(mux, handler, httpx.LivenessPath, httpx.ReadinessPath)
handler = httpx.RequestID(handler)

// after srv.Shutdown
return tel.Shutdown(shutdownCtx)
```

A service of a workspace only needs `telemetry.LoadConfig` with its name, `telemetry.Start`, `telemetry.ObserveDB(db)` and `tel.Shutdown`: the gRPC interceptors in `internal/platform/grpcx` already trace and count every call once telemetry is started.
