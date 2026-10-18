package loans

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/database"
)

var ErrAlreadyReturned = errors.New("loans: the loan was already returned")

const markReturnedQuery = `
UPDATE "loans"
SET "returned_at" = ?
WHERE "id" = ? AND "returned_at" IS NULL`

func (r *Repository) MarkReturned(ctx context.Context, id int64, at time.Time) error {
	res, err := r.db.ExecContext(ctx, markReturnedQuery, database.TimeArg(at), id)
	if err != nil {
		return fmt.Errorf("return loan: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("return loan: %w", err)
	}
	if n == 1 {
		return nil
	}
	found, err := r.Exists(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return database.ErrNotFound
	}
	return ErrAlreadyReturned
}

func (s *Service) Return(ctx context.Context, id int64) (Loan, error) {
	var loan Loan
	err := database.InTx(ctx, s.db, func(tx *sql.Tx) error {
		repo := s.repo.WithTx(tx)
		if err := repo.MarkReturned(ctx, id, s.now()); err != nil {
			return err
		}
		returned, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		loan = returned
		return s.books.WithTx(tx).Release(ctx, returned.BookID)
	})
	return loan, err
}
