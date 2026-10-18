-- get args: id(Code)
SELECT
    `code`,
    `name`
FROM `countries`
WHERE `code` = ?;

-- exists args: id(Code)
SELECT 1
FROM `countries`
WHERE `code` = ?;

-- list args: limit offset
SELECT
    `code`,
    `name`
FROM `countries`
ORDER BY `code`
LIMIT ? OFFSET ?;

-- count args:
SELECT COUNT(*)
FROM `countries`;

