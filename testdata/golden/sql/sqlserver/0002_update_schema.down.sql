ALTER TABLE [users] DROP CONSTRAINT [ck_users_role];

DROP INDEX [ix_users_phone] ON [users];

DROP INDEX [ux_tags_name] ON [tags];

ALTER TABLE [users] ALTER COLUMN [name] NVARCHAR(120) NOT NULL;

ALTER TABLE [users] DROP COLUMN [phone];

ALTER TABLE [tags] DROP COLUMN [deleted_at];

DROP TABLE [comments];

CREATE TABLE [countries] (
    [code] NVARCHAR(2) NOT NULL,
    [name] NVARCHAR(100) NOT NULL,
    CONSTRAINT [pk_countries] PRIMARY KEY ([code])
);

ALTER TABLE [users] ADD [bio] NVARCHAR(500) NULL;

CREATE UNIQUE INDEX [ux_tags_name] ON [tags] ([name]);

ALTER TABLE [users] ADD CONSTRAINT [ck_users_role] CHECK ([role] IN (N'admin', N'operator', N'viewer'));
