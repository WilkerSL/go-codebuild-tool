package loans_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/WilkerSL/go-codebuild-tool/example/internal/authors"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/books"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/loans"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/database"
	"github.com/WilkerSL/go-codebuild-tool/example/internal/platform/validation"
	"github.com/WilkerSL/go-codebuild-tool/example/migrations"
)

func openLoansDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Connect(filepath.Join(t.TempDir(), "loans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := database.MigrateUp(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedBook(t *testing.T, ctx context.Context, db *sql.DB) books.Book {
	t.Helper()
	author, err := authors.NewRepository(db).Create(ctx, authors.CreateInput{Name: "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := books.NewRepository(db).Create(ctx, books.CreateInput{
		AuthorID: author.ID,
		Title:    "A Wizard of Earthsea",
		ISBN:     fmt.Sprintf("978%010d", rand.Int64N(10_000_000_000)),
		Genre:    books.GenreFiction,
		Pages:    183,
	})
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func available(t *testing.T, ctx context.Context, db *sql.DB, book books.Book) bool {
	t.Helper()
	current, err := books.NewRepository(db).Get(ctx, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	return current.Available
}

func hasFieldError(err error, field string) bool {
	errs, ok := errors.AsType[validation.Errors](err)
	return ok && errs.Has(field)
}

func TestLoanLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openLoansDB(t)
	svc := loans.NewService(db)
	book := seedBook(t, ctx, db)
	in := loans.CreateInput{BookID: book.ID, Borrower: "reader@example.com", DueAt: time.Now().Add(14 * 24 * time.Hour)}

	loan, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if available(t, ctx, db, book) {
		t.Fatal("a borrowed book is still available")
	}
	if _, err := svc.Create(ctx, in); !hasFieldError(err, "bookId") {
		t.Fatalf("second loan of the same book: %v", err)
	}
	returned, err := svc.Return(ctx, loan.ID)
	if err != nil || returned.ReturnedAt == nil {
		t.Fatalf("return: %+v %v", returned, err)
	}
	if !available(t, ctx, db, book) {
		t.Fatal("a returned book is not available")
	}
	if _, err := svc.Return(ctx, loan.ID); !errors.Is(err, loans.ErrAlreadyReturned) {
		t.Fatalf("second return: %v", err)
	}
	if _, err := svc.Update(ctx, loan.ID, loans.UpdateInput{Borrower: in.Borrower, DueAt: in.DueAt.Add(time.Hour)}); !hasFieldError(err, "dueAt") {
		t.Fatalf("update of a returned loan: %v", err)
	}
	if _, err := svc.Return(ctx, 999999); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("return of a missing loan: %v", err)
	}

	active, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("create again: %v", err)
	}
	if err := svc.Delete(ctx, active.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !available(t, ctx, db, book) {
		t.Fatal("deleting an active loan did not release the book")
	}

	in.DueAt = time.Now().Add(-time.Hour)
	if _, err := svc.Create(ctx, in); !hasFieldError(err, "dueAt") {
		t.Fatalf("loan due in the past: %v", err)
	}
}

func TestConcurrentLoansOfOneBook(t *testing.T) {
	ctx := context.Background()
	db := openLoansDB(t)
	svc := loans.NewService(db)
	book := seedBook(t, ctx, db)
	const attempts = 8
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Go(func() {
			_, err := svc.Create(ctx, loans.CreateInput{
				BookID:   book.ID,
				Borrower: fmt.Sprintf("reader%d@example.com", i),
				DueAt:    time.Now().Add(24 * time.Hour),
			})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case !hasFieldError(err, "bookId"):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d concurrent loans succeeded, want exactly 1", succeeded)
	}
}
