package domain

import "github.com/KucherenkoIvan/go-kernel/ddd"

type EmptyContentError struct{ ddd.DomainError }

func (e *EmptyContentError) Error() string { return "empty_content" }

type InvalidSummaryError struct{ ddd.DomainError }

func (e *InvalidSummaryError) Error() string { return "invalid_summary" }

type InvalidKindError struct{ ddd.DomainError }

func (e *InvalidKindError) Error() string { return "invalid_kind" }

type NoKeywordsError struct{ ddd.DomainError }

func (e *NoKeywordsError) Error() string { return "no_keywords" }

type EmptySourceError struct{ ddd.DomainError }

func (e *EmptySourceError) Error() string { return "empty_source" }

type InvalidTTLError struct{ ddd.DomainError }

func (e *InvalidTTLError) Error() string { return "invalid_ttl" }

type MemoryNotFoundError struct{ ddd.DomainError }

func (e *MemoryNotFoundError) Error() string { return "memory_not_found" }

type MemorySupersededError struct{ ddd.DomainError }

func (e *MemorySupersededError) Error() string { return "memory_superseded" }

type InvalidSupersedeError struct{ ddd.DomainError }

func (e *InvalidSupersedeError) Error() string { return "invalid_supersede" }
