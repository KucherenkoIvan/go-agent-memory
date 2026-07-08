package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

type resultItem struct{ r domain.SearchResult }

func (resultItem) FilterValue() string { return "" } // search is server-side

// listModel is home: one search box (terms hit keywords, text, and fuzzy at
// once) over the ranked results.
type listModel struct {
	st      *styles
	search  textinput.Model
	results list.Model
	// seq marks the latest issued query; older responses are dropped
	seq         int
	kind        string // "" = all kinds; cycled with f
	review      bool
	includeDead bool
	recent      bool     // 3-day cutoff filter
	sort        sortMode // display order; tie-breaker inside layers when searching
	// pendingReset snaps the cursor to the top when the NEXT results land —
	// set when the query changes (typing, toggles, clear), not on refresh
	pendingReset bool
	// pagination: exhausted means the store has no more rows for this
	// query; pendingMore guards against double fetches; growLimit is the
	// current per-layer depth of a layered search
	exhausted   bool
	pendingMore bool
	growLimit   int
	// total is the store's exact match count for the current query
	// (timeline); 0 = unknown (layered search until exhausted)
	total int
	// remoteAddr, when set, badges the session as connected to a server
	remoteAddr string
	// terms is shared with the row delegate: the current search terms drive
	// match highlighting, and emptying them clears it
	terms *[]string
}

// sortMode orders the list. With search terms present, relevance (layer
// order) stays primary and the mode sorts within each layer.
type sortMode int

const (
	sortCreatedAsc  sortMode = iota // earliest on top — the default
	sortCreatedDesc                 // latest on top
	sortRatingDesc
	sortRatingAsc
	sortAccessDesc
	sortAccessAsc
	sortModes // count, for cycling
)

// order maps the mode to the reader's server-side display order — stable
// ordering is what makes offset pagination possible.
func (s sortMode) order() string {
	switch s {
	case sortCreatedDesc:
		return ports.OrderCreatedDesc
	case sortRatingDesc:
		return ports.OrderRatingDesc
	case sortRatingAsc:
		return ports.OrderRatingAsc
	case sortAccessDesc:
		return ports.OrderReadsDesc
	case sortAccessAsc:
		return ports.OrderReadsAsc
	default:
		return ports.OrderCreatedAsc
	}
}

func (s sortMode) label() string {
	switch s {
	case sortCreatedDesc:
		return "created↓"
	case sortRatingDesc:
		return "rating↓"
	case sortRatingAsc:
		return "rating↑"
	case sortAccessDesc:
		return "reads↓"
	case sortAccessAsc:
		return "reads↑"
	default:
		return "created↑"
	}
}

// cmpMode is the sort-mode comparator, shared by the timeline sort and the
// tie-breaking inside relevance-ranked layers.
func cmpMode(mode sortMode) func(a, b domain.SearchResult) int {
	rating := func(r domain.SearchResult) int { return r.VotesUp - r.VotesDown }
	return func(a, b domain.SearchResult) int {
		switch mode {
		case sortCreatedDesc:
			return b.CreatedAt.Compare(a.CreatedAt)
		case sortRatingDesc:
			return rating(b) - rating(a)
		case sortRatingAsc:
			return rating(a) - rating(b)
		case sortAccessDesc:
			return b.AccessCount - a.AccessCount
		case sortAccessAsc:
			return a.AccessCount - b.AccessCount
		default:
			return a.CreatedAt.Compare(b.CreatedAt)
		}
	}
}

// sortLayer keeps relevance (higher first) as the primary order and lets
// the mode break exact ties only — closer matches always rank above
// weaker ones, whatever the sort.
func sortLayer(results []domain.SearchResult, relevance func(domain.SearchResult) float64, mode sortMode) {
	tie := cmpMode(mode)
	slices.SortStableFunc(results, func(a, b domain.SearchResult) int {
		if ra, rb := relevance(a), relevance(b); ra != rb {
			if ra > rb {
				return -1
			}
			return 1
		}
		return tie(a, b)
	})
}

