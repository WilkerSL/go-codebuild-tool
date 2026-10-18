ALTER TABLE "users" DROP CONSTRAINT "ck_users_role";

DROP INDEX "ix_users_phone";

DROP INDEX "ux_tags_name";

ALTER TABLE "users"
    ALTER COLUMN "name" TYPE VARCHAR(120);

ALTER TABLE "users" DROP COLUMN "phone";

ALTER TABLE "tags" DROP COLUMN "deleted_at";

DROP TABLE "comments";

CREATE TABLE "countries" (
    "code" VARCHAR(2) NOT NULL,
    "name" VARCHAR(100) NOT NULL,
    CONSTRAINT "pk_countries" PRIMARY KEY ("code")
);

ALTER TABLE "users" ADD COLUMN "bio" VARCHAR(500);

CREATE UNIQUE INDEX "ux_tags_name" ON "tags" ("name");

ALTER TABLE "users" ADD CONSTRAINT "ck_users_role" CHECK ("role" IN ('admin', 'operator', 'viewer'));
