package contract

import (
	"bytes"
	"testing"
)

func FuzzUnmarshalRegistry(f *testing.F) {
	f.Add([]byte(`{"version":1,"messages":{}}`))
	f.Add([]byte(`{"version":1,"messages":{"billing.v1.Invoice":{"fields":{"id":1,"number":3},"reserved":[2]}}}`))
	f.Add([]byte(`{"version":1,"messages":{"x":null}}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := UnmarshalRegistry(data)
		if err != nil {
			return
		}
		for name := range r.Messages {
			if r.Messages[name] == nil {
				r.Messages[name] = &MessageNumbers{Fields: map[string]int32{}}
			}
		}
		first, err := r.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		again, err := UnmarshalRegistry(first)
		if err != nil {
			t.Fatalf("a marshaled registry does not decode: %v", err)
		}
		second, err := again.Marshal()
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("registry encoding is not stable: %v", err)
		}
	})
}
