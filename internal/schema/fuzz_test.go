package schema_test

import (
	"bytes"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

func FuzzUnmarshalSnapshot(f *testing.F) {
	empty, _ := schema.NewSnapshot("postgres", &schema.Database{}, nil).Marshal()
	f.Add(empty)
	f.Add([]byte(`{"version":1,"dialect":"sqlite","migrations":["0001_init"],"tables":[{"name":"t","columns":[{"name":"id","kind":"int64","identity":true}],"primaryKey":{"name":"pk_t","columns":["id"]}}]}`))
	f.Add([]byte(`{"version":2}`))
	f.Add([]byte(`{`))
	f.Fuzz(func(t *testing.T, data []byte) {
		snap, err := schema.UnmarshalSnapshot(data)
		if err != nil {
			return
		}
		first, err := snap.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		again, err := schema.UnmarshalSnapshot(first)
		if err != nil {
			t.Fatalf("a marshaled snapshot does not decode: %v", err)
		}
		second, err := again.Marshal()
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("snapshot encoding is not stable: %v", err)
		}
		_ = schema.Diff(snap.Database(), snap.Database(), schema.DiffOptions{})
	})
}
