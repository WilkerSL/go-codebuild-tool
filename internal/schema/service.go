package schema

type DatabaseMode string

const (
	DatabaseOwn    DatabaseMode = "own"
	DatabaseShared DatabaseMode = "shared"
)

const SharedDatabase = "shared"

type Service struct {
	Name     string
	TypeName string
	File     string
	Pos      Pos
	Database DatabaseMode
	Entities []*Entity
}

func (s *Service) DatabaseName() string {
	if s.Database == DatabaseShared {
		return SharedDatabase
	}
	return s.Name
}

func (m *Model) Workspace() bool {
	return len(m.Services) > 0
}

func (m *Model) Service(name string) *Service {
	for _, s := range m.Services {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (e *Entity) DatabaseName() string {
	if e.Service == nil {
		return ""
	}
	return e.Service.DatabaseName()
}

func SameDatabase(a, b *Entity) bool {
	return a.DatabaseName() == b.DatabaseName()
}

func (f *Field) External(owner *Entity) bool {
	return f.Ref != nil && f.Ref.Target != nil && !SameDatabase(owner, f.Ref.Target)
}

func (f *Field) CrossService(owner *Entity) bool {
	return f.Ref != nil && f.Ref.Target != nil && owner.Service != f.Ref.Target.Service
}
