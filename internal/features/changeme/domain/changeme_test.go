package domain_test

import (
	"testing"
	"time"

	"github.com/KucherenkoIvan/go-kernel/ddd"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

var now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNewChangeMe_InvariantAndEvent(t *testing.T) {
	if _, err := domain.NewChangeMe("c-1", "   ", now); err == nil || !ddd.IsDomainError(err) {
		t.Fatalf("blank name must be a domain error, got %v", err)
	}

	aggregate, err := domain.NewChangeMe("c-1", "  first  ", now)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Snapshot().Name != "first" {
		t.Errorf("name not normalized: %q", aggregate.Snapshot().Name)
	}
	if events := aggregate.PopEvents(); len(events) != 1 || events[0].EventName() != domain.ChangeMeCreatedEventName {
		t.Errorf("creation must emit the created event, got %v", events)
	}
}

func TestUpdate_InvariantAndEvent(t *testing.T) {
	aggregate, _ := domain.NewChangeMe("c-1", "first", now)
	aggregate.PopEvents()

	if err := aggregate.Update(""); err == nil {
		t.Fatal("blank name must be rejected on update too")
	}
	if events := aggregate.PopEvents(); len(events) != 0 {
		t.Fatalf("failed update must not emit events: %v", events)
	}

	if err := aggregate.Update("second"); err != nil {
		t.Fatal(err)
	}
	if events := aggregate.PopEvents(); len(events) != 1 || events[0].EventName() != domain.ChangeMeUpdatedEventName {
		t.Errorf("update must emit the updated event, got %v", events)
	}
}

func TestSnapshotRestore_RoundTrip(t *testing.T) {
	aggregate, _ := domain.NewChangeMe("c-1", "first", now)
	restored := domain.RestoreChangeMe(aggregate.Snapshot())
	if restored.Snapshot() != aggregate.Snapshot() {
		t.Fatalf("roundtrip mismatch: %+v != %+v", restored.Snapshot(), aggregate.Snapshot())
	}
	if events := restored.PopEvents(); len(events) != 0 {
		t.Fatalf("restore must not emit events: %v", events)
	}
}
