package books

import (
	"context"
	"errors"
	"fmt"

	"uuid"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/database"
)

var ErrUnavailable = errors.New("books: the book is not available")

const reserveQuery = `
UPDATE "books"
SET "available" = 0, "updated_at" = strftime('%Y-%m-%d %H:%M:%f', 'now')
WHERE "id" = ? AND "available" = 1 AND "deleted_at" IS NULL`

const releaseQuery = `
UPDATE "books"
SET "available" = 1, "updated_at" = strftime('%Y-%m-%d %H:%M:%f', 'now')
WHERE "id" = ?`

func (r *Repository) Reserve(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, reserveQuery, database.UUIDArg(id))
	if err != nil {
		return fmt.Errorf("reserve book: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reserve book: %w", err)
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
	return ErrUnavailable
}

func (r *Repository) Release(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.ExecContext(ctx, releaseQuery, database.UUIDArg(id)); err != nil {
		return fmt.Errorf("release book: %w", err)
	}
	return nil
}
