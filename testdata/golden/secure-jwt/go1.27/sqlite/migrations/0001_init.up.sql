CREATE TABLE "auth_rate_limits" (
    "key_hash" TEXT NOT NULL,
    "hits" INTEGER NOT NULL,
    "reset_at" DATETIME NOT NULL,
    CONSTRAINT "pk_auth_rate_limits" PRIMARY KEY ("key_hash")
);

CREATE TABLE "idempotency_keys" (
    "key_hash" TEXT NOT NULL,
    "request_hash" TEXT NOT NULL,
    "status" INTEGER NOT NULL,
    "location" TEXT,
    "body" BLOB,
    "created_at" DATETIME NOT NULL,
    "expires_at" DATETIME NOT NULL,
    CONSTRAINT "pk_idempotency_keys" PRIMARY KEY ("key_hash")
);

CREATE TABLE "labels" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL
);

CREATE TABLE "users" (
    "id" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "password_hash" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'member',
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_users" PRIMARY KEY ("id"),
    CONSTRAINT "ck_users_role" CHECK ("role" IN ('admin', 'member'))
);

CREATE TABLE "auth_api_keys" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "prefix" TEXT NOT NULL,
    "key_hash" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL,
    "expires_at" DATETIME,
    CONSTRAINT "pk_auth_api_keys" PRIMARY KEY ("id"),
    CONSTRAINT "fk_auth_api_keys_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE TABLE "auth_refresh_tokens" (
    "id" TEXT NOT NULL,
    "family_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL,
    "expires_at" DATETIME NOT NULL,
    "used_at" DATETIME,
    CONSTRAINT "pk_auth_refresh_tokens" PRIMARY KEY ("id"),
    CONSTRAINT "fk_auth_refresh_tokens_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE TABLE "folders" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "owner_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    CONSTRAINT "fk_folders_owner_id" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE TABLE "notes" (
    "id" TEXT NOT NULL,
    "owner_id" TEXT NOT NULL,
    "folder_id" INTEGER,
    "title" TEXT NOT NULL,
    "body" TEXT,
    "pinned" BOOLEAN NOT NULL DEFAULT 0,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_notes" PRIMARY KEY ("id"),
    CONSTRAINT "fk_notes_folder_id" FOREIGN KEY ("folder_id") REFERENCES "folders" ("id") ON DELETE RESTRICT,
    CONSTRAINT "fk_notes_owner_id" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE TABLE "note_labels" (
    "label_id" INTEGER NOT NULL,
    "note_id" TEXT NOT NULL,
    CONSTRAINT "pk_note_labels" PRIMARY KEY ("label_id", "note_id"),
    CONSTRAINT "fk_note_labels_label_id" FOREIGN KEY ("label_id") REFERENCES "labels" ("id") ON DELETE CASCADE,
    CONSTRAINT "fk_note_labels_note_id" FOREIGN KEY ("note_id") REFERENCES "notes" ("id") ON DELETE CASCADE
);

CREATE TABLE "reminders" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "note_id" TEXT NOT NULL,
    "text" TEXT NOT NULL,
    "due_at" DATETIME NOT NULL,
    "done" BOOLEAN NOT NULL DEFAULT 0,
    CONSTRAINT "fk_reminders_note_id" FOREIGN KEY ("note_id") REFERENCES "notes" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_auth_rate_limits_reset_at" ON "auth_rate_limits" ("reset_at");

CREATE INDEX "ix_idempotency_keys_expires_at" ON "idempotency_keys" ("expires_at");

CREATE UNIQUE INDEX "ux_labels_name" ON "labels" ("name");

CREATE INDEX "lx_users" ON "users" ("created_at", "id");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_auth_api_keys_user_id" ON "auth_api_keys" ("user_id");

CREATE UNIQUE INDEX "ux_auth_api_keys_key_hash" ON "auth_api_keys" ("key_hash");

CREATE INDEX "ix_auth_refresh_tokens_family_id" ON "auth_refresh_tokens" ("family_id");

CREATE INDEX "ix_auth_refresh_tokens_user_id" ON "auth_refresh_tokens" ("user_id");

CREATE INDEX "ix_folders_owner_id" ON "folders" ("owner_id");

CREATE INDEX "lx_folders_owner_id" ON "folders" ("owner_id", "id");

CREATE INDEX "ix_notes_folder_id" ON "notes" ("folder_id");

CREATE INDEX "ix_notes_owner_id" ON "notes" ("owner_id");

CREATE INDEX "ix_notes_pinned" ON "notes" ("pinned");

CREATE INDEX "lx_notes" ON "notes" ("created_at", "id");

CREATE INDEX "lx_notes_folder_id" ON "notes" ("folder_id", "created_at", "id");

CREATE INDEX "lx_notes_owner_id" ON "notes" ("owner_id", "created_at", "id");

CREATE INDEX "ix_note_labels_note_id" ON "note_labels" ("note_id");

CREATE INDEX "ix_reminders_note_id" ON "reminders" ("note_id");

CREATE INDEX "lx_reminders_note_id" ON "reminders" ("note_id", "id");
