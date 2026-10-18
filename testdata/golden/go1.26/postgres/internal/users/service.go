package users

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
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

func (s *Service) Get(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (User, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListOrgUsers(ctx context.Context, orgID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListOrgUsers(ctx, orgID, p)
}

func (s *Service) ListTagUsers(ctx context.Context, tagID int64, p ListParams) (Page, error) {
	return s.repo.ListTagUsers(ctx, tagID, p)
}

func (s *Service) LinkTagUsers(ctx context.Context, tagID int64, id uuid.UUID) error {
	return s.repo.LinkTagUsers(ctx, tagID, id)
}

func (s *Service) UnlinkTagUsers(ctx context.Context, tagID int64, id uuid.UUID) error {
	return s.repo.UnlinkTagUsers(ctx, tagID, id)
}
