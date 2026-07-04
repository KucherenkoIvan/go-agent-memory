// Package domain — placeholder business logic. Everything named ChangeMe is
// scaffolding: rename the aggregate, replace the placeholder field and
// invariant with real ones, and delete these comments.
package domain

import (
	"strings"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"
)

type ChangeMeID string

type changeMeState struct {
	id        ChangeMeID
	name      string // placeholder field — replace with real state
	createdAt time.Time
}

// ChangeMe is the placeholder aggregate: state is unexported, mutations go
// through methods, methods hold the invariants and emit events.
type ChangeMe struct {
	ddd.EventRecorder[ddd.DomainEvent]
	state changeMeState
}

// NewChangeMe validates and creates the aggregate. Time comes in as an
// argument — domain code never calls time.Now().
func NewChangeMe(id ChangeMeID, name string, createdAt time.Time) (*ChangeMe, error) {
	name = strings.TrimSpace(name)
	// placeholder invariant — replace with real rules
	if name == "" {
		return nil, &InvalidNameError{}
	}

	m := &ChangeMe{state: changeMeState{id: id, name: name, createdAt: createdAt}}
	m.PushEvent(NewChangeMeCreatedEvent(ChangeMeCreatedData{ID: id}))
	return m, nil
}

func (m *ChangeMe) ID() ChangeMeID { return m.state.id }

// Update is the placeholder mutation — same shape as any real one:
// validate, mutate, emit.
func (m *ChangeMe) Update(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &InvalidNameError{}
	}
	m.state.name = name
	m.PushEvent(NewChangeMeUpdatedEvent(ChangeMeUpdatedData{ID: m.state.id}))
	return nil
}

// ChangeMeSnapshot is the persistence view — explicit mapping, no reflection.
type ChangeMeSnapshot struct {
	ID        ChangeMeID
	Name      string
	CreatedAt time.Time
}

func (m *ChangeMe) Snapshot() ChangeMeSnapshot {
	return ChangeMeSnapshot{ID: m.state.id, Name: m.state.name, CreatedAt: m.state.createdAt}
}

// RestoreChangeMe re-creates the aggregate from DB data — no events, no
// validation; the data passed both when written.
func RestoreChangeMe(s ChangeMeSnapshot) *ChangeMe {
	return &ChangeMe{state: changeMeState{id: s.ID, name: s.Name, createdAt: s.CreatedAt}}
}
