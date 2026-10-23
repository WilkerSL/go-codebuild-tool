CREATE TABLE [auth_rate_limits] (
    [key_hash] NVARCHAR(64) NOT NULL,
    [hits] INT NOT NULL,
    [reset_at] DATETIME2(7) NOT NULL,
    CONSTRAINT [pk_auth_rate_limits] PRIMARY KEY ([key_hash])
);

CREATE TABLE [labels] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [name] NVARCHAR(40) NOT NULL,
    CONSTRAINT [pk_labels] PRIMARY KEY ([id])
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

CREATE TABLE [auth_refresh_tokens] (
    [id] NVARCHAR(64) NOT NULL,
    [family_id] NVARCHAR(32) NOT NULL,
    [user_id] UNIQUEIDENTIFIER NOT NULL,
    [created_at] DATETIME2(7) NOT NULL,
    [expires_at] DATETIME2(7) NOT NULL,
    [used_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_auth_refresh_tokens] PRIMARY KEY ([id]),
    CONSTRAINT [fk_auth_refresh_tokens_user_id] FOREIGN KEY ([user_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE TABLE [folders] (
    [id] BIGINT IDENTITY(1,1) NOT NULL,
    [owner_id] UNIQUEIDENTIFIER NOT NULL,
    [name] NVARCHAR(80) NOT NULL,
    CONSTRAINT [pk_folders] PRIMARY KEY ([id]),
    CONSTRAINT [fk_folders_owner_id] FOREIGN KEY ([owner_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE TABLE [notes] (
    [id] UNIQUEIDENTIFIER NOT NULL,
    [owner_id] UNIQUEIDENTIFIER NOT NULL,
    [folder_id] BIGINT NULL,
    [title] NVARCHAR(200) NOT NULL,
    [body] NVARCHAR(MAX) NULL,
    [pinned] BIT NOT NULL CONSTRAINT [df_notes_pinned] DEFAULT 0,
    [created_at] DATETIME2(7) NOT NULL CONSTRAINT [df_notes_created_at] DEFAULT SYSUTCDATETIME(),
    [updated_at] DATETIME2(7) NOT NULL CONSTRAINT [df_notes_updated_at] DEFAULT SYSUTCDATETIME(),
    [deleted_at] DATETIME2(7) NULL,
    CONSTRAINT [pk_notes] PRIMARY KEY ([id]),
    CONSTRAINT [fk_notes_folder_id] FOREIGN KEY ([folder_id]) REFERENCES [folders] ([id]) ON DELETE NO ACTION,
    CONSTRAINT [fk_notes_owner_id] FOREIGN KEY ([owner_id]) REFERENCES [users] ([id]) ON DELETE CASCADE
);

CREATE TABLE [note_labels] (
    [label_id] BIGINT NOT NULL,
    [note_id] UNIQUEIDENTIFIER NOT NULL,
    CONSTRAINT [pk_note_labels] PRIMARY KEY ([label_id], [note_id]),
    CONSTRAINT [fk_note_labels_label_id] FOREIGN KEY ([label_id]) REFERENCES [labels] ([id]) ON DELETE CASCADE,
    CONSTRAINT [fk_note_labels_note_id] FOREIGN KEY ([note_id]) REFERENCES [notes] ([id]) ON DELETE CASCADE
);

CREATE INDEX [ix_auth_rate_limits_reset_at] ON [auth_rate_limits] ([reset_at]);

CREATE UNIQUE INDEX [ux_labels_name] ON [labels] ([name]);

CREATE INDEX [lx_users] ON [users] ([created_at], [id]);

CREATE UNIQUE INDEX [ux_users_email] ON [users] ([email]) WHERE [deleted_at] IS NULL;

CREATE INDEX [ix_auth_api_keys_user_id] ON [auth_api_keys] ([user_id]);

CREATE UNIQUE INDEX [ux_auth_api_keys_key_hash] ON [auth_api_keys] ([key_hash]);

CREATE INDEX [ix_auth_refresh_tokens_family_id] ON [auth_refresh_tokens] ([family_id]);

CREATE INDEX [ix_auth_refresh_tokens_user_id] ON [auth_refresh_tokens] ([user_id]);

CREATE INDEX [ix_folders_owner_id] ON [folders] ([owner_id]);

CREATE INDEX [lx_folders_owner_id] ON [folders] ([owner_id], [id]);

CREATE INDEX [ix_notes_folder_id] ON [notes] ([folder_id]);

CREATE INDEX [ix_notes_owner_id] ON [notes] ([owner_id]);

CREATE INDEX [ix_notes_pinned] ON [notes] ([pinned]);

CREATE INDEX [lx_notes] ON [notes] ([created_at], [id]);

CREATE INDEX [lx_notes_folder_id] ON [notes] ([folder_id], [created_at], [id]);

CREATE INDEX [lx_notes_owner_id] ON [notes] ([owner_id], [created_at], [id]);

CREATE INDEX [ix_note_labels_note_id] ON [note_labels] ([note_id]);
