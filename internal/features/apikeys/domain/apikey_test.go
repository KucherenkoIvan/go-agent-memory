package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
)

func TestValidateSpaceName(t *testing.T) {
	valid := []string{"team-x", "a", "personal-ivan-2", "0-0"}
	for _, name := range valid {
		if err := domain.ValidateSpaceName(name); err != nil {
			t.Errorf("%q must be valid: %v", name, err)
		}
	}

	invalid := []string{
		"", "Team-X", "with space", "under_score", "dot.db",
		"../escape", "a/b", "..", strings.Repeat("x", 65),
	}
	for _, name := range invalid {
		if err := domain.ValidateSpaceName(name); err == nil {
			t.Errorf("%q must be rejected (space names become file names)", name)
		}
	}
}

func TestNewAPIKey_Validation(t *testing.T) {
	now := time.Now()

	if _, err := domain.NewAPIKey("id", "  ", "team", "h", "p", now); err == nil {
		t.Fatal("blank name must be rejected")
	}
	if _, err := domain.NewAPIKey("id", "laptop", "../etc", "h", "p", now); err == nil {
		t.Fatal("path-traversal space must be rejected")
	}

	key, err := domain.NewAPIKey("id", " laptop ", "team", "h", "p", now)
	if err != nil {
		t.Fatal(err)
	}
	if key.Name() != "laptop" || key.Revoked() {
		t.Fatalf("key state: %+v", key.Snapshot())
	}

	first := now.Add(time.Hour)
	key.Revoke(first)
	key.Revoke(now.Add(2 * time.Hour)) // idempotent — first timestamp sticks
	if !key.Revoked() || !key.Snapshot().RevokedAt.Equal(first) {
		t.Fatalf("revoke: %+v", key.Snapshot())
	}
}

func TestGenerateToken_Format(t *testing.T) {
	raw, hash, prefix, err := domain.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, domain.TokenPrefix) || len(raw) != len(domain.TokenPrefix)+64 {
		t.Fatalf("raw token format: %q", raw)
	}
	if hash != domain.HashToken(raw) || len(hash) != 64 {
		t.Fatalf("hash mismatch: %q", hash)
	}
	if !strings.HasPrefix(raw, prefix) || len(prefix) != 12 {
		t.Fatalf("prefix: %q", prefix)
	}
	if !domain.LooksLikeToken(raw) || domain.LooksLikeToken("nope") {
		t.Fatal("LooksLikeToken sanity check failed")
	}

	raw2, _, _, _ := domain.GenerateToken()
	if raw == raw2 {
		t.Fatal("tokens must be unique")
	}
}
