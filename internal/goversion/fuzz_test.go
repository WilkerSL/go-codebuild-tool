package goversion

import "testing"

func FuzzParseModFile(f *testing.F) {
	f.Add([]byte("module example.com/x\n\ngo 1.27.0\n"))
	f.Add([]byte("module example.com/x\n\ngo 1.26\ntoolchain go1.27.1\n"))
	f.Add([]byte("module\n"))
	f.Add([]byte("go 1.999999999999999999\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if m, err := ParseModFile("go.mod", data); err == nil {
			_, _ = m.Target()
			_ = m.RequiredToolchain()
		}
		if w, err := ParseWorkFile("go.work", data); err == nil {
			_ = w.RequiredToolchain()
		}
	})
}

func FuzzResolve(f *testing.F) {
	for _, seed := range []string{"1.26", "1.26.0", "1.27.3", "1.30", "1.21", "2.0", "1.27rc1", "", "go1.27"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, v string) {
		target, err := Resolve(v)
		if err != nil {
			return
		}
		if target.GoVersion == "" {
			t.Fatalf("Resolve(%q) returned an empty version", v)
		}
		_ = UpgradeTarget("go" + v)
	})
}
