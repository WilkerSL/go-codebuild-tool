package gen

import (
	"path"
	"slices"
	"strconv"
	"strings"
)

// The observability stack is generated next to the code: Jaeger with its
// Monitor tab fed by span metrics, Prometheus with alert rules, and Grafana
// with a dashboard of the project. Docker Compose runs it under the
// observability profile and Kubernetes gets it as a kustomize component. All
// three panels live under /_ops/, so the API or the gateway can put them
// behind the login of the project.
const (
	ObservabilityDir     = "deploy/observability"
	KubeObservabilityDir = "deploy/kubernetes/components/observability"
	JaegerImage          = "jaegertracing/jaeger:2.21.0"
	PrometheusImage      = "prom/prometheus:v3.14.0"
	GrafanaImage         = "grafana/grafana:12.4.2"
	// opsRoute keeps proxied panels out of the alerts and the dashboard: their
	// requests are the operators', not the users'.
	opsRoute = `route!~"/_ops/.*"`
)

// opsView describes the operations panels of a project whose accounts have
// roles. Without roles any registered account could open them, so such
// projects keep the panels off the API and reachable only from inside.
type opsView struct {
	Roles         string
	DefaultRoles  string
	IdentityLabel string
}

func newOpsView(av *authView) *opsView {
	if av == nil || av.Entity.Role() == nil {
		return nil
	}
	enum := av.Entity.Role().Enum
	quoted := make([]string, len(enum))
	for i, r := range enum {
		quoted[i] = strconv.Quote(r)
	}
	label := noun(av.Entity.Identity().Name)
	ov := &opsView{Roles: strings.Join(quoted, ", "), IdentityLabel: strings.ToUpper(label[:1]) + label[1:]}
	if slices.Contains(enum, "admin") {
		ov.DefaultRoles = "admin"
	}
	return ov
}

type alertRule struct {
	Name     string
	Expr     string
	For      string
	Severity string
	Summary  string
}

type scrapeTarget struct {
	Service string
	Address string
}

type observabilityView struct {
	YAMLHeader      string
	Project         string
	Entry           string
	Workspace       bool
	Ops             *opsView
	Targets         []scrapeTarget
	Alerts          []alertRule
	JaegerImage     string
	PrometheusImage string
	GrafanaImage    string
}

func newObservabilityView(project, entry string, workspace bool, ops *opsView, services []string) *observabilityView {
	ov := &observabilityView{YAMLHeader: YAMLHeader, Project: project, Entry: entry, Workspace: workspace, Ops: ops,
		JaegerImage: JaegerImage, PrometheusImage: PrometheusImage, GrafanaImage: GrafanaImage}
	for _, name := range append([]string{entry}, services...) {
		ov.Targets = append(ov.Targets, scrapeTarget{Service: name, Address: name + ":9090"})
	}
	ov.Alerts = alertRules(project, workspace)
	return ov
}

