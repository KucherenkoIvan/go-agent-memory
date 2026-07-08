package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// detailModel shows one memory in full: metadata header + scrollable body,
// with an optional find bar ("/") that fuzzy-highlights hits in the content
// and keywords.
type detailModel struct {
	st            *styles
	m             *domain.MemoryReadModel
	vp            viewport.Model
	find          textinput.Model
	width, height int
}

func newDetailModel(st *styles, m *domain.MemoryReadModel, width, height int) *detailModel {
	find := textinput.New()
	find.Placeholder = "find in memory"
	find.Prompt = "/ "
	d := &detailModel{st: st, m: m, find: find}
	d.setSize(width, height)
	return d
}

func (d *detailModel) setSize(width, height int) {
	d.width, d.height = width, height
	d.find.Width = max(10, width-4)
	header := d.header(width)
	d.vp = viewport.New(width, max(1, height-lipgloss.Height(header)-1))
	d.vp.SetContent(d.body())
}

// refresh re-renders header and body for the current find terms, keeping
// the scroll position (clamped by the viewport itself).
func (d *detailModel) refresh() {
	offset := d.vp.YOffset
	header := d.header(d.width)
	d.vp.Height = max(1, d.height-lipgloss.Height(header)-1)
	d.vp.SetContent(d.body())
	d.vp.SetYOffset(offset)
}

// body hard-wraps the content (long lines scroll vertically, not
// horizontally), then highlights find hits per wrapped line — offsets from
// the matcher index into plain text, so wrap first, style after.
func (d *detailModel) body() string {
	wrapped := lipgloss.NewStyle().Width(max(20, d.width-2)).Render(d.m.Content)
	terms := d.findTerms()
	if len(terms) == 0 {
		return wrapped
	}
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		lines[i] = highlightFuzzy(line, terms, lipgloss.NewStyle(), d.st.hit)
	}
	return strings.Join(lines, "\n")
}

func (d *detailModel) findTerms() []string {
	return strings.Fields(d.find.Value())
}

func (d *detailModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return cmd
}

func (d *detailModel) bumpVotes(up bool) {
	if up {
		d.m.VotesUp++
	} else {
		d.m.VotesDown++
	}
}

func (d *detailModel) header(width int) string {
	m := d.m
	title := fmt.Sprintf("%s %s", d.st.kindBadge(m.Kind), d.st.title.Render(m.Summary))

	meta := fmt.Sprintf("id %s · %s · source %s · ↑%d ↓%d · read %d×",
		m.ID, m.CreatedAt.Format("2006-01-02"), m.Source, m.VotesUp, m.VotesDown, m.AccessCount)
	if m.ExpiresAt != nil {
		meta += " · expires " + m.ExpiresAt.Format("2006-01-02")
	}
	keywords := "keywords: " + strings.Join(m.Keywords, ", ")
	lines := []string{title, d.st.dim.Render(meta),
		highlightFuzzy(keywords, d.findTerms(), d.st.dim, d.st.hit)}
	if m.SupersededBy != "" {
		lines = append(lines, d.st.errText.Render("superseded by "+m.SupersededBy+"  (o to follow)"))
	}
	if d.find.Focused() || d.find.Value() != "" {
		lines = append(lines, d.find.View())
	}
	lines = append(lines, "")
	return lipgloss.NewStyle().Width(max(20, width-2)).Render(strings.Join(lines, "\n"))
}

func (d *detailModel) view(width int) string {
	return d.header(width) + "\n" + d.vp.View()
}
