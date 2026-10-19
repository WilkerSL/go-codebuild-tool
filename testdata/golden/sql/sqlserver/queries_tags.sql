-- get args: id(ID)
SELECT
    [id],
    [name],
    [color]
FROM [tags]
WHERE [id] = @p1;

-- exists args: id(ID)
SELECT 1
FROM [tags]
WHERE [id] = @p1;

-- list args: filter(Name) offset limit
SELECT
    [id],
    [name],
    [color]
FROM [tags]
WHERE (@p1 IS NULL OR [name] = @p1)
ORDER BY [id]
OFFSET @p2 ROWS FETCH NEXT @p3 ROWS ONLY
OPTION (RECOMPILE);

-- count args: filter(Name)
SELECT COUNT(*)
FROM [tags]
WHERE (@p1 IS NULL OR [name] = @p1)
OPTION (RECOMPILE);

-- create args: input(Name) input(Color)
DECLARE @rows TABLE (
    [id] BIGINT,
    [name] NVARCHAR(50),
    [color] NVARCHAR(7)
);
INSERT INTO [tags] (
    [name],
    [color]
)
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[color]
INTO @rows
VALUES (
    @p1,
    @p2
);
SELECT
    [id],
    [name],
    [color]
FROM @rows;

-- update args: input(Name) input(Color) id(ID)
DECLARE @rows TABLE (
    [id] BIGINT,
    [name] NVARCHAR(50),
    [color] NVARCHAR(7)
);
UPDATE [tags]
SET
    [name] = @p1,
    [color] = @p2
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[color]
INTO @rows
WHERE [id] = @p3;
SELECT
    [id],
    [name],
    [color]
FROM @rows;

-- delete args: id(ID)
DELETE FROM [tags]
WHERE [id] = @p1;

-- exists User args: source(ID)
SELECT 1
FROM [users]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- list User.Tags args: source(ID) filter(Name) offset limit
SELECT
    [tags].[id],
    [tags].[name],
    [tags].[color]
FROM [tags]
JOIN [user_tags] ON [user_tags].[tag_id] = [tags].[id]
WHERE [user_tags].[user_id] = @p1
    AND (@p2 IS NULL OR [tags].[name] = @p2)
ORDER BY [tags].[id]
OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY
OPTION (RECOMPILE);

-- count User.Tags args: source(ID) filter(Name)
SELECT COUNT(*)
FROM [tags]
JOIN [user_tags] ON [user_tags].[tag_id] = [tags].[id]
WHERE [user_tags].[user_id] = @p1
    AND (@p2 IS NULL OR [tags].[name] = @p2)
OPTION (RECOMPILE);

-- link User.Tags args: source(ID) id(ID)
INSERT INTO [user_tags] ([user_id], [tag_id])
SELECT @p1, @p2
WHERE NOT EXISTS (
    SELECT 1 FROM [user_tags] WHERE [user_id] = @p1 AND [tag_id] = @p2
);

-- unlink User.Tags args: source(ID) id(ID)
DELETE FROM [user_tags]
WHERE [user_id] = @p1 AND [tag_id] = @p2;

