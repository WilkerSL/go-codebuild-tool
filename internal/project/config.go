package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/gen"
)

const (
	ConfigFile    = "codebuild.json"
	SpecsDir      = "specs"
	MigrationsDir = "migrations"
	SnapshotName  = "codebuild.snapshot.json"
	SnapshotFile  = MigrationsDir + "/" + SnapshotName
)

type Config struct {
	Dialect      string         `json:"dialect"`
	Pagination   gen.Pagination `json:"pagination"`
	ManualWiring []string       `json:"manualWiring"`
	Generate     gen.Parts      `json:"generate"`
}

func DefaultConfig(dialectName string) Config {
	return Config{
		Dialect:      dialectName,
		Pagination:   gen.Pagination{DefaultLimit: 20, MaxLimit: 100},
		ManualWiring: []string{},
		Generate:     gen.AllParts(),
	}
}

func ParseConfig(data []byte) (Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	cfg := DefaultConfig("")
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	if cfg.ManualWiring == nil {
		cfg.ManualWiring = []string{}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if _, err := dialect.Get(c.Dialect); err != nil {
		return fmt.Errorf("%s: %w", ConfigFile, err)
	}
	p := c.Pagination
	if p.DefaultLimit < 1 || p.MaxLimit < p.DefaultLimit || p.MaxLimit > 10000 {
		return fmt.Errorf("%s: pagination must satisfy 1 <= defaultLimit <= maxLimit <= 10000", ConfigFile)
	}
	return nil
}

func (c Config) Marshal() []byte {
	out := c
	out.ManualWiring = slices.Clone(c.ManualWiring)
	slices.Sort(out.ManualWiring)
	data, _ := json.MarshalIndent(out, "", "  ")
	return append(data, '\n')
}
