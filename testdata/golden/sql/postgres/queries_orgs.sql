-- get args: id(ID)
SELECT
    "id",
    "name",
    "slug",
    "plan",
    "active",
    "locale",
    "created_at",
    "updated_at"
FROM "orgs"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- exists args: id(ID)
SELECT 1
FROM "orgs"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- list args: filter(Slug) filter(Plan) limit offset
SELECT
    "id",
    "name",
    "slug",
    "plan",
    "active",
    "locale",
    "created_at",
    "updated_at"
FROM "orgs"
WHERE "deleted_at" IS NULL
    AND ($1::text IS NULL OR "slug" = $1)
    AND ($2::text IS NULL OR "plan" = $2)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $3 OFFSET $4;

-- count args: filter(Slug) filter(Plan)
SELECT COUNT(*)
FROM "orgs"
WHERE "deleted_at" IS NULL
    AND ($1::text IS NULL OR "slug" = $1)
    AND ($2::text IS NULL OR "plan" = $2);

-- create args: id(ID) input(Name) input(Slug) input(Plan) input(Active) input(Locale)
INSERT INTO "orgs" (
    "id",
    "name",
    "slug",
    "plan",
    "active",
    "locale",
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
    "slug",
    "plan",
    "active",
    "locale",
    "created_at",
    "updated_at";

-- update args: input(Name) input(Slug) input(Plan) input(Active) input(Locale) id(ID)
UPDATE "orgs"
SET
    "name" = $1,
    "slug" = $2,
    "plan" = $3,
    "active" = $4,
    "locale" = $5,
    "updated_at" = now()
WHERE "id" = $6 AND "deleted_at" IS NULL
RETURNING
    "id",
    "name",
    "slug",
    "plan",
    "active",
    "locale",
    "created_at",
    "updated_at";

-- delete args: id(ID)
UPDATE "orgs"
SET "deleted_at" = now(), "updated_at" = now()
WHERE "id" = $1 AND "deleted_at" IS NULL;

