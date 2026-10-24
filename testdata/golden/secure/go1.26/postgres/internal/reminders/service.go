package reminders

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

func (s *Service) List(ctx context.Context, ownerID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.List(ctx, ownerID, p)
}

func (s *Service) Get(ctx context.Context, ownerID uuid.UUID, id int64) (Reminder, error) {
	return s.repo.Get(ctx, ownerID, id)
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (Reminder, error) {
	return s.repo.Create(ctx, ownerID, in)
}

func (s *Service) Update(ctx context.Context, ownerID uuid.UUID, id int64, in UpdateInput) (Reminder, error) {
	return s.repo.Update(ctx, ownerID, id, in)
}

func (s *Service) Delete(ctx context.Context, ownerID uuid.UUID, id int64) error {
	return s.repo.Delete(ctx, ownerID, id)
}

func (s *Service) ListNoteReminders(ctx context.Context, ownerID uuid.UUID, noteID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListNoteReminders(ctx, ownerID, noteID, p)
}
