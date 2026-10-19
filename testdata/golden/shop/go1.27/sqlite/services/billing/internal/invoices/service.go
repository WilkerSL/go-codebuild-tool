package invoices

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

func (s *Service) Get(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) (Invoice, error) {
	return s.repo.Get(ctx, ownerID, id)
}

func (s *Service) Exists(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) (bool, error) {
	return s.repo.Exists(ctx, ownerID, id)
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (Invoice, error) {
	return s.repo.Create(ctx, ownerID, in)
}

func (s *Service) Update(ctx context.Context, ownerID uuid.UUID, id uuid.UUID, in UpdateInput) (Invoice, error) {
	return s.repo.Update(ctx, ownerID, id, in)
}

func (s *Service) Delete(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) error {
	return s.repo.Delete(ctx, ownerID, id)
}
