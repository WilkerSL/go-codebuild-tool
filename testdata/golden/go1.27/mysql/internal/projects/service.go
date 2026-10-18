package projects

import (
	"context"
	"database/sql"
	"uuid"
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

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Project, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Project, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Project, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListOrgProjects(ctx context.Context, orgID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListOrgProjects(ctx, orgID, p)
}
