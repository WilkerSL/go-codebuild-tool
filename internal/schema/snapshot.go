package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const SnapshotVersion = 1

type Snapshot struct {
	Version int      `json:"version"`
	Dialect string   `json:"dialect"`
	Tables  []*Table `json:"tables"`
}

func NewSnapshot(dialect string, db *Database) *Snapshot {
	tables := db.Tables
	if tables == nil {
		tables = []*Table{}
	}
	return &Snapshot{Version: SnapshotVersion, Dialect: dialect, Tables: tables}
}

func (s *Snapshot) Database() *Database {
	return &Database{Tables: s.Tables}
}

func (s *Snapshot) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode schema snapshot: %w", err)
	}
	if s.Version != SnapshotVersion {
		return nil, fmt.Errorf("unsupported schema snapshot version %d (expected %d)", s.Version, SnapshotVersion)
	}
	return &s, nil
}
