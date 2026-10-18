package users

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	*Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repository: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (User, error) {
	in.Email = normalizeEmail(in.Email)
	return s.Repository.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (User, error) {
	in.Email = normalizeEmail(in.Email)
	return s.Repository.Update(ctx, id, in)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
