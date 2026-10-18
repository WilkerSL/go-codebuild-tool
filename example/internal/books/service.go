package books

import (
	"context"
	"database/sql"
	"time"

	"uuid"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/validation"
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

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Book, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Book, error) {
	if err := checkPublishedAt(in.PublishedAt); err != nil {
		return Book{}, err
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Book, error) {
	if err := checkPublishedAt(in.PublishedAt); err != nil {
		return Book{}, err
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListAuthorBooks(ctx context.Context, authorID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListAuthorBooks(ctx, authorID, p)
}

func (s *Service) ListTagBooks(ctx context.Context, tagID int64, p ListParams) (Page, error) {
	return s.repo.ListTagBooks(ctx, tagID, p)
}

func (s *Service) LinkTagBooks(ctx context.Context, tagID int64, id uuid.UUID) error {
	return s.repo.LinkTagBooks(ctx, tagID, id)
}

func (s *Service) UnlinkTagBooks(ctx context.Context, tagID int64, id uuid.UUID) error {
	return s.repo.UnlinkTagBooks(ctx, tagID, id)
}

func checkPublishedAt(publishedAt *time.Time) error {
	if publishedAt != nil && publishedAt.After(time.Now()) {
		return validation.Errors{{Field: "publishedAt", Message: "cannot be in the future"}}
	}
	return nil
}
