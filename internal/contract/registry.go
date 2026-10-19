package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

const RegistryVersion = 1

type Registry struct {
	Version  int                        `json:"version"`
	Messages map[string]*MessageNumbers `json:"messages"`
}

type MessageNumbers struct {
	Fields   map[string]int32 `json:"fields"`
	Reserved []int32          `json:"reserved,omitempty"`
}

func NewRegistry() *Registry {
	return &Registry{Version: RegistryVersion, Messages: map[string]*MessageNumbers{}}
}

func UnmarshalRegistry(data []byte) (*Registry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var r Registry
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("decode contract registry: %w", err)
	}
	if r.Version != RegistryVersion {
		return nil, fmt.Errorf("unsupported contract registry version %d (expected %d)", r.Version, RegistryVersion)
	}
	if r.Messages == nil {
		r.Messages = map[string]*MessageNumbers{}
	}
	return &r, nil
}

func (r *Registry) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (r *Registry) assign(message string, names []string) (map[string]int32, []int32) {
	entry := r.Messages[message]
	if entry == nil {
		entry = &MessageNumbers{Fields: map[string]int32{}}
		r.Messages[message] = entry
	}
	var highest int32
	for _, n := range entry.Fields {
		highest = max(highest, n)
	}
	for _, n := range entry.Reserved {
		highest = max(highest, n)
	}
	for name, n := range entry.Fields {
		if !slices.Contains(names, name) {
			delete(entry.Fields, name)
			entry.Reserved = append(entry.Reserved, n)
		}
	}
	for _, name := range names {
		if _, ok := entry.Fields[name]; !ok {
			highest++
			entry.Fields[name] = highest
		}
	}
	slices.Sort(entry.Reserved)
	entry.Reserved = slices.Compact(entry.Reserved)
	return entry.Fields, slices.Clone(entry.Reserved)
}

func (r *Registry) prune(keep map[string]bool) {
	for name := range r.Messages {
		if !keep[name] {
			delete(r.Messages, name)
		}
	}
}
