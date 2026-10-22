package project

import "testing"

func FuzzParseConfig(f *testing.F) {
	f.Add([]byte(`{"dialect":"postgres","pagination":{"defaultLimit":20,"maxLimit":100},"manualWiring":[]}`))
	f.Add([]byte(`{"dialect":"sqlite"}`))
	f.Add([]byte(`{"dialect":"oracle"}`))
	f.Add([]byte(`{"dialect":"mysql","pagination":{"defaultLimit":0,"maxLimit":-1}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := ParseConfig(data)
		if err != nil {
			return
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("ParseConfig accepted an invalid configuration: %v", err)
		}
		again, err := ParseConfig(cfg.Marshal())
		if err != nil {
			t.Fatalf("a marshaled configuration does not parse: %v", err)
		}
		if string(again.Marshal()) != string(cfg.Marshal()) {
			t.Fatal("configuration encoding is not stable")
		}
	})
}
