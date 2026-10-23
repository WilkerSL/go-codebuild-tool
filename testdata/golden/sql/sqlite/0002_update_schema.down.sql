CREATE TABLE "users__new" (
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

INSERT INTO "users__new" ("id", "name", "email", "role", "org_id", "created_at", "updated_at", "deleted_at")
SELECT "id", "name", "email", "role", "org_id", "created_at", "updated_at", "deleted_at"
FROM "users";

DROP TABLE "users";

ALTER TABLE "users__new" RENAME TO "users";

CREATE INDEX "ix_users_org_id" ON "users" ("org_id");

CREATE INDEX "lx_users" ON "users" ("created_at", "id");

CREATE INDEX "lx_users_org_id" ON "users" ("org_id", "created_at", "id");

CREATE UNIQUE INDEX "ux_users_email" ON "users" ("email") WHERE "deleted_at" IS NULL;

DROP INDEX "ux_tags_name";

ALTER TABLE "tags" DROP COLUMN "deleted_at";

DROP TABLE "comments";

CREATE TABLE "countries" (
    "code" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    CONSTRAINT "pk_countries" PRIMARY KEY ("code")
);

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name");
