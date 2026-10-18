-- get args: id(ID)
SELECT
    `id`,
    `name`,
    `slug`,
    `plan`,
    `active`,
    `created_at`,
    `updated_at`
FROM `orgs`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- exists args: id(ID)
SELECT 1
FROM `orgs`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- list args: filter(Slug) filter(Slug) filter(Plan) filter(Plan) limit offset
SELECT
    `id`,
    `name`,
    `slug`,
    `plan`,
    `active`,
    `created_at`,
    `updated_at`
FROM `orgs`
WHERE `deleted_at` IS NULL
    AND (? IS NULL OR `slug` = ?)
    AND (? IS NULL OR `plan` = ?)
ORDER BY `created_at` DESC, `id` DESC
LIMIT ? OFFSET ?;

-- count args: filter(Slug) filter(Slug) filter(Plan) filter(Plan)
SELECT COUNT(*)
FROM `orgs`
WHERE `deleted_at` IS NULL
    AND (? IS NULL OR `slug` = ?)
    AND (? IS NULL OR `plan` = ?);

-- create args: id(ID) input(Name) input(Slug) input(Plan) input(Active)
INSERT INTO `orgs` (
    `id`,
    `name`,
    `slug`,
    `plan`,
    `active`,
    `created_at`,
    `updated_at`
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    UTC_TIMESTAMP(6),
    UTC_TIMESTAMP(6)
);

-- update args: input(Name) input(Slug) input(Plan) input(Active) id(ID)
UPDATE `orgs`
SET
    `name` = ?,
    `slug` = ?,
    `plan` = ?,
    `active` = ?,
    `updated_at` = UTC_TIMESTAMP(6)
WHERE `id` = ? AND `deleted_at` IS NULL;

-- delete args: id(ID)
UPDATE `orgs`
SET `deleted_at` = UTC_TIMESTAMP(6), `updated_at` = UTC_TIMESTAMP(6)
WHERE `id` = ? AND `deleted_at` IS NULL;

