package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/KucherenkoIvan/go-kernel/events"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/shared/infra/storage"
)

// The Elm loop is pure functions over messages — testable without a
// terminal: run the tea.Cmds by hand and feed their messages back.
func setup(t *testing.T) Model {
	t.Helper()
	db, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	pub := events.NewChannelPublisher()
	t.Cleanup(func() { _ = pub.Close(context.Background()) })

	return New(changeme.New(db, pub).UseCases)
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestCreateFlow(t *testing.T) {
	m := setup(t)

	// initial load: empty list
	m, _ = step(t, m, m.load())
	if !strings.Contains(m.View(), "nothing here yet") {
		t.Fatalf("empty view: %q", m.View())
	}

	// n → create mode, type, enter
	m, _ = step(t, m, key("n"))
	if m.mode != modeCreate {
		t.Fatalf("mode = %v", m.mode)
	}
	m.input.SetValue("first")
	m, cmd := step(t, m, key("enter"))
	m, cmd = step(t, m, cmd()) // actedMsg → back to list + reload
	m, _ = step(t, m, cmd())   // itemsMsg

	if len(m.items) != 1 || m.items[0].Name != "first" {
		t.Fatalf("items: %+v", m.items)
	}
	if !strings.Contains(m.View(), "first") {
		t.Fatalf("view: %q", m.View())
	}
}

func TestDomainErrorInStatusBar(t *testing.T) {
	m := setup(t)
	m, _ = step(t, m, m.load())

	m, _ = step(t, m, key("n"))
	m.input.SetValue("   ")
	m, cmd := step(t, m, key("enter"))
	m, _ = step(t, m, cmd()) // errMsg

	if m.mode != modeCreate {
		t.Fatal("a failed submit must keep the input open")
	}
	if !strings.Contains(m.View(), "invalid_name") {
		t.Fatalf("status missing: %q", m.View())
	}
}

func TestEditDeleteAndRefresh(t *testing.T) {
	m := setup(t)
	m, _ = step(t, m, m.load())

	// seed via the create flow
	m, _ = step(t, m, key("n"))
	m.input.SetValue("first")
	m, cmd := step(t, m, key("enter"))
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())

	// edit
	m, _ = step(t, m, key("e"))
	if m.mode != modeEdit || m.input.Value() != "first" {
		t.Fatalf("edit mode: %v %q", m.mode, m.input.Value())
	}
	m.input.SetValue("second")
	m, cmd = step(t, m, key("enter"))
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	if m.items[0].Name != "second" {
		t.Fatalf("after edit: %+v", m.items)
	}

	// RefreshMsg (the event-subscription path) reloads
	m, cmd = step(t, m, RefreshMsg{})
	m, _ = step(t, m, cmd())
	if len(m.items) != 1 {
		t.Fatalf("after refresh: %+v", m.items)
	}

	// delete
	m, cmd = step(t, m, key("d"))
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	if len(m.items) != 0 {
		t.Fatalf("after delete: %+v", m.items)
	}
}
