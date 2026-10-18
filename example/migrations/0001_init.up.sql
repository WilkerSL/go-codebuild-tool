CREATE TABLE "authors" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT,
    "bio" TEXT,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_authors" PRIMARY KEY ("id")
);

CREATE TABLE "languages" (
    "code" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    CONSTRAINT "pk_languages" PRIMARY KEY ("code")
);

CREATE TABLE "books" (
    "id" TEXT NOT NULL,
    "author_id" TEXT NOT NULL,
    "language_code" TEXT,
    "title" TEXT NOT NULL,
    "isbn" TEXT NOT NULL,
    "genre" TEXT NOT NULL DEFAULT 'fiction',
    "pages" INTEGER NOT NULL,
    "price_cents" INTEGER NOT NULL,
    "available" BOOLEAN NOT NULL DEFAULT 1,
    "published_at" DATETIME,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_books" PRIMARY KEY ("id"),
    CONSTRAINT "fk_books_author_id" FOREIGN KEY ("author_id") REFERENCES "authors" ("id") ON DELETE CASCADE,
    CONSTRAINT "fk_books_language_code" FOREIGN KEY ("language_code") REFERENCES "languages" ("code") ON DELETE RESTRICT,
    CONSTRAINT "ck_books_genre" CHECK ("genre" IN ('fiction', 'nonfiction', 'science', 'history'))
);

CREATE TABLE "loans" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "book_id" TEXT NOT NULL,
    "borrower" TEXT NOT NULL,
    "due_at" DATETIME NOT NULL,
    "returned_at" DATETIME,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    CONSTRAINT "fk_loans_book_id" FOREIGN KEY ("book_id") REFERENCES "books" ("id") ON DELETE CASCADE
);

CREATE TABLE "tags" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL
);

CREATE TABLE "book_tags" (
    "book_id" TEXT NOT NULL,
    "tag_id" INTEGER NOT NULL,
    CONSTRAINT "pk_book_tags" PRIMARY KEY ("book_id", "tag_id"),
    CONSTRAINT "fk_book_tags_book_id" FOREIGN KEY ("book_id") REFERENCES "books" ("id") ON DELETE CASCADE,
    CONSTRAINT "fk_book_tags_tag_id" FOREIGN KEY ("tag_id") REFERENCES "tags" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_books_author_id" ON "books" ("author_id");

CREATE INDEX "ix_books_available" ON "books" ("available");

CREATE INDEX "ix_books_genre" ON "books" ("genre");

CREATE INDEX "ix_books_language_code" ON "books" ("language_code");

CREATE UNIQUE INDEX "ux_books_isbn" ON "books" ("isbn") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_loans_book_id" ON "loans" ("book_id");

CREATE INDEX "ix_loans_borrower" ON "loans" ("borrower");

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name");

CREATE INDEX "ix_book_tags_tag_id" ON "book_tags" ("tag_id");
