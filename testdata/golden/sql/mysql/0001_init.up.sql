CREATE TABLE `audit_log` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `action` VARCHAR(64) NOT NULL,
    `payload` LONGTEXT,
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    CONSTRAINT `pk_audit_log` PRIMARY KEY (`id`),
    INDEX `ix_audit_log_action` (`action`),
    INDEX `ix_audit_log_created_at` (`created_at`),
    INDEX `lx_audit_log` (`created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `countries` (
    `code` VARCHAR(2) NOT NULL,
    `name` VARCHAR(100) NOT NULL,
    CONSTRAINT `pk_countries` PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `idempotency_keys` (
    `key_hash` VARCHAR(64) NOT NULL,
    `request_hash` VARCHAR(64) NOT NULL,
    `status` INT NOT NULL,
    `location` VARCHAR(2048),
    `body` LONGBLOB,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    CONSTRAINT `pk_idempotency_keys` PRIMARY KEY (`key_hash`),
    INDEX `ix_idempotency_keys_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `orgs` (
    `id` CHAR(36) NOT NULL,
    `name` VARCHAR(120) NOT NULL,
    `slug` VARCHAR(64) NOT NULL,
    `plan` VARCHAR(32) NOT NULL DEFAULT 'free',
    `active` BOOLEAN NOT NULL DEFAULT TRUE,
    `locale` LONGTEXT NOT NULL DEFAULT ('en-US'),
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `updated_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_orgs` PRIMARY KEY (`id`),
    INDEX `ix_orgs_plan` (`plan`),
    INDEX `lx_orgs` (`created_at`, `id`),
    UNIQUE INDEX `ux_orgs_slug` (`slug`),
    CONSTRAINT `ck_orgs_plan` CHECK (`plan` IN ('free', 'pro', 'enterprise'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `tags` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(50) NOT NULL,
    `color` VARCHAR(7),
    CONSTRAINT `pk_tags` PRIMARY KEY (`id`),
    UNIQUE INDEX `ux_tags_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `users` (
    `id` CHAR(36) NOT NULL,
    `name` VARCHAR(120) NOT NULL,
    `email` VARCHAR(254) NOT NULL,
    `role` VARCHAR(32) NOT NULL DEFAULT 'viewer',
    `bio` VARCHAR(500),
    `org_id` CHAR(36) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `updated_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_users` PRIMARY KEY (`id`),
    INDEX `ix_users_org_id` (`org_id`),
    INDEX `lx_users` (`created_at`, `id`),
    INDEX `lx_users_org_id` (`org_id`, `created_at`, `id`),
    UNIQUE INDEX `ux_users_email` (`email`),
    CONSTRAINT `fk_users_org_id` FOREIGN KEY (`org_id`) REFERENCES `orgs` (`id`) ON DELETE RESTRICT,
    CONSTRAINT `ck_users_role` CHECK (`role` IN ('admin', 'operator', 'viewer'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `projects` (
    `id` CHAR(36) NOT NULL,
    `org_id` CHAR(36) NOT NULL,
    `owner_id` CHAR(36),
    `name` VARCHAR(200) NOT NULL,
    `budget` DECIMAL(14, 2) NOT NULL,
    `priority` INT NOT NULL DEFAULT 3,
    `archived` BOOLEAN NOT NULL,
    `logo` LONGBLOB NOT NULL,
    `due_at` DATETIME(6),
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    CONSTRAINT `pk_projects` PRIMARY KEY (`id`),
    INDEX `ix_projects_archived` (`archived`),
    INDEX `ix_projects_org_id` (`org_id`),
    INDEX `ix_projects_owner_id` (`owner_id`),
    INDEX `lx_projects` (`created_at`, `id`),
    INDEX `lx_projects_org_id` (`org_id`, `created_at`, `id`),
    INDEX `lx_projects_owner_id` (`owner_id`, `created_at`, `id`),
    CONSTRAINT `fk_projects_org_id` FOREIGN KEY (`org_id`) REFERENCES `orgs` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_projects_owner_id` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `user_tags` (
    `tag_id` BIGINT NOT NULL,
    `user_id` CHAR(36) NOT NULL,
    CONSTRAINT `pk_user_tags` PRIMARY KEY (`tag_id`, `user_id`),
    INDEX `ix_user_tags_user_id` (`user_id`),
    CONSTRAINT `fk_user_tags_tag_id` FOREIGN KEY (`tag_id`) REFERENCES `tags` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_user_tags_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
