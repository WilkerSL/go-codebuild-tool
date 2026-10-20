package gen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
)

func pinnedModules(t *testing.T) [][2]string {
	t.Helper()
	pinned := [][2]string{
		{GRPCModule, GRPCVersion},
		{ProtobufModule, ProtobufVersion},
		{googleUUID, googleUUIDVersion},
	}
	for _, name := range dialect.Names() {
		d, err := dialect.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		pinned = append(pinned, [2]string{d.Driver().Module, d.Driver().Version})
	}
	return pinned
}

func TestProtobufRuntimeMatchesTheGenerator(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ProtobufModule {
			if fields[1] != ProtobufVersion {
				t.Fatalf("go.mod uses %s %s to generate .pb.go files, but generated modules require %s; set ProtobufVersion to %s and run go test ./internal/project -update",
					ProtobufModule, fields[1], ProtobufVersion, fields[1])
			}
			return
		}
	}
	t.Fatalf("go.mod does not require %s", ProtobufModule)
}

func escapeModule(path string) string {
	var b strings.Builder
	for _, r := range path {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestPinnedModulesAreCurrent(t *testing.T) {
	if os.Getenv("CODEBUILD_CHECK_PINNED") != "1" {
		t.Skip("set CODEBUILD_CHECK_PINNED=1 to compare the module versions written into generated go.mod files with the module proxy")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for _, m := range pinnedModules(t) {
		resp, err := client.Get(fmt.Sprintf("https://proxy.golang.org/%s/@latest", escapeModule(m[0])))
		if err != nil {
			t.Fatalf("%s: %v", m[0], err)
		}
		var latest struct{ Version string }
		err = json.NewDecoder(resp.Body).Decode(&latest)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d: %v", m[0], resp.StatusCode, err)
		}
		if latest.Version != m[1] {
			t.Errorf("%s is pinned at %s but %s is available", m[0], m[1], latest.Version)
		}
	}
}
