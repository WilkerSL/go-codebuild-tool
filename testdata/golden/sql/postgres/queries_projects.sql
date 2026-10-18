-- get args: id(ID)
SELECT
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at"
FROM "projects"
WHERE "id" = $1;

-- exists args: id(ID)
SELECT 1
FROM "projects"
WHERE "id" = $1;

-- list args: filter(OrgID) filter(OwnerID) filter(Archived) limit offset
SELECT
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at"
FROM "projects"
WHERE ($1::uuid IS NULL OR "org_id" = $1)
    AND ($2::uuid IS NULL OR "owner_id" = $2)
    AND ($3::boolean IS NULL OR "archived" = $3)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $4 OFFSET $5;

-- count args: filter(OrgID) filter(OwnerID) filter(Archived)
SELECT COUNT(*)
FROM "projects"
WHERE ($1::uuid IS NULL OR "org_id" = $1)
    AND ($2::uuid IS NULL OR "owner_id" = $2)
    AND ($3::boolean IS NULL OR "archived" = $3);

-- create args: id(ID) input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt)
INSERT INTO "projects" (
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at"
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    now()
)
RETURNING
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at";

-- update args: input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt) id(ID)
UPDATE "projects"
SET
    "org_id" = $1,
    "owner_id" = $2,
    "name" = $3,
    "budget" = $4,
    "priority" = $5,
    "archived" = $6,
    "logo" = $7,
    "due_at" = $8
WHERE "id" = $9
RETURNING
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at";

-- delete args: id(ID)
DELETE FROM "projects"
WHERE "id" = $1;

-- exists Org args: source(ID)
SELECT 1
FROM "orgs"
WHERE "id" = $1 AND "deleted_at" IS NULL;

-- list Org.Projects args: source(ID) filter(OwnerID) filter(Archived) limit offset
SELECT
    "id",
    "org_id",
    "owner_id",
    "name",
    "budget",
    "priority",
    "archived",
    "logo",
    "due_at",
    "created_at"
FROM "projects"
WHERE "org_id" = $1
    AND ($2::uuid IS NULL OR "owner_id" = $2)
    AND ($3::boolean IS NULL OR "archived" = $3)
ORDER BY "created_at" DESC, "id" DESC
LIMIT $4 OFFSET $5;

-- count Org.Projects args: source(ID) filter(OwnerID) filter(Archived)
SELECT COUNT(*)
FROM "projects"
WHERE "org_id" = $1
    AND ($2::uuid IS NULL OR "owner_id" = $2)
    AND ($3::boolean IS NULL OR "archived" = $3);

