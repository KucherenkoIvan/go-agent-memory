package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
)

// remote field indexes — navigation order.
const (
	remoteAddr = iota
	remoteKey
	remoteFieldCount
)

// confirmUnset joins the form's confirm actions: unset drops the remote
// endpoint and reconnects locally — worth a second look, like save/discard.
const confirmUnset = "unset"

// remoteModel views and edits the remote endpoint, with the same modal flow
// as the memory editor: opens in navigation, enter edits, ctrl+s/ctrl+x arm
// confirmations. Saving or clearing triggers a reconnect through the
// composition root's connect; while a reconnect has failed, the app parks
// here — the old service is gone.
type remoteModel struct {
	st         *styles
	addr       textinput.Model
	key        textinput.Model
	focus      int
	state      formState
	confirmAct string
	initial    [remoteFieldCount]string
	broken     bool
}

func newRemoteModel(st *styles, width int) *remoteModel {
	addr := textinput.New()
	addr.Prompt = ""
	addr.Placeholder = "host:7846"
	key := textinput.New()
	key.Prompt = ""
	key.Placeholder = "rcl_..."
	key.EchoMode = textinput.EchoPassword

	if cfg, err := remotecfg.Load(); err == nil && cfg != nil {
		addr.SetValue(cfg.Addr)
		key.SetValue(cfg.APIKey)
	}

	r := &remoteModel{st: st, addr: addr, key: key}
	r.initial = r.values()
	r.setSize(width)
	return r
}

func (r *remoteModel) values() [remoteFieldCount]string {
	return [remoteFieldCount]string{remoteAddr: r.addr.Value(), remoteKey: r.key.Value()}
}

func (r *remoteModel) dirty() bool {
	return r.values() != r.initial
}

func (r *remoteModel) setSize(width int) {
	r.addr.Width = max(20, width-4)
	r.key.Width = max(20, width-4)
}

func (r *remoteModel) startEdit() {
	r.state = formEdit
	r.stopEditFocus()
	if r.focus == remoteAddr {
		r.addr.Focus()
	} else {
		r.key.Focus()
	}
}

func (r *remoteModel) stopEdit() {
	r.state = formNav
	r.stopEditFocus()
}

func (r *remoteModel) stopEditFocus() {
	r.addr.Blur()
	r.key.Blur()
}

func (r *remoteModel) move(step int) {
	r.focus = (r.focus + step + remoteFieldCount) % remoteFieldCount
}

func (r *remoteModel) typing() bool {
	return r.state == formEdit
}

func (r *remoteModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if r.focus == remoteAddr {
		r.addr, cmd = r.addr.Update(msg)
	} else {
		r.key, cmd = r.key.Update(msg)
	}
	return cmd
}

// save persists the entered endpoint; empty addr means "go local".
func (r *remoteModel) save() error {
	addr := strings.TrimSpace(r.addr.Value())
	if addr == "" {
		return remotecfg.Remove()
	}
	return remotecfg.Save(remotecfg.Config{Addr: addr, APIKey: strings.TrimSpace(r.key.Value())})
}

func (r *remoteModel) view() string {
	current := "current: local (no remote configured)"
	if cfg, err := remotecfg.Resolve(); err == nil && cfg != nil {
		current = "current: remote " + cfg.Addr
	} else if err != nil {
		current = "current config broken: " + err.Error()
	}

	label := func(idx int, text string) string {
		style, marker := r.st.blurred, "  "
		if r.focus == idx {
			style = r.st.focused
			marker = r.st.focused.Render("> ")
			if r.state == formEdit {
				marker = r.st.focused.Render("✎ ")
			}
		}
		return marker + style.Render(text)
	}

	var b strings.Builder
	title := r.st.title.Render("remote memory")
	if r.dirty() {
		title += r.st.errText.Render("  [unsaved]")
	}
	b.WriteString(title + "\n")
	b.WriteString(r.st.dim.Render(current) + "\n")
	if r.broken {
		b.WriteString(r.st.errText.Render("reconnect failed — fix the endpoint and save, or unset to go local") + "\n")
	}
	b.WriteString("\n" + label(remoteAddr, "address") + "\n" + r.addr.View() + "\n\n")
	b.WriteString(label(remoteKey, "api key") + "\n" + r.key.View() + "\n\n")

	// key hints live in the app footer; only confirmation warnings here
	switch r.confirmAct {
	case confirmSave:
		b.WriteString(r.st.errText.Render("save endpoint and reconnect?"))
	case confirmUnset:
		b.WriteString(r.st.errText.Render("unset remote and go local?"))
	case confirmDiscard:
		b.WriteString(r.st.errText.Render("discard unsaved changes?"))
	}
	return b.String()
}
