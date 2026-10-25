CREATE TABLE [idempotency_keys] (
    [key_hash] NVARCHAR(64) NOT NULL,
    [request_hash] NVARCHAR(64) NOT NULL,
    [status] INT NOT NULL,
    [location] NVARCHAR(2048) NULL,
    [body] VARBINARY(MAX) NULL,
    [created_at] DATETIME2(7) NOT NULL,
    [expires_at] DATETIME2(7) NOT NULL,
    CONSTRAINT [pk_idempotency_keys] PRIMARY KEY ([key_hash])
);

CREATE TABLE [invoices] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [owner_id] UNIQUEIDENTIFIER NOT NULL,
    [number] NVARCHAR(32) NOT NULL,
    [status] NVARCHAR(32) NOT NULL CONSTRAINT [df_invoices_status] DEFAULT N'draft',
    [notes] NVARCHAR(500) NULL,
    [discount] DECIMAL(10, 2) NOT NULL CONSTRAINT [df_invoices_discount] DEFAULT N'0.00',
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_invoices_created_at] DEFAULT SYSUTCDATETIME(),
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_invoices] PRIMARY KEY ([id]),
    CONSTRAINT [ck_invoices_status] CHECK ([status] IN (N'draft', N'issued', N'paid'))
);

CREATE TABLE [invoice_items] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [invoice_id] UNIQUEIDENTIFIER NOT NULL,
    [product_id] BIGINT NOT NULL,
    [quantity] INT NOT NULL CONSTRAINT [df_invoice_items_quantity] DEFAULT 1,
    [price] FLOAT NOT NULL,
    CONSTRAINT [pk_invoice_items] PRIMARY KEY ([id]),
    CONSTRAINT [fk_invoice_items_invoice_id] FOREIGN KEY ([invoice_id]) REFERENCES [invoices] ([id]) ON DELETE CASCADE
);

CREATE INDEX [ix_idempotency_keys_expires_at] ON [idempotency_keys] ([expires_at]);

CREATE INDEX [ix_invoices_owner_id] ON [invoices] ([owner_id]);

CREATE INDEX [ix_invoices_status] ON [invoices] ([status]);

CREATE INDEX [lx_invoices] ON [invoices] ([created_at], [id]);

CREATE INDEX [lx_invoices_owner_id] ON [invoices] ([owner_id], [created_at], [id]);

CREATE UNIQUE INDEX [ux_invoices_number] ON [invoices] ([number]) WHERE [deleted_at] IS NULL;

CREATE INDEX [ix_invoice_items_invoice_id] ON [invoice_items] ([invoice_id]);

CREATE INDEX [ix_invoice_items_product_id] ON [invoice_items] ([product_id]);

CREATE INDEX [lx_invoice_items_invoice_id] ON [invoice_items] ([invoice_id], [id]);

CREATE INDEX [lx_invoice_items_product_id] ON [invoice_items] ([product_id], [id]);
