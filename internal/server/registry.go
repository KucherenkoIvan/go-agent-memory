// Package server composes hosted mode: the space registry, config, and the
// gRPC serving loop. Like cmd/, it is a composition root — it may import
// features and shared infra.
package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/KucherenkoIvan/go-kernel/events"

	apikeysdomain "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/domain"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// SpaceRegistry lazily opens one memory database per space and hands out
// the per-space Service. A space file is byte-identical to a local
// memory.db — isolation and migrate-to-local by construction.
type SpaceRegistry struct {
	dir    string
	mu     sync.Mutex
	spaces map[string]*spaceHandle
}

type spaceHandle struct {
	store *storage.Store
	pub   *events.ChannelPublisher
	svc   memories.Service
}

func NewSpaceRegistry(dir string) *SpaceRegistry {
	return &SpaceRegistry{dir: dir, spaces: map[string]*spaceHandle{}}
}

// ServiceFor implements grpcadapter.ServiceResolver. The mutex spans the
// lazy open: first-opens serialize, which is fine at this scale.
func (r *SpaceRegistry) ServiceFor(ctx context.Context, space string) (memories.Service, error) {
	// key creation validates too; re-check before touching the filesystem
	if err := apikeysdomain.ValidateSpaceName(space); err != nil {
		return nil, fmt.Errorf("space registry: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if handle, ok := r.spaces[space]; ok {
		return handle.svc, nil
	}

	store, err := storage.Open(ctx, filepath.Join(r.dir, space+".db"))
	if err != nil {
		return nil, fmt.Errorf("space registry: opening %s: %w", space, err)
	}
	pub := events.NewChannelPublisher()
	handle := &spaceHandle{store: store, pub: pub, svc: memories.New(store.DB, pub)}
	r.spaces[space] = handle
	return handle.svc, nil
}

// Close drains every space's publisher and closes its database.
func (r *SpaceRegistry) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for space, handle := range r.spaces {
		if err := handle.pub.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("space %s publisher: %w", space, err))
		}
		if err := handle.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("space %s store: %w", space, err))
		}
	}
	r.spaces = map[string]*spaceHandle{}
	return errors.Join(errs...)
}
