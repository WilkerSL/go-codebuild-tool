CREATE TABLE `invoices` (
    `id` CHAR(36) NOT NULL,
    `owner_id` CHAR(36) NOT NULL,
    `number` VARCHAR(32) NOT NULL,
    `status` VARCHAR(32) NOT NULL DEFAULT 'draft',
    `notes` VARCHAR(500),
    `created_at` DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_invoices` PRIMARY KEY (`id`),
    INDEX `ix_invoices_owner_id` (`owner_id`),
    INDEX `ix_invoices_status` (`status`),
    UNIQUE INDEX `ux_invoices_number` (`number`),
    CONSTRAINT `ck_invoices_status` CHECK (`status` IN ('draft', 'issued', 'paid'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `invoice_items` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `invoice_id` CHAR(36) NOT NULL,
    `product_id` BIGINT NOT NULL,
    `quantity` INT NOT NULL DEFAULT 1,
    `price` DOUBLE NOT NULL,
    CONSTRAINT `pk_invoice_items` PRIMARY KEY (`id`),
    INDEX `ix_invoice_items_invoice_id` (`invoice_id`),
    INDEX `ix_invoice_items_product_id` (`product_id`),
    CONSTRAINT `fk_invoice_items_invoice_id` FOREIGN KEY (`invoice_id`) REFERENCES `invoices` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
