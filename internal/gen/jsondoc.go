package gen

import (
	"bytes"
	"encoding/json"
)

// object is a JSON object that keeps its keys in insertion order, so generated
// documents read top-down ("openapi", "info", "paths") and stay deterministic.
type object []member

type member struct {
	key   string
	value any
}

func (o object) set(key string, value any) object {
	for i := range o {
		if o[i].key == key {
			o[i].value = value
			return o
		}
	}
	return append(o, member{key, value})
}

func (o object) get(key string) any {
	for _, m := range o {
		if m.key == key {
			return m.value
		}
	}
	return nil
}

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := marshalPlain(m.key)
		if err != nil {
			return nil, err
		}
		value, err := marshalPlain(m.value)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// marshalPlain encodes v without escaping <, > and &, which appear in
// descriptions and patterns and would otherwise become unicode escapes.
func marshalPlain(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// indentDocument renders a document as indented JSON with a final newline.
func indentDocument(doc object) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
