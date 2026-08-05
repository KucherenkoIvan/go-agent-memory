package tui

import (
	"errors"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// form field indexes — navigation order.
const (
	fieldSummary = iota
	fieldContent
	fieldKeywords
	fieldTTL
	fieldCount
)

// formState: the editor opens in navigation — nothing focused, j/k walks
// the fields, enter starts typing, esc steps back out.
type formState int

const (
	formNav formState = iota
	formEdit
	formFind
)

// confirmable pending actions, armed by save/discard and resolved by y.
const (
	confirmNone    = ""
	confirmSave    = "save"
	confirmDiscard = "discard"
)

// formModel is the supersede editor: pre-filled from the old memory, the
// domain validates on submit.
type formModel struct {
	st         *styles
	supersedes string
	summary    textinput.Model
	content    textarea.Model
	keywords   textinput.Model
	ttl        textinput.Model
	focus      int
	state      formState
	find       textinput.Model
	confirmAct string
	// initial values back dirty(): discard warns only when edits would be lost
	initial  [fieldCount]string
	fieldErr string
	errField int
	returnTo mode
}

func newFormModel(st *styles, from *domain.MemoryReadModel, returnTo mode, width, height int) *formModel {
	summary := textinput.New()
	summary.SetValue(from.Summary)
	summary.Prompt = ""

	content := textarea.New()
	content.SetValue(from.Content)

	keywords := textinput.New()
	keywords.SetValue(strings.Join(from.Keywords, ", "))
	keywords.Prompt = ""

	// ttl is required, so the correction inherits the original's lifetime —
	// the editor is for changing what the memory says, not its shelf life
	ttl := textinput.New()
	ttl.Prompt = ""
	ttl.Placeholder = "hours (required) — 720 = 30 days, 8760 = a year"
	ttl.SetValue(strconv.Itoa(max(1, int(from.ExpiresAt.Sub(from.CreatedAt).Hours()))))

	find := textinput.New()
	find.Prompt = "/ "
	find.Placeholder = "find in fields"

	f := &formModel{
		st: st, supersedes: from.ID,
		summary: summary, content: content, keywords: keywords,
		ttl: ttl, find: find,
		errField: -1, returnTo: returnTo,
	}
	f.initial = f.values()
	f.setSize(width, height)
	return f
}

// values snapshots every field as text, for dirty comparison.
func (f *formModel) values() [fieldCount]string {
	return [fieldCount]string{
		fieldSummary:  f.summary.Value(),
		fieldContent:  f.content.Value(),
		fieldKeywords: f.keywords.Value(),
		fieldTTL:      f.ttl.Value(),
	}
}

func (f *formModel) dirty() bool {
	return f.values() != f.initial
}

func (f *formModel) setSize(width, height int) {
	w := max(20, width-4)
	f.summary.Width = w
	f.keywords.Width = w
	f.ttl.Width = w
	f.find.Width = w
	f.content.SetWidth(w)
	f.content.SetHeight(max(3, height-15)) // room for labels + other fields
}

// startEdit focuses the current field's widget and enters typing state.
func (f *formModel) startEdit() {
	f.state = formEdit
	f.blurAll()
	switch f.focus {
	case fieldSummary:
		f.summary.Focus()
	case fieldContent:
		f.content.Focus()
	case fieldKeywords:
		f.keywords.Focus()
	case fieldTTL:
		f.ttl.Focus()
	}
}

// stopEdit blurs everything and returns to navigation.
func (f *formModel) stopEdit() {
	f.state = formNav
	f.blurAll()
}

func (f *formModel) blurAll() {
	f.summary.Blur()
	f.content.Blur()
	f.keywords.Blur()
	f.ttl.Blur()
	f.find.Blur()
}

func (f *formModel) move(step int) {
	f.focus = (f.focus + step + fieldCount) % fieldCount
}

// typing captures every text field; global single-letter keys stay inert.
func (f *formModel) typing() bool {
	switch f.state {
	case formEdit:
		return true
	case formFind:
		return true
	default:
		return false
	}
}

func (f *formModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch f.focus {
	case fieldSummary:
		f.summary, cmd = f.summary.Update(msg)
	case fieldContent:
		f.content, cmd = f.content.Update(msg)
	case fieldKeywords:
		f.keywords, cmd = f.keywords.Update(msg)
	case fieldTTL:
		f.ttl, cmd = f.ttl.Update(msg)
	}
	return cmd
}

// input assembles the StoreInput; only TTL needs pre-parsing — everything
// else is the domain's job to validate.
func (f *formModel) input() (managememories.StoreInput, error) {
	ttlHours, err := strconv.Atoi(strings.TrimSpace(f.ttl.Value()))
	if err != nil {
		return managememories.StoreInput{}, errors.New("ttl must be a number of hours")
	}

	var keywords []string
	for _, kw := range strings.Split(f.keywords.Value(), ",") {
		if kw = strings.TrimSpace(kw); kw != "" {
			keywords = append(keywords, kw)
		}
	}

	return managememories.StoreInput{
		Content:    f.content.Value(),
		Summary:    f.summary.Value(),
		Keywords:   keywords,
		Source:     "tui",
		TTLHours:   ttlHours,
		Supersedes: f.supersedes,
	}, nil
}

// applyError maps a domain error to a field-anchored message and drops the
// user straight into editing the offending field.
func (f *formModel) applyError(err error) {
	var (
		summaryErr  *domain.InvalidSummaryError
		contentErr  *domain.EmptyContentError
		keywordsErr *domain.NoKeywordsError
		ttlErr      *domain.InvalidTTLError
		superErr    *domain.MemorySupersededError
	)
	switch {
	case errors.As(err, &summaryErr):
		f.fieldErr, f.errField = "summary: required, one line, ≤200 chars", fieldSummary
	case errors.As(err, &contentErr):
		f.fieldErr, f.errField = "content: must not be empty", fieldContent
	case errors.As(err, &keywordsErr):
		f.fieldErr, f.errField = "keywords: at least one, each ≤64 chars", fieldKeywords
	case errors.As(err, &ttlErr):
		f.fieldErr, f.errField = "ttl: 1 hour to 10 years — every memory expires", fieldTTL
	case errors.As(err, &superErr):
		f.fieldErr, f.errField = "already superseded elsewhere — refresh the list", -1
	default:
		f.fieldErr, f.errField = err.Error(), -1
	}
	if f.errField >= 0 {
		f.focus = f.errField
		f.startEdit()
	}
}

func (f *formModel) findTerms() []string {
	return strings.Fields(f.find.Value())
}

func (f *formModel) label(field int, text string) string {
	style := f.st.blurred
	marker := "  "
	if f.focus == field {
		style = f.st.focused
		marker = f.st.focused.Render("> ")
		if f.state == formEdit {
			marker = f.st.focused.Render("✎ ")
		}
	}
	out := marker + style.Render(text)
	if f.errField == field && f.fieldErr != "" {
		out += "  " + f.st.errText.Render(f.fieldErr)
	}
	return out
}

// staticField renders a field's value as plain text with find hits lit —
// widgets render only while being edited, static text is highlightable.
func (f *formModel) staticField(value string, maxLines int) string {
	if value == "" {
		return f.st.dim.Render("—")
	}
	lines := strings.Split(value, "\n")
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[:maxLines]
	}
	terms := f.findTerms()
	for i, line := range lines {
		lines[i] = highlightFuzzy(line, terms, lipgloss.NewStyle(), f.st.hit)
	}
	if truncated {
		lines = append(lines, f.st.dim.Render("…"))
	}
	return strings.Join(lines, "\n")
}

