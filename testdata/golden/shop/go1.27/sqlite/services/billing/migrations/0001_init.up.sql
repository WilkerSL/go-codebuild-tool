CREATE TABLE "invoices" (
    "id" TEXT NOT NULL,
    "owner_id" TEXT NOT NULL,
    "number" TEXT NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'draft',
    "notes" TEXT,
    "created_at" DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
    "deleted_at" DATETIME,
    CONSTRAINT "pk_invoices" PRIMARY KEY ("id"),
    CONSTRAINT "ck_invoices_status" CHECK ("status" IN ('draft', 'issued', 'paid'))
);

CREATE TABLE "invoice_items" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "invoice_id" TEXT NOT NULL,
    "product_id" INTEGER NOT NULL,
    "quantity" INTEGER NOT NULL DEFAULT 1,
    "price" REAL NOT NULL,
    CONSTRAINT "fk_invoice_items_invoice_id" FOREIGN KEY ("invoice_id") REFERENCES "invoices" ("id") ON DELETE CASCADE
);

CREATE INDEX "ix_invoices_owner_id" ON "invoices" ("owner_id");

CREATE INDEX "ix_invoices_status" ON "invoices" ("status");

CREATE INDEX "lx_invoices" ON "invoices" ("created_at", "id");

CREATE INDEX "lx_invoices_owner_id" ON "invoices" ("owner_id", "created_at", "id");

CREATE UNIQUE INDEX "ux_invoices_number" ON "invoices" ("number") WHERE "deleted_at" IS NULL;

CREATE INDEX "ix_invoice_items_invoice_id" ON "invoice_items" ("invoice_id");

CREATE INDEX "ix_invoice_items_product_id" ON "invoice_items" ("product_id");

CREATE INDEX "lx_invoice_items_invoice_id" ON "invoice_items" ("invoice_id", "id");

CREATE INDEX "lx_invoice_items_product_id" ON "invoice_items" ("product_id", "id");
