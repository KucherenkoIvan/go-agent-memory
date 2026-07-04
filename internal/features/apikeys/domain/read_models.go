package domain

import "time"

// APIKeyView is what listings show — never the hash, never the raw token.
type APIKeyView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Space     string     `json:"space"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

// SpaceView summarizes a space: its name and how many active keys unlock it.
type SpaceView struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	Keys      int       `json:"keys"`
}
