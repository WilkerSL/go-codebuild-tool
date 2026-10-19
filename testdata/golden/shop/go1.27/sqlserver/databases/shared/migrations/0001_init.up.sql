CREATE TABLE [categories] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [name] NVARCHAR(80) NOT NULL,
    CONSTRAINT [pk_categories] PRIMARY KEY ([id])
);

CREATE TABLE [products] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [category_id] BIGINT NULL,
    [name] NVARCHAR(120) NOT NULL,
    [price] FLOAT NOT NULL,
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_products] PRIMARY KEY ([id]),
    CONSTRAINT [fk_products_category_id] FOREIGN KEY ([category_id]) REFERENCES [categories] ([id]) ON DELETE SET NULL
);

CREATE TABLE [stocks] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [product_id] BIGINT NOT NULL,
    [quantity] INT NOT NULL,
    CONSTRAINT [pk_stocks] PRIMARY KEY ([id]),
    CONSTRAINT [fk_stocks_product_id] FOREIGN KEY ([product_id]) REFERENCES [products] ([id]) ON DELETE CASCADE
);

CREATE UNIQUE INDEX [ux_categories_name] ON [categories] ([name]);

CREATE INDEX [ix_products_category_id] ON [products] ([category_id]);

CREATE UNIQUE INDEX [ux_stocks_product_id] ON [stocks] ([product_id]);
