package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestEsc_ClosesTopmostThing_NeverQuits(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	// typing: esc blurs the box but keeps the filter text
	press(t, app, "/", "a", "b", "esc")
	if app.list.search.Focused() || app.list.search.Value() != "ab" {
		t.Fatalf("esc must blur and keep text: focused=%v value=%q",
			app.list.search.Focused(), app.list.search.Value())
	}

	// expanded help collapses before anything else
	press(t, app, "?")
	if !app.help.ShowAll {
		t.Fatal("? must expand help")
	}
	press(t, app, "esc")
	if app.help.ShowAll {
		t.Fatal("esc must collapse help first")
	}
	if app.list.search.Value() != "ab" {
		t.Fatal("collapsing help must not clear the search")
	}

	// next esc clears the filter and refreshes
	baseline := len(fake.searches)
	press(t, app, "esc")
	if app.list.search.Value() != "" {
		t.Fatalf("esc must clear the search: %q", app.list.search.Value())
	}
	if len(fake.searches) == baseline {
		t.Fatal("clearing the search must refresh the list")
	}

	// bare list: esc is a no-op, never a quit (the embedded list's default
	// keymap used to bind esc to Quit)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, quits := cmd().(tea.QuitMsg); quits {
			t.Fatal("esc must not quit the TUI")
		}
	}
	if app.mode != modeList {
		t.Fatalf("esc left list mode: %v", app.mode)
	}
}

func sortableResult(id string, created time.Time, votesUp, votesDown, access int) domain.SearchResult {
	return domain.SearchResult{
		ID: id, Summary: id, Kind: "fact", Keywords: []string{"k"},
		Source: "test", CreatedAt: created, VotesUp: votesUp, VotesDown: votesDown,
		AccessCount: access,
	}
}

func TestSort_DefaultEarliestFirst_CycleReorders(t *testing.T) {
	now := time.Now()
	fake := &fakeService{results: []domain.SearchResult{
		sortableResult("newest", now, 0, 0, 1),
		sortableResult("oldest", now.Add(-48*time.Hour), 1, 0, 5),
		sortableResult("mid", now.Add(-24*time.Hour), 5, 0, 2),
	}}
	app := newTestApp(fake)
	drain(t, app, app.issueSearch())

	first := func() string {
		return app.list.results.Items()[0].(resultItem).r.ID
	}
	if first() != "oldest" {
		t.Fatalf("default sort must put earliest on top, got %s", first())
	}

	press(t, app, "s") // created↑ → created↓
	if app.list.sort != sortCreatedDesc || first() != "newest" {
		t.Fatalf("created↓: sort=%v first=%s", app.list.sort, first())
	}
	press(t, app, "s") // → rating↓
	if app.list.sort != sortRatingDesc || first() != "mid" {
		t.Fatalf("rating↓: sort=%v first=%s", app.list.sort, first())
	}
	press(t, app, "s") // → rating↑
	if first() != "newest" {
		t.Fatalf("rating↑ must put unrated first, got %s", first())
	}
	press(t, app, "s") // → reads↓
	if first() != "oldest" {
		t.Fatalf("reads↓ must put most-read first, got %s", first())
	}
}

func TestRecentFilter_SetsSinceCutoff(t *testing.T) {
	fake := &fakeService{}
	app := newTestApp(fake)
	press(t, app, "t")
	last := fake.searches[len(fake.searches)-1]
	if last.Since.IsZero() || time.Since(last.Since) > 73*time.Hour {
		t.Fatalf("recent filter must set a ~3d Since cutoff: %v", last.Since)
	}
	press(t, app, "t")
	last = fake.searches[len(fake.searches)-1]
	if !last.Since.IsZero() {
		t.Fatal("toggling recent off must drop the cutoff")
	}
}

func TestCursorResets_OnQueryChange_NotOnRefresh(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{
		someResult("m1", "one"), someResult("m2", "two"), someResult("m3", "three"),
	}}
	app := newTestApp(fake)
	seed(app, t, fake)

	app.list.results.Select(2)
	press(t, app, "r") // refresh keeps the cursor
	if app.list.results.Index() != 2 {
		t.Fatalf("refresh moved the cursor to %d", app.list.results.Index())
	}
	press(t, app, "f") // kind filter changes the query → cursor to top
	if app.list.results.Index() != 0 {
		t.Fatalf("query change must reset the cursor, got %d", app.list.results.Index())
	}
}

