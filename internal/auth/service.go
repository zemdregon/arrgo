package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Permission enumerates capabilities that can be granted to API clients.
type Permission string

// Well-known permission constants.
const (
	PermissionLibraryRead   Permission = "library:read"
	PermissionLibraryWrite  Permission = "library:write"
	PermissionWorkerRead    Permission = "worker:read"
	PermissionWorkerManage  Permission = "worker:manage"
	PermissionIdentityAdmin Permission = "identity:admin"
)

// Role describes a named collection of permissions available to API keys.
type Role struct {
	Name        string
	Description string
	Permissions []Permission
}

// APIKey captures metadata for a generated API credential.
type APIKey struct {
	ID            string
	Label         string
	Role          string
	Version       int
	Active        bool
	CreatedAt     time.Time
	LastRotatedAt time.Time
	LastUsedAt    time.Time
	RevokedAt     time.Time
}

// APIKeyMaterial is returned when a key is generated or rotated. It includes
// the metadata and the plaintext token that should be stored by the caller.
type APIKeyMaterial struct {
	Key   APIKey
	Token string
}

// Principal describes an authenticated caller and their permissions.
type Principal struct {
	Key APIKey

	role        Role
	permissions map[Permission]struct{}
}

// Can reports whether the principal is allowed to exercise the supplied permission.
func (p Principal) Can(permission Permission) bool {
	_, ok := p.permissions[permission]
	return ok
}

