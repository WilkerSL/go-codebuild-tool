# Authentication

codebuild generates authentication from the specs, using only the standard library. Mark one entity as the account entity and every route of the project requires a logged-in caller unless a spec opens it.

```go
type User struct {
	_            struct{}  `gen:"auth=session|apikey,register,roles=admin"`
	ID           uuid.UUID `gen:"pk,auto"`
	Email        string    `gen:"required,unique,email,max=254,identity"`
	Name         string    `gen:"required,notempty,max=120"`
	PasswordHash string    `gen:"password,min=10"`
	Role         string    `gen:"enum=admin|member,default=member,role"`
	CreatedAt    time.Time `gen:"auto=create"`
}

type Note struct {
	ID      uuid.UUID `gen:"pk,auto"`
	OwnerID uuid.UUID `gen:"rel=belongs_to:User,ondelete=cascade,owner"`
	Title   string    `gen:"required,max=200"`
}

type Label struct {
	_    struct{} `gen:"public=read,writeroles=admin"`
	ID   int64    `gen:"pk,auto"`
	Name string   `gen:"required,unique,max=40"`
}
```

With these specs, anyone can list labels, only admins can change them, every user sees and edits only their own notes, and only admins manage accounts through `/users`.

## Methods

`auth=` takes one login method and, optionally, API keys.

| Method | Credential | Stored as | Revocation |
|---|---|---|---|
| `session` | Opaque token `ses_…` with 130 random bits | SHA-256 hash in `auth_sessions` with an expiry | Immediate: logout deletes the row; a password change deletes every other session. |
| `jwt` | HS256 access token (15 minutes by default) plus a refresh token `rt_…` | Refresh tokens as SHA-256 hashes in `auth_refresh_tokens` | Refresh tokens rotate on every use inside a transaction. A client that presents the same token again within `AUTH_REFRESH_REUSE_GRACE`, because it lost the response or two tabs refreshed at once, receives the same successor; once that successor has been used, or the window has passed, presenting the old token revokes the whole family (theft detection). Access tokens stay valid until they expire on ordinary routes. Routes restricted by `roles=` or `writeroles=`, and API key creation, check the token against the database first, so logout, `accounts revoke`, deleting the account and role changes take effect there immediately. |
| `apikey` | Key `key_…` created by a logged-in user | SHA-256 hash in `auth_api_keys`, optional expiry | `DELETE /auth/api-keys/{id}`. Keys cannot create, list or delete keys, change passwords or log out. |

Passwords are hashed with PBKDF2-SHA256, 600,000 iterations and a 16-byte random salt (`crypto/pbkdf2`). The iteration count is stored with the hash, so raising it later rehashes old passwords on the next successful login. Comparisons are constant time, and a login for an unknown account spends the same hashing time as a real one, so response timing does not reveal which accounts exist.

## Markers

| Marker | Where | Effect |
|---|---|---|
| `auth=session\|jwt[\|apikey]` | account entity | Generates authentication. One entity per project. |
| `identity` | `string` field of the account entity, with `unique` | Login name. Email identities are lowercased. |
| `password` | `string` field of the account entity | Stores the hash. Requests send `password` (the field name without `Hash`) on create and register only. `min=` sets the minimum length (8 or more); the maximum is 1024 characters. |
| `role` | `string` enum field of the account entity | Role checked by `roles=` and `writeroles=`. The account routes must then be restricted, or users could promote themselves. |
| `register` | account entity | Adds `POST /auth/register`. The role always takes its default. |
| `public` / `public=read\|create\|update\|delete` | any entity | Opens operations to anonymous callers. |
| `roles=a\|b` | any entity | Every protected operation requires one of the roles. |
| `writeroles=a\|b` | any entity | Roles for create, update, delete, link and unlink; overrides `roles` for them. |
| `owner` | `rel=belongs_to:<account entity>` field | Scopes every query of the entity to the caller. |

Relation routes combine both sides: listing `GET /sources/{id}/targets` needs read access to both entities. Linking and unlinking write the join table both entities share, so they need read and update access to both entities, whichever side the route starts from: `writeroles=admin` on `Label` makes linking labels to notes admin-only through `/notes/{id}/labels/{labelId}` and `/labels/{id}/notes/{noteId}` alike. If the role sets of the two sides have nothing in common, `codebuild check` reports it.

## Ownership

An `owner` field never appears in requests. Handlers take the caller from the request context and pass it explicitly:

```go
func (s *Service) Get(ctx context.Context, ownerID uuid.UUID, id uuid.UUID) (Note, error) {
	return s.repo.Get(ctx, ownerID, id)
}
```

Every repository query of the entity filters by the owner, so rows of other users answer 404 exactly like rows that do not exist. When an owned entity references another owned entity, for example a note inside a folder, create and update check that the referenced row belongs to the same caller.

The service is yours. To let admins see every row, read the caller with `auth.FromContext(ctx)` and write an extra repository method in a file you own.

## Endpoints

