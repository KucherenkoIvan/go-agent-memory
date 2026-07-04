package domain

import "github.com/KucherenkoIvan/go-kernel/ddd"

type InvalidNameError struct{ ddd.DomainError }

func (e *InvalidNameError) Error() string { return "invalid_name" }

type ChangeMeNotFoundError struct{ ddd.DomainError }

func (e *ChangeMeNotFoundError) Error() string { return "changeme_not_found" }
