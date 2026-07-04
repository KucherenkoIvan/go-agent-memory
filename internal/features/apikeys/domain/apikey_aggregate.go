// Package domain holds the APIKey aggregate: a hashed credential that both
// authenticates a caller and selects the memory space it may touch.
package domain

import (
	"regexp"
	"strings"
	"time"
)

type APIKeyID string

const maxSpaceNameLength = 64

// spaceNameRe is the tenancy safety line: space names become file names
// (spaces/<name>.db), so the alphabet excludes any path syntax.
var spaceNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateSpaceName gates every space name at key creation; the server
// re-checks before touching the filesystem (defense in depth).
func ValidateSpaceName(name string) error {
	if len(name) > maxSpaceNameLength || !spaceNameRe.MatchString(name) {
		return &InvalidSpaceNameError{}
	}
	return nil
}

type apiKeyState struct {
	id        APIKeyID
	name      string
	space     string
	tokenHash string
	prefix    string
	createdAt time.Time
	revokedAt time.Time // zero = active
}

// APIKey is the aggregate. The raw token never enters it — only the hash.
type APIKey struct {
	state apiKeyState
}

func NewAPIKey(id APIKeyID, name, space, tokenHash, prefix string, createdAt time.Time) (*APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &EmptyKeyNameError{}
	}
	if err := ValidateSpaceName(space); err != nil {
		return nil, err
	}
	return &APIKey{state: apiKeyState{
		id: id, name: name, space: space,
		tokenHash: tokenHash, prefix: prefix, createdAt: createdAt,
	}}, nil
}

func (k *APIKey) ID() APIKeyID { return k.state.id }

func (k *APIKey) Space() string { return k.state.space }

func (k *APIKey) Name() string { return k.state.name }

func (k *APIKey) Revoked() bool { return !k.state.revokedAt.IsZero() }

// Revoke is idempotent: the first revocation timestamp sticks.
func (k *APIKey) Revoke(now time.Time) {
	if k.state.revokedAt.IsZero() {
		k.state.revokedAt = now
	}
}

// APIKeySnapshot maps the aggregate to persistence and back.
type APIKeySnapshot struct {
	ID        APIKeyID
	Name      string
	Space     string
	TokenHash string
	Prefix    string
	CreatedAt time.Time
	RevokedAt time.Time
}

func (k *APIKey) Snapshot() APIKeySnapshot {
	return APIKeySnapshot{
		ID: k.state.id, Name: k.state.name, Space: k.state.space,
		TokenHash: k.state.tokenHash, Prefix: k.state.prefix,
		CreatedAt: k.state.createdAt, RevokedAt: k.state.revokedAt,
	}
}

func RestoreAPIKey(snap APIKeySnapshot) *APIKey {
	return &APIKey{state: apiKeyState{
		id: snap.ID, name: snap.Name, space: snap.Space,
		tokenHash: snap.TokenHash, prefix: snap.Prefix,
		createdAt: snap.CreatedAt, revokedAt: snap.RevokedAt,
	}}
}
