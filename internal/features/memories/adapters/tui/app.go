package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories"
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeForm
	modeConfirm
	modeRemote
)

// App is the root bubbletea model: a flat screen enum over sub-models, one
// Service handle, and a rebuildable connection for remote config changes.
type App struct {
	ctx     context.Context
	svc     memories.Service
	cleanup func()
	connect Connect
	version string

	mode    mode
	list    listModel
	detail  *detailModel
	form    *formModel
	confirm *confirmModel
	remote  *remoteModel

	width, height int
	ready         bool
	spin          spinner.Model
	inFlight      int
	status        string
	statusErr     bool
	keys          keyMap
	help          help.Model
	st            *styles
}

func newApp(ctx context.Context, svc memories.Service, cleanup func(), connect Connect, version string) *App {
	st := newStyles()
	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return &App{
		ctx: ctx, svc: svc, cleanup: cleanup, connect: connect, version: version,
		list: newListModel(st), spin: spin, keys: newKeyMap(), help: help.New(), st: st,
	}
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.issueSearch(), a.spin.Tick)
}

// issueSearch runs the list's current filters immediately (init, refresh,
// toggles). Debounced typing goes through debounceMsg instead.
func (a *App) issueSearch() tea.Cmd {
	if a.svc == nil {
		return nil
	}
	a.list.seq++
	a.inFlight++
	return searchCmd(a.ctx, a.svc, a.list.seq, a.list.spec())
}

func (a *App) settle() { a.inFlight = max(0, a.inFlight-1) }

func (a *App) toast(text string, isErr bool) {
	a.status, a.statusErr = text, isErr
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.ready = true
		bodyH := max(1, a.height-2)
		a.list.setSize(a.width, bodyH)
		if a.detail != nil {
			a.detail.setSize(a.width, bodyH)
		}
		if a.form != nil {
			a.form.setSize(a.width, bodyH)
		}
		if a.remote != nil {
			a.remote.setSize(a.width)
		}
		return a, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(msg)
		return a, cmd

	case debounceMsg:
		if msg.seq != a.list.seq || a.svc == nil {
			return a, nil // superseded by newer typing
		}
		a.inFlight++
		return a, searchCmd(a.ctx, a.svc, msg.seq, a.list.spec())

	case searchDoneMsg:
		a.settle()
		if msg.seq == a.list.seq {
			a.list.apply(msg.results)
		}
		return a, nil

	case memoryLoadedMsg:
		a.settle()
		bodyH := max(1, a.height-2)
		if msg.openForm {
			a.form = newFormModel(a.st, msg.model, a.mode, a.width, bodyH)
			a.mode = modeForm
		} else {
			a.detail = newDetailModel(a.st, msg.model, a.width, bodyH)
			a.mode = modeDetail
		}
		return a, nil

	case ratedMsg:
		a.settle()
		verdict := "up"
		if !msg.up {
			verdict = "down"
		}
		a.toast("rated "+verdict, false)
		a.list.bumpVotes(msg.id, msg.up)
		if a.detail != nil && a.detail.m.ID == msg.id {
			a.detail.bumpVotes(msg.up)
		}
		return a, nil

	case deletedMsg:
		a.settle()
		a.list.removeItem(msg.id)
		a.toast("deleted "+msg.id, false)
		a.detail = nil
		a.mode = modeList
		return a, nil

	case storedMsg:
		a.settle()
		a.toast("superseded → "+string(msg.id), false)
		a.form = nil
		a.mode = modeList
		// refresh (the old memory drops out) and open the correction
		a.inFlight++
		return a, tea.Batch(a.issueSearch(), getCmd(a.ctx, a.svc, string(msg.id), false))

	case reconnectedMsg:
		a.settle()
		a.svc, a.cleanup = msg.svc, msg.cleanup
		if a.remote != nil {
			a.remote.broken = false
		}
		a.toast("connected", false)
		a.mode = modeList
		return a, a.issueSearch()

	case reconnectFailedMsg:
		a.settle()
		// the old handle is gone — park on the remote screen until fixed
		a.svc, a.cleanup = nil, nil
		if a.remote == nil {
			a.remote = newRemoteModel(a.st, a.width)
		}
		a.remote.broken = true
		a.mode = modeRemote
		a.toast("reconnect failed: "+msg.err.Error(), true)
		return a, nil

	case errMsg:
		a.settle()
		if a.mode == modeForm && msg.op == "store" && a.form != nil {
			a.form.applyError(msg.err)
			return a, nil
		}
		a.toast(msg.op+": "+msg.err.Error(), true)
		return a, nil

	case tea.KeyMsg:
		return a.updateKey(msg)
	}
	return a, nil
}

