package contract

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

var dump = flag.String("dump", "", "write the generated contracts to this directory")

func shop(t *testing.T) *schema.Model {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	sources, err := spec.LoadDir(root, "specs/shop")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	return model
}

func TestGenerateShopContracts(t *testing.T) {
	set := Build("example.com/shop", shop(t), nil)
	out, err := set.Generate()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, o := range out {
		files[o.Path] = string(o.Content)
		if !strings.HasPrefix(string(o.Content), Header+"\n") {
			t.Errorf("%s lacks the generated header", o.Path)
		}
		if strings.HasSuffix(o.Path, ".go") && strings.Count(string(o.Content), "//") != 1 {
			t.Errorf("%s contains comments besides the header", o.Path)
		}
	}
	for _, p := range []string{"common/v1/common.proto", "common/v1/common.pb.go", "billing/v1/billing.proto", "billing/v1/billing.pb.go", "billing/v1/billing_grpc.pb.go", "identity/v1/identity_grpc.pb.go"} {
		if files[p] == "" {
			t.Errorf("missing %s", p)
		}
	}
	billing := files["billing/v1/billing.proto"]
	for _, want := range []string{"package billing.v1;", "message Invoice {", "optional string notes = ", "service InvoiceItemService {", "rpc ListInvoiceItems(ListInvoiceItemsRequest) returns (InvoiceItemPage);", "rpc Exists(InvoiceExistsRequest) returns (common.v1.ExistsResponse);"} {
		if !strings.Contains(billing, want) {
			t.Errorf("billing.proto lacks %q:\n%s", want, billing)
		}
	}
	identity := files["identity/v1/identity.proto"]
	if !strings.Contains(identity, "service AuthService {") || strings.Contains(identity, "password_hash") {
		t.Errorf("identity.proto:\n%s", identity)
	}
	if *dump != "" {
		for p, content := range files {
			full := filepath.Join(*dump, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestFieldNumbersAreStable(t *testing.T) {
	model := shop(t)
	registry := NewRegistry()
	Build("example.com/shop", model, registry)
	invoice := model.Entity("Invoice")
	before := registry.Messages["billing.v1.Invoice"].Fields["number"]
	notes := invoice.Field("Notes")
	invoice.Fields = append([]*schema.Field{{Name: "Reference", Kind: schema.KindString, Column: "reference", JSON: "reference"}}, invoice.Fields...)
	var kept []*schema.Field
	for _, f := range invoice.Fields {
		if f != notes {
			kept = append(kept, f)
		}
	}
	invoice.Fields = kept
	data, err := registry.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := UnmarshalRegistry(data)
	if err != nil {
		t.Fatal(err)
	}
	set := Build("example.com/shop", model, reloaded)
	msg := set.File("billing").Message("Invoice")
	if msg.Field("number").Number != before {
		t.Fatalf("number moved from %d to %d", before, msg.Field("number").Number)
	}
	if msg.Field("reference").Number <= before || len(msg.Reserved) != 1 {
		t.Fatalf("reference = %d, reserved = %v", msg.Field("reference").Number, msg.Reserved)
	}
	if !strings.Contains(string(set.File("billing").Proto()), "reserved ") {
		t.Fatal("the removed field number is not reserved")
	}
}
