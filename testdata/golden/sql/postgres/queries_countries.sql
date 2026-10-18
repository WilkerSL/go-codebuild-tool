-- get args: id(Code)
SELECT
    "code",
    "name"
FROM "countries"
WHERE "code" = $1;

-- exists args: id(Code)
SELECT 1
FROM "countries"
WHERE "code" = $1;

-- list args: limit offset
SELECT
    "code",
    "name"
FROM "countries"
ORDER BY "code"
LIMIT $1 OFFSET $2;

-- count args:
SELECT COUNT(*)
FROM "countries";