func (f *formModel) view() string {
	editing := func(field int) bool { return f.state == formEdit && f.focus == field }
	contentLines := f.content.Height()

	var b strings.Builder
	title := f.st.title.Render("supersede " + f.supersedes)
	if f.dirty() {
		title += f.st.errText.Render("  [unsaved]")
	}
	b.WriteString(title + "\n\n")

	writeField := func(field int, label, value string, widget func() string, lines int) {
		b.WriteString(f.label(field, label) + "\n")
		if editing(field) {
			b.WriteString(widget() + "\n\n")
		} else {
			b.WriteString(f.staticField(value, lines) + "\n\n")
		}
	}
	writeField(fieldSummary, "summary", f.summary.Value(), f.summary.View, 1)
	writeField(fieldContent, "content", f.content.Value(), f.content.View, contentLines)
	writeField(fieldKeywords, "keywords (comma-separated)", f.keywords.Value(), f.keywords.View, 1)
	writeField(fieldTTL, "ttl", f.ttl.Value(), f.ttl.View, 1)

	if f.state == formFind || f.find.Value() != "" {
		b.WriteString(f.find.View() + "\n")
	}
	if f.errField < 0 && f.fieldErr != "" {
		b.WriteString(f.st.errText.Render(f.fieldErr) + "\n")
	}

	// key hints live in the app footer (modeHelp); only confirmation
	// warnings render here — they are content, not bindings
	switch f.confirmAct {
	case confirmSave:
		b.WriteString(f.st.errText.Render("store correction superseding " + f.supersedes + "?"))
	case confirmDiscard:
		b.WriteString(f.st.errText.Render("discard unsaved changes?"))
	}
	return b.String()
}
