package audits

import (
	"context"
	"database/sql"
)

type Service struct {
	repo *Repository
}

func NewService(db *sql.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

func (s *Service) List(ctx context.Context, p ListParams) (Page, error) {
	return s.repo.List(ctx, p)
}

func (s *Service) Get(ctx context.Context, id int64) (Audit, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Audit, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (Audit, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
