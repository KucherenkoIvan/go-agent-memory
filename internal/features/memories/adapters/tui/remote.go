package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
)

// remoteModel views and edits the remote endpoint. Saving or clearing
// triggers a reconnect through the composition root's connect; while a
// reconnect has failed, the app parks here — the old service is gone.
type remoteModel struct {
	st     *styles
	addr   textinput.Model
	key    textinput.Model
	focus  int // 0 addr, 1 key
	broken bool
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
	r.setSize(width)
	r.setFocus(0)
	return r
}

func (r *remoteModel) setSize(width int) {
	r.addr.Width = max(20, width-4)
	r.key.Width = max(20, width-4)
}

func (r *remoteModel) setFocus(focus int) {
	r.focus = focus
	r.addr.Blur()
	r.key.Blur()
	if focus == 0 {
		r.addr.Focus()
	} else {
		r.key.Focus()
	}
}

func (r *remoteModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if r.focus == 0 {
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
		if r.focus == idx {
			return r.st.focused.Render(text)
		}
		return r.st.blurred.Render(text)
	}

	var b strings.Builder
	b.WriteString(r.st.title.Render("remote memory") + "\n")
	b.WriteString(r.st.dim.Render(current) + "\n")
	if r.broken {
		b.WriteString(r.st.errText.Render("reconnect failed — fix the endpoint and save, or unset to go local") + "\n")
	}
	b.WriteString("\n" + label(0, "address") + "\n" + r.addr.View() + "\n\n")
	b.WriteString(label(1, "api key") + "\n" + r.key.View() + "\n\n")
	b.WriteString(r.st.dim.Render("tab: switch field · ctrl+s: save + reconnect · ctrl+x: unset + go local · esc: back"))
	return b.String()
}
