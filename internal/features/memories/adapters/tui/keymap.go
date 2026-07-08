package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines every binding once; single-letter keys are gated in the
// update loop on "no text input focused".
type keyMap struct {
	Search    key.Binding
	Enter     key.Binding
	RateUp    key.Binding
	RateDown  key.Binding
	Supersede key.Binding
	Delete    key.Binding
	Kind      key.Binding
	Dead      key.Binding
	Review    key.Binding
	Remote    key.Binding
	Refresh   key.Binding
	Follow    key.Binding
	Submit    key.Binding
	Unset     key.Binding
	Back      key.Binding
	Quit      key.Binding
	Help      key.Binding
	HalfDown  key.Binding
	HalfUp    key.Binding
	Find      key.Binding
	Confirm   key.Binding
	Sort      key.Binding
	Recent    key.Binding
}

func newKeyMap() keyMap {
	b := func(keys, help string) key.Binding {
		return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, help))
	}
	return keyMap{
		Search:    b("/", "search"),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		RateUp:    b("+", "rate up"),
		RateDown:  b("-", "rate down"),
		Supersede: b("e", "supersede"),
		Delete:    b("d", "delete"),
		Kind:      b("f", "cycle kind"),
		Dead:      b("a", "±dead"),
		Review:    b("R", "review candidates"),
		Remote:    b("c", "remote config"),
		Refresh:   b("r", "refresh"),
		Follow:    b("o", "follow supersede"),
		Submit:    key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
		Unset:     key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl+x", "unset remote")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:      b("?", "help"),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		Find:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "find in memory")),
		Confirm:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		Sort:      b("s", "cycle sort"),
		Recent:    b("t", "recent 3d"),
	}
}

// helpSet is an ad-hoc help.KeyMap: the footer shows different bindings per
// screen, so each mode assembles its own set (App.modeHelp).
type helpSet struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h helpSet) ShortHelp() []key.Binding { return h.short }

func (h helpSet) FullHelp() [][]key.Binding {
	if h.full == nil {
		return [][]key.Binding{h.short}
	}
	return h.full
}

// ShortHelp/FullHelp implement help.KeyMap for the list footer. Short stays
// essentials-only — everything else lives behind ? (FullHelp).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Enter, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Search, k.Enter, k.Refresh, k.Follow},
		{k.RateUp, k.RateDown, k.Supersede, k.Delete},
		{k.Kind, k.Dead, k.Recent, k.Sort},
		{k.Review, k.Remote, k.HalfDown, k.HalfUp},
		{k.Back, k.Quit},
	}
}
