CREATE TABLE "audit_log" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "action" TEXT NOT NULL,
    "payload" TEXT,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now'))
);

CREATE TABLE "countries" (
    "code" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    CONSTRAINT "pk_countries" PRIMARY KEY ("code")
);

CREATE TABLE "orgs" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "slug" TEXT NOT NULL,
    "plan" TEXT NOT NULL DEFAULT 'free',
    "active" BOOLEAN NOT NULL DEFAULT 1,
    "locale" TEXT NOT NULL DEFAULT 'en-US',
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_orgs" PRIMARY KEY ("id"),
    CONSTRAINT "ck_orgs_plan" CHECK ("plan" IN ('free', 'pro', 'enterprise'))
);

CREATE TABLE "tags" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL,
    "color" TEXT
);

CREATE TABLE "users" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'viewer',
    "bio" TEXT,
    "org_id" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_users" PRIMARY KEY ("id"),
    CONSTRAINT "fk_users_org_id" FOREIGN KEY ("org_id") REFERENCES "orgs" ("id") ON DELETE RESTRICT,
    CONSTRAINT "ck_users_role" CHECK ("role" IN ('admin', 'operator', 'viewer'))
);

CREATE TABLE "projects" (
    "id" TEXT NOT NULL,
    "org_id" TEXT NOT NULL,
    "owner_id" TEXT,
    "name" TEXT NOT NULL,
    "budget" REAL NOT NULL,
    "priority" INTEGER NOT NULL DEFAULT 3,
    "archived" BOOLEAN NOT NULL,
    "logo" BLOB NOT NULL,
    "due_at" DATETIME,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    CONSTRAINT "pk_projects" PRIMARY KEY ("id"),
    CONSTRAINT "fk_projects_org_id" FOREIGN KEY ("org_id") REFERENCES "orgs" ("id") ON DELETE CASCADE,
    CONSTRAINT "fk_projects_owner_id" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE SET NULL
);

CREATE TABLE "user_tags" (
    "tag_id" INTEGER NOT NULL,
    "user_id" TEXT NOT NULL,
    CONSTRAINT "pk_user_tags" PRIMARY KEY ("tag_id", "user_id"),
    CONSTRAINT "fk_user_tags_tag_id" FOREIGN KEY ("tag_id") REFERENCES "tags" ("id") ON DELETE CASCADE,
    CONSTRAINT "fk_user_tags_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_audit_log_action" ON "audit_log" ("action");

CREATE INDEX "ix_audit_log_created_at" ON "audit_log" ("created_at");

CREATE INDEX "ix_orgs_plan" ON "orgs" ("plan");

CREATE UNIQUE INDEX "ux_orgs_slug" ON "orgs" ("slug") WHERE "deleted_at" IS NULL;

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name");

CREATE INDEX "ix_users_org_id" ON "users" ("org_id");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_projects_archived" ON "projects" ("archived");

CREATE INDEX "ix_projects_org_id" ON "projects" ("org_id");

CREATE INDEX "ix_projects_owner_id" ON "projects" ("owner_id");

CREATE INDEX "ix_user_tags_user_id" ON "user_tags" ("user_id");