func alertRules(project string, workspace bool) []alertRule {
	sel := `project="` + project + `"`
	rules := []alertRule{
		{Name: "TargetDown", Expr: `up{` + sel + `} == 0`, For: "2m", Severity: "critical",
			Summary: "{{ $labels.service }} ({{ $labels.instance }}) does not answer the metrics scrape"},
		{Name: "HTTPServerErrors", For: "5m", Severity: "critical",
			Expr:    `sum by (service) (rate(http_server_requests_total{` + sel + `,` + opsRoute + `,status=~"5.."}[5m])) / sum by (service) (rate(http_server_requests_total{` + sel + `,` + opsRoute + `}[5m])) > 0.05`,
			Summary: "More than 5% of the HTTP requests of {{ $labels.service }} fail with a server error"},
		{Name: "HTTPLatency", For: "10m", Severity: "warning",
			Expr:    `histogram_quantile(0.95, sum by (service, le) (rate(http_server_request_duration_seconds_bucket{` + sel + `,` + opsRoute + `}[5m]))) > 1`,
			Summary: "The slowest 5% of the HTTP requests of {{ $labels.service }} take more than {{ $value | humanizeDuration }}"},
	}
	if workspace {
		rules = append(rules,
			alertRule{Name: "GRPCServerErrors", For: "5m", Severity: "critical",
				Expr:    `sum by (service) (rate(grpc_server_handled_total{` + sel + `,code=~"Unknown|Internal|Unavailable|DataLoss|DeadlineExceeded"}[5m])) / sum by (service) (rate(grpc_server_handled_total{` + sel + `}[5m])) > 0.05`,
				Summary: "More than 5% of the gRPC calls to {{ $labels.service }} fail"},
			alertRule{Name: "GRPCLatency", For: "10m", Severity: "warning",
				Expr:    `histogram_quantile(0.95, sum by (service, le) (rate(grpc_server_handling_seconds_bucket{` + sel + `}[5m]))) > 1`,
				Summary: "The slowest 5% of the gRPC calls to {{ $labels.service }} take more than {{ $value | humanizeDuration }}"},
			alertRule{Name: "CircuitOpen", For: "1m", Severity: "warning",
				Expr:    `max by (service, target) (grpc_client_circuit_open{` + sel + `}) > 0`,
				Summary: "{{ $labels.service }} stopped calling {{ $labels.target }}: the circuit breaker is open"},
			alertRule{Name: "CertificateExpiresSoon", For: "1h", Severity: "warning",
				Expr:    `min by (service) (tls_certificate_expiry_timestamp_seconds{` + sel + `}) - time() < 7 * 86400`,
				Summary: "The certificate of {{ $labels.service }} expires in {{ $value | humanizeDuration }}"},
			alertRule{Name: "CertificateReloadFails", For: "0m", Severity: "warning",
				Expr:    `increase(tls_reload_errors_total{` + sel + `}[15m]) > 0`,
				Summary: "{{ $labels.service }} cannot load its new certificate files and keeps the old ones"},
		)
	}
	rules = append(rules,
		alertRule{Name: "DatabaseConnectionWaits", For: "10m", Severity: "warning",
			Expr:    `sum by (service) (rate(db_client_connection_wait_seconds_total{` + sel + `}[5m])) > 0.1`,
			Summary: "Queries of {{ $labels.service }} wait for a free database connection"},
		alertRule{Name: "Restarts", For: "0m", Severity: "warning",
			Expr:    `changes(process_start_time_seconds{` + sel + `}[15m]) > 2`,
			Summary: "{{ $labels.service }} ({{ $labels.instance }}) restarted {{ $value }} times in 15 minutes"},
		alertRule{Name: "SpansDropped", For: "0m", Severity: "warning",
			Expr:    `increase(telemetry_spans_dropped_total{` + sel + `}[15m]) > 0`,
			Summary: "{{ $labels.service }} drops spans: the trace collector is down or too slow"},
	)
	for i := range rules {
		rules[i].Expr = strconv.Quote(rules[i].Expr)
		rules[i].Summary = strconv.Quote(rules[i].Summary)
	}
	return rules
}

