-- get args: id(ID)
SELECT
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
FROM [projects]
WHERE [id] = @p1 AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL);

-- exists args: id(ID)
SELECT 1
FROM [projects]
WHERE [id] = @p1 AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL);

-- list args: filter(OrgID) filter(OwnerID) filter(Archived) offset limit
SELECT
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
FROM [projects]
WHERE EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL)
    AND (@p1 IS NULL OR [org_id] = @p1)
    AND (@p2 IS NULL OR [owner_id] = @p2)
    AND (@p3 IS NULL OR [archived] = @p3)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p4 ROWS FETCH NEXT @p5 ROWS ONLY
OPTION (RECOMPILE);

-- count args: filter(OrgID) filter(OwnerID) filter(Archived)
SELECT COUNT(*)
FROM [projects]
WHERE EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL)
    AND (@p1 IS NULL OR [org_id] = @p1)
    AND (@p2 IS NULL OR [owner_id] = @p2)
    AND (@p3 IS NULL OR [archived] = @p3)
OPTION (RECOMPILE);

-- create args: id(ID) input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt)
DECLARE @rows TABLE (
    [id] UNIQUEIDENTIFIER,
    [org_id] UNIQUEIDENTIFIER,
    [owner_id] UNIQUEIDENTIFIER,
    [name] NVARCHAR(200),
    [budget] FLOAT,
    [priority] INT,
    [archived] BIT,
    [logo] VARBINARY(MAX),
    [due_at] DATETIME2(7),
    [created_at] DATETIME2(7)
);
INSERT INTO [projects] (
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
)
OUTPUT
    INSERTED.[id],
    INSERTED.[org_id],
    INSERTED.[owner_id],
    INSERTED.[name],
    INSERTED.[budget],
    INSERTED.[priority],
    INSERTED.[archived],
    INSERTED.[logo],
    INSERTED.[due_at],
    INSERTED.[created_at]
INTO @rows
VALUES (
    @p1,
    @p2,
    @p3,
    @p4,
    @p5,
    @p6,
    @p7,
    @p8,
    @p9,
    SYSUTCDATETIME()
);
SELECT
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
FROM @rows;

-- update args: input(OrgID) input(OwnerID) input(Name) input(Budget) input(Priority) input(Archived) input(Logo) input(DueAt) id(ID)
DECLARE @rows TABLE (
    [id] UNIQUEIDENTIFIER,
    [org_id] UNIQUEIDENTIFIER,
    [owner_id] UNIQUEIDENTIFIER,
    [name] NVARCHAR(200),
    [budget] FLOAT,
    [priority] INT,
    [archived] BIT,
    [logo] VARBINARY(MAX),
    [due_at] DATETIME2(7),
    [created_at] DATETIME2(7)
);
UPDATE [projects]
SET
    [org_id] = @p1,
    [owner_id] = @p2,
    [name] = @p3,
    [budget] = @p4,
    [priority] = @p5,
    [archived] = @p6,
    [logo] = @p7,
    [due_at] = @p8
OUTPUT
    INSERTED.[id],
    INSERTED.[org_id],
    INSERTED.[owner_id],
    INSERTED.[name],
    INSERTED.[budget],
    INSERTED.[priority],
    INSERTED.[archived],
    INSERTED.[logo],
    INSERTED.[due_at],
    INSERTED.[created_at]
INTO @rows
WHERE [id] = @p9 AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL);
SELECT
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
FROM @rows;

-- delete args: id(ID)
DELETE FROM [projects]
WHERE [id] = @p1 AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL);

-- exists Org args: source(ID)
SELECT 1
FROM [orgs]
WHERE [id] = @p1 AND [deleted_at] IS NULL;

-- list Org.Projects args: source(ID) filter(OwnerID) filter(Archived) offset limit
SELECT
    [id],
    [org_id],
    [owner_id],
    [name],
    [budget],
    [priority],
    [archived],
    [logo],
    [due_at],
    [created_at]
FROM [projects]
WHERE [org_id] = @p1
    AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL)
    AND (@p2 IS NULL OR [owner_id] = @p2)
    AND (@p3 IS NULL OR [archived] = @p3)
ORDER BY [created_at] DESC, [id] DESC
OFFSET @p4 ROWS FETCH NEXT @p5 ROWS ONLY
OPTION (RECOMPILE);

-- count Org.Projects args: source(ID) filter(OwnerID) filter(Archived)
SELECT COUNT(*)
FROM [projects]
WHERE [org_id] = @p1
    AND EXISTS (SELECT 1 FROM [orgs] WHERE [orgs].[id] = [projects].[org_id] AND [orgs].[deleted_at] IS NULL)
    AND (@p2 IS NULL OR [owner_id] = @p2)
    AND (@p3 IS NULL OR [archived] = @p3)
OPTION (RECOMPILE);