func (a *App) typing() bool {
	switch a.mode {
	case modeList:
		return a.list.search.Focused()
	case modeForm:
		return a.form != nil && a.form.typing()
	case modeRemote:
		return true
	default:
		return false
	}
}

func (a *App) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" || (msg.String() == "q" && !a.typing()) {
		if a.cleanup != nil {
			a.cleanup()
		}
		return a, tea.Quit
	}
	a.status = "" // any key clears the toast

	switch a.mode {
	case modeList:
		return a.updateListKey(msg)
	case modeDetail:
		return a.updateDetailKey(msg)
	case modeForm:
		return a.updateFormKey(msg)
	case modeConfirm:
		return a.updateConfirmKey(msg)
	case modeRemote:
		return a.updateRemoteKey(msg)
	}
	return a, nil
}

func (a *App) updateListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.list.search.Focused() {
		switch msg.String() {
		case "esc":
			a.list.search.Blur()
			return a, nil
		case "enter":
			a.list.search.Blur()
			return a, a.issueSearch()
		}
		before := a.list.search.Value()
		var cmd tea.Cmd
		a.list.search, cmd = a.list.search.Update(msg)
		if a.list.search.Value() != before {
			a.list.seq++
			return a, tea.Batch(cmd, debounceCmd(a.list.seq))
		}
		return a, cmd
	}

	switch {
	case key.Matches(msg, a.keys.Search):
		a.list.search.Focus()
		return a, nil
	case key.Matches(msg, a.keys.Enter):
		if r, ok := a.list.selected(); ok {
			a.inFlight++
			return a, getCmd(a.ctx, a.svc, r.ID, false)
		}
	case key.Matches(msg, a.keys.RateUp), key.Matches(msg, a.keys.RateDown):
		if r, ok := a.list.selected(); ok {
			a.inFlight++
			return a, rateCmd(a.ctx, a.svc, r.ID, key.Matches(msg, a.keys.RateUp))
		}
	case key.Matches(msg, a.keys.Supersede):
		if r, ok := a.list.selected(); ok {
			a.inFlight++
			return a, getCmd(a.ctx, a.svc, r.ID, true)
		}
	case key.Matches(msg, a.keys.Delete):
		if r, ok := a.list.selected(); ok {
			a.confirm = &confirmModel{st: a.st, id: r.ID, summary: r.Summary, returnTo: modeList}
			a.mode = modeConfirm
		}
	case key.Matches(msg, a.keys.Kind):
		a.list.cycleKind()
		return a, a.issueSearch()
	case key.Matches(msg, a.keys.Dead):
		a.list.includeDead = !a.list.includeDead
		return a, a.issueSearch()
	case key.Matches(msg, a.keys.Review):
		a.list.review = !a.list.review
		return a, a.issueSearch()
	case key.Matches(msg, a.keys.Remote):
		a.remote = newRemoteModel(a.st, a.width)
		a.mode = modeRemote
	case key.Matches(msg, a.keys.Refresh):
		return a, a.issueSearch()
	case key.Matches(msg, a.keys.Help):
		a.help.ShowAll = !a.help.ShowAll
	default:
		var cmd tea.Cmd
		a.list.results, cmd = a.list.results.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *App) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := a.detail
	switch {
	case key.Matches(msg, a.keys.Back):
		a.detail = nil
		a.mode = modeList
	case key.Matches(msg, a.keys.RateUp), key.Matches(msg, a.keys.RateDown):
		a.inFlight++
		return a, rateCmd(a.ctx, a.svc, d.m.ID, key.Matches(msg, a.keys.RateUp))
	case key.Matches(msg, a.keys.Supersede):
		a.form = newFormModel(a.st, d.m, modeDetail, a.width, max(1, a.height-2))
		a.mode = modeForm
	case key.Matches(msg, a.keys.Delete):
		a.confirm = &confirmModel{st: a.st, id: d.m.ID, summary: d.m.Summary, returnTo: modeDetail}
		a.mode = modeConfirm
	case key.Matches(msg, a.keys.Follow):
		if d.m.SupersededBy != "" {
			a.inFlight++
			return a, getCmd(a.ctx, a.svc, d.m.SupersededBy, false)
		}
	case key.Matches(msg, a.keys.Help):
		a.help.ShowAll = !a.help.ShowAll
	default:
		return a, d.update(msg)
	}
	return a, nil
}

