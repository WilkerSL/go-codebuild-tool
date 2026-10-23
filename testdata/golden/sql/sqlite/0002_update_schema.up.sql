CREATE TABLE "users__new" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'viewer',
    "phone" TEXT,
    "org_id" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "updated_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_users" PRIMARY KEY ("id"),
    CONSTRAINT "fk_users_org_id" FOREIGN KEY ("org_id") REFERENCES "orgs" ("id") ON DELETE RESTRICT,
    CONSTRAINT "ck_users_role" CHECK ("role" IN ('owner', 'admin', 'operator', 'viewer'))
);

INSERT INTO "users__new" ("id", "name", "email", "role", "org_id", "created_at", "updated_at", "deleted_at")
SELECT "id", "name", "email", "role", "org_id", "created_at", "updated_at", "deleted_at"
FROM "users";

DROP TABLE "users";

ALTER TABLE "users__new" RENAME TO "users";

CREATE INDEX "ix_users_org_id" ON "users" ("org_id");

CREATE INDEX "ix_users_phone" ON "users" ("phone");

CREATE INDEX "lx_users" ON "users" ("created_at", "id");

CREATE INDEX "lx_users_org_id" ON "users" ("org_id", "created_at", "id");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

DROP INDEX "ux_tags_name";

DROP TABLE "countries";

CREATE TABLE "comments" (
    "id" TEXT NOT NULL,
    "project_id" TEXT NOT NULL,
    "body" TEXT NOT NULL,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    CONSTRAINT "pk_comments" PRIMARY KEY ("id"),
    CONSTRAINT "fk_comments_project_id" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE
);

ALTER TABLE "tags" ADD COLUMN "deleted_at" DATETIME;

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_comments_project_id" ON "comments" ("project_id");

CREATE INDEX "lx_comments" ON "comments" ("created_at", "id");

CREATE INDEX "lx_comments_project_id" ON "comments" ("project_id", "created_at", "id");
