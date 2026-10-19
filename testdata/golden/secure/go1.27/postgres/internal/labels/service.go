package labels

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

func (s *Service) Get(ctx context.Context, id int64) (Label, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Label, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (Label, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListNoteLabels(ctx context.Context, ownerID uuid.UUID, noteID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListNoteLabels(ctx, ownerID, noteID, p)
}

func (s *Service) LinkNoteLabels(ctx context.Context, ownerID uuid.UUID, noteID uuid.UUID, id int64) error {
	return s.repo.LinkNoteLabels(ctx, ownerID, noteID, id)
}

func (s *Service) UnlinkNoteLabels(ctx context.Context, ownerID uuid.UUID, noteID uuid.UUID, id int64) error {
	return s.repo.UnlinkNoteLabels(ctx, ownerID, noteID, id)
}
