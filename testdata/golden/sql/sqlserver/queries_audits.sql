-- get args: id(ID)
SELECT
    [id],
    [action],
    [payload],
    [created_at]
FROM [audit_log]
WHERE [id] = @p1;

-- exists args: id(ID)
SELECT 1
FROM [audit_log]
WHERE [id] = @p1;

-- list args: filter(Action) offset limit
SELECT
    [id],
    [action],
    [payload],
    [created_at]
FROM [audit_log]
WHERE (@p1 IS NULL OR [action] = @p1)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p2 ROWS FETCH NEXT @p3 ROWS ONLY
OPTION (RECOMPILE);

-- count args: filter(Action)
SELECT COUNT(*)
FROM [audit_log]
WHERE (@p1 IS NULL OR [action] = @p1)
OPTION (RECOMPILE);

-- create args: input(Action) input(Payload)
DECLARE @rows TABLE (
    [id] BIGINT,
    [action] NVARCHAR(64),
    [payload] NVARCHAR(MAX),
    [created_at] DATETIME2(7)
);
INSERT INTO [audit_log] (
    [action],
    [payload],
    [created_at]
)
OUTPUT
    INSERTED.[id],
    INSERTED.[action],
    INSERTED.[payload],
    INSERTED.[created_at]
INTO @rows
VALUES (
    @p1,
    @p2,
    SYSUTCDATETIME()
);
SELECT
    [id],
    [action],
    [payload],
    [created_at]
FROM @rows;

-- update args: input(Action) input(Payload) id(ID)
DECLARE @rows TABLE (
    [id] BIGINT,
    [action] NVARCHAR(64),
    [payload] NVARCHAR(MAX),
    [created_at] DATETIME2(7)
);
UPDATE [audit_log]
SET
    [action] = @p1,
    [payload] = @p2
OUTPUT
    INSERTED.[id],
    INSERTED.[action],
    INSERTED.[payload],
    INSERTED.[created_at]
INTO @rows
WHERE [id] = @p3;
SELECT
    [id],
    [action],
    [payload],
    [created_at]
FROM @rows;

-- delete args: id(ID)
DELETE FROM [audit_log]
WHERE [id] = @p1;

