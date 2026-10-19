CREATE TABLE `labels` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(40) NOT NULL,
    CONSTRAINT `pk_labels` PRIMARY KEY (`id`),
    UNIQUE INDEX `ux_labels_name` (`name`)
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

CREATE TABLE `folders` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `owner_id` CHAR(36) NOT NULL,
    `name` VARCHAR(80) NOT NULL,
    CONSTRAINT `pk_folders` PRIMARY KEY (`id`),
    INDEX `ix_folders_owner_id` (`owner_id`),
    CONSTRAINT `fk_folders_owner_id` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `notes` (
    `id` CHAR(36) NOT NULL,
    `owner_id` CHAR(36) NOT NULL,
    `folder_id` BIGINT,
    `title` VARCHAR(200) NOT NULL,
    `body` LONGTEXT,
    `pinned` BOOLEAN NOT NULL DEFAULT FALSE,
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `updated_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_notes` PRIMARY KEY (`id`),
    INDEX `ix_notes_folder_id` (`folder_id`),
    INDEX `ix_notes_owner_id` (`owner_id`),
    INDEX `ix_notes_pinned` (`pinned`),
    CONSTRAINT `fk_notes_folder_id` FOREIGN KEY (`folder_id`) REFERENCES `folders` (`id`) ON DELETE RESTRICT,
    CONSTRAINT `fk_notes_owner_id` FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `note_labels` (
    `label_id` BIGINT NOT NULL,
    `note_id` CHAR(36) NOT NULL,
    CONSTRAINT `pk_note_labels` PRIMARY KEY (`label_id`, `note_id`),
    INDEX `ix_note_labels_note_id` (`note_id`),
    CONSTRAINT `fk_note_labels_label_id` FOREIGN KEY (`label_id`) REFERENCES `labels` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_note_labels_note_id` FOREIGN KEY (`note_id`) REFERENCES `notes` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