| Route | Access | Notes |
|---|---|---|
| `POST /auth/register` | public, rate limited per IP | With `register`. Body: the create fields plus `password`, without the role. |
| `POST /auth/login` | public, rate limited | Body: the identity member (for example `email`, at most its `max=` length) and `password`. Returns `accessToken`, `tokenType`, `expiresAt` and, for JWT, `refreshToken` and `refreshExpiresAt`. |
| `POST /auth/refresh` | public | JWT only. Body `{"refreshToken": …}` or the `refresh_token` cookie. |
| `POST /auth/logout` | logged in | Revokes the current session or refresh token family and clears the cookies. |
| `GET /auth/me` | logged in | The account of the caller. |
| `PUT /auth/password` | logged in, rate limited | `currentPassword` and `newPassword`; revokes every other session or refresh family. API keys stay valid; delete them, or run `accounts revoke`, when an account may be compromised. |
| `POST`, `GET /auth/api-keys`, `DELETE /auth/api-keys/{id}` | logged in | With `apikey`. The key is returned once, on creation; at most 100 keys per account. |

Unauthenticated requests get 401 with `WWW-Authenticate: Bearer`, missing roles get 403, and rate-limited requests get 429 with `Retry-After`. Request bodies must be sent with `Content-Type: application/json` (or another `+json` type); anything else gets 415. All errors are RFC 9457 problem details.

## Transport and CSRF

Clients send `Authorization: Bearer <token>`. Login also sets an `HttpOnly`, `SameSite=Strict` cookie (`session` or `access_token`, plus `refresh_token` limited to `/auth` for JWT) for browser front ends. Requests authenticated by cookie that change data must pass `net/http`'s `CrossOriginProtection`: cross-site requests are rejected unless their origin is listed in `AUTH_TRUSTED_ORIGINS`. Bearer requests are not affected. Login and refresh only accept JSON bodies, so a cross-site HTML form cannot log a browser into another account: a JSON request from another origin needs a CORS preflight, which only the origins in `CORS_ALLOWED_ORIGINS` pass.

## Configuration

`internal/platform/auth.LoadConfig` reads these variables when `internal/app` registers the routes:

| Variable | Default | Meaning |
|---|---|---|
| `AUTH_SESSION_TTL` | `24h` | Session lifetime (`session`). |
| `AUTH_ACCESS_TTL` | `15m` | Access token lifetime (`jwt`). |
| `AUTH_REFRESH_TTL` | `720h` | Refresh token lifetime (`jwt`). |
| `AUTH_REFRESH_REUSE_GRACE` | `30s` | How long a used refresh token is still accepted, for concurrent refreshes (`jwt`). |
| `AUTH_JWT_SECRET` | required for `jwt` | At least 32 bytes, for example `openssl rand -base64 48`. |
| `AUTH_JWT_PREVIOUS_SECRET` | empty | Still accepted for verification while you rotate the secret. |
| `AUTH_COOKIE_SECURE` | `true` | Set `false` only for plain HTTP during local development. |
| `AUTH_TRUSTED_ORIGINS` | empty | Comma-separated origins allowed to use the cookie cross-site. |
| `AUTH_LOGIN_LIMIT` | `10` | Attempts per client and identity per window (the client alone gets ten times more across login, registration and password changes); password changes are also limited per session. `0` disables the limit. |
| `AUTH_LOGIN_WINDOW` | `15m` | Rate limit window. |
| `AUTH_TRUST_FORWARDED_FOR` | `false` | Use the last hop of the last `X-Forwarded-For` header as the client IP; enable only behind a proxy you control. |

The limiter keeps its counters in memory, so each replica limits on its own. It stores a hash of each key and at most 100,000 keys, dropping the oldest window first, so floods of distinct identities cannot exhaust memory. IPv6 clients are counted per /64 network, the block a single host usually controls.

## Administration

Projects with authentication get `cmd/accounts`, which works directly on the database:

```bash
go run ./cmd/accounts set-role ann@example.com admin
printf '%s\n' 'a new password' | go run ./cmd/accounts set-password ann@example.com
go run ./cmd/accounts revoke ann@example.com
```

Use it to promote the first admin after registering. `set-password` and `revoke` remove every session, refresh token and API key of the account.

## Customizing

`internal/<domain>/auth_service.go` is yours and created once. The handlers call `AuthService`, which composes the generated `Accounts` primitives:

```go
func (s *AuthService) Login(ctx context.Context, in LoginInput) (auth.Tokens, error) {
	user, err := s.accounts.VerifyPassword(ctx, in.Identity, in.Password)
	if err != nil {
		return auth.Tokens{}, err
	}
	if !user.EmailVerified {
		return auth.Tokens{}, httpx.NewError(http.StatusForbidden, "verify your email first")
	}
	return s.accounts.Issue(ctx, user)
}
```

Password hashing, token issuance, refresh rotation and the guard stay in generated files, so security fixes in codebuild reach your project on the next build. When a spec change needs a new `AuthService` method, codebuild writes it to a new `auth_service_<method>.go` file.

## Generated files

| Path | Owner |
|---|---|
| `internal/platform/auth/*_gen.go` | codebuild: principal, guard, password hashing, tokens, JWT, limiter, configuration and their tests |
| `internal/<domain>/accounts_gen.go`, `auth_handler_gen.go`, `auth_routes_gen.go`, `auth_queries_gen.go`, `accounts_gen_test.go` | codebuild |
| `internal/<domain>/auth_service.go`, `auth_service_<method>.go` | you |
| `internal/app/accounts_gen.go` | codebuild |
| `cmd/accounts/main.go` | you |

Projects created before authentication existed keep their `cmd/api/main.go`. If it calls `app.RegisterRoutes(mux, db, logger)` without checking the returned error, `codebuild plan` warns: invalid `AUTH_*` settings are reported through that error.
