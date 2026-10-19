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
WHERE "id" = ? AND "deleted_at" IS NULL;

-- exists args: id(ID)
SELECT 1
FROM "orgs"
WHERE "id" = ? AND "deleted_at" IS NULL;

-- list args: filter(Slug) filter(Slug) filter(Plan) filter(Plan) limit offset
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
    AND (? IS NULL OR "slug" = ?)
    AND (? IS NULL OR "plan" = ?)
ORDER BY "created_at" DESC, "id" DESC
LIMIT ? OFFSET ?;

-- count args: filter(Slug) filter(Slug) filter(Plan) filter(Plan)
SELECT COUNT(*)
FROM "orgs"
WHERE "deleted_at" IS NULL
    AND (? IS NULL OR "slug" = ?)
    AND (? IS NULL OR "plan" = ?);

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
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    strftime('%Y-%m-%d %H:%M:%f', 'now'),
    strftime('%Y-%m-%d %H:%M:%f', 'now')
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
    "name" = ?,
    "slug" = ?,
    "plan" = ?,
    "active" = ?,
    "locale" = ?,
    "updated_at" = strftime('%Y-%m-%d %H:%M:%f', 'now')
WHERE "id" = ? AND "deleted_at" IS NULL
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
SET "deleted_at" = strftime('%Y-%m-%d %H:%M:%f', 'now'), "updated_at" = strftime('%Y-%m-%d %H:%M:%f', 'now')
WHERE "id" = ? AND "deleted_at" IS NULL;

