package notes

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

func (s *Service) Get(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) (Note, error) {
	return s.repo.Get(ctx, ownerID, id)
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (Note, error) {
	return s.repo.Create(ctx, ownerID, in)
}

func (s *Service) Update(ctx context.Context, ownerID uuid.UUID, id uuid.UUID, in UpdateInput) (Note, error) {
	return s.repo.Update(ctx, ownerID, id, in)
}

func (s *Service) Delete(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) error {
	return s.repo.Delete(ctx, ownerID, id)
}

func (s *Service) ListFolderNotes(ctx context.Context, ownerID uuid.UUID, folderID int64, p ListParams) (Page, error) {
	return s.repo.ListFolderNotes(ctx, ownerID, folderID, p)
}

func (s *Service) ListLabelNotes(ctx context.Context, ownerID uuid.UUID, labelID int64, p ListParams) (Page, error) {
	return s.repo.ListLabelNotes(ctx, ownerID, labelID, p)
}

func (s *Service) LinkLabelNotes(ctx context.Context, ownerID uuid.UUID, labelID int64, id uuid.UUID) error {
	return s.repo.LinkLabelNotes(ctx, ownerID, labelID, id)
}

func (s *Service) UnlinkLabelNotes(ctx context.Context, ownerID uuid.UUID, labelID int64, id uuid.UUID) error {
	return s.repo.UnlinkLabelNotes(ctx, ownerID, labelID, id)
}
