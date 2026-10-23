package gen

import (
	"slices"
	"strings"
)

// Kubernetes manifests live in a generated kustomize base and a user-owned
// overlay. The base only references Secrets by name: codebuild never writes
// credentials, and the overlay is where images, namespaces and patches go.
const (
	KubeBaseDir    = "deploy/kubernetes/base"
	KubeOverlayDir = "deploy/kubernetes/overlays/production"
	kubeCertsPath  = "/etc/tls"
	kubeTokenPath  = "/etc/internal-token"
)

type kubePort struct {
	Name     string
	Port     int
	Protocol string
}

type kubeSecretEnv struct {
	Env      string
	Secret   string
	Key      string
	Optional bool
}

type kubeVolume struct {
	Name     string
	Path     string
	Secret   string
	Items    [][2]string
	Claim    string
	ReadOnly bool
}

type kubeWorkload struct {
	YAMLHeader string
	Project    string
	Name       string
	Component  string
	Image      string
	Ports      []kubePort
	Service    []kubePort
	Probe      string
	Config     [][2]string
	Secrets    []kubeSecretEnv
	Volumes    []kubeVolume
	Replicas   int
	Recreate   bool
	Command    []string
	Metrics    bool
}

type kubeView struct {
	YAMLHeader string
	Project    string
	Workloads  []*kubeWorkload
	Jobs       []*kubeWorkload
	Claims     []string
	Resources  []string
	Images     []string
}

func (w *kubeWorkload) File() string {
	return w.Name + ".yaml"
}

func newKubeView(project string) *kubeView {
	return &kubeView{YAMLHeader: YAMLHeader, Project: project}
}

func (kv *kubeView) finish() {
	for _, w := range append(slices.Clone(kv.Workloads), kv.Jobs...) {
		w.YAMLHeader, w.Project = kv.YAMLHeader, kv.Project
	}
	for _, w := range kv.Workloads {
		kv.Resources = append(kv.Resources, w.File())
	}
	for _, j := range kv.Jobs {
		kv.Resources = append(kv.Resources, j.File())
	}
	if len(kv.Claims) > 0 {
		kv.Resources = append(kv.Resources, "data.yaml")
	}
	slices.Sort(kv.Resources)
	for _, w := range append(slices.Clone(kv.Workloads), kv.Jobs...) {
		if !slices.Contains(kv.Images, w.Image) {
			kv.Images = append(kv.Images, w.Image)
		}
	}
	slices.Sort(kv.Images)
}

func (g *generator) kubernetes(kv *kubeView) {
	kv.finish()
	for _, w := range kv.Workloads {
		g.generatedFile(KubeBaseDir+"/"+w.File(), "kubernetes/workload.yaml.tmpl", w)
	}
	for _, j := range kv.Jobs {
		g.generatedFile(KubeBaseDir+"/"+j.File(), "kubernetes/job.yaml.tmpl", j)
	}
	if len(kv.Claims) > 0 {
		g.generatedFile(KubeBaseDir+"/data.yaml", "kubernetes/data.yaml.tmpl", kv)
	}
	g.generatedFile(KubeBaseDir+"/kustomization.yaml", "kubernetes/kustomization.yaml.tmpl", kv)
	g.userFile(KubeOverlayDir+"/kustomization.yaml", "kubernetes/overlay.yaml.tmpl", kv)
	g.userFile(KubeOverlayDir+"/deployments.yaml", "kubernetes/overlay_patch.yaml.tmpl", kv)
}

func monolithKube(pv *projectView, module string) *kubeView {
	project := composeProject(module)
	kv := newKubeView(project)
	sqlite := pv.Dialect.Embedded
	api := &kubeWorkload{Name: "api", Component: "api", Image: project + "-api", Probe: "http", Replicas: 2, Metrics: true,
		Ports:   []kubePort{{"http", 8080, ""}, {"metrics", 9090, ""}},
		Service: []kubePort{{"http", 8080, ""}},
		Config:  [][2]string{{"HTTP_ADDR", ":8080"}, {"METRICS_ADDR", ":9090"}, {"SHUTDOWN_DELAY", "5s"}},
	}
	migrate := &kubeWorkload{Name: "migrate", Component: "migration", Image: api.Image, Command: []string{"/app/migrate", "up"}}
	if sqlite {
		api.Replicas, api.Recreate = 1, true
		kv.Claims = append(kv.Claims, project+"-data")
		for _, w := range []*kubeWorkload{api, migrate} {
			w.Config = append(w.Config, [2]string{"DATABASE_URL", "/data/app.db"})
			w.Volumes = append(w.Volumes, kubeVolume{Name: "data", Path: "/data", Claim: project + "-data"})
		}
	} else {
		for _, w := range []*kubeWorkload{api, migrate} {
			w.Secrets = append(w.Secrets, kubeSecretEnv{Env: "DATABASE_URL", Secret: project + "-database", Key: "url"})
		}
	}
	if pv.Auth != nil {
		if !sqlite {
			api.Config = append(api.Config, [2]string{"AUTH_LIMIT_STORE", "database"})
		}
		if pv.Auth.JWT {
			api.Secrets = append(api.Secrets,
				kubeSecretEnv{Env: "AUTH_JWT_SECRET", Secret: project + "-auth", Key: "jwt-secret"},
				kubeSecretEnv{Env: "AUTH_JWT_PREVIOUS_SECRET", Secret: project + "-auth", Key: "jwt-previous-secret", Optional: true})
		}
	}
	kv.Workloads = []*kubeWorkload{api}
	kv.Jobs = []*kubeWorkload{migrate}
	return kv
}

