-- get args: id(ID)
SELECT
    "id",
    "name",
    "color"
FROM "tags"
WHERE "id" = $1;

-- exists args: id(ID)
SELECT 1
FROM "tags"
WHERE "id" = $1;

-- list args: filter(Name) limit offset
SELECT
    "id",
    "name",
    "color"
FROM "tags"
WHERE ($1::text IS NULL OR "name" = $1)
ORDER BY "id"
LIMIT $2 OFFSET $3;

-- count args: filter(Name)
SELECT COUNT(*)
FROM "tags"
WHERE ($1::text IS NULL OR "name" = $1);

-- create args: input(Name) input(Color)
INSERT INTO "tags" (
    "name",
    "color"
) VALUES (
    $1,
    $2
)
RETURNING
    "id",
    "name",
    "color";

-- update args: input(Name) input(Color) id(ID)
UPDATE "tags"
SET
    "name" = $1,
    "color" = $2
WHERE "id" = $3
RETURNING
    "id",
    "name",
    "color";

-- delete args: id(ID)
DELETE FROM "tags"
WHERE "id" = $1;

-- exists User args: source(ID)
SELECT 1
FROM "users"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- list User.Tags args: source(ID) filter(Name) limit offset
SELECT
    "tags"."id",
    "tags"."name",
    "tags"."color"
FROM "tags"
JOIN "user_tags" ON "user_tags"."tag_id" = "tags"."id"
WHERE "user_tags"."user_id" = $1
    AND ($2::text IS NULL OR "tags"."name" = $2)
ORDER BY "tags"."id"
LIMIT $3 OFFSET $4;

-- count User.Tags args: source(ID) filter(Name)
SELECT COUNT(*)
FROM "tags"
JOIN "user_tags" ON "user_tags"."tag_id" = "tags"."id"
WHERE "user_tags"."user_id" = $1
    AND ($2::text IS NULL OR "tags"."name" = $2);

-- link User.Tags args: source(ID) id(ID)
INSERT INTO "user_tags" ("user_id", "tag_id")
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- unlink User.Tags args: source(ID) id(ID)
DELETE FROM "user_tags"
WHERE "user_id" = $1 AND "tag_id" = $2;

