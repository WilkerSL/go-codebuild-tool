CREATE TABLE "auth_rate_limits" (
    "key_hash" TEXT NOT NULL,
    "hits" INTEGER NOT NULL,
    "reset_at" DATETIME NOT NULL,
    CONSTRAINT "pk_auth_rate_limits" PRIMARY KEY ("key_hash")
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

CREATE TABLE "auth_sessions" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL,
    "expires_at" DATETIME NOT NULL,
    CONSTRAINT "pk_auth_sessions" PRIMARY KEY ("id"),
    CONSTRAINT "fk_auth_sessions_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_auth_rate_limits_reset_at" ON "auth_rate_limits" ("reset_at");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_auth_api_keys_user_id" ON "auth_api_keys" ("user_id");

CREATE UNIQUE INDEX "ux_auth_api_keys_key_hash" ON "auth_api_keys" ("key_hash");

CREATE INDEX "ix_auth_sessions_user_id" ON "auth_sessions" ("user_id");