func TestTabInSearch_ReturnsFocusToList(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "/", "x", "tab")
	if app.list.search.Focused() {
		t.Fatal("tab must blur the search box")
	}
	if app.list.search.Value() != "x" {
		t.Fatal("tab must keep the query text")
	}
}

func TestHalfPageMotions(t *testing.T) {
	results := make([]domain.SearchResult, 12)
	for i := range results {
		results[i] = someResult(fmt.Sprintf("m%d", i), "row")
	}
	fake := &fakeService{results: results}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "ctrl+d")
	down := app.list.results.Index()
	if down == 0 {
		t.Fatal("ctrl+d must move the cursor down")
	}
	press(t, app, "ctrl+u")
	if app.list.results.Index() != 0 {
		t.Fatalf("ctrl+u must move back up, got %d", app.list.results.Index())
	}
}

func TestTimeline_PaginatesToTheEnd(t *testing.T) {
	// 70 rows, page size 50: reaching the end must fetch the remaining 20
	// by offset and then mark the query exhausted
	now := time.Now()
	results := make([]domain.SearchResult, 70)
	for i := range results {
		results[i] = sortableResult(fmt.Sprintf("m%02d", i), now.Add(time.Duration(i)*time.Minute), 0, 0, 0)
	}
	fake := &fakeService{results: results}
	app := newTestApp(fake)
	drain(t, app, app.issueSearch())

	if n := len(app.list.results.Items()); n != layerFetchLimit {
		t.Fatalf("first page must be %d rows, got %d", layerFetchLimit, n)
	}
	if app.list.exhausted {
		t.Fatal("a full first page must not be exhausted")
	}

	if app.list.total != 70 || !strings.Contains(app.list.pageInfo(), "70 memories") {
		t.Fatalf("numeric pagination must show the exact total: total=%d info=%q",
			app.list.total, app.list.pageInfo())
	}

	press(t, app, "G") // jump to end → triggers load-more
	if n := len(app.list.results.Items()); n != 70 {
		t.Fatalf("end of page must append the rest: %d rows", n)
	}
	last := fake.searches[len(fake.searches)-1]
	if last.Offset != layerFetchLimit || last.Order != "created_asc" {
		t.Fatalf("second page must fetch by offset with server order: %+v", last)
	}

	press(t, app, "G")
	baseline := len(fake.searches)
	press(t, app, "G")
	if len(fake.searches) != baseline {
		t.Fatal("exhausted timeline must stop fetching")
	}
}

func TestHelpFooter_FollowsMode(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	fake.memory = &domain.MemoryReadModel{
		ID: "m1", Summary: "one", Kind: "fact", Content: "alpha beta gamma",
		Keywords: []string{"k"}, CreatedAt: time.Now(),
	}
	app := newTestApp(fake)
	seed(app, t, fake)

	listShort := app.modeHelp().ShortHelp()
	press(t, app, "enter")
	if app.mode != modeDetail {
		t.Fatalf("enter must open detail, mode=%v", app.mode)
	}
	detailShort := app.modeHelp().ShortHelp()
	if len(detailShort) == 0 || len(listShort) == 0 ||
		detailShort[0].Help().Key == listShort[0].Help().Key {
		t.Fatal("detail footer must differ from list footer")
	}
}

func TestDetailFind_EscLadder(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	fake.memory = &domain.MemoryReadModel{
		ID: "m1", Summary: "one", Kind: "fact", Content: "alpha beta gamma",
		Keywords: []string{"k"}, CreatedAt: time.Now(),
	}
	app := newTestApp(fake)
	seed(app, t, fake)
	press(t, app, "enter")

	press(t, app, "/")
	if !app.detail.find.Focused() {
		t.Fatal("/ must focus the find bar")
	}
	press(t, app, "b", "e") // type into find, not the app keys
	if app.detail.find.Value() != "be" || len(fake.deletes) != 0 || app.mode != modeDetail {
		t.Fatalf("typing must stay in the find bar: %q", app.detail.find.Value())
	}
	press(t, app, "esc")
	if app.detail.find.Focused() || app.detail.find.Value() != "be" {
		t.Fatal("first esc blurs, keeps terms")
	}
	press(t, app, "esc")
	if app.detail.find.Value() != "" || app.mode != modeDetail {
		t.Fatal("second esc clears the find, stays in detail")
	}
	press(t, app, "esc")
	if app.mode != modeList {
		t.Fatal("third esc closes the view")
	}
}

