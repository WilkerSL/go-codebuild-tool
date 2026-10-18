ALTER TABLE `users` DROP CHECK `ck_users_role`;

DROP INDEX `ix_users_phone` ON `users`;

DROP INDEX `ux_tags_name` ON `tags`;

ALTER TABLE `users` MODIFY COLUMN `name` VARCHAR(120) NOT NULL;

ALTER TABLE `users` DROP COLUMN `phone`;

ALTER TABLE `tags` DROP COLUMN `deleted_at`;

DROP TABLE `comments`;

CREATE TABLE `countries` (
    `code` VARCHAR(2) NOT NULL,
    `name` VARCHAR(100) NOT NULL,
    CONSTRAINT `pk_countries` PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `users` ADD COLUMN `bio` VARCHAR(500);

CREATE UNIQUE INDEX `ux_tags_name` ON `tags` (`name`);

ALTER TABLE `users` ADD CONSTRAINT `ck_users_role` CHECK (`role` IN ('admin', 'operator', 'viewer'));
