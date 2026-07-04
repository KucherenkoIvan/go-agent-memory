package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

func newTestApp(fake *fakeService) *App {
	connect := func(context.Context) (memories.Service, func(), error) {
		return fake, func() {}, nil
	}
	app := newApp(context.Background(), fake, func() {}, connect, "test")
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return app
}

// drain executes a cmd tree synchronously, feeding messages back until quiet.
func drain(t *testing.T, app *App, cmd tea.Cmd) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range batch {
				drain(t, app, sub)
			}
			return
		}
		_, cmd = app.Update(msg)
	}
}

func press(t *testing.T, app *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "ctrl+s":
			msg = tea.KeyMsg{Type: tea.KeyCtrlS}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := app.Update(msg)
		drain(t, app, cmd)
	}
}

func seed(app *App, t *testing.T, fake *fakeService) {
	t.Helper()
	drain(t, app, app.issueSearch())
	if len(app.list.results.Items()) != len(fake.results) {
		t.Fatalf("seed: %d items", len(app.list.results.Items()))
	}
}

func TestTypingInSearch_DoesNotTriggerGlobalKeys(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "/")
	if !app.list.search.Focused() {
		t.Fatal("/ must focus search")
	}
	press(t, app, "d") // would open delete-confirm if not typing
	if app.mode != modeList || app.confirm != nil {
		t.Fatal("typing 'd' in search must not open the delete modal")
	}
	if len(fake.deletes) != 0 {
		t.Fatal("no delete may fire while typing")
	}
	if !strings.Contains(app.list.search.Value(), "d") {
		t.Fatalf("search box must receive the rune: %q", app.list.search.Value())
	}
}

func TestSearch_DebounceDropsStaleSeq(t *testing.T) {
	fake := &fakeService{}
	app := newTestApp(fake)
	baseline := len(fake.searches)

	press(t, app, "/")
	// type two characters without draining: two debounce cmds, one stale.
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	staleSeq := app.list.seq
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	freshSeq := app.list.seq

	if _, cmd := app.Update(debounceMsg{seq: staleSeq}); cmd != nil {
		t.Fatal("stale debounce must not search")
	}
	_, cmd := app.Update(debounceMsg{seq: freshSeq})
	if cmd == nil {
		t.Fatal("fresh debounce must search")
	}
	drain(t, app, cmd)
	if len(fake.searches) != baseline+1 {
		t.Fatalf("exactly one search after debounce, got %d", len(fake.searches)-baseline)
	}
	if got := fake.searches[len(fake.searches)-1].Query; got != "ab" {
		t.Fatalf("searched %q, want ab", got)
	}
}

func TestDelete_RequiresConfirmation(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "the memory")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "d")
	if app.mode != modeConfirm {
		t.Fatal("d must open the confirm modal")
	}
	press(t, app, "n")
	if len(fake.deletes) != 0 || app.mode != modeList {
		t.Fatalf("non-y must cancel: deletes=%v mode=%v", fake.deletes, app.mode)
	}

	press(t, app, "d", "y")
	if len(fake.deletes) != 1 || fake.deletes[0] != "m1" {
		t.Fatalf("y must delete: %v", fake.deletes)
	}
	if len(app.list.results.Items()) != 0 {
		t.Fatal("deleted item must leave the list")
	}
}

func TestRate_OptimisticBump(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "s")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "+")
	if len(fake.rates) != 1 || !fake.rates[0].Up || fake.rates[0].ID != "m1" {
		t.Fatalf("rates: %+v", fake.rates)
	}
	item := app.list.results.Items()[0].(resultItem)
	if item.r.VotesUp != 1 {
		t.Fatalf("optimistic bump missing: %+v", item.r)
	}
}

func TestSupersedeForm_SubmitsCorrection(t *testing.T) {
	fake := &fakeService{
		results: []domain.SearchResult{someResult("m1", "old summary")},
		memory: &domain.MemoryReadModel{
			ID: "m1", Content: "old content", Summary: "old summary",
			Kind: "fact", Keywords: []string{"topic"},
		},
		storeID: "m2",
	}
	fake.results[0].ID = "m1"
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "e")
	if app.mode != modeForm || app.form == nil {
		t.Fatalf("e must open the form, mode=%v", app.mode)
	}
	if app.form.summary.Value() != "old summary" || app.form.content.Value() != "old content" {
		t.Fatal("form must be pre-filled from the old memory")
	}

	// summary focused: type an amendment, then submit
	press(t, app, "!")
	press(t, app, "ctrl+s")

	if len(fake.stores) != 1 {
		t.Fatalf("stores: %+v", fake.stores)
	}
	in := fake.stores[0]
	if in.Supersedes != "m1" || in.Source != "tui" || in.Summary != "old summary!" {
		t.Fatalf("store input: %+v", in)
	}
	if app.mode != modeList && app.mode != modeDetail {
		t.Fatalf("after store: mode=%v", app.mode)
	}
}

func TestSupersedeForm_DomainErrorLandsOnField(t *testing.T) {
	fake := &fakeService{
		results:  []domain.SearchResult{someResult("m1", "s")},
		memory:   &domain.MemoryReadModel{ID: "m1", Content: "c", Summary: "s", Kind: "fact", Keywords: []string{"k"}},
		storeErr: &domain.InvalidSummaryError{},
	}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "e", "ctrl+s")
	if app.mode != modeForm {
		t.Fatal("failed store must stay on the form")
	}
	if app.form.errField != fieldSummary || app.form.fieldErr == "" {
		t.Fatalf("error must land on summary: field=%d msg=%q", app.form.errField, app.form.fieldErr)
	}
}

func TestReviewPreset_FiltersNetNegative(t *testing.T) {
	bad := someResult("bad", "downvoted")
	bad.VotesDown = 3
	good := someResult("good", "upvoted")
	good.VotesUp = 2
	fake := &fakeService{results: []domain.SearchResult{good, bad}}
	app := newTestApp(fake)

	press(t, app, "R")
	items := app.list.results.Items()
	if len(items) != 1 || items[0].(resultItem).r.ID != "bad" {
		t.Fatalf("review preset: %+v", items)
	}
	if got := fake.searches[len(fake.searches)-1].Limit; got != reviewFetchLimit {
		t.Fatalf("review must fetch wide: limit=%d", got)
	}
}

func TestReconnect_SwapsServiceOrParks(t *testing.T) {
	fake := &fakeService{}
	app := newTestApp(fake)

	second := &fakeService{}
	app.Update(reconnectedMsg{svc: second, cleanup: func() {}})
	if app.svc != memories.Service(second) || app.mode != modeList {
		t.Fatal("reconnect must swap the service and return to the list")
	}

	app.Update(reconnectFailedMsg{err: context.DeadlineExceeded})
	if app.svc != nil || app.mode != modeRemote || !app.remote.broken {
		t.Fatal("failed reconnect must park on the remote screen with no service")
	}
	press(t, app, "esc")
	if app.mode != modeRemote {
		t.Fatal("esc must not leave a broken remote screen")
	}
}
