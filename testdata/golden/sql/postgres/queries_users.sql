-- get args: id(ID)
SELECT
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at"
FROM "users"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- exists args: id(ID)
SELECT 1
FROM "users"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- list args: filter(Email) filter(OrgID) limit offset
SELECT
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at"
FROM "users"
WHERE "deleted_at" IS NULL
    AND ($1::text IS NULL OR "email" = $1)
    AND ($2::uuid IS NULL OR "org_id" = $2)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $3 OFFSET $4;

-- count args: filter(Email) filter(OrgID)
SELECT COUNT(*)
FROM "users"
WHERE "deleted_at" IS NULL
    AND ($1::text IS NULL OR "email" = $1)
    AND ($2::uuid IS NULL OR "org_id" = $2);

-- create args: id(ID) input(Name) input(Email) input(Role) input(Bio) input(OrgID)
INSERT INTO "users" (
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at"
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    now(),
    now()
)
RETURNING
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at";

-- update args: input(Name) input(Email) input(Role) input(Bio) input(OrgID) id(ID)
UPDATE "users"
SET
    "name" = $1,
    "email" = $2,
    "role" = $3,
    "bio" = $4,
    "org_id" = $5,
    "updated_at" = now()
WHERE "id" = $6 AND "deleted_at" IS NULL
RETURNING
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at";

-- delete args: id(ID)
UPDATE "users"
SET "deleted_at" = now(), "updated_at" = now()
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- exists Org args: source(ID)
SELECT 1
FROM "orgs"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- list Org.Users args: source(ID) filter(Email) limit offset
SELECT
    "id",
    "name",
    "email",
    "role",
    "bio",
    "org_id",
    "created_at",
    "updated_at"
FROM "users"
WHERE "org_id" = $1
    AND "deleted_at" IS NULL
    AND ($2::text IS NULL OR "email" = $2)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $3 OFFSET $4;

-- count Org.Users args: source(ID) filter(Email)
SELECT COUNT(*)
FROM "users"
WHERE "org_id" = $1
    AND "deleted_at" IS NULL
    AND ($2::text IS NULL OR "email" = $2);

-- exists Tag args: source(ID)
SELECT 1
FROM "tags"
WHERE "id" = $1;

-- list Tag.Users args: source(ID) filter(Email) filter(OrgID) limit offset
SELECT
    "users"."id",
    "users"."name",
    "users"."email",
    "users"."role",
    "users"."bio",
    "users"."org_id",
    "users"."created_at",
    "users"."updated_at"
FROM "users"
JOIN "user_tags" ON "user_tags"."user_id" = "users"."id"
WHERE "user_tags"."tag_id" = $1
    AND "users"."deleted_at" IS NULL
    AND ($2::text IS NULL OR "users"."email" = $2)
    AND ($3::uuid IS NULL OR "users"."org_id" = $3)
ORDER BY "users"."created_at" DESC, "users"."id" DESC
LIMIT $4 OFFSET $5;

-- count Tag.Users args: source(ID) filter(Email) filter(OrgID)
SELECT COUNT(*)
FROM "users"
JOIN "user_tags" ON "user_tags"."user_id" = "users"."id"
WHERE "user_tags"."tag_id" = $1
    AND "users"."deleted_at" IS NULL
    AND ($2::text IS NULL OR "users"."email" = $2)
    AND ($3::uuid IS NULL OR "users"."org_id" = $3);

-- link Tag.Users args: source(ID) id(ID)
INSERT INTO "user_tags" ("tag_id", "user_id")
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- unlink Tag.Users args: source(ID) id(ID)
DELETE FROM "user_tags"
WHERE "tag_id" = $1 AND "user_id" = $2;

