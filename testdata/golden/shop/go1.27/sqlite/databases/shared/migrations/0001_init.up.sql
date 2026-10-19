CREATE TABLE "categories" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL
);

CREATE TABLE "products" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "category_id" INTEGER,
    "name" TEXT NOT NULL,
    "price" REAL NOT NULL,
    "deleted_at" DATETIME,
    CONSTRAINT "fk_products_category_id" FOREIGN KEY ("category_id") REFERENCES "categories" ("id") ON DELETE SET NULL
);

CREATE TABLE "stocks" (
    "id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    "product_id" INTEGER NOT NULL,
    "quantity" INTEGER NOT NULL,
    CONSTRAINT "fk_stocks_product_id" FOREIGN KEY ("product_id") REFERENCES "products" ("id") ON DELETE CASCADE
);

CREATE UNIQUE INDEX "ux_categories_name" ON "categories" ("name");

CREATE INDEX "ix_products_category_id" ON "products" ("category_id");

CREATE UNIQUE INDEX "ux_stocks_product_id" ON "stocks" ("product_id");