func newListModel(st *styles) listModel {
	search := textinput.New()
	search.Placeholder = "search — terms match keywords, text, and fuzzy"
	search.Prompt = "/ "

	terms := &[]string{}
	l := list.New(nil, itemDelegate{st: st, terms: terms}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	// quitting is the app's job (q/ctrl+c in updateKey); the component's
	// default keymap binds esc to Quit, which must not close the TUI
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)
	// the dot paginator scales terribly (100k rows = thousands of dots);
	// the badges line shows numeric pages instead
	l.SetShowPagination(false)

	return listModel{st: st, search: search, results: l, terms: terms}
}

// spec snapshots the screen state into a query.
func (m *listModel) spec() searchSpec {
	return searchSpec{
		terms:       strings.Fields(m.search.Value()),
		kind:        m.kind,
		includeDead: m.includeDead,
		review:      m.review,
		recent:      m.recent,
		sort:        m.sort,
	}
}

func (m *listModel) apply(results []domain.SearchResult) {
	if m.review {
		results = slices.DeleteFunc(slices.Clone(results), func(r domain.SearchResult) bool {
			return r.VotesDown <= r.VotesUp
		})
		slices.SortStableFunc(results, func(a, b domain.SearchResult) int {
			return (a.VotesUp - a.VotesDown) - (b.VotesUp - b.VotesDown) // most negative first
		})
	}
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = resultItem{r: r}
	}
	m.results.SetItems(items)
	if m.pendingReset {
		m.results.ResetSelected()
		m.pendingReset = false
	}
}

// appendResults extends the list with the next timeline page; ids already
// shown are skipped (offset pages are disjoint, this is belt-and-braces).
func (m *listModel) appendResults(results []domain.SearchResult) {
	seen := make(map[string]bool, len(m.results.Items()))
	for _, it := range m.results.Items() {
		if item, ok := it.(resultItem); ok {
			seen[item.r.ID] = true
		}
	}
	items := m.results.Items()
	for _, r := range results {
		if !seen[r.ID] {
			items = append(items, resultItem{r: r})
		}
	}
	m.results.SetItems(items)
}

func (m *listModel) selectedID() string {
	if r, ok := m.selected(); ok {
		return r.ID
	}
	return ""
}

// selectByID restores the cursor after a re-merge; unknown ids keep the
// current position.
func (m *listModel) selectByID(id string) {
	for i, it := range m.results.Items() {
		if item, ok := it.(resultItem); ok && item.r.ID == id {
			m.results.Select(i)
			return
		}
	}
}

func (m *listModel) selected() (domain.SearchResult, bool) {
	item, ok := m.results.SelectedItem().(resultItem)
	if !ok {
		return domain.SearchResult{}, false
	}
	return item.r, true
}

// bumpVotes optimistically updates the selected item after a rate.
func (m *listModel) bumpVotes(id string, up bool) {
	for i, it := range m.results.Items() {
		item, ok := it.(resultItem)
		if !ok || item.r.ID != id {
			continue
		}
		if up {
			item.r.VotesUp++
		} else {
			item.r.VotesDown++
		}
		m.results.SetItem(i, item)
		return
	}
}

func (m *listModel) removeItem(id string) {
	for i, it := range m.results.Items() {
		if item, ok := it.(resultItem); ok && item.r.ID == id {
			m.results.RemoveItem(i)
			return
		}
	}
}

// cycleKind rotates the kind filter: all → fact → … → reference → all.
func (m *listModel) cycleKind() {
	ring := append([]string{""}, kindStrings()...)
	m.kind = ring[(slices.Index(ring, m.kind)+1)%len(ring)]
}

// cycleSort rotates the display order: created↑ → created↓ → rating↓ →
// rating↑ → reads↓ → reads↑ → created↑.
func (m *listModel) cycleSort() {
	m.sort = (m.sort + 1) % sortModes
}

func kindStrings() []string {
	out := make([]string, len(domain.Kinds))
	for i, k := range domain.Kinds {
		out[i] = string(k)
	}
	return out
}

