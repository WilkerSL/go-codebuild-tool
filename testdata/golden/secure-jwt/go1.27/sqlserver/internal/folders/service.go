package folders

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

func (s *Service) List(ctx context.Context, ownerID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.List(ctx, ownerID, p)
}

func (s *Service) Get(ctx context.Context, ownerID uuid.UUID, id int64) (Folder, error) {
	return s.repo.Get(ctx, ownerID, id)
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (Folder, error) {
	return s.repo.Create(ctx, ownerID, in)
}

func (s *Service) Update(ctx context.Context, ownerID uuid.UUID, id int64, in UpdateInput) (Folder, error) {
	return s.repo.Update(ctx, ownerID, id, in)
}

func (s *Service) Delete(ctx context.Context, ownerID uuid.UUID, id int64) error {
	return s.repo.Delete(ctx, ownerID, id)
}
