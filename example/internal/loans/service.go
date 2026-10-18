package loans

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"uuid"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/books"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/database"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/validation"
)

type Service struct {
	db    *sql.DB
	repo  *Repository
	books *books.Repository
	now   func() time.Time
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, repo: NewRepository(db), books: books.NewRepository(db), now: time.Now}
}

func (s *Service) List(ctx context.Context, p ListParams) (Page, error) {
	return s.repo.List(ctx, p)
}

func (s *Service) Get(ctx context.Context, id int64) (Loan, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Loan, error) {
	if err := s.checkDueAt(in.DueAt); err != nil {
		return Loan{}, err
	}
	var loan Loan
	err := database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.books.WithTx(tx).Reserve(ctx, in.BookID); err != nil {
			return err
		}
		created, err := s.repo.WithTx(tx).Create(ctx, in)
		loan = created
		return err
	})
	switch {
	case errors.Is(err, database.ErrNotFound):
		return Loan{}, validation.Errors{{Field: "bookId", Message: "references a book that does not exist"}}
	case errors.Is(err, books.ErrUnavailable):
		return Loan{}, validation.Errors{{Field: "bookId", Message: "is already on loan"}}
	}
	return loan, err
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (Loan, error) {
	if err := s.checkDueAt(in.DueAt); err != nil {
		return Loan{}, err
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Loan{}, err
	}
	if current.ReturnedAt != nil {
		return Loan{}, validation.Errors{{Field: "dueAt", Message: "cannot change after the loan was returned"}}
	}
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		repo := s.repo.WithTx(tx)
		loan, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := repo.Delete(ctx, id); err != nil {
			return err
		}
		if loan.ReturnedAt != nil {
			return nil
		}
		return s.books.WithTx(tx).Release(ctx, loan.BookID)
	})
}

func (s *Service) ListBookLoans(ctx context.Context, bookID uuid.UUID, p ListParams) (Page, error) {
	return s.repo.ListBookLoans(ctx, bookID, p)
}

func (s *Service) checkDueAt(dueAt time.Time) error {
	if !dueAt.After(s.now()) {
		return validation.Errors{{Field: "dueAt", Message: "must be in the future"}}
	}
	return nil
}
