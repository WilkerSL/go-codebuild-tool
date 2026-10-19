-- get args: id(ID)
SELECT
    [id],
    [name],
    [slug],
    [plan],
    [active],
    [locale],
    [created_at],
    [updated_at]
FROM [orgs]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- exists args: id(ID)
SELECT 1
FROM [orgs]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- list args: filter(Slug) filter(Plan) offset limit
SELECT
    [id],
    [name],
    [slug],
    [plan],
    [active],
    [locale],
    [created_at],
    [updated_at]
FROM [orgs]
WHERE [deleted_at] IS NULL
    AND (@p1 IS NULL OR [slug] = @p1)
    AND (@p2 IS NULL OR [plan] = @p2)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY
OPTION (RECOMPILE);

-- count args: filter(Slug) filter(Plan)
SELECT COUNT(*)
FROM [orgs]
WHERE [deleted_at] IS NULL
    AND (@p1 IS NULL OR [slug] = @p1)
    AND (@p2 IS NULL OR [plan] = @p2)
OPTION (RECOMPILE);

-- create args: id(ID) input(Name) input(Slug) input(Plan) input(Active) input(Locale)
DECLARE @rows TABLE (
    [id] UNIQUEIDENTIFIER,
    [name] NVARCHAR(120),
    [slug] NVARCHAR(64),
    [plan] NVARCHAR(32),
    [active] BIT,
    [locale] NVARCHAR(MAX),
    [created_at] DATETIME2(7),
    [updated_at] DATETIME2(7)
);
INSERT INTO [orgs] (
    [id],
    [name],
    [slug],
    [plan],
    [active],
    [locale],
    [created_at],
    [updated_at]
)
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[slug],
    INSERTED.[plan],
    INSERTED.[active],
    INSERTED.[locale],
    INSERTED.[created_at],
    INSERTED.[updated_at]
INTO @rows
VALUES (
    @p1,
    @p2,
    @p3,
    @p4,
    @p5,
    @p6,
    SYSUTCDATETIME(),
    SYSUTCDATETIME()
);
SELECT
    [id],
    [name],
    [slug],
    [plan],
    [active],
    [locale],
    [created_at],
    [updated_at]
FROM @rows;

-- update args: input(Name) input(Slug) input(Plan) input(Active) input(Locale) id(ID)
DECLARE @rows TABLE (
    [id] UNIQUEIDENTIFIER,
    [name] NVARCHAR(120),
    [slug] NVARCHAR(64),
    [plan] NVARCHAR(32),
    [active] BIT,
    [locale] NVARCHAR(MAX),
    [created_at] DATETIME2(7),
    [updated_at] DATETIME2(7)
);
UPDATE [orgs]
SET
    [name] = @p1,
    [slug] = @p2,
    [plan] = @p3,
    [active] = @p4,
    [locale] = @p5,
    [updated_at] = SYSUTCDATETIME()
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[slug],
    INSERTED.[plan],
    INSERTED.[active],
    INSERTED.[locale],
    INSERTED.[created_at],
    INSERTED.[updated_at]
INTO @rows
WHERE [id] = @p6 AND [deleted_at] IS NULL;
SELECT
    [id],
    [name],
    [slug],
    [plan],
    [active],
    [locale],
    [created_at],
    [updated_at]
FROM @rows;

-- delete args: id(ID)
UPDATE [orgs]
SET [deleted_at] = SYSUTCDATETIME(), [updated_at] = SYSUTCDATETIME()
WHERE [id] = @p1 AND [deleted_at] IS NULL;

