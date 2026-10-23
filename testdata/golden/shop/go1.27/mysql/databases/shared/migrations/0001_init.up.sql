CREATE TABLE `categories` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(80) NOT NULL,
    CONSTRAINT `pk_categories` PRIMARY KEY (`id`),
    UNIQUE INDEX `ux_categories_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `products` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `category_id` BIGINT,
    `name` VARCHAR(120) NOT NULL,
    `price` DOUBLE NOT NULL,
    `deleted_at` DATETIME(6),
    CONSTRAINT `pk_products` PRIMARY KEY (`id`),
    INDEX `ix_products_category_id` (`category_id`),
    INDEX `lx_products_category_id` (`category_id`, `id`),
    CONSTRAINT `fk_products_category_id` FOREIGN KEY (`category_id`) REFERENCES `categories` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `stocks` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `product_id` BIGINT NOT NULL,
    `quantity` INT NOT NULL,
    CONSTRAINT `pk_stocks` PRIMARY KEY (`id`),
    INDEX `lx_stocks_product_id` (`product_id`, `id`),
    UNIQUE INDEX `ux_stocks_product_id` (`product_id`),
    CONSTRAINT `fk_stocks_product_id` FOREIGN KEY (`product_id`) REFERENCES `products` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
