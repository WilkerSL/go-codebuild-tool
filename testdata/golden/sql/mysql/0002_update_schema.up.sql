ALTER TABLE `users` DROP CHECK `ck_users_role`;

DROP INDEX `ux_tags_name` ON `tags`;

ALTER TABLE `users` DROP COLUMN `bio`;

DROP TABLE `countries`;

CREATE TABLE `comments` (
    `id` CHAR(36) NOT NULL,
    `project_id` CHAR(36) NOT NULL,
    `body` VARCHAR(2000) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    CONSTRAINT `pk_comments` PRIMARY KEY (`id`),
    INDEX `ix_comments_project_id` (`project_id`),
    CONSTRAINT `fk_comments_project_id` FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE `tags` ADD COLUMN `deleted_at` DATETIME(6);

ALTER TABLE `users` ADD COLUMN `phone` VARCHAR(20);

ALTER TABLE `users` MODIFY COLUMN `name` VARCHAR(200) NOT NULL;

CREATE UNIQUE INDEX `ux_tags_name` ON `tags` (`name`);

CREATE INDEX `ix_users_phone` ON `users` (`phone`);

ALTER TABLE `users` ADD CONSTRAINT `ck_users_role` CHECK (`role` IN ('owner', 'admin', 'operator', 'viewer'));
