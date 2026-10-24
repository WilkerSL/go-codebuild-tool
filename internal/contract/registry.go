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
	// Renumbered describes the fields that changed type in this build and
	// therefore received a new number.
	Renumbered []string `json:"-"`
}

type MessageNumbers struct {
	Fields   map[string]int32  `json:"fields"`
	Types    map[string]string `json:"types,omitempty"`
	Reserved []int32           `json:"reserved,omitempty"`
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

// assign keeps field numbers stable: known fields keep their number, new ones take the
// next free number and removed ones are reserved, so old clients can still read what
// new servers send. A field whose wire type changes is treated as removed and added
// again: reusing its number would make old clients fail to parse, or misread, it.
func (r *Registry) assign(message string, names, types []string) (map[string]int32, []int32) {
	entry := r.Messages[message]
	if entry == nil {
		entry = &MessageNumbers{Fields: map[string]int32{}}
		r.Messages[message] = entry
	}
	if entry.Types == nil {
		entry.Types = map[string]string{}
	}
	var highest int32
	for _, n := range entry.Fields {
		highest = max(highest, n)
	}
	for _, n := range entry.Reserved {
		highest = max(highest, n)
	}
	for name, n := range entry.Fields {
		i := slices.Index(names, name)
		old := entry.Types[name]
		changed := i >= 0 && old != "" && old != types[i]
		if i < 0 || changed {
			delete(entry.Fields, name)
			delete(entry.Types, name)
			entry.Reserved = append(entry.Reserved, n)
		}
		if changed {
			r.Renumbered = append(r.Renumbered, fmt.Sprintf("%s.%s changed from %s to %s; its number %d is now reserved", message, name, old, types[i], n))
		}
	}
	for i, name := range names {
		if _, ok := entry.Fields[name]; !ok {
			highest++
			entry.Fields[name] = highest
		}
		entry.Types[name] = types[i]
	}
	slices.Sort(r.Renumbered)
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
