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
}

func newListModel(st *styles) listModel {
	search := textinput.New()
	search.Placeholder = "search — terms match keywords, text, and fuzzy"
	search.Prompt = "/ "

	l := list.New(nil, itemDelegate{st: st}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	// quitting is the app's job (q/ctrl+c in updateKey); the component's
	// default keymap binds esc to Quit, which must not close the TUI
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)

	return listModel{st: st, search: search, results: l}
}

// spec snapshots the screen state into a query.
func (m *listModel) spec() searchSpec {
	return searchSpec{
		terms:       strings.Fields(m.search.Value()),
		kind:        m.kind,
		includeDead: m.includeDead,
		review:      m.review,
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
	if m.kind != "" {
		parts = append(parts, m.st.kindBadge(m.kind))
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

// itemDelegate renders what humans scan by — kind, keywords, date — loud,
// with the summary as small secondary text underneath.
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
		keywordStyle.Render(keywords), d.st.dim.Render(date))

	secondary := r.Summary
	if r.VotesUp > 0 || r.VotesDown > 0 {
		secondary += fmt.Sprintf("  ↑%d ↓%d", r.VotesUp, r.VotesDown)
	}
	second := "   " + d.st.dim.Render(ansi.Truncate(secondary, width, "…"))

	_, _ = fmt.Fprintf(w, "%s%s\n%s", cursor, first, second)
}
