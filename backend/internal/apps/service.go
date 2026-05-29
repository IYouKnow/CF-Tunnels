package apps

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrAppNotFound   = errors.New("app not found")
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenInvalid  = errors.New("invalid token")
	ErrTokenExpired  = errors.New("expired token")
	ErrTokenRevoked  = errors.New("revoked token")
	ErrDuplicateSlug = errors.New("duplicate slug")
	ErrAppHasTokens  = errors.New("app has tokens")
	ErrInvalidSlug   = errors.New("invalid slug")
	ErrMissingToken  = errors.New("missing token")
	slugPattern      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

const tokenPrefix = "cft_app_"
const visiblePrefixLen = 20

type App struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

type AppToken struct {
	ID          int64      `json:"id"`
	AppID       int64      `json:"appId"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"tokenPrefix"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

type CreateAppInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type CreateAppTokenInput struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type CreatedToken struct {
	AppToken
	Token string `json:"token"`
}

type AuthenticatedApp struct {
	App   App
	Token AppToken
}

type Service struct {
	DB *sql.DB
}

// Service provides app identity primitives that will later back both the
// dashboard admin experience and app-to-app Cloudflare Central API calls.
func NewService(db *sql.DB) *Service {
	return &Service{DB: db}
}

func normalizeSlug(slug string) string {
	return strings.TrimSpace(strings.ToLower(slug))
}

func validateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func visibleTokenPrefix(token string) string {
	if len(token) <= visiblePrefixLen {
		return token
	}
	return token[:visiblePrefixLen]
}

func GenerateToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain := tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, hashToken(plain), nil
}

func scopesToJSON(scopes []string) (string, error) {
	if scopes == nil {
		scopes = []string{}
	}
	normalized := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func scopesFromJSON(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return []string{}
	}
	return scopes
}

func nullableTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func (s *Service) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id, name, slug, description, created_at, updated_at FROM apps ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := make([]App, 0)
	for rows.Next() {
		var app App
		var created, updated sql.NullTime
		if err := rows.Scan(&app.ID, &app.Name, &app.Slug, &app.Description, &created, &updated); err != nil {
			return nil, err
		}
		app.CreatedAt = nullableTime(created)
		app.UpdatedAt = nullableTime(updated)
		apps = append(apps, app)
	}
	return apps, nil
}

func (s *Service) CreateApp(ctx context.Context, input CreateAppInput) (App, error) {
	// TODO: This registry is the anchor point for future app-owned tunnel and DNS resources.
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeSlug(input.Slug)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || input.Slug == "" {
		return App{}, fmt.Errorf("name and slug are required")
	}
	if err := validateSlug(input.Slug); err != nil {
		return App{}, err
	}

	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO apps (name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
	`, input.Name, input.Slug, input.Description)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return App{}, ErrDuplicateSlug
		}
		return App{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetApp(ctx, id)
}

func (s *Service) GetApp(ctx context.Context, id int64) (App, error) {
	var app App
	var created, updated sql.NullTime
	err := s.DB.QueryRowContext(ctx, "SELECT id, name, slug, description, created_at, updated_at FROM apps WHERE id = ?", id).
		Scan(&app.ID, &app.Name, &app.Slug, &app.Description, &created, &updated)
	if err == sql.ErrNoRows {
		return App{}, ErrAppNotFound
	}
	if err != nil {
		return App{}, err
	}
	app.CreatedAt = nullableTime(created)
	app.UpdatedAt = nullableTime(updated)
	return app, nil
}

func (s *Service) DeleteApp(ctx context.Context, id int64) error {
	var tokenCount int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_tokens WHERE app_id = ? AND revoked_at IS NULL", id).Scan(&tokenCount); err != nil {
		return err
	}
	if tokenCount > 0 {
		return ErrAppHasTokens
	}
	res, err := s.DB.ExecContext(ctx, "DELETE FROM apps WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrAppNotFound
	}
	return nil
}

