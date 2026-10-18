-- get args: id(ID)
SELECT
    "id",
    "action",
    "payload",
    "created_at"
FROM "audit_log"
WHERE "id" = ?;

-- exists args: id(ID)
SELECT 1
FROM "audit_log"
WHERE "id" = ?;

-- list args: filter(Action) filter(Action) limit offset
SELECT
    "id",
    "action",
    "payload",
    "created_at"
FROM "audit_log"
WHERE (? IS NULL OR "action" = ?)
ORDER BY "created_at" DESC, "id" DESC
LIMIT ? OFFSET ?;

-- count args: filter(Action) filter(Action)
SELECT COUNT(*)
FROM "audit_log"
WHERE (? IS NULL OR "action" = ?);

-- create args: input(Action) input(Payload)
INSERT INTO "audit_log" (
    "action",
    "payload",
    "created_at"
) VALUES (
    ?,
    ?,
    strftime('%Y-%m-%d %H:%M:%f', 'now')
)
RETURNING
    "id",
    "action",
    "payload",
    "created_at";

-- update args: input(Action) input(Payload) id(ID)
UPDATE "audit_log"
SET
    "action" = ?,
    "payload" = ?
WHERE "id" = ?
RETURNING
    "id",
    "action",
    "payload",
    "created_at";

-- delete args: id(ID)
DELETE FROM "audit_log"
WHERE "id" = ?;

