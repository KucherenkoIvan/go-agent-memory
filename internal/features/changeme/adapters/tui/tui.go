// Package tui is the terminal presentation adapter (Bubble Tea, Elm-style):
// Update maps key presses to commands and queries, View renders read-models.
// The same containment rule as every transport: tea types never leave this
// package; use-cases see context.Context and typed arguments.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/KucherenkoIvan/go-kernel/ddd"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/application/usecases/managechangeme"
	"github.com/KucherenkoIvan/go-tinycore-template/internal/features/changeme/domain"
)

// RefreshMsg is sent from outside the loop (the event subscription in
// cmd/tui) — committed domain events drive the UI, terminal edition.
type RefreshMsg struct{}

type (
	itemsMsg []domain.ChangeMeReadModel
	actedMsg struct{}
	errMsg   struct{ err error }
)

type mode int

const (
	modeList mode = iota
	modeCreate
	modeEdit
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
)

type Model struct {
	uc     managechangeme.UseCases
	items  []domain.ChangeMeReadModel
	cursor int
	mode   mode
	input  textinput.Model
	editID domain.ChangeMeID
	status string
}

func New(uc managechangeme.UseCases) Model {
	input := textinput.New()
	input.Placeholder = "name"
	input.CharLimit = 200
	return Model{uc: uc, input: input}
}

func (m Model) Init() tea.Cmd { return m.load }

// --- tea.Cmds: the bridge to the application layer ---

func (m Model) load() tea.Msg {
	items, err := m.uc.List.Execute(context.Background())
	if err != nil {
		return errMsg{err}
	}
	return itemsMsg(items)
}

func (m Model) submit() tea.Cmd {
	name := m.input.Value()
	if m.mode == modeEdit {
		id := m.editID
		return func() tea.Msg {
			if err := m.uc.Update.Execute(context.Background(), id, name); err != nil {
				return errMsg{err}
			}
			return actedMsg{}
		}
	}
	return func() tea.Msg {
		if _, err := m.uc.Create.Execute(context.Background(), name); err != nil {
			return errMsg{err}
		}
		return actedMsg{}
	}
}

func (m Model) remove(id domain.ChangeMeID) tea.Cmd {
	return func() tea.Msg {
		if err := m.uc.Delete.Execute(context.Background(), id); err != nil {
			return errMsg{err}
		}
		return actedMsg{}
	}
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.mode == modeList {
			return m.updateList(msg)
		}
		return m.updateInput(msg)

	case itemsMsg:
		m.items = msg
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}
		return m, nil

	case actedMsg:
		m.mode = modeList
		m.status = ""
		return m, m.load

	case RefreshMsg:
		return m, m.load

	case errMsg:
		m.status = errorText(msg.err)
		return m, nil
	}
	return m, nil
}

func (m Model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "n":
		m.mode = modeCreate
		m.status = ""
		m.input.SetValue("")
		m.input.Focus()
	case "e":
		if item, ok := m.selected(); ok {
			m.mode = modeEdit
			m.status = ""
			m.editID = domain.ChangeMeID(item.ID)
			m.input.SetValue(item.Name)
			m.input.Focus()
			m.input.CursorEnd()
		}
	case "d":
		if item, ok := m.selected(); ok {
			return m, m.remove(domain.ChangeMeID(item.ID))
		}
	case "r":
		return m, m.load
	}
	return m, nil
}

func (m Model) updateInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.mode = modeList
		m.status = ""
		return m, nil
	case "enter":
		return m, m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

func (m Model) selected() (domain.ChangeMeReadModel, bool) {
	if len(m.items) == 0 {
		return domain.ChangeMeReadModel{}, false
	}
	return m.items[m.cursor], true
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("changeme — tinycore tui"))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("  nothing here yet — press n to create"))
		b.WriteString("\n")
	}
	for i, item := range m.items {
		line := fmt.Sprintf("%s  %s", item.Name,
			dimStyle.Render(fmt.Sprintf("%s · %s", item.ID[:8], item.CreatedAt.Local().Format("2006-01-02 15:04"))))
		if i == m.cursor && m.mode == modeList {
			b.WriteString(selectedStyle.Render("› " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	switch m.mode {
	case modeCreate:
		b.WriteString(promptStyle.Render("new: ") + m.input.View() + "\n")
		b.WriteString(dimStyle.Render("enter save · esc cancel") + "\n")
	case modeEdit:
		b.WriteString(promptStyle.Render("edit: ") + m.input.View() + "\n")
		b.WriteString(dimStyle.Render("enter save · esc cancel") + "\n")
	default:
		b.WriteString(dimStyle.Render("n new · e edit · d delete · r refresh · ↑/↓ move · q quit") + "\n")
	}

	if m.status != "" {
		b.WriteString(errorStyle.Render(m.status) + "\n")
	}
	return b.String()
}

func errorText(err error) string {
	if ddd.IsDomainError(err) {
		return "✗ " + err.Error() // the domain code, e.g. invalid_name
	}
	return "✗ error: " + err.Error()
}
