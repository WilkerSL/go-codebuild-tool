# Observability and CORS

Generated projects can expose Prometheus metrics, export OpenTelemetry traces and answer cross-origin requests. All three are off until you set an environment variable, and all of them use the standard library only: there is no Prometheus client or OpenTelemetry SDK in `go.mod`.

The code lives in `internal/platform/telemetry` and `internal/platform/httpx/cors_gen.go`, both generated. A `main.go` created by codebuild 0.3.0 or later wires them; see [Wiring an older project](#wiring-an-older-project) for projects created before.

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

## CORS

Browsers only let a page call an API on another origin when the API allows it. Set `CORS_ALLOWED_ORIGINS` and the API, or the gateway of a workspace, answers preflight requests and adds the CORS headers for the listed origins.

| Variable | Default | Meaning |
|---|---|---|
| `CORS_ALLOWED_ORIGINS` | empty | Comma-separated origins such as `https://app.example.com`, subdomain patterns such as `https://*.example.com`, or `*` for any origin. Empty disables CORS. |
| `CORS_ALLOW_CREDENTIALS` | `false` | Lets the browser send cookies cross-origin. Requires explicit origins, not `*`. |
| `CORS_MAX_AGE` | `10m` | How long the browser may cache a preflight answer. |

Allowed methods are `GET`, `POST`, `PUT` and `DELETE`; allowed request headers are `Authorization`, `Content-Type`, `X-Request-ID`, `traceparent` and `tracestate`; `Location`, `Retry-After`, `WWW-Authenticate` and `X-Request-ID` are exposed to scripts. Requests from origins that are not listed get no CORS headers, so the browser blocks them.

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
