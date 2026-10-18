-- get args: id(ID)
SELECT
    "id",
    "action",
    "payload",
    "created_at"
FROM "audit_log"
WHERE "id" = $1;

-- exists args: id(ID)
SELECT 1
FROM "audit_log"
WHERE "id" = $1;

-- list args: filter(Action) limit offset
SELECT
    "id",
    "action",
    "payload",
    "created_at"
FROM "audit_log"
WHERE ($1::text IS NULL OR "action" = $1)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $2 OFFSET $3;

-- count args: filter(Action)
SELECT COUNT(*)
FROM "audit_log"
WHERE ($1::text IS NULL OR "action" = $1);

-- create args: input(Action) input(Payload)
INSERT INTO "audit_log" (
    "action",
    "payload",
    "created_at"
) VALUES (
    $1,
    $2,
    now()
)
RETURNING
    "id",
    "action",
    "payload",
    "created_at";

-- update args: input(Action) input(Payload) id(ID)
UPDATE "audit_log"
SET
    "action" = $1,
    "payload" = $2
WHERE "id" = $3
RETURNING
    "id",
    "action",
    "payload",
    "created_at";

-- delete args: id(ID)
DELETE FROM "audit_log"
WHERE "id" = $1;

