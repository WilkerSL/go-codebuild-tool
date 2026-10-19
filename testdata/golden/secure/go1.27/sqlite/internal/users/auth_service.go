package users

import (
	"context"
	"database/sql"

	"example.com/secure/internal/platform/auth"
)

type AuthService struct {
	accounts *Accounts
	repo     *Repository
}

func NewAuthService(db *sql.DB, accounts *Accounts) *AuthService {
	return &AuthService{accounts: accounts, repo: NewRepository(db)}
}

func (s *AuthService) Register(ctx context.Context, in CreateInput) (User, error) {
	return s.repo.Create(ctx, in)
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (auth.Tokens, error) {
	user, err := s.accounts.VerifyPassword(ctx, in.Identity, in.Password)
	if err != nil {
		return auth.Tokens{}, err
	}
	return s.accounts.Issue(ctx, user)
}

func (s *AuthService) Logout(ctx context.Context, caller auth.Principal) error {
	return s.accounts.Revoke(ctx, caller)
}

func (s *AuthService) Me(ctx context.Context, caller auth.Principal) (User, error) {
	return s.repo.Get(ctx, caller.ID)
}

func (s *AuthService) ChangePassword(ctx context.Context, caller auth.Principal, in PasswordChange) error {
	return s.accounts.ChangePassword(ctx, caller, in)
}

func (s *AuthService) CreateAPIKey(ctx context.Context, caller auth.Principal, in APIKeyInput) (CreatedAPIKey, error) {
	return s.accounts.CreateAPIKey(ctx, caller, in)
}

func (s *AuthService) ListAPIKeys(ctx context.Context, caller auth.Principal) ([]APIKey, error) {
	return s.accounts.ListAPIKeys(ctx, caller)
}

func (s *AuthService) DeleteAPIKey(ctx context.Context, caller auth.Principal, id string) error {
	return s.accounts.DeleteAPIKey(ctx, caller, id)
}
