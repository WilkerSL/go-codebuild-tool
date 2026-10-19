package spec

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

func TestParseShopWorkspace(t *testing.T) {
	sources, err := LoadDir(filepath.Join(repoRoot(t), "testdata"), "specs/shop")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := Parse(sources)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diags.Error())
	}
	var names []string
	for _, s := range model.Services {
		names = append(names, s.Name+"/"+string(s.Database))
	}
	if !slices.Equal(names, []string{"billing/own", "catalog/shared", "identity/own", "inventory/shared"}) {
		t.Fatalf("services = %v", names)
	}
	item := model.Entity("InvoiceItem")
	if item == nil || item.Domain != "invoiceitems" || item.Service.Name != "billing" || item.Table != "invoice_items" {
		t.Fatalf("invoice item = %+v", item)
	}
	if f := item.Field("ProductID"); !f.External(item) || !f.CrossService(item) {
		t.Fatal("billing references the shared catalog database through an external reference")
	}
	if f := item.Field("InvoiceID"); f.External(item) || f.CrossService(item) {
		t.Fatal("references inside a service are local")
	}
	stock := model.Entity("Stock")
	if f := stock.Field("ProductID"); f.External(stock) || !f.CrossService(stock) {
		t.Fatal("services sharing a database keep their foreign keys")
	}
	if model.AuthEntity().Service.Name != "identity" {
		t.Fatal("the account entity lives in the identity service")
	}
	dbs := schema.BuildDatabases(model)
	if len(dbs) != 3 || dbs["shared"].Table("products") == nil || dbs["shared"].Table("stocks") == nil || dbs["identity"].Table(schema.SessionsTable) == nil {
		t.Fatalf("databases = %v", dbs)
	}
	if fks := dbs["billing"].Table("invoice_items").ForeignKeys; len(fks) != 1 || fks[0].RefTable != "invoices" {
		t.Fatalf("billing foreign keys = %+v", fks)
	}
	if fks := dbs["billing"].Table("invoices").ForeignKeys; len(fks) != 0 {
		t.Fatalf("the owner reference crosses databases and has no foreign key: %+v", fks)
	}
	if fks := dbs["shared"].Table("stocks").ForeignKeys; len(fks) != 1 || fks[0].RefTable != "products" {
		t.Fatalf("shared foreign keys = %+v", fks)
	}
}

func TestServiceDiagnostics(t *testing.T) {
	user := header + "import \"github.com/google/uuid\"\n\ntype IdentityService struct {\n\t_    struct{} `gen:\"database=own\"`\n\tUser struct {\n\t\tID uuid.UUID `gen:\"pk,auto\"`\n\t}\n}\n"
	tests := []struct {
		name  string
		files map[string]string
		want  []wantDiag
	}{
		{
			name:  "missing database",
			files: map[string]string{"specs/billing.go": header + "type BillingService struct {\n\tInvoice struct {\n\t\tID int64 `gen:\"pk,auto\"`\n\t}\n}\n"},
			want:  []wantDiag{{at: "BillingService", code: CodeInvalidValue, contains: "must declare its database"}},
		},
		{
			name:  "unknown database mode",
			files: map[string]string{"specs/billing.go": header + "type BillingService struct {\n\t_ struct{} `gen:\"database=owned\"`\n\tInvoice struct {\n\t\tID int64 `gen:\"pk,auto\"`\n\t}\n}\n"},
			want:  []wantDiag{{at: "database=owned", code: CodeInvalidValue, suggestion: "own"}},
		},
		{
			name:  "file named after the service",
			files: map[string]string{"specs/invoices.go": header + "type BillingService struct {\n\t_ struct{} `gen:\"database=own\"`\n\tInvoice struct {\n\t\tID int64 `gen:\"pk,auto\"`\n\t}\n}\n"},
			want:  []wantDiag{{code: CodeDomainName, suggestion: "billing.go"}},
		},
		{
			name:  "fields of a service are entities",
			files: map[string]string{"specs/billing.go": header + "type BillingService struct {\n\t_     struct{} `gen:\"database=own\"`\n\tTotal int64\n}\n"},
			want:  []wantDiag{{at: "int64", code: CodeUnsupportedType}},
		},
		{
			name: "top-level entity next to services",
			files: map[string]string{
				"specs/identity.go": user,
				"specs/tags.go":     header + "type Tag struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n",
			},
			want: []wantDiag{{file: "specs/tags.go", at: "Tag struct", code: CodeEntityCount, contains: "must be a field of one of them"}},
		},
		{
			name: "many_to_many across databases",
			files: map[string]string{
				"specs/identity.go": header + "import \"github.com/google/uuid\"\n\ntype IdentityService struct {\n\t_    struct{} `gen:\"database=own\"`\n\tUser struct {\n\t\tID   uuid.UUID `gen:\"pk,auto\"`\n\t\tTags []Tag     `gen:\"rel=many_to_many:Tag,via=user_tags\"`\n\t}\n}\n",
				"specs/tagging.go":  header + "type TaggingService struct {\n\t_   struct{} `gen:\"database=own\"`\n\tTag struct {\n\t\tID int64 `gen:\"pk,auto\"`\n\t}\n}\n",
			},
			want: []wantDiag{{file: "specs/identity.go", at: "rel=many_to_many:Tag", code: CodeInvalidCombination, contains: "different databases"}},
		},
		{
			name:  "microservices without an account entity",
			files: map[string]string{"specs/billing.go": header + "type BillingService struct {\n\t_       struct{} `gen:\"database=own\"`\n\tInvoice struct {\n\t\tID int64 `gen:\"pk,auto\"`\n\t}\n}\n"},
			want:  []wantDiag{{at: "BillingService", code: CodeEntityCount, contains: "gateway authenticates every request"}},
		},
		{
			name:  "service marker on an entity",
			files: map[string]string{"specs/billing.go": header + "type BillingService struct {\n\t_       struct{} `gen:\"database=own\"`\n\tInvoice struct {\n\t\t_  struct{} `gen:\"database=own\"`\n\t\tID int64    `gen:\"pk,auto\"`\n\t}\n}\n"},
			want:  []wantDiag{{at: "database=own\"`\n\t\tID", code: CodeMarkerScope}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := parseFiles(tt.files)
			for _, want := range tt.want {
				file := want.file
				if file == "" {
					for name := range tt.files {
						file = name
					}
				}
				if !containsDiag(diags, tt.files[file], file, want) {
					t.Errorf("missing diagnostic %+v in:\n%s", want, diags.Error())
				}
			}
		})
	}
}
