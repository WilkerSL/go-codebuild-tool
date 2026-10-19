package tags

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

func (s *Service) Get(ctx context.Context, id int64) (Tag, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Tag, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (Tag, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListUserTags(ctx context.Context, userID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListUserTags(ctx, userID, p)
}

func (s *Service) LinkUserTags(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.LinkUserTags(ctx, userID, id)
}

func (s *Service) UnlinkUserTags(ctx context.Context, userID uuid.UUID, id int64) error {
	return s.repo.UnlinkUserTags(ctx, userID, id)
}
