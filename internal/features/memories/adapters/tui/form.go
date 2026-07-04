package tui

import (
	"errors"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/usecases/managememories"
	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/domain"
)

// form field indexes — tab order.
const (
	fieldSummary = iota
	fieldContent
	fieldKeywords
	fieldKind
	fieldTTL
	fieldCount
)

// formModel is the supersede editor: pre-filled from the old memory, the
// domain validates on submit.
type formModel struct {
	st         *styles
	supersedes string
	summary    textinput.Model
	content    textarea.Model
	keywords   textinput.Model
	kindIdx    int
	ttl        textinput.Model
	focus      int
	fieldErr   string
	errField   int
	returnTo   mode
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

	ttl := textinput.New()
	ttl.Prompt = ""
	ttl.Placeholder = "hours, empty = never"

	kindIdx := max(0, indexOfKind(from.Kind))

	f := &formModel{
		st: st, supersedes: from.ID,
		summary: summary, content: content, keywords: keywords,
		kindIdx: kindIdx, ttl: ttl, errField: -1, returnTo: returnTo,
	}
	f.setSize(width, height)
	f.setFocus(fieldSummary)
	return f
}

func indexOfKind(kind string) int {
	for i, k := range domain.Kinds {
		if string(k) == kind {
			return i
		}
	}
	return 0
}

func (f *formModel) setSize(width, height int) {
	w := max(20, width-4)
	f.summary.Width = w
	f.keywords.Width = w
	f.ttl.Width = w
	f.content.SetWidth(w)
	f.content.SetHeight(max(3, height-14)) // room for labels + other fields
}

func (f *formModel) setFocus(field int) {
	f.focus = field
	f.summary.Blur()
	f.content.Blur()
	f.keywords.Blur()
	f.ttl.Blur()
	switch field {
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

func (f *formModel) cycleFocus(back bool) {
	step := 1
	if back {
		step = fieldCount - 1
	}
	f.setFocus((f.focus + step) % fieldCount)
}

// typing captures every text field; global single-letter keys stay inert.
func (f *formModel) typing() bool {
	return f.focus != fieldKind
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
	case fieldKind:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "left", "h":
				f.kindIdx = (f.kindIdx + len(domain.Kinds) - 1) % len(domain.Kinds)
			case "right", "l", " ":
				f.kindIdx = (f.kindIdx + 1) % len(domain.Kinds)
			}
		}
	case fieldTTL:
		f.ttl, cmd = f.ttl.Update(msg)
	}
	return cmd
}

// input assembles the StoreInput; only TTL needs pre-parsing — everything
// else is the domain's job to validate.
func (f *formModel) input() (managememories.StoreInput, error) {
	ttlHours := 0
	if raw := strings.TrimSpace(f.ttl.Value()); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return managememories.StoreInput{}, errors.New("ttl must be a number of hours")
		}
		ttlHours = parsed
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
		Kind:       string(domain.Kinds[f.kindIdx]),
		Keywords:   keywords,
		Source:     "tui",
		TTLHours:   ttlHours,
		Supersedes: f.supersedes,
	}, nil
}

// applyError maps a domain error to a field-anchored message.
func (f *formModel) applyError(err error) {
	var (
		summaryErr  *domain.InvalidSummaryError
		contentErr  *domain.EmptyContentError
		keywordsErr *domain.NoKeywordsError
		kindErr     *domain.InvalidKindError
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
	case errors.As(err, &kindErr):
		f.fieldErr, f.errField = "kind: pick one of the listed kinds", fieldKind
	case errors.As(err, &ttlErr):
		f.fieldErr, f.errField = "ttl: must be ≥ 0 hours", fieldTTL
	case errors.As(err, &superErr):
		f.fieldErr, f.errField = "already superseded elsewhere — refresh the list", -1
	default:
		f.fieldErr, f.errField = err.Error(), -1
	}
	if f.errField >= 0 {
		f.setFocus(f.errField)
	}
}

func (f *formModel) label(field int, text string) string {
	style := f.st.blurred
	if f.focus == field {
		style = f.st.focused
	}
	if f.errField == field && f.fieldErr != "" {
		return style.Render(text) + "  " + f.st.errText.Render(f.fieldErr)
	}
	return style.Render(text)
}

func (f *formModel) view() string {
	kinds := make([]string, len(domain.Kinds))
	for i, k := range domain.Kinds {
		if i == f.kindIdx {
			kinds[i] = f.st.selected.Render("[" + string(k) + "]")
		} else {
			kinds[i] = f.st.dim.Render(string(k))
		}
	}

	var b strings.Builder
	b.WriteString(f.st.title.Render("supersede "+f.supersedes) + "\n\n")
	b.WriteString(f.label(fieldSummary, "summary") + "\n" + f.summary.View() + "\n\n")
	b.WriteString(f.label(fieldContent, "content") + "\n" + f.content.View() + "\n\n")
	b.WriteString(f.label(fieldKeywords, "keywords (comma-separated)") + "\n" + f.keywords.View() + "\n\n")
	b.WriteString(f.label(fieldKind, "kind (←/→)") + "  " + strings.Join(kinds, " ") + "\n\n")
	b.WriteString(f.label(fieldTTL, "ttl") + "\n" + f.ttl.View() + "\n\n")
	if f.errField < 0 && f.fieldErr != "" {
		b.WriteString(f.st.errText.Render(f.fieldErr) + "\n")
	}
	b.WriteString(f.st.dim.Render("tab: next field · ctrl+s: store correction · esc: abandon"))
	return b.String()
}
