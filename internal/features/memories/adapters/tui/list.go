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

const reviewFetchLimit = 200

type resultItem struct{ r domain.SearchResult }

func (resultItem) FilterValue() string { return "" } // search is server-side

// listModel is home: search box + ranked results.
type listModel struct {
	st      *styles
	search  textinput.Model
	results list.Model
	// seq marks the latest issued query; older responses are dropped
	seq         int
	review      bool
	includeDead bool
}

func newListModel(st *styles) listModel {
	search := textinput.New()
	search.Placeholder = "search — free text, k:<keyword>, kind:<kind>"
	search.Prompt = "/ "

	l := list.New(nil, itemDelegate{st: st}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	return listModel{st: st, search: search, results: l}
}

// filters builds the effective query from the single source of truth: the
// search box text plus the two toggles.
func (m *listModel) filters() ports.SearchFilters {
	f := ParseQuery(m.search.Value())
	f.IncludeDead = m.includeDead
	if m.review {
		f.Limit = reviewFetchLimit
	}
	return f
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

// cycleKind rotates the kind: token in the search text — text stays the
// single source of truth for filters.
func (m *listModel) cycleKind() {
	current := ParseQuery(m.search.Value()).Kind
	ring := append([]string{""}, kindStrings()...)
	next := ring[(slices.Index(ring, current)+1)%len(ring)]

	fields := slices.DeleteFunc(strings.Fields(m.search.Value()), func(tok string) bool {
		return strings.HasPrefix(tok, "kind:")
	})
	if next != "" {
		fields = append(fields, "kind:"+next)
	}
	m.search.SetValue(strings.Join(fields, " "))
	if m.search.Focused() {
		m.search.CursorEnd()
	}
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
	if f := ParseQuery(m.search.Value()); f.Kind != "" {
		parts = append(parts, m.st.accent.Render("[kind:"+f.Kind+"]"))
	}
	if m.includeDead {
		parts = append(parts, m.st.dim.Render("[+dead]"))
	}
	if m.review {
		parts = append(parts, m.st.errText.Render("[review candidates]"))
	}
	return strings.Join(parts, " ")
}

func (m *listModel) view() string {
	header := m.search.View()
	if badges := m.badges(); badges != "" {
		header += "\n" + badges
	} else {
		header += "\n"
	}
	return header + "\n" + m.results.View()
}

// itemDelegate renders a two-line entry: [kind] summary / metadata.
type itemDelegate struct{ st *styles }

func (itemDelegate) Height() int  { return 2 }
func (itemDelegate) Spacing() int { return 1 }

func (itemDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	item, ok := it.(resultItem)
	if !ok {
		return
	}
	r := item.r

	cursor, style := "  ", d.st.dim
	if index == m.Index() {
		cursor, style = d.st.selected.Render("> "), d.st.selected
	}

	width := max(20, m.Width()-4)
	first := fmt.Sprintf("%s %s", d.st.kindBadge(r.Kind), style.Render(ansi.Truncate(r.Summary, width, "…")))

	meta := fmt.Sprintf("%s · %s · ↑%d ↓%d",
		r.CreatedAt.Format("2006-01-02"), strings.Join(r.Keywords, ", "), r.VotesUp, r.VotesDown)
	if r.ExpiresAt != nil {
		meta += " · expires " + r.ExpiresAt.Format("2006-01-02")
	}
	second := "   " + d.st.dim.Render(ansi.Truncate(meta, width, "…"))

	_, _ = fmt.Fprintf(w, "%s%s\n%s", cursor, first, second)
}
