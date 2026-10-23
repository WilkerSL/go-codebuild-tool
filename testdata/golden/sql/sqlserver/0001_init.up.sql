CREATE TABLE [audit_log] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [action] NVARCHAR(64) NOT NULL,
    [payload] NVARCHAR(MAX) NULL,
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_audit_log_created_at] DEFAULT SYSUTCDATETIME(),
    CONSTRAINT [pk_audit_log] PRIMARY KEY ([id])
);

CREATE TABLE [countries] (
    [code] NVARCHAR(2) NOT NULL,
    [name] NVARCHAR(100) NOT NULL,
    CONSTRAINT [pk_countries] PRIMARY KEY ([code])
);

CREATE TABLE [orgs] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [name] NVARCHAR(120) NOT NULL,
    [slug] NVARCHAR(64) NOT NULL,
    [plan] NVARCHAR(32) NOT NULL CONSTRAINT [df_orgs_plan] DEFAULT N'free',
    [active] BIT NOT NULL CONSTRAINT [df_orgs_active] DEFAULT 1,
    [locale] NVARCHAR(MAX) NOT NULL CONSTRAINT [df_orgs_locale] DEFAULT N'en-US',
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_orgs_created_at] DEFAULT SYSUTCDATETIME(),
    [updated_at] DATETIME2(7) NOT NULL CONSTRAINT [df_orgs_updated_at] DEFAULT SYSUTCDATETIME(),
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_orgs] PRIMARY KEY ([id]),
    CONSTRAINT [ck_orgs_plan] CHECK ([plan] IN (N'free', N'pro', N'enterprise'))
);

CREATE TABLE [tags] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [name] NVARCHAR(50) NOT NULL,
    [color] NVARCHAR(7) NULL,
    CONSTRAINT [pk_tags] PRIMARY KEY ([id])
);

CREATE TABLE [users] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [name] NVARCHAR(120) NOT NULL,
    [email] NVARCHAR(254) NOT NULL,
    [role] NVARCHAR(32) NOT NULL CONSTRAINT [df_users_role] DEFAULT N'viewer',
    [bio] NVARCHAR(500) NULL,
    [org_id] UNIQUEIDENTIFIER NOT NULL,
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_users_created_at] DEFAULT SYSUTCDATETIME(),
    [updated_at] DATETIME2(7) NOT NULL CONSTRAINT [df_users_updated_at] DEFAULT SYSUTCDATETIME(),
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_users] PRIMARY KEY ([id]),
    CONSTRAINT [fk_users_org_id] FOREIGN KEY ([org_id]) REFERENCES [orgs] ([id]) ON DELETE NO ACTION,
    CONSTRAINT [ck_users_role] CHECK ([role] IN (N'admin', N'operator', N'viewer'))
);

CREATE TABLE [projects] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [org_id] UNIQUEIDENTIFIER NOT NULL,
    [owner_id] UNIQUEIDENTIFIER NULL,
    [name] NVARCHAR(200) NOT NULL,
    [budget] FLOAT NOT NULL,
    [priority] INT NOT NULL CONSTRAINT [df_projects_priority] DEFAULT 3,
    [archived] BIT NOT NULL,
    [logo] VARBINARY(MAX) NOT NULL,
    [due_at] DATETIME2(7) NULL,
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_projects_created_at] DEFAULT SYSUTCDATETIME(),
    CONSTRAINT [pk_projects] PRIMARY KEY ([id]),
    CONSTRAINT [fk_projects_org_id] FOREIGN KEY ([org_id]) REFERENCES [orgs] ([id]) ON DELETE CASCADE,
    CONSTRAINT [fk_projects_owner_id] FOREIGN KEY ([owner_id]) REFERENCES [users] ([id]) ON DELETE SET NULL
);

CREATE TABLE [user_tags] (
    [tag_id] BIGINT NOT NULL,
    [user_id] UNIQUEIDENTIFIER NOT NULL,
    CONSTRAINT [pk_user_tags] PRIMARY KEY ([tag_id], [user_id]),
    CONSTRAINT [fk_user_tags_tag_id] FOREIGN KEY ([tag_id]) REFERENCES [tags] ([id]) ON DELETE CASCADE,
    CONSTRAINT [fk_user_tags_user_id] FOREIGN KEY ([user_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE INDEX [ix_audit_log_action] ON [audit_log] ([action]);

CREATE INDEX [ix_audit_log_created_at] ON [audit_log] ([created_at]);

CREATE INDEX [lx_audit_log] ON [audit_log] ([created_at], [id]);

CREATE INDEX [ix_orgs_plan] ON [orgs] ([plan]);

CREATE INDEX [lx_orgs] ON [orgs] ([created_at], [id]);

CREATE UNIQUE INDEX [ux_orgs_slug] ON [orgs] ([slug]) WHERE [deleted_at] IS NULL;

CREATE UNIQUE INDEX [ux_tags_name] ON [tags] ([name]);

CREATE INDEX [ix_users_org_id] ON [users] ([org_id]);

CREATE INDEX [lx_users] ON [users] ([created_at], [id]);

CREATE INDEX [lx_users_org_id] ON [users] ([org_id], [created_at], [id]);

CREATE UNIQUE INDEX [ux_users_email] ON [users] ([email]) WHERE [deleted_at] IS NULL;

CREATE INDEX [ix_projects_archived] ON [projects] ([archived]);

CREATE INDEX [ix_projects_org_id] ON [projects] ([org_id]);

CREATE INDEX [ix_projects_owner_id] ON [projects] ([owner_id]);

CREATE INDEX [lx_projects] ON [projects] ([created_at], [id]);

CREATE INDEX [lx_projects_org_id] ON [projects] ([org_id], [created_at], [id]);

CREATE INDEX [lx_projects_owner_id] ON [projects] ([owner_id], [created_at], [id]);

CREATE INDEX [ix_user_tags_user_id] ON [user_tags] ([user_id]);
