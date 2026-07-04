package changeme_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/KucherenkoIvan/go-kernel/events"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

// Domain events through the whole stack: HTTP request → command → commit →
// channel publisher → subscriber. Close drains synchronously, so no polling.
func TestEventFlow(t *testing.T) {
	feature, pub := setupFeatureWithPub(t)
	api := newRESTHelper(t, feature)

	var mu sync.Mutex
	var seen []string
	pub.Subscribe(events.Handler{
		Name: "test-recorder",
		Handle: func(_ context.Context, e ddd.DomainEvent) error {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, e.EventName())
			return nil
		},
	})

	w := api("POST", "/api/changeme", `{"name": "first"}`)
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	api("PUT", "/api/changeme/"+created.ID, `{"name": "second"}`)
	api("DELETE", "/api/changeme/"+created.ID, "")

	if err := pub.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{domain.ChangeMeCreatedEventName, domain.ChangeMeUpdatedEventName, domain.ChangeMeDeletedEventName}
	if len(seen) != len(want) {
		t.Fatalf("events: got %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("events: got %v, want %v", seen, want)
		}
	}
}

// Failed transactions publish nothing (commit-gating end to end).
func TestNoGhostEvents(t *testing.T) {
	feature, pub := setupFeatureWithPub(t)
	api := newRESTHelper(t, feature)

	var mu sync.Mutex
	count := 0
	pub.Subscribe(events.Handler{
		Name: "counter",
		Handle: func(context.Context, ddd.DomainEvent) error {
			mu.Lock()
			defer mu.Unlock()
			count++
			return nil
		},
	})

	if w := api("POST", "/api/changeme", `{"name": "keeper"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d", w.Code)
	}
	// the update command fails inside its transaction — its events must vanish
	if w := api("PUT", "/api/changeme/missing", `{"name": "x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("update missing: %d", w.Code)
	}

	_ = pub.Close(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("delivered %d events, want 1 (failed tx must publish nothing)", count)
	}
}