func TestHighlightFuzzy_MatchesAndClears(t *testing.T) {
	if m := matchedBytes("alpha beta", []string{"beta"}); len(m) != 4 {
		t.Fatalf("beta must match 4 bytes, got %v", m)
	}
	if m := matchedBytes("alpha beta", nil); len(m) != 0 {
		t.Fatal("no terms must match nothing")
	}
	// typo-grade fuzz passes the cutoff: "keywrds" holds a 4+ run of 7
	if m := matchedBytes("memory_keywords mirror", []string{"keywrds"}); len(m) == 0 {
		t.Fatal("coherent fuzzy match must highlight")
	}
	// scattered subsequence fails it: "mkm" never runs 2+ chars together
	if m := matchedBytes("memory_keywords mirror", []string{"mkm"}); len(m) != 0 {
		t.Fatalf("scattered match must not highlight: %v", m)
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
	// one debounce = one layered query: keywords, text, wide fuzzy pool
	if len(fake.searches) != baseline+3 {
		t.Fatalf("layered search must issue 3 fetches, got %d", len(fake.searches)-baseline)
	}
	issued := fake.searches[baseline:]
	if len(issued[0].KeywordsAny) != 1 || issued[0].KeywordsAny[0] != "ab" {
		t.Fatalf("keyword layer: %+v", issued[0])
	}
	if issued[1].Query != "ab" {
		t.Fatalf("text layer: %+v", issued[1])
	}
	if issued[2].Query != "" || issued[2].Limit != widePoolLimit {
		t.Fatalf("fuzzy pool layer: %+v", issued[2])
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
	if app.form.state != formNav || app.form.summary.Focused() {
		t.Fatal("form must open unfocused, in navigation")
	}

	// nothing types until a field is entered
	press(t, app, "!")
	if app.form.summary.Value() != "old summary" {
		t.Fatal("runes in navigation must not edit fields")
	}

	// enter edits the highlighted field; ctrl+s asks, y saves
	press(t, app, "enter", "!")
	press(t, app, "ctrl+s")
	if len(fake.stores) != 0 || app.form.confirmAct != confirmSave {
		t.Fatal("ctrl+s must ask for confirmation before storing")
	}
	press(t, app, "y")

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

	press(t, app, "e", "ctrl+s", "y")
	if app.mode != modeForm {
		t.Fatal("failed store must stay on the form")
	}
	if app.form.errField != fieldSummary || app.form.fieldErr == "" {
		t.Fatalf("error must land on summary: field=%d msg=%q", app.form.errField, app.form.fieldErr)
	}
	if app.form.state != formEdit || !app.form.summary.Focused() {
		t.Fatal("error must drop the user into editing the offending field")
	}
}

func TestForm_DiscardConfirmsOnlyWhenDirty(t *testing.T) {
	fake := &fakeService{
		results: []domain.SearchResult{someResult("m1", "s")},
		memory:  &domain.MemoryReadModel{ID: "m1", Content: "c", Summary: "s", Kind: "fact", Keywords: []string{"k"}},
	}
	app := newTestApp(fake)
	seed(app, t, fake)

	// clean form: esc closes straight away
	press(t, app, "e", "esc")
	if app.mode != modeList {
		t.Fatalf("clean discard must close, mode=%v", app.mode)
	}

	// dirty form: esc warns; y discards; q must not quit the app
	press(t, app, "e", "enter", "x", "esc") // edit summary, back to nav
	press(t, app, "esc")
	if app.form == nil || app.form.confirmAct != confirmDiscard {
		t.Fatal("dirty discard must ask first")
	}
	press(t, app, "n") // anything but y keeps editing
	if app.form == nil || app.form.confirmAct != confirmNone {
		t.Fatal("non-y must cancel the discard")
	}
	press(t, app, "q")
	if app.form == nil || app.form.confirmAct != confirmDiscard {
		t.Fatal("q in the form must route to discard, not quit")
	}
	press(t, app, "y")
	if app.mode != modeList {
		t.Fatalf("confirmed discard must close, mode=%v", app.mode)
	}
}

func TestForm_FindHighlightsFields(t *testing.T) {
	fake := &fakeService{
		results: []domain.SearchResult{someResult("m1", "s")},
		memory:  &domain.MemoryReadModel{ID: "m1", Content: "alpha beta", Summary: "s", Kind: "fact", Keywords: []string{"k"}},
	}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "e", "/")
	if app.form.state != formFind || !app.form.find.Focused() {
		t.Fatal("/ must open the find bar")
	}
	press(t, app, "b", "e") // types into find, not a field or global key
	if app.form.find.Value() != "be" || app.form.summary.Value() != "s" {
		t.Fatalf("find must capture typing: find=%q", app.form.find.Value())
	}
	press(t, app, "esc")
	if app.form.state != formNav || app.form.find.Value() != "be" {
		t.Fatal("esc leaves find, terms stay lit")
	}
	press(t, app, "esc")
	if app.form.find.Value() != "" || app.mode != modeForm {
		t.Fatal("next esc clears the find terms, stays on the form")
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
	if got := fake.searches[len(fake.searches)-1].Limit; got != widePoolLimit {
		t.Fatalf("review must fetch wide: limit=%d", got)
	}
}

func TestRemote_ModalFlowMatchesForm(t *testing.T) {
	fake := &fakeService{results: []domain.SearchResult{someResult("m1", "one")}}
	app := newTestApp(fake)
	seed(app, t, fake)

	press(t, app, "c")
	if app.mode != modeRemote || app.remote.state != formNav ||
		app.remote.addr.Focused() || app.remote.key.Focused() {
		t.Fatal("remote must open unfocused, in navigation")
	}

	// runes navigate, they don't type; enter starts editing
	press(t, app, "x")
	if app.remote.addr.Value() != "" {
		t.Fatal("runes in navigation must not edit fields")
	}
	press(t, app, "enter", "x")
	if !app.remote.addr.Focused() || app.remote.addr.Value() != "x" {
		t.Fatalf("enter must focus the field for typing: %q", app.remote.addr.Value())
	}
	press(t, app, "esc")
	if app.remote.state != formNav || app.remote.addr.Focused() {
		t.Fatal("esc must stop editing, back to navigation")
	}

	// q routes to the dirty-discard flow instead of quitting
	press(t, app, "q")
	if app.remote == nil || app.remote.confirmAct != confirmDiscard {
		t.Fatal("q on a dirty remote must ask to discard")
	}
	press(t, app, "y")
	if app.mode != modeList {
		t.Fatalf("confirmed discard must close, mode=%v", app.mode)
	}
}

func TestReconnect_ProbesBeforeDeclaringConnected(t *testing.T) {
	// grpc connects lazily — a service that cannot serve must fail the
	// reconnect and release its handle, not report "connected"
	bad := &fakeService{searchErr: context.DeadlineExceeded}
	released := false
	msg := reconnectCmd(context.Background(), nil,
		func(context.Context) (memories.Service, func(), error) {
			return bad, func() { released = true }, nil
		})()
	if _, failed := msg.(reconnectFailedMsg); !failed {
		t.Fatalf("unservable connection must fail the probe: %T", msg)
	}
	if !released {
		t.Fatal("failed probe must close the new handle")
	}

	good := &fakeService{}
	msg = reconnectCmd(context.Background(), nil,
		func(context.Context) (memories.Service, func(), error) {
			return good, func() {}, nil
		})()
	if _, ok := msg.(reconnectedMsg); !ok {
		t.Fatalf("servable connection must succeed: %T", msg)
	}
	if len(good.searches) != 1 {
		t.Fatal("success must be based on a served probe request")
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