func (m *listModel) setSize(width, height int) {
	m.search.Width = max(10, width-4)
	m.results.SetSize(width, max(1, height-3)) // search line + badges + spacer
}

func (m *listModel) badges() string {
	var parts []string
	if m.remoteAddr != "" {
		parts = append(parts, m.st.accent.Render("[remote "+m.remoteAddr+"]"))
	}
	if m.kind != "" {
		parts = append(parts, m.st.kindBadge(m.kind))
	}
	if m.includeDead {
		parts = append(parts, m.st.dim.Render("[+dead]"))
	}
	if m.recent {
		parts = append(parts, m.st.dim.Render("[recent 3d]"))
	}
	if m.sort != sortCreatedAsc {
		parts = append(parts, m.st.dim.Render("[sort: "+m.sort.label()+"]"))
	}
	if m.review {
		parts = append(parts, m.st.errText.Render("[review candidates]"))
	}
	if pg := m.pageInfo(); pg != "" {
		parts = append(parts, m.st.dim.Render(pg))
	}
	return strings.Join(parts, " ")
}

// pageInfo renders numeric pagination: exact page count when the store
// reported a total, loaded count (with a trailing + while more may exist)
// otherwise.
func (m *listModel) pageInfo() string {
	per := m.results.Paginator.PerPage
	loaded := len(m.results.Items())
	if per <= 0 || loaded == 0 {
		return ""
	}
	current := m.results.Paginator.Page + 1
	pages := func(n int) int { return (n + per - 1) / per }
	switch {
	case m.total > 0:
		return fmt.Sprintf("pg %d/%d · %d memories", current, pages(m.total), m.total)
	case m.exhausted:
		return fmt.Sprintf("pg %d/%d · %d matches", current, pages(loaded), loaded)
	default:
		return fmt.Sprintf("pg %d/%d+ · %d+ matches", current, pages(loaded), loaded)
	}
}

func (m *listModel) view() string {
	// the delegate highlights by whatever is in the box right now — an
	// emptied box clears highlights on the very next frame
	*m.terms = strings.Fields(m.search.Value())
	header := m.search.View()
	if badges := m.badges(); badges != "" {
		header += "\n" + badges
	} else {
		header += "\n"
	}
	return header + "\n" + m.results.View()
}

// itemDelegate renders what humans scan by — kind, keywords, date — loud,
// with the summary as small secondary text underneath. terms (shared with
// the list model) drive fuzzy-hit highlighting.
type itemDelegate struct {
	st    *styles
	terms *[]string
}

func (itemDelegate) Height() int  { return 2 }
func (itemDelegate) Spacing() int { return 1 }

func (itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	item, ok := it.(resultItem)
	if !ok {
		return
	}
	r := item.r

	cursor, keywordStyle := "  ", d.st.title
	if index == m.Index() {
		cursor, keywordStyle = d.st.selected.Render("> "), d.st.selected
	}

	width := max(20, m.Width()-4)
	date := r.CreatedAt.Format("2006-01-02")
	if r.ExpiresAt != nil {
		date += " ⏳" + r.ExpiresAt.Format("2006-01-02")
	}
	keywords := ansi.Truncate(strings.Join(r.Keywords, ", "),
		max(8, width-len(date)-len(r.Kind)-6), "…")
	first := fmt.Sprintf("%s %s  %s", d.st.kindBadge(r.Kind),
		highlightFuzzy(keywords, *d.terms, keywordStyle, d.st.hit), d.st.dim.Render(date))

	secondary := r.Summary
	if r.VotesUp > 0 || r.VotesDown > 0 {
		secondary += fmt.Sprintf("  ↑%d ↓%d", r.VotesUp, r.VotesDown)
	}
	second := "   " + highlightFuzzy(ansi.Truncate(secondary, width, "…"), *d.terms, d.st.dim, d.st.hit)

	_, _ = fmt.Fprintf(w, "%s%s\n%s", cursor, first, second)
}