// dashboard builds the Grafana dashboard of the project: the rate, errors and
// duration of every module, then the resources behind them. Panels read the
// datasource from a variable, so the same file works with the provisioned
// Prometheus of Docker Compose and with the one of a cluster.
func dashboard(ov *observabilityView) ([]byte, error) {
	sel := `project="` + ov.Project + `"`
	httpSel := `{` + sel + `,` + opsRoute
	var panels []any
	y := 0
	add := func(title, unit string, exprs ...[2]string) {
		var targets []any
		for i, e := range exprs {
			targets = append(targets, object{}.
				set("refId", string(rune('A'+i))).
				set("datasource", object{}.set("type", "prometheus").set("uid", "${datasource}")).
				set("expr", e[0]).
				set("legendFormat", e[1]))
		}
		x := 0
		if len(panels)%2 == 1 {
			x = 12
		}
		panels = append(panels, object{}.
			set("id", len(panels)+1).
			set("type", "timeseries").
			set("title", title).
			set("datasource", object{}.set("type", "prometheus").set("uid", "${datasource}")).
			set("gridPos", object{}.set("h", 8).set("w", 12).set("x", x).set("y", y)).
			set("fieldConfig", object{}.set("defaults", object{}.set("unit", unit)).set("overrides", []any{})).
			set("options", object{}.
				set("legend", object{}.set("displayMode", "list").set("placement", "bottom").set("showLegend", true)).
				set("tooltip", object{}.set("mode", "multi").set("sort", "desc"))).
			set("targets", targets))
		if x == 12 {
			y += 8
		}
	}
	add("HTTP requests", "reqps",
		[2]string{`sum by (service) (rate(http_server_requests_total` + httpSel + `}[$__rate_interval]))`, "{{service}}"})
	add("HTTP server errors", "percentunit",
		[2]string{`(sum by (service) (rate(http_server_requests_total` + httpSel + `,status=~"5.."}[$__rate_interval])) or 0 * sum by (service) (rate(http_server_requests_total` + httpSel + `}[$__rate_interval]))) / sum by (service) (rate(http_server_requests_total` + httpSel + `}[$__rate_interval]))`, "{{service}}"})
	add("HTTP latency (p95)", "s",
		[2]string{`histogram_quantile(0.95, sum by (service, le) (rate(http_server_request_duration_seconds_bucket` + httpSel + `}[$__rate_interval])))`, "{{service}}"})
	add("Slowest routes (p95)", "s",
		[2]string{`topk(5, histogram_quantile(0.95, sum by (method, route, le) (rate(http_server_request_duration_seconds_bucket` + httpSel + `}[$__rate_interval]))))`, "{{method}} {{route}}"})
	if ov.Workspace {
		add("gRPC calls", "reqps",
			[2]string{`sum by (service) (rate(grpc_server_handled_total{` + sel + `}[$__rate_interval]))`, "{{service}}"})
		add("gRPC errors", "percentunit",
			[2]string{`(sum by (service) (rate(grpc_server_handled_total{` + sel + `,code=~"Unknown|Internal|Unavailable|DataLoss|DeadlineExceeded"}[$__rate_interval])) or 0 * sum by (service) (rate(grpc_server_handled_total{` + sel + `}[$__rate_interval]))) / sum by (service) (rate(grpc_server_handled_total{` + sel + `}[$__rate_interval]))`, "{{service}}"})
		add("gRPC latency (p95)", "s",
			[2]string{`histogram_quantile(0.95, sum by (service, le) (rate(grpc_server_handling_seconds_bucket{` + sel + `}[$__rate_interval])))`, "{{service}}"})
		add("Retries and open circuits", "short",
			[2]string{`sum by (service, target) (rate(grpc_client_retries_total{` + sel + `}[$__rate_interval]))`, "retries {{service}} → {{target}}"},
			[2]string{`sum by (service, target) (grpc_client_circuit_open{` + sel + `})`, "open {{service}} → {{target}}"})
	}
	add("Database connections in use", "short",
		[2]string{`sum by (service) (db_client_connections_in_use{` + sel + `})`, "{{service}}"},
		[2]string{`sum by (service) (db_client_connections_max{` + sel + `})`, "max {{service}}"})
	add("Time waiting for a database connection", "s",
		[2]string{`sum by (service) (rate(db_client_connection_wait_seconds_total{` + sel + `}[$__rate_interval]))`, "{{service}}"})
	add("Heap", "bytes",
		[2]string{`sum by (service) (go_heap_objects_bytes{` + sel + `})`, "{{service}}"})
	add("Goroutines and dropped spans", "short",
		[2]string{`sum by (service) (go_goroutines{` + sel + `})`, "goroutines {{service}}"},
		[2]string{`sum by (service) (increase(telemetry_spans_dropped_total{` + sel + `}[$__rate_interval]))`, "dropped spans {{service}}"})
	uid := ov.Project
	if len(uid) > 40 {
		uid = uid[:40]
	}
	doc := object{}.
		set("x-codebuild", OpenAPIMarker).
		set("uid", uid).
		set("title", ov.Project).
		set("tags", []string{"codebuild"}).
		set("timezone", "browser").
		set("schemaVersion", 39).
		set("version", 1).
		set("editable", false).
		set("refresh", "30s").
		set("time", object{}.set("from", "now-1h").set("to", "now")).
		set("templating", object{}.set("list", []any{object{}.
			set("name", "datasource").
			set("label", "Prometheus").
			set("type", "datasource").
			set("query", "prometheus").
			set("current", object{}).
			set("hide", 0)})).
		set("annotations", object{}.set("list", []any{})).
		set("panels", panels)
	return indentDocument(doc)
}