func (a *App) updateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := a.form
	switch msg.String() {
	case "esc":
		returnTo := f.returnTo
		a.form = nil
		if returnTo == modeDetail && a.detail != nil {
			a.mode = modeDetail
		} else {
			a.mode = modeList
		}
		return a, nil
	case "tab":
		f.cycleFocus(false)
		return a, nil
	case "shift+tab":
		f.cycleFocus(true)
		return a, nil
	case "ctrl+s":
		in, err := f.input()
		if err != nil {
			f.fieldErr, f.errField = err.Error(), fieldTTL
			f.setFocus(fieldTTL)
			return a, nil
		}
		a.inFlight++
		return a, storeCmd(a.ctx, a.svc, in)
	}
	return a, f.update(msg)
}

func (a *App) updateConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := a.confirm
	a.confirm = nil
	a.mode = c.returnTo
	if msg.String() == "y" {
		a.inFlight++
		return a, deleteCmd(a.ctx, a.svc, c.id)
	}
	a.toast("delete cancelled", false)
	return a, nil
}

func (a *App) updateRemoteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := a.remote
	switch msg.String() {
	case "esc":
		if r.broken || a.svc == nil {
			a.toast("no working connection — save a valid endpoint or unset", true)
			return a, nil
		}
		a.remote = nil
		a.mode = modeList
		return a, nil
	case "tab", "shift+tab":
		r.setFocus((r.focus + 1) % 2)
		return a, nil
	case "ctrl+s":
		if err := r.save(); err != nil {
			a.toast(err.Error(), true)
			return a, nil
		}
		a.inFlight++
		return a, reconnectCmd(a.ctx, a.cleanup, a.connect)
	case "ctrl+x":
		if err := remoteUnset(); err != nil {
			a.toast(err.Error(), true)
			return a, nil
		}
		a.inFlight++
		return a, reconnectCmd(a.ctx, a.cleanup, a.connect)
	}
	return a, r.update(msg)
}

func (a *App) View() string {
	if !a.ready {
		return "loading…"
	}

	var body string
	switch a.mode {
	case modeList:
		body = a.list.view()
	case modeDetail:
		body = a.detail.view(a.width)
	case modeForm:
		body = a.form.view()
	case modeConfirm:
		body = lipgloss.Place(a.width, max(1, a.height-2), lipgloss.Center, lipgloss.Center, a.confirm.view(a.width))
	case modeRemote:
		body = a.remote.view()
	}

	status := " "
	if a.inFlight > 0 {
		status = a.spin.View() + " "
	}
	if a.status != "" {
		if a.statusErr {
			status += a.st.errText.Render(a.status)
		} else {
			status += a.st.okText.Render(a.status)
		}
	}

	bodyH := max(1, a.height-2)
	return lipgloss.NewStyle().Height(bodyH).MaxHeight(bodyH).Render(body) +
		"\n" + status + "\n" + a.help.View(a.keys)
}
