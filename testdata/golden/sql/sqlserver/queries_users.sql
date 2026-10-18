-- get args: id(ID)
SELECT
    [id],
    [name],
    [email],
    [role],
    [bio],
    [org_id],
    [created_at],
    [updated_at]
FROM [users]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- exists args: id(ID)
SELECT 1
FROM [users]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- list args: filter(Email) filter(OrgID) offset limit
SELECT
    [id],
    [name],
    [email],
    [role],
    [bio],
    [org_id],
    [created_at],
    [updated_at]
FROM [users]
WHERE [deleted_at] IS NULL
    AND (@p1 IS NULL OR [email] = @p1)
    AND (@p2 IS NULL OR [org_id] = @p2)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY
OPTION (RECOMPILE);

-- count args: filter(Email) filter(OrgID)
SELECT COUNT(*)
FROM [users]
WHERE [deleted_at] IS NULL
    AND (@p1 IS NULL OR [email] = @p1)
    AND (@p2 IS NULL OR [org_id] = @p2)
OPTION (RECOMPILE);

-- create args: id(ID) input(Name) input(Email) input(Role) input(Bio) input(OrgID)
INSERT INTO [users] (
    [id],
    [name],
    [email],
    [role],
    [bio],
    [org_id],
    [created_at],
    [updated_at]
)
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[email],
    INSERTED.[role],
    INSERTED.[bio],
    INSERTED.[org_id],
    INSERTED.[created_at],
    INSERTED.[updated_at]
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

-- update args: input(Name) input(Email) input(Role) input(Bio) input(OrgID) id(ID)
UPDATE [users]
SET
    [name] = @p1,
    [email] = @p2,
    [role] = @p3,
    [bio] = @p4,
    [org_id] = @p5,
    [updated_at] = SYSUTCDATETIME()
OUTPUT
    INSERTED.[id],
    INSERTED.[name],
    INSERTED.[email],
    INSERTED.[role],
    INSERTED.[bio],
    INSERTED.[org_id],
    INSERTED.[created_at],
    INSERTED.[updated_at]
WHERE [id] = @p6 AND [deleted_at] IS NULL;

-- delete args: id(ID)
UPDATE [users]
SET [deleted_at] = SYSUTCDATETIME(), [updated_at] = SYSUTCDATETIME()
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- exists Org args: source(ID)
SELECT 1
FROM [orgs]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- list Org.Users args: source(ID) filter(Email) offset limit
SELECT
    [id],
    [name],
    [email],
    [role],
    [bio],
    [org_id],
    [created_at],
    [updated_at]
FROM [users]
WHERE [org_id] = @p1
    AND [deleted_at] IS NULL
    AND (@p2 IS NULL OR [email] = @p2)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY
OPTION (RECOMPILE);

-- count Org.Users args: source(ID) filter(Email)
SELECT COUNT(*)
FROM [users]
WHERE [org_id] = @p1
    AND [deleted_at] IS NULL
    AND (@p2 IS NULL OR [email] = @p2)
OPTION (RECOMPILE);

-- exists Tag args: source(ID)
SELECT 1
FROM [tags]
WHERE [id] = @p1;

-- list Tag.Users args: source(ID) filter(Email) filter(OrgID) offset limit
SELECT
    [users].[id],
    [users].[name],
    [users].[email],
    [users].[role],
    [users].[bio],
    [users].[org_id],
    [users].[created_at],
    [users].[updated_at]
FROM [users]
JOIN [user_tags] ON [user_tags].[user_id] = [users].[id]
WHERE [user_tags].[tag_id] = @p1
    AND [users].[deleted_at] IS NULL
    AND (@p2 IS NULL OR [users].[email] = @p2)
    AND (@p3 IS NULL OR [users].[org_id] = @p3)
ORDER BY [users].[created_at] DESC, [users].[id] DESC
OFFSET @p4 ROWS FETCH NEXT @p5 ROWS ONLY
OPTION (RECOMPILE);

-- count Tag.Users args: source(ID) filter(Email) filter(OrgID)
SELECT COUNT(*)
FROM [users]
JOIN [user_tags] ON [user_tags].[user_id] = [users].[id]
WHERE [user_tags].[tag_id] = @p1
    AND [users].[deleted_at] IS NULL
    AND (@p2 IS NULL OR [users].[email] = @p2)
    AND (@p3 IS NULL OR [users].[org_id] = @p3)
OPTION (RECOMPILE);

-- link Tag.Users args: source(ID) id(ID)
INSERT INTO [user_tags] ([tag_id], [user_id])
SELECT @p1, @p2
WHERE NOT EXISTS (
    SELECT 1 FROM [user_tags] WHERE [tag_id] = @p1 AND [user_id] = @p2
);

-- unlink Tag.Users args: source(ID) id(ID)
DELETE FROM [user_tags]
WHERE [tag_id] = @p1 AND [user_id] = @p2;