func (s *Service) ListTokens(ctx context.Context, appID int64) ([]AppToken, error) {
	if _, err := s.GetApp(ctx, appID); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, app_id, name, token_prefix, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM app_tokens
		WHERE app_id = ?
		ORDER BY created_at DESC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]AppToken, 0)
	for rows.Next() {
		var token AppToken
		var scopes string
		var created, expires, lastUsed, revoked sql.NullTime
		if err := rows.Scan(&token.ID, &token.AppID, &token.Name, &token.TokenPrefix, &scopes, &created, &expires, &lastUsed, &revoked); err != nil {
			return nil, err
		}
		token.Scopes = scopesFromJSON(scopes)
		token.CreatedAt = nullableTime(created)
		token.ExpiresAt = nullableTime(expires)
		token.LastUsedAt = nullableTime(lastUsed)
		token.RevokedAt = nullableTime(revoked)
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (s *Service) CreateToken(ctx context.Context, appID int64, input CreateAppTokenInput) (CreatedToken, error) {
	// TODO: Reuse these tokens for the future internal API, dynamic DNS integrations,
	// and app-driven tunnel provisioning once those endpoints are introduced.
	if _, err := s.GetApp(ctx, appID); err != nil {
		return CreatedToken{}, err
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return CreatedToken{}, fmt.Errorf("token name is required")
	}
	scopesJSON, err := scopesToJSON(input.Scopes)
	if err != nil {
		return CreatedToken{}, err
	}
	plain, hash, err := GenerateToken()
	if err != nil {
		return CreatedToken{}, err
	}
	prefix := visibleTokenPrefix(plain)

	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO app_tokens (app_id, name, token_hash, token_prefix, scopes, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), ?, NULL)
	`, appID, input.Name, hash, prefix, scopesJSON, input.ExpiresAt)
	if err != nil {
		return CreatedToken{}, err
	}
	id, _ := res.LastInsertId()

	var token AppToken
	var scopes string
	var created, expires, lastUsed, revoked sql.NullTime
	err = s.DB.QueryRowContext(ctx, `
		SELECT id, app_id, name, token_prefix, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM app_tokens WHERE id = ?
	`, id).Scan(&token.ID, &token.AppID, &token.Name, &token.TokenPrefix, &scopes, &created, &expires, &lastUsed, &revoked)
	if err != nil {
		return CreatedToken{}, err
	}
	token.Scopes = scopesFromJSON(scopes)
	token.CreatedAt = nullableTime(created)
	token.ExpiresAt = nullableTime(expires)
	token.LastUsedAt = nullableTime(lastUsed)
	token.RevokedAt = nullableTime(revoked)

	return CreatedToken{
		AppToken: token,
		Token:    plain,
	}, nil
}

func (s *Service) RevokeToken(ctx context.Context, appID, tokenID int64) error {
	if _, err := s.GetApp(ctx, appID); err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE app_tokens SET revoked_at = datetime('now') WHERE id = ? AND app_id = ? AND revoked_at IS NULL", tokenID, appID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

func (s *Service) AuthenticateToken(ctx context.Context, token string) (AuthenticatedApp, error) {
	// TODO: This shared auth path will protect future central API routes called by other apps.
	token = strings.TrimSpace(token)
	if token == "" {
		return AuthenticatedApp{}, ErrMissingToken
	}
	if !strings.HasPrefix(token, tokenPrefix) {
		return AuthenticatedApp{}, ErrTokenInvalid
	}
	prefix := visibleTokenPrefix(token)
	candidateHash := hashToken(token)

	rows, err := s.DB.QueryContext(ctx, `
		SELECT
			t.id, t.app_id, t.name, t.token_hash, t.token_prefix, t.scopes, t.created_at, t.expires_at, t.last_used_at, t.revoked_at,
			a.id, a.name, a.slug, a.description, a.created_at, a.updated_at
		FROM app_tokens t
		JOIN apps a ON a.id = t.app_id
		WHERE t.token_prefix = ?
	`, prefix)
	if err != nil {
		return AuthenticatedApp{}, err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var auth AuthenticatedApp
		var storedHash, scopes string
		var tokenCreated, tokenExpires, tokenLastUsed, tokenRevoked sql.NullTime
		var appCreated, appUpdated sql.NullTime
		err := rows.Scan(
			&auth.Token.ID, &auth.Token.AppID, &auth.Token.Name, &storedHash, &auth.Token.TokenPrefix, &scopes,
			&tokenCreated, &tokenExpires, &tokenLastUsed, &tokenRevoked,
			&auth.App.ID, &auth.App.Name, &auth.App.Slug, &auth.App.Description, &appCreated, &appUpdated,
		)
		if err != nil {
			return AuthenticatedApp{}, err
		}
		if !hmac.Equal([]byte(storedHash), []byte(candidateHash)) {
			continue
		}
		auth.Token.Scopes = scopesFromJSON(scopes)
		auth.Token.CreatedAt = nullableTime(tokenCreated)
		auth.Token.ExpiresAt = nullableTime(tokenExpires)
		auth.Token.LastUsedAt = nullableTime(tokenLastUsed)
		auth.Token.RevokedAt = nullableTime(tokenRevoked)
		auth.App.CreatedAt = nullableTime(appCreated)
		auth.App.UpdatedAt = nullableTime(appUpdated)

		if auth.Token.RevokedAt != nil {
			return AuthenticatedApp{}, ErrTokenRevoked
		}
		if auth.Token.ExpiresAt != nil && auth.Token.ExpiresAt.Before(now) {
			return AuthenticatedApp{}, ErrTokenExpired
		}
		_, _ = s.DB.ExecContext(ctx, "UPDATE app_tokens SET last_used_at = datetime('now') WHERE id = ?", auth.Token.ID)
		auth.Token.LastUsedAt = &now
		return auth, nil
	}
	return AuthenticatedApp{}, ErrTokenInvalid
}

func RequireScope(scopes []string, required string) error {
	if hasScope(scopes, required) {
		return nil
	}
	return fmt.Errorf("missing required scope: %s", required)
}
