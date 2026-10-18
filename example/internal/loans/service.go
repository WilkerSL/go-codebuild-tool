package loans

import (
	"context"
	"errors"
	"time"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/books"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/database"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/validation"
)

type Service struct {
	*Repository
	books *books.Repository
	now   func() time.Time
}

func NewService(repo *Repository, books *books.Repository) *Service {
	return &Service{Repository: repo, books: books, now: time.Now}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Loan, error) {
	var errs validation.Errors
	if !in.DueAt.After(s.now()) {
		errs.Add("dueAt", "must be in the future")
	}
	book, err := s.books.Get(ctx, in.BookID)
	switch {
	case errors.Is(err, database.ErrNotFound):
		errs.Add("bookId", "references a book that does not exist")
	case err != nil:
		return Loan{}, err
	case !book.Available:
		errs.Add("bookId", "is not available for loans")
	}
	if err := errs.Err(); err != nil {
		return Loan{}, err
	}
	return s.Repository.Create(ctx, in)
}
