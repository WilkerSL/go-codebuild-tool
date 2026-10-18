-- get args: id(ID)
SELECT
    `id`,
    `name`,
    `email`,
    `role`,
    `bio`,
    `org_id`,
    `created_at`,
    `updated_at`
FROM `users`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- exists args: id(ID)
SELECT 1
FROM `users`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- list args: filter(Email) filter(Email) filter(OrgID) filter(OrgID) limit offset
SELECT
    `id`,
    `name`,
    `email`,
    `role`,
    `bio`,
    `org_id`,
    `created_at`,
    `updated_at`
FROM `users`
WHERE `deleted_at` IS NULL
    AND (? IS NULL OR `email` = ?)
    AND (? IS NULL OR `org_id` = ?)
ORDER BY `created_at` DESC, `id` DESC
LIMIT ? OFFSET ?;

-- count args: filter(Email) filter(Email) filter(OrgID) filter(OrgID)
SELECT COUNT(*)
FROM `users`
WHERE `deleted_at` IS NULL
    AND (? IS NULL OR `email` = ?)
    AND (? IS NULL OR `org_id` = ?);

-- create args: id(ID) input(Name) input(Email) input(Role) input(Bio) input(OrgID)
INSERT INTO `users` (
    `id`,
    `name`,
    `email`,
    `role`,
    `bio`,
    `org_id`,
    `created_at`,
    `updated_at`
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?,
    UTC_TIMESTAMP(6),
    UTC_TIMESTAMP(6)
);

-- update args: input(Name) input(Email) input(Role) input(Bio) input(OrgID) id(ID)
UPDATE `users`
SET
    `name` = ?,
    `email` = ?,
    `role` = ?,
    `bio` = ?,
    `org_id` = ?,
    `updated_at` = UTC_TIMESTAMP(6)
WHERE `id` = ? AND `deleted_at` IS NULL;

-- delete args: id(ID)
UPDATE `users`
SET `deleted_at` = UTC_TIMESTAMP(6), `updated_at` = UTC_TIMESTAMP(6)
WHERE `id` = ? AND `deleted_at` IS NULL;

-- exists Org args: source(ID)
SELECT 1
FROM `orgs`
WHERE `id` = ? AND `deleted_at` IS NULL;

-- list Org.Users args: source(ID) filter(Email) filter(Email) limit offset
SELECT
    `id`,
    `name`,
    `email`,
    `role`,
    `bio`,
    `org_id`,
    `created_at`,
    `updated_at`
FROM `users`
WHERE `org_id` = ?
    AND `deleted_at` IS NULL
    AND (? IS NULL OR `email` = ?)
ORDER BY `created_at` DESC, `id` DESC
LIMIT ? OFFSET ?;

-- count Org.Users args: source(ID) filter(Email) filter(Email)
SELECT COUNT(*)
FROM `users`
WHERE `org_id` = ?
    AND `deleted_at` IS NULL
    AND (? IS NULL OR `email` = ?);

-- exists Tag args: source(ID)
SELECT 1
FROM `tags`
WHERE `id` = ?;

-- list Tag.Users args: source(ID) filter(Email) filter(Email) filter(OrgID) filter(OrgID) limit offset
SELECT
    `users`.`id`,
    `users`.`name`,
    `users`.`email`,
    `users`.`role`,
    `users`.`bio`,
    `users`.`org_id`,
    `users`.`created_at`,
    `users`.`updated_at`
FROM `users`
JOIN `user_tags` ON `user_tags`.`user_id` = `users`.`id`
WHERE `user_tags`.`tag_id` = ?
    AND `users`.`deleted_at` IS NULL
    AND (? IS NULL OR `users`.`email` = ?)
    AND (? IS NULL OR `users`.`org_id` = ?)
ORDER BY `users`.`created_at` DESC, `users`.`id` DESC
LIMIT ? OFFSET ?;

-- count Tag.Users args: source(ID) filter(Email) filter(Email) filter(OrgID) filter(OrgID)
SELECT COUNT(*)
FROM `users`
JOIN `user_tags` ON `user_tags`.`user_id` = `users`.`id`
WHERE `user_tags`.`tag_id` = ?
    AND `users`.`deleted_at` IS NULL
    AND (? IS NULL OR `users`.`email` = ?)
    AND (? IS NULL OR `users`.`org_id` = ?);

-- link Tag.Users args: source(ID) id(ID)
INSERT INTO `user_tags` (`tag_id`, `user_id`)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE `tag_id` = `tag_id`;

-- unlink Tag.Users args: source(ID) id(ID)
DELETE FROM `user_tags`
WHERE `tag_id` = ? AND `user_id` = ?;

