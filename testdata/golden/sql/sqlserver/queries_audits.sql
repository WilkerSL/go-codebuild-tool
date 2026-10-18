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
VALUES (
    @p1,
    @p2,
    SYSUTCDATETIME()
);

-- update args: input(Action) input(Payload) id(ID)
UPDATE [audit_log]
SET
    [action] = @p1,
    [payload] = @p2
OUTPUT
    INSERTED.[id],
    INSERTED.[action],
    INSERTED.[payload],
    INSERTED.[created_at]
WHERE [id] = @p3;

-- delete args: id(ID)
DELETE FROM [audit_log]
WHERE [id] = @p1;

