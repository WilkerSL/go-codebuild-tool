package tags

type Service struct {
	*Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repository: repo}
}
