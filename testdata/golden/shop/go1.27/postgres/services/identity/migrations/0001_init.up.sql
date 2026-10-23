CREATE TABLE "auth_rate_limits" (
    "key_hash" VARCHAR(64) NOT NULL,
    "hits" INTEGER NOT NULL,
    "reset_at" TIMESTAMPTZ NOT NULL,
    CONSTRAINT "pk_auth_rate_limits" PRIMARY KEY ("key_hash")
);

CREATE TABLE "users" (
    "id" UUID NOT NULL,
    "email" VARCHAR(254) NOT NULL,
    "name" VARCHAR(120) NOT NULL,
    "password_hash" VARCHAR(255) NOT NULL,
    "role" VARCHAR(32) NOT NULL DEFAULT 'member',
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "deleted_at" TIMESTAMPTZ,
    CONSTRAINT "pk_users" PRIMARY KEY ("id"),
    CONSTRAINT "ck_users_role" CHECK ("role" IN ('admin', 'member'))
);

CREATE TABLE "auth_api_keys" (
    "id" VARCHAR(32) NOT NULL,
    "user_id" UUID NOT NULL,
    "name" VARCHAR(120) NOT NULL,
    "prefix" VARCHAR(16) NOT NULL,
    "key_hash" VARCHAR(64) NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ,
    CONSTRAINT "pk_auth_api_keys" PRIMARY KEY ("id"),
    CONSTRAINT "fk_auth_api_keys_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE TABLE "auth_sessions" (
    "id" VARCHAR(64) NOT NULL,
    "user_id" UUID NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL,
    "expires_at" TIMESTAMPTZ NOT NULL,
    CONSTRAINT "pk_auth_sessions" PRIMARY KEY ("id"),
    CONSTRAINT "fk_auth_sessions_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_auth_rate_limits_reset_at" ON "auth_rate_limits" ("reset_at");

CREATE INDEX "lx_users" ON "users" ("created_at", "id");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_auth_api_keys_user_id" ON "auth_api_keys" ("user_id");

CREATE UNIQUE INDEX "ux_auth_api_keys_key_hash" ON "auth_api_keys" ("key_hash");

CREATE INDEX "ix_auth_sessions_user_id" ON "auth_sessions" ("user_id");