func workspaceKube(cv *composeView, wv *workspaceView, pv *projectView) *kubeView {
	kv := newKubeView(cv.Project)
	sqlite := cv.Dialect.Name == "sqlite"
	tls := func(w *kubeWorkload, module string) {
		w.Config = append(w.Config,
			[2]string{"TLS_CA_FILE", kubeCertsPath + "/ca.crt"},
			[2]string{"TLS_CERT_FILE", kubeCertsPath + "/tls.crt"},
			[2]string{"TLS_KEY_FILE", kubeCertsPath + "/tls.key"})
		w.Volumes = append(w.Volumes, kubeVolume{Name: "tls", Path: kubeCertsPath, Secret: cv.Project + "-" + module + "-tls", ReadOnly: true})
	}
	database := func(w *kubeWorkload, db *databaseView) {
		if sqlite {
			claim := cv.Project + "-" + db.Name + "-data"
			if !slices.Contains(kv.Claims, claim) {
				kv.Claims = append(kv.Claims, claim)
			}
			w.Config = append(w.Config, [2]string{"DATABASE_URL", db.URL})
			w.Volumes = append(w.Volumes, kubeVolume{Name: "data", Path: "/data", Claim: claim})
			w.Replicas, w.Recreate = 1, true
			return
		}
		w.Secrets = append(w.Secrets, kubeSecretEnv{Env: "DATABASE_URL", Secret: cv.Project + "-" + db.Name + "-database", Key: "url"})
	}
	for _, job := range cv.Jobs {
		j := &kubeWorkload{Name: job.Name, Component: "migration", Image: job.Image, Command: []string{"/app/migrate", "up"}}
		database(j, job.Database)
		kv.Jobs = append(kv.Jobs, j)
	}
	gateway := &kubeWorkload{Name: "gateway", Component: "gateway", Image: cv.Project + "-gateway", Probe: "http", Replicas: 2, Metrics: true,
		Ports:   []kubePort{{"http", 8080, ""}, {"metrics", 9090, ""}},
		Service: []kubePort{{"http", 8080, ""}},
		Config:  [][2]string{{"HTTP_ADDR", ":8080"}, {"METRICS_ADDR", ":9090"}, {"SHUTDOWN_DELAY", "5s"}},
	}
	if pv.Auth != nil {
		gateway.Config = append(gateway.Config, [2]string{"AUTH_LIMIT_STORE", "database"})
	}
	tls(gateway, "gateway")
	gateway.Config = append(gateway.Config, [2]string{"INTERNAL_TOKEN_PRIVATE_KEY_FILE", kubeTokenPath + "/private.pem"})
	gateway.Volumes = append(gateway.Volumes, kubeVolume{Name: "internal-token", Path: kubeTokenPath, Secret: cv.Project + "-internal-token", Items: [][2]string{{"private.pem", "private.pem"}}, ReadOnly: true})
	for _, sv := range wv.Services {
		db := cv.database(sv.Database)
		w := &kubeWorkload{Name: sv.Name, Component: "service", Image: cv.Project + "-" + sv.Name, Probe: "health", Replicas: 2, Metrics: true,
			Ports:   []kubePort{{"grpc", 9000, "grpc"}, {"health", 8081, ""}, {"metrics", 9090, ""}},
			Service: []kubePort{{"grpc", 9000, "grpc"}},
			Config:  [][2]string{{"GRPC_ADDR", ":9000"}, {"HEALTH_ADDR", ":8081"}, {"METRICS_ADDR", ":9090"}, {"SHUTDOWN_DELAY", "5s"}},
		}
		database(w, db)
		tls(w, sv.Name)
		w.Config = append(w.Config, [2]string{"INTERNAL_TOKEN_PUBLIC_KEY_FILE", kubeTokenPath + "/public.pem"})
		w.Volumes = append(w.Volumes, kubeVolume{Name: "internal-token", Path: kubeTokenPath, Secret: cv.Project + "-internal-token", Items: [][2]string{{"public.pem", "public.pem"}}, ReadOnly: true})
		for _, peer := range sv.Peers {
			w.Config = append(w.Config, [2]string{peer.Env, peer.Name + ":9000"})
		}
		if sv.Identity && pv.Auth != nil && pv.Auth.JWT {
			w.Secrets = append(w.Secrets,
				kubeSecretEnv{Env: "AUTH_JWT_SECRET", Secret: cv.Project + "-auth", Key: "jwt-secret"},
				kubeSecretEnv{Env: "AUTH_JWT_PREVIOUS_SECRET", Secret: cv.Project + "-auth", Key: "jwt-previous-secret", Optional: true})
		}
		kv.Workloads = append(kv.Workloads, w)
		gateway.Config = append(gateway.Config, [2]string{sv.Env, sv.Name + ":9000"})
	}
	kv.Workloads = append(kv.Workloads, gateway)
	slices.SortFunc(kv.Workloads, func(a, b *kubeWorkload) int { return strings.Compare(a.Name, b.Name) })
	slices.Sort(kv.Claims)
	return kv
}

func (cv *composeView) database(name string) *databaseView {
	for _, db := range cv.Databases {
		if db.Name == name {
			return db
		}
	}
	return &databaseView{Name: name}
}
