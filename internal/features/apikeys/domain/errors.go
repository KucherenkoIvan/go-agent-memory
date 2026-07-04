package domain

import "github.com/KucherenkoIvan/go-kernel/ddd"

// InvalidSpaceNameError — space names must match [a-z0-9-]+ (they become
// file names).
type InvalidSpaceNameError struct{ ddd.DomainError }

func (e *InvalidSpaceNameError) Error() string { return "invalid_space_name" }

// EmptyKeyNameError — every key needs a human label; it becomes `source`.
type EmptyKeyNameError struct{ ddd.DomainError }

func (e *EmptyKeyNameError) Error() string { return "empty_key_name" }

// KeyNotFoundError — no key with that id.
type KeyNotFoundError struct{ ddd.DomainError }

func (e *KeyNotFoundError) Error() string { return "key_not_found" }

// InvalidAPIKeyError — unknown or revoked token; callers never learn which.
type InvalidAPIKeyError struct{ ddd.DomainError }

func (e *InvalidAPIKeyError) Error() string { return "invalid_api_key" }
