package books

import (
	"context"
	"time"

	"uuid"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/validation"
)

type Service struct {
	*Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repository: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Book, error) {
	if err := checkPublishedAt(in.PublishedAt); err != nil {
		return Book{}, err
	}
	return s.Repository.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Book, error) {
	if err := checkPublishedAt(in.PublishedAt); err != nil {
		return Book{}, err
	}
	return s.Repository.Update(ctx, id, in)
}

func checkPublishedAt(publishedAt *time.Time) error {
	if publishedAt != nil && publishedAt.After(time.Now()) {
		return validation.Errors{{Field: "publishedAt", Message: "cannot be in the future"}}
	}
	return nil
}
