CREATE TABLE [auth_rate_limits] (
    [key_hash] NVARCHAR(64) NOT NULL,
    [hits] INT NOT NULL,
    [reset_at] DATETIME2(7) NOT NULL,
    CONSTRAINT [pk_auth_rate_limits] PRIMARY KEY ([key_hash])
);

CREATE TABLE [idempotency_keys] (
    [key_hash] NVARCHAR(64) NOT NULL,
    [request_hash] NVARCHAR(64) NOT NULL,
    [status] INT NOT NULL,
    [location] NVARCHAR(2048) NULL,
    [body] VARBINARY(MAX) NULL,
    [created_at] DATETIME2(7) NOT NULL,
    [expires_at] DATETIME2(7) NOT NULL,
    CONSTRAINT [pk_idempotency_keys] PRIMARY KEY ([key_hash])
);

CREATE TABLE [users] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [email] NVARCHAR(254) NOT NULL,
    [name] NVARCHAR(120) NOT NULL,
    [password_hash] NVARCHAR(255) NOT NULL,
    [role] NVARCHAR(32) NOT NULL CONSTRAINT [df_users_role] DEFAULT N'member',
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_users_created_at] DEFAULT SYSUTCDATETIME(),
    [updated_at] DATETIME2(7) NOT NULL CONSTRAINT [df_users_updated_at] DEFAULT SYSUTCDATETIME(),
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_users] PRIMARY KEY ([id]),
    CONSTRAINT [ck_users_role] CHECK ([role] IN (N'admin', N'member'))
);

CREATE TABLE [auth_api_keys] (
    [id] NVARCHAR(32) NOT NULL,
    [user_id] UNIQUEIDENTIFIER NOT NULL,
    [name] NVARCHAR(120) NOT NULL,
    [prefix] NVARCHAR(16) NOT NULL,
    [key_hash] NVARCHAR(64) NOT NULL,
    [created_at] DATETIME2(7) NOT NULL,
    [expires_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_auth_api_keys] PRIMARY KEY ([id]),
    CONSTRAINT [fk_auth_api_keys_user_id] FOREIGN KEY ([user_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE TABLE [auth_sessions] (
    [id] NVARCHAR(64) NOT NULL,
    [user_id] UNIQUEIDENTIFIER NOT NULL,
    [created_at] DATETIME2(7) NOT NULL,
    [expires_at] DATETIME2(7) NOT NULL,
    CONSTRAINT [pk_auth_sessions] PRIMARY KEY ([id]),
    CONSTRAINT [fk_auth_sessions_user_id] FOREIGN KEY ([user_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE INDEX [ix_auth_rate_limits_reset_at] ON [auth_rate_limits] ([reset_at]);

CREATE INDEX [ix_idempotency_keys_expires_at] ON [idempotency_keys] ([expires_at]);

CREATE INDEX [lx_users] ON [users] ([created_at], [id]);

CREATE UNIQUE INDEX [ux_users_email] ON [users] ([email]) WHERE [deleted_at] IS NULL;

CREATE INDEX [ix_auth_api_keys_user_id] ON [auth_api_keys] ([user_id]);

CREATE UNIQUE INDEX [ux_auth_api_keys_key_hash] ON [auth_api_keys] ([key_hash]);

CREATE INDEX [ix_auth_sessions_user_id] ON [auth_sessions] ([user_id]);