// Permissions returns the set of permissions granted to the principal.
func (p Principal) Permissions() []Permission {
	out := make([]Permission, 0, len(p.permissions))
	for perm := range p.permissions {
		out = append(out, perm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Role returns the role definition associated with the principal.
func (p Principal) Role() Role {
	return p.role
}

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// IDGenerator produces identifiers for new API keys.
type IDGenerator func() string

// SecretGenerator emits plaintext API secrets.
type SecretGenerator func() (string, error)

// Config drives Service construction.
type Config struct {
	Roles           []Role
	Clock           Clock
	IDGenerator     IDGenerator
	SecretGenerator SecretGenerator
}

// Service coordinates role management and API key issuance.
type Service struct {
	clock   Clock
	idGen   IDGenerator
	secret  SecretGenerator
	mu      sync.RWMutex
	roles   map[string]Role
	apiKeys map[string]*apiKeyRecord
}

type apiKeyRecord struct {
	APIKey
	secretHash [32]byte
}

// Errors returned by the Service operations.
var (
	ErrInvalidRoleName  = errors.New("auth: role name is required")
	ErrRoleExists       = errors.New("auth: role already exists")
	ErrRoleNotFound     = errors.New("auth: role not found")
	ErrInvalidKeyLabel  = errors.New("auth: api key label is required")
	ErrAPIKeyNotFound   = errors.New("auth: api key not found")
	ErrAPIKeyRevoked    = errors.New("auth: api key revoked")
	ErrInvalidToken     = errors.New("auth: invalid api token")
	ErrPermissionDenied = errors.New("auth: permission denied")
)

// DefaultRoles returns the baseline set of roles exposed by the platform.
func DefaultRoles() []Role {
	return []Role{
		{
			Name:        "admin",
			Description: "Full control over library orchestration, worker pools, and identity management.",
			Permissions: []Permission{
				PermissionLibraryRead,
				PermissionLibraryWrite,
				PermissionWorkerRead,
				PermissionWorkerManage,
				PermissionIdentityAdmin,
			},
		},
		{
			Name:        "operator",
			Description: "Manage library jobs and worker pools without touching identity settings.",
			Permissions: []Permission{
				PermissionLibraryRead,
				PermissionLibraryWrite,
				PermissionWorkerRead,
				PermissionWorkerManage,
			},
		},
		{
			Name:        "observer",
			Description: "Read-only access to catalog metadata and worker status dashboards.",
			Permissions: []Permission{
				PermissionLibraryRead,
				PermissionWorkerRead,
			},
		},
	}
}

// NewService constructs a role and API key manager.
func NewService(cfg Config) (*Service, error) {
	roles := cfg.Roles
	if len(roles) == 0 {
		roles = DefaultRoles()
	}

	svc := &Service{
		clock:   cfg.Clock,
		idGen:   cfg.IDGenerator,
		secret:  cfg.SecretGenerator,
		roles:   make(map[string]Role, len(roles)),
		apiKeys: make(map[string]*apiKeyRecord),
	}

	if svc.clock == nil {
		svc.clock = systemClock{}
	}
	if svc.idGen == nil {
		svc.idGen = func() string { return uuid.NewString() }
	}
	if svc.secret == nil {
		svc.secret = defaultSecretGenerator
	}

	for _, role := range roles {
		if err := svc.storeRole(role, false); err != nil {
			return nil, err
		}
	}

	return svc, nil
}

// CreateRole registers a new role definition.
func (s *Service) CreateRole(role Role) (Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.storeRole(role, true); err != nil {
		return Role{}, err
	}

	stored := s.roles[normalizeRoleName(role.Name)]
	return cloneRole(stored), nil
}

// ListRoles returns the known roles sorted by name.
func (s *Service) ListRoles() []Role {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Role, 0, len(s.roles))
	for _, role := range s.roles {
		out = append(out, cloneRole(role))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CreateAPIKey generates a new API key for the requested role and label.
func (s *Service) CreateAPIKey(ctx context.Context, label, roleName string) (APIKeyMaterial, error) {
	_ = ctx

	label = strings.TrimSpace(label)
	if label == "" {
		return APIKeyMaterial{}, ErrInvalidKeyLabel
	}
	normalizedRole := normalizeRoleName(roleName)
	if normalizedRole == "" {
		return APIKeyMaterial{}, ErrRoleNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	role, ok := s.roles[normalizedRole]
	if !ok {
		return APIKeyMaterial{}, ErrRoleNotFound
	}

	secret, err := s.secret()
	if err != nil {
		return APIKeyMaterial{}, err
	}
	keyID := strings.TrimSpace(s.idGen())
	if keyID == "" {
		return APIKeyMaterial{}, fmt.Errorf("auth: id generator returned empty identifier")
	}
	now := s.clock.Now().UTC()

	record := &apiKeyRecord{
		APIKey: APIKey{
			ID:            keyID,
			Label:         label,
			Role:          role.Name,
			Version:       1,
			Active:        true,
			CreatedAt:     now,
			LastRotatedAt: now,
		},
		secretHash: hashSecret(secret),
	}
	s.apiKeys[keyID] = record

	return APIKeyMaterial{Key: cloneAPIKey(record.APIKey), Token: assembleToken(keyID, secret)}, nil
}

// RotateAPIKey issues new credentials for an existing API key. The previous token
// becomes invalid immediately.
func (s *Service) RotateAPIKey(ctx context.Context, keyID, newLabel string) (APIKeyMaterial, error) {
	_ = ctx

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return APIKeyMaterial{}, ErrAPIKeyNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.apiKeys[keyID]
	if !ok {
		return APIKeyMaterial{}, ErrAPIKeyNotFound
	}
	if !record.Active {
		return APIKeyMaterial{}, ErrAPIKeyRevoked
	}

	secret, err := s.secret()
	if err != nil {
		return APIKeyMaterial{}, err
	}
	record.secretHash = hashSecret(secret)
	record.Version++
	record.LastRotatedAt = s.clock.Now().UTC()
	trimmedLabel := strings.TrimSpace(newLabel)
	if trimmedLabel != "" {
		record.Label = trimmedLabel
	}

	return APIKeyMaterial{Key: cloneAPIKey(record.APIKey), Token: assembleToken(record.ID, secret)}, nil
}

// RevokeAPIKey disables a credential so it can no longer be used.
func (s *Service) RevokeAPIKey(ctx context.Context, keyID string) error {
	_ = ctx

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return ErrAPIKeyNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.apiKeys[keyID]
	if !ok {
		return ErrAPIKeyNotFound
	}
	if !record.Active {
		return ErrAPIKeyRevoked
	}

	record.Active = false
	record.RevokedAt = s.clock.Now().UTC()
	return nil
}

// ListAPIKeys returns metadata for all issued API keys.
func (s *Service) ListAPIKeys(ctx context.Context) []APIKey {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]APIKey, 0, len(s.apiKeys))
	for _, record := range s.apiKeys {
		out = append(out, cloneAPIKey(record.APIKey))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Authenticate validates a presented token and returns the associated principal.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	_ = ctx

	token = strings.TrimSpace(token)
	if token == "" {
		return Principal{}, ErrInvalidToken
	}
	keyID, secret, ok := splitToken(token)
	if !ok {
		return Principal{}, ErrInvalidToken
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.apiKeys[keyID]
	if !ok {
		return Principal{}, ErrInvalidToken
	}
	if !record.Active {
		return Principal{}, ErrAPIKeyRevoked
	}
	if record.secretHash != hashSecret(secret) {
		return Principal{}, ErrInvalidToken
	}

	normalizedRole := normalizeRoleName(record.Role)
	role, ok := s.roles[normalizedRole]
	if !ok {
		return Principal{}, ErrRoleNotFound
	}

	record.LastUsedAt = s.clock.Now().UTC()
	meta := cloneAPIKey(record.APIKey)
	roleCopy := cloneRole(role)
	principal := buildPrincipal(meta, roleCopy)
	record.APIKey = meta

	return principal, nil
}

// Authorize authenticates a token and checks for the requested permission.
func (s *Service) Authorize(ctx context.Context, token string, permission Permission) (Principal, error) {
	principal, err := s.Authenticate(ctx, token)
	if err != nil {
		return Principal{}, err
	}
	if !principal.Can(permission) {
		return Principal{}, ErrPermissionDenied
	}
	return principal, nil
}

func (s *Service) storeRole(role Role, failOnExisting bool) error {
	name := normalizeRoleName(role.Name)
	if name == "" {
		return ErrInvalidRoleName
	}
	if len(role.Permissions) == 0 {
		return fmt.Errorf("auth: role %s must define at least one permission", role.Name)
	}

	sanitized := cloneRole(role)
	sanitized.Name = name
	sanitized.Permissions = uniquePermissions(sanitized.Permissions)

	if _, exists := s.roles[name]; exists {
		if failOnExisting {
			return ErrRoleExists
		}
		s.roles[name] = sanitized
		return nil
	}

	s.roles[name] = sanitized
	return nil
}

func uniquePermissions(perms []Permission) []Permission {
	if len(perms) == 0 {
		return nil
	}
	seen := make(map[Permission]struct{}, len(perms))
	out := make([]Permission, 0, len(perms))
	for _, perm := range perms {
		if _, exists := seen[perm]; exists {
			continue
		}
		seen[perm] = struct{}{}
		out = append(out, perm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeRoleName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneRole(role Role) Role {
	cloned := role
	cloned.Permissions = append([]Permission(nil), role.Permissions...)
	return cloned
}

func cloneAPIKey(key APIKey) APIKey {
	cloned := key
	return cloned
}

func hashSecret(secret string) [32]byte {
	return sha256.Sum256([]byte(secret))
}

func assembleToken(id, secret string) string {
	return fmt.Sprintf("%s.%s", id, secret)
}

func splitToken(token string) (string, string, bool) {
	idx := strings.Index(token, ".")
	if idx <= 0 || idx == len(token)-1 {
		return "", "", false
	}
	return token[:idx], token[idx+1:], true
}

func defaultSecretGenerator() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func buildPrincipal(key APIKey, role Role) Principal {
	perms := make(map[Permission]struct{}, len(role.Permissions))
	for _, perm := range role.Permissions {
		perms[perm] = struct{}{}
	}
	return Principal{Key: key, role: role, permissions: perms}
}
