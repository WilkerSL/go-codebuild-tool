ALTER TABLE "users" DROP CONSTRAINT "ck_users_role";

DROP INDEX "ux_tags_name";

ALTER TABLE "users" DROP COLUMN "bio";

DROP TABLE "countries";

CREATE TABLE "comments" (
    "id" UUID NOT NULL,
    "project_id" UUID NOT NULL,
    "body" VARCHAR(2000) NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT "pk_comments" PRIMARY KEY ("id"),
    CONSTRAINT "fk_comments_project_id" FOREIGN KEY ("project_id") REFERENCES "projects" ("id") ON DELETE CASCADE
);

ALTER TABLE "tags" ADD COLUMN "deleted_at" TIMESTAMPTZ;

ALTER TABLE "users" ADD COLUMN "phone" VARCHAR(20);

ALTER TABLE "users"
    ALTER COLUMN "name" TYPE VARCHAR(200);

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_users_phone" ON "users" ("phone");

CREATE INDEX "ix_comments_project_id" ON "comments" ("project_id");

ALTER TABLE "users" ADD CONSTRAINT "ck_users_role" CHECK ("role" IN ('owner', 'admin', 'operator', 'viewer'));
