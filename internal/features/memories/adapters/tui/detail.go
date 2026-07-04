package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// detailModel shows one memory in full: metadata header + scrollable body.
type detailModel struct {
	st *styles
	m  *domain.MemoryReadModel
	vp viewport.Model
}

func newDetailModel(st *styles, m *domain.MemoryReadModel, width, height int) *detailModel {
	d := &detailModel{st: st, m: m}
	d.setSize(width, height)
	return d
}

func (d *detailModel) setSize(width, height int) {
	header := d.header(width)
	d.vp = viewport.New(width, max(1, height-lipgloss.Height(header)-1))
	// hard-wrap so long lines scroll vertically, not horizontally
	d.vp.SetContent(lipgloss.NewStyle().Width(max(20, width-2)).Render(d.m.Content))
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
	lines := []string{title, d.st.dim.Render(meta), d.st.dim.Render("keywords: " + strings.Join(m.Keywords, ", "))}
	if m.SupersededBy != "" {
		lines = append(lines, d.st.errText.Render("superseded by "+m.SupersededBy+"  (o to follow)"))
	}
	lines = append(lines, "")
	return lipgloss.NewStyle().Width(max(20, width-2)).Render(strings.Join(lines, "\n"))
}

func (d *detailModel) view(width int) string {
	return d.header(width) + "\n" + d.vp.View()
}
