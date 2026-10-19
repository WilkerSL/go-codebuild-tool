-- get args: id(ID)
SELECT
    `id`,
    `org_id`,
    `owner_id`,
    `name`,
    `budget`,
    `priority`,
    `archived`,
    `logo`,
    `due_at`,
    `created_at`
FROM `projects`
WHERE `id` = ? AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL);

-- exists args: id(ID)
SELECT 1
FROM `projects`
WHERE `id` = ? AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL);

-- list args: filter(OrgID) filter(OrgID) filter(OwnerID) filter(OwnerID) filter(Archived) filter(Archived) limit offset
SELECT
    `id`,
    `org_id`,
    `owner_id`,
    `name`,
    `budget`,
    `priority`,
    `archived`,
    `logo`,
    `due_at`,
    `created_at`
FROM `projects`
WHERE EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL)
    AND (? IS NULL OR `org_id` = ?)
    AND (? IS NULL OR `owner_id` = ?)
    AND (? IS NULL OR `archived` = ?)
ORDER BY `created_at` DESC, `id` DESC
LIMIT ? OFFSET ?;

-- count args: filter(OrgID) filter(OrgID) filter(OwnerID) filter(OwnerID) filter(Archived) filter(Archived)
SELECT COUNT(*)
FROM `projects`
WHERE EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL)
    AND (? IS NULL OR `org_id` = ?)
    AND (? IS NULL OR `owner_id` = ?)
    AND (? IS NULL OR `archived` = ?);

-- create args: id(ID) input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt)
INSERT INTO `projects` (
    `id`,
    `org_id`,
    `owner_id`,
    `name`,
    `budget`,
    `priority`,
    `archived`,
    `logo`,
    `due_at`,
    `created_at`
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    UTC_TIMESTAMP(6)
);

-- update args: input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt) id(ID)
UPDATE `projects`
SET
    `org_id` = ?,
    `owner_id` = ?,
    `name` = ?,
    `budget` = ?,
    `priority` = ?,
    `archived` = ?,
    `logo` = ?,
    `due_at` = ?
WHERE `id` = ? AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL);

-- delete args: id(ID)
DELETE FROM `projects`
WHERE `id` = ? AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL);

-- exists Org args: source(ID)
SELECT 1
FROM `orgs`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- list Org.Projects args: source(ID) filter(OwnerID) filter(OwnerID) filter(Archived) filter(Archived) limit offset
SELECT
    `id`,
    `org_id`,
    `owner_id`,
    `name`,
    `budget`,
    `priority`,
    `archived`,
    `logo`,
    `due_at`,
    `created_at`
FROM `projects`
WHERE `org_id` = ?
    AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL)
    AND (? IS NULL OR `owner_id` = ?)
    AND (? IS NULL OR `archived` = ?)
ORDER BY `created_at` DESC, `id` DESC
LIMIT ? OFFSET ?;

-- count Org.Projects args: source(ID) filter(OwnerID) filter(OwnerID) filter(Archived) filter(Archived)
SELECT COUNT(*)
FROM `projects`
WHERE `org_id` = ?
    AND EXISTS (SELECT 1 FROM `orgs` WHERE `orgs`.`id` = `projects`.`org_id` AND `orgs`.`deleted_at` IS NULL)
    AND (? IS NULL OR `owner_id` = ?)
    AND (? IS NULL OR `archived` = ?);

