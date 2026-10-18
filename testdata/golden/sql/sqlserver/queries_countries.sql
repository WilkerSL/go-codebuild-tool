-- get args: id(Code)
SELECT
    [code],
    [name]
FROM [countries]
WHERE [code] = @p1;

-- exists args: id(Code)
SELECT 1
FROM [countries]
WHERE [code] = @p1;

-- list args: offset limit
SELECT
    [code],
    [name]
FROM [countries]
ORDER BY [code]
OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY;

-- count args:
SELECT COUNT(*)
FROM [countries];

