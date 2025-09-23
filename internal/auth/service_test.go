package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubClock struct {
	now time.Time
}

func (s *stubClock) Now() time.Time {
	return s.now
}

func (s *stubClock) Advance(d time.Duration) {
	s.now = s.now.Add(d)
}

type stubSecretGenerator struct {
	secrets []string
	idx     int
}

func (s *stubSecretGenerator) Next() (string, error) {
	if s.idx >= len(s.secrets) {
		return "", errors.New("no more secrets")
	}
	val := s.secrets[s.idx]
	s.idx++
	return val, nil
}

func TestCreateAndAuthenticateAPIKey(t *testing.T) {
	clock := &stubClock{now: time.Unix(1700000000, 0)}
	secrets := &stubSecretGenerator{secrets: []string{"plain-secret"}}
	svc := mustService(t, Config{
		Clock:           clock,
		SecretGenerator: secrets.Next,
		IDGenerator: func() string {
			return "key-1"
		},
	})

	material, err := svc.CreateAPIKey(context.Background(), "dashboard", "admin")
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	if material.Token != "key-1.plain-secret" {
		t.Fatalf("unexpected token %q", material.Token)
	}
	if !material.Key.Active || material.Key.Version != 1 {
		t.Fatalf("expected active key version 1, got %+v", material.Key)
	}
	if got := material.Key.Label; got != "dashboard" {
		t.Fatalf("expected label dashboard, got %q", got)
	}
	if got := material.Key.Role; got != "admin" {
		t.Fatalf("expected role admin, got %q", got)
	}

	principal, err := svc.Authenticate(context.Background(), material.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if !principal.Can(PermissionLibraryRead) || !principal.Can(PermissionWorkerManage) {
		t.Fatalf("expected principal to have broad permissions, got %#v", principal.Permissions())
	}
	if !principal.Key.LastUsedAt.Equal(clock.now) {
		t.Fatalf("expected last used at to match clock, got %v", principal.Key.LastUsedAt)
	}
}

func TestAuthorizePermissionDenied(t *testing.T) {
	clock := &stubClock{now: time.Unix(1700000000, 0)}
	secrets := &stubSecretGenerator{secrets: []string{"observer-secret"}}
	svc := mustService(t, Config{
		Clock:           clock,
		SecretGenerator: secrets.Next,
		IDGenerator:     func() string { return "key-obs" },
	})

	material, err := svc.CreateAPIKey(context.Background(), "monitoring", "observer")
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	if _, err := svc.Authorize(context.Background(), material.Token, PermissionWorkerManage); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestRotateAPIKey(t *testing.T) {
	clock := &stubClock{now: time.Unix(1700000000, 0)}
	secrets := &stubSecretGenerator{secrets: []string{"first", "second"}}
	svc := mustService(t, Config{
		Clock:           clock,
		SecretGenerator: secrets.Next,
		IDGenerator:     func() string { return "key-rotate" },
	})

	material, err := svc.CreateAPIKey(context.Background(), "automation", "operator")
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	if material.Token != "key-rotate.first" {
		t.Fatalf("unexpected token %q", material.Token)
	}

	clock.Advance(time.Hour)
	rotated, err := svc.RotateAPIKey(context.Background(), material.Key.ID, "automation-rotated")
	if err != nil {
		t.Fatalf("RotateAPIKey returned error: %v", err)
	}
	if rotated.Token != "key-rotate.second" {
		t.Fatalf("unexpected rotated token %q", rotated.Token)
	}
	if rotated.Key.Version != 2 {
		t.Fatalf("expected version 2 after rotation, got %d", rotated.Key.Version)
	}
	if rotated.Key.Label != "automation-rotated" {
		t.Fatalf("expected updated label, got %q", rotated.Key.Label)
	}
	if rotated.Key.LastRotatedAt.Equal(material.Key.LastRotatedAt) {
		t.Fatalf("expected rotation timestamp to advance")
	}

	if _, err := svc.Authenticate(context.Background(), material.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected old token to be invalid, got %v", err)
	}

	if _, err := svc.Authenticate(context.Background(), rotated.Token); err != nil {
		t.Fatalf("expected rotated token to authenticate, got %v", err)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	clock := &stubClock{now: time.Unix(1700000000, 0)}
	secrets := &stubSecretGenerator{secrets: []string{"revoker"}}
	svc := mustService(t, Config{
		Clock:           clock,
		SecretGenerator: secrets.Next,
		IDGenerator:     func() string { return "key-revoke" },
	})

	material, err := svc.CreateAPIKey(context.Background(), "integration", "operator")
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	if err := svc.RevokeAPIKey(context.Background(), material.Key.ID); err != nil {
		t.Fatalf("RevokeAPIKey returned error: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), material.Token); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("expected ErrAPIKeyRevoked, got %v", err)
	}
	if err := svc.RevokeAPIKey(context.Background(), material.Key.ID); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("expected redundant revoke to return ErrAPIKeyRevoked, got %v", err)
	}
}

func mustService(t *testing.T, cfg Config) *Service {
	t.Helper()
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc
}
