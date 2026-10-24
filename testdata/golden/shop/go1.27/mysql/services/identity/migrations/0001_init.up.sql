CREATE TABLE `auth_rate_limits` (
    `key_hash` VARCHAR(64) NOT NULL,
    `hits` INT NOT NULL,
    `reset_at` DATETIME(6) NOT NULL,
    CONSTRAINT `pk_auth_rate_limits` PRIMARY KEY (`key_hash`),
    INDEX `ix_auth_rate_limits_reset_at` (`reset_at`)
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

CREATE TABLE `users` (
    `id` CHAR(36) NOT NULL,
    `email` VARCHAR(254) NOT NULL,
    `name` VARCHAR(120) NOT NULL,
    `password_hash` VARCHAR(255) NOT NULL,
    `role` VARCHAR(32) NOT NULL DEFAULT 'member',
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `updated_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_users` PRIMARY KEY (`id`),
    INDEX `lx_users` (`created_at`, `id`),
    UNIQUE INDEX `ux_users_email` (`email`),
    CONSTRAINT `ck_users_role` CHECK (`role` IN ('admin', 'member'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `auth_api_keys` (
    `id` VARCHAR(32) NOT NULL,
    `user_id` CHAR(36) NOT NULL,
    `name` VARCHAR(120) NOT NULL,
    `prefix` VARCHAR(16) NOT NULL,
    `key_hash` VARCHAR(64) NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6),
    CONSTRAINT `pk_auth_api_keys` PRIMARY KEY (`id`),
    INDEX `ix_auth_api_keys_user_id` (`user_id`),
    UNIQUE INDEX `ux_auth_api_keys_key_hash` (`key_hash`),
    CONSTRAINT `fk_auth_api_keys_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `auth_sessions` (
    `id` VARCHAR(64) NOT NULL,
    `user_id` CHAR(36) NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `expires_at` DATETIME(6) NOT NULL,
    CONSTRAINT `pk_auth_sessions` PRIMARY KEY (`id`),
    INDEX `ix_auth_sessions_user_id` (`user_id`),
    CONSTRAINT `fk_auth_sessions_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
