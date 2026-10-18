-- get args: id(ID)
SELECT
    "id",
    "name",
    "color"
FROM "tags"
WHERE "id" = ?;

-- exists args: id(ID)
SELECT 1
FROM "tags"
WHERE "id" = ?;

-- list args: filter(Name) filter(Name) limit offset
SELECT
    "id",
    "name",
    "color"
FROM "tags"
WHERE (? IS NULL OR "name" = ?)
ORDER BY "id"
LIMIT ? OFFSET ?;

-- count args: filter(Name) filter(Name)
SELECT COUNT(*)
FROM "tags"
WHERE (? IS NULL OR "name" = ?);

-- create args: input(Name) input(Color)
INSERT INTO "tags" (
    "name",
    "color"
) VALUES (
    ?,
    ?
)
RETURNING
    "id",
    "name",
    "color";

-- update args: input(Name) input(Color) id(ID)
UPDATE "tags"
SET
    "name" = ?,
    "color" = ?
WHERE "id" = ?
RETURNING
    "id",
    "name",
    "color";

-- delete args: id(ID)
DELETE FROM "tags"
WHERE "id" = ?;

-- exists User args: source(ID)
SELECT 1
FROM "users"
WHERE "id" = ? AND "deleted_at" IS NULL;

-- list User.Tags args: source(ID) filter(Name) filter(Name) limit offset
SELECT
    "tags"."id",
    "tags"."name",
    "tags"."color"
FROM "tags"
JOIN "user_tags" ON "user_tags"."tag_id" = "tags"."id"
WHERE "user_tags"."user_id" = ?
    AND (? IS NULL OR "tags"."name" = ?)
ORDER BY "tags"."id"
LIMIT ? OFFSET ?;

-- count User.Tags args: source(ID) filter(Name) filter(Name)
SELECT COUNT(*)
FROM "tags"
JOIN "user_tags" ON "user_tags"."tag_id" = "tags"."id"
WHERE "user_tags"."user_id" = ?
    AND (? IS NULL OR "tags"."name" = ?);

-- link User.Tags args: source(ID) id(ID)
INSERT INTO "user_tags" ("user_id", "tag_id")
VALUES (?, ?)
ON CONFLICT DO NOTHING;

-- unlink User.Tags args: source(ID) id(ID)
DELETE FROM "user_tags"
WHERE "user_id" = ? AND "tag_id" = ?;

