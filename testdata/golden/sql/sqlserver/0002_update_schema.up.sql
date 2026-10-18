ALTER TABLE [users] DROP CONSTRAINT [ck_users_role];

DROP INDEX [ux_tags_name] ON [tags];

ALTER TABLE [users] DROP COLUMN [bio];

DROP TABLE [countries];

CREATE TABLE [comments] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [project_id] UNIQUEIDENTIFIER NOT NULL,
    [body] NVARCHAR(2000) NOT NULL,
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_comments_created_at] DEFAULT SYSUTCDATETIME(),
    CONSTRAINT [pk_comments] PRIMARY KEY ([id]),
    CONSTRAINT [fk_comments_project_id] FOREIGN KEY ([project_id]) REFERENCES [projects] ([id]) ON DELETE CASCADE
);

ALTER TABLE [tags] ADD [deleted_at] DATETIME2(7) NULL;

ALTER TABLE [users] ADD [phone] NVARCHAR(20) NULL;

ALTER TABLE [users] ALTER COLUMN [name] NVARCHAR(200) NOT NULL;

CREATE UNIQUE INDEX [ux_tags_name] ON [tags] ([name]) WHERE [deleted_at] IS NULL;

CREATE INDEX [ix_users_phone] ON [users] ([phone]);

CREATE INDEX [ix_comments_project_id] ON [comments] ([project_id]);

ALTER TABLE [users] ADD CONSTRAINT [ck_users_role] CHECK ([role] IN (N'owner', N'admin', N'operator', N'viewer'));