// jaegerUI turns on the Monitor and System Architecture tabs, which read the
// span metrics and the dependencies of the stored traces.
const jaegerUI = `{
  "x-codebuild": "` + OpenAPIMarker + `",
  "monitor": {
    "menuEnabled": true
  },
  "dependencies": {
    "menuEnabled": true
  }
}
`

func (g *generator) observability(ov *observabilityView, compose, kube bool) {
	if g.err != nil {
		return
	}
	board, err := dashboard(ov)
	if err != nil {
		g.err = err
		return
	}
	raw := func(filePath string, content []byte) {
		g.files = append(g.files, File{Path: path.Join(g.prefix, filePath), Content: content, Owner: OwnerGenerated, Module: g.prefix})
	}
	if compose {
		g.generatedFile(ObservabilityDir+"/jaeger.yaml", "observability/jaeger.yaml.tmpl", ov)
		raw(ObservabilityDir+"/jaeger-ui.json", []byte(jaegerUI))
		g.generatedFile(ObservabilityDir+"/prometheus.yml", "observability/prometheus.yml.tmpl", ov)
		g.generatedFile(ObservabilityDir+"/alerts.yml", "observability/alerts.yml.tmpl", ov)
		g.generatedFile(ObservabilityDir+"/grafana/provisioning/datasources/datasources.yml", "observability/datasources.yml.tmpl", ov)
		g.generatedFile(ObservabilityDir+"/grafana/provisioning/dashboards/dashboards.yml", "observability/dashboards.yml.tmpl", ov)
		raw(ObservabilityDir+"/grafana/dashboards/"+ov.Project+".json", board)
	}
	if !kube {
		return
	}
	g.generatedFile(KubeObservabilityDir+"/kustomization.yaml", "observability/kustomization.yaml.tmpl", ov)
	g.generatedFile(KubeObservabilityDir+"/jaeger.yaml", "observability/kube_jaeger.yaml.tmpl", ov)
	g.generatedFile(KubeObservabilityDir+"/monitoring.yaml", "observability/kube_monitoring.yaml.tmpl", ov)
	g.generatedFile(KubeObservabilityDir+"/jaeger-config.yaml", "observability/jaeger.yaml.tmpl", ov)
	raw(KubeObservabilityDir+"/jaeger-ui.json", []byte(jaegerUI))
	raw(KubeObservabilityDir+"/dashboard.json", board)
}

// ops adds the reverse proxy that serves the panels under /_ops/ to the
// accounts with an operations role.
func (g *generator) ops(pv *projectView) {
	if pv.Ops == nil {
		return
	}
	g.goFile("internal/platform/ops/ops_gen.go", "platform/ops.go.tmpl", pv)
	g.goFile("internal/platform/ops/ops_gen_test.go", "platform/ops_test.go.tmpl", pv)
}
