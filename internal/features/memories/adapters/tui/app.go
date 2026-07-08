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
		a.list.pendingReset = true // typed query change → cursor to top
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
	case modeDetail:
		return a.detail != nil && a.detail.find.Focused()
	case modeForm:
		return a.form != nil && a.form.typing()
	case modeRemote:
		return true
	default:
		return false
	}
}

func (a *App) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// q never hard-quits out of the editor — unsaved work goes through the
	// form's own discard confirmation instead
	if msg.String() == "ctrl+c" || (msg.String() == "q" && !a.typing() && a.mode != modeForm) {
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

// issueSearchReset is issueSearch for query *changes*: the cursor snaps to
// the top when the results land. Plain refresh keeps the cursor.
func (a *App) issueSearchReset() tea.Cmd {
	a.list.pendingReset = true
	return a.issueSearch()
}

func (a *App) updateListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.list.search.Focused() {
		switch msg.String() {
		case "esc":
			a.list.search.Blur()
			return a, nil
		case "enter", "tab": // hand focus back to the list scroll
			a.list.search.Blur()
			return a, a.issueSearchReset()
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
	case key.Matches(msg, a.keys.Back):
		// esc closes the topmost thing, never the TUI: expanded help
		// first, then the active search filter
		switch {
		case a.help.ShowAll:
			a.help.ShowAll = false
		case a.list.search.Value() != "":
			a.list.search.SetValue("")
			return a, a.issueSearchReset()
		}
		return a, nil
	case key.Matches(msg, a.keys.HalfDown), key.Matches(msg, a.keys.HalfUp):
		if n := len(a.list.results.Items()); n > 0 {
			half := max(1, a.list.results.Paginator.PerPage/2)
			if key.Matches(msg, a.keys.HalfDown) {
				a.list.results.Select(min(n-1, a.list.results.Index()+half))
			} else {
				a.list.results.Select(max(0, a.list.results.Index()-half))
			}
		}
		return a, nil
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
		return a, a.issueSearchReset()
	case key.Matches(msg, a.keys.Dead):
		a.list.includeDead = !a.list.includeDead
		return a, a.issueSearchReset()
	case key.Matches(msg, a.keys.Recent):
		a.list.recent = !a.list.recent
		return a, a.issueSearchReset()
	case key.Matches(msg, a.keys.Sort):
		a.list.cycleSort()
		a.toast("sort: "+a.list.sort.label(), false)
		return a, a.issueSearchReset()
	case key.Matches(msg, a.keys.Review):
		a.list.review = !a.list.review
		return a, a.issueSearchReset()
	case key.Matches(msg, a.keys.Remote):
		a.remote = newRemoteModel(a.st, a.width)
		a.mode = modeRemote
	case key.Matches(msg, a.keys.Refresh):
		return a, a.issueSearch()
	case key.Matches(msg, a.keys.Help):
		a.help.ShowAll = !a.help.ShowAll
	default:
		prevPage := a.list.results.Paginator.Page
		var cmd tea.Cmd
		a.list.results, cmd = a.list.results.Update(msg)
		// explicit paging lands the cursor at the top of the new page;
		// j/k crossing a boundary is not paging and keeps its position
		if page := a.list.results.Paginator.Page; page != prevPage &&
			(key.Matches(msg, a.list.results.KeyMap.NextPage) || key.Matches(msg, a.list.results.KeyMap.PrevPage)) {
			a.list.results.Select(page * a.list.results.Paginator.PerPage)
		}
		return a, cmd
	}
	return a, nil
}

func (a *App) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := a.detail
	if d.find.Focused() {
		switch msg.String() {
		case "esc", "enter", "tab": // back to scrolling; terms stay lit
			d.find.Blur()
			return a, nil
		}
		before := d.find.Value()
		var cmd tea.Cmd
		d.find, cmd = d.find.Update(msg)
		if d.find.Value() != before {
			d.refresh()
		}
		return a, cmd
	}

	switch {
	case key.Matches(msg, a.keys.Back):
		switch { // esc peels one layer, same ladder as the list
		case a.help.ShowAll:
			a.help.ShowAll = false
		case d.find.Value() != "":
			d.find.SetValue("")
			d.refresh()
		default:
			a.detail = nil
			a.mode = modeList
		}
		return a, nil
	case key.Matches(msg, a.keys.Find):
		d.find.Focus()
		d.refresh()
		return a, nil
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

// closeForm leaves the editor for wherever it was opened from.
func (a *App) closeForm() {
	returnTo := a.form.returnTo
	a.form = nil
	if returnTo == modeDetail && a.detail != nil {
		a.mode = modeDetail
	} else {
		a.mode = modeList
	}
}

// requestDiscard warns when edits would be lost; a clean form just closes.
func (a *App) requestDiscard() {
	if a.form.dirty() {
		a.form.confirmAct = confirmDiscard
		return
	}
	a.closeForm()
}

func (a *App) updateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := a.form

	// a pending confirmation intercepts everything: y commits, all else cancels
	if f.confirmAct != confirmNone {
		act := f.confirmAct
		f.confirmAct = confirmNone
		if msg.String() != "y" {
			return a, nil
		}
		if act == confirmDiscard {
			a.closeForm()
			return a, nil
		}
		in, err := f.input()
		if err != nil {
			f.fieldErr, f.errField = err.Error(), fieldTTL
			f.focus = fieldTTL
			f.startEdit()
			return a, nil
		}
		a.inFlight++
		return a, storeCmd(a.ctx, a.svc, in)
	}

	if msg.String() == "ctrl+s" { // save from any state
		f.confirmAct = confirmSave
		f.stopEdit()
		return a, nil
	}

	switch f.state {
	case formFind:
		switch msg.String() {
		case "esc", "enter", "tab": // terms stay lit, back to the fields
			f.stopEdit()
			return a, nil
		}
		var cmd tea.Cmd
		f.find, cmd = f.find.Update(msg)
		return a, cmd

	case formEdit:
		switch msg.String() {
		case "esc": // done typing — back to navigation
			f.stopEdit()
			return a, nil
		case "tab":
			f.move(1)
			f.startEdit()
			return a, nil
		case "shift+tab":
			f.move(-1)
			f.startEdit()
			return a, nil
		}
		return a, f.update(msg)

	default: // formNav
		switch msg.String() {
		case "esc":
			if f.find.Value() != "" {
				f.find.SetValue("")
				return a, nil
			}
			a.requestDiscard()
			return a, nil
		case "j", "down", "tab":
			f.move(1)
			return a, nil
		case "k", "up", "shift+tab":
			f.move(-1)
			return a, nil
		case "enter":
			f.startEdit()
			return a, nil
		case "/":
			f.state = formFind
			f.find.Focus()
			return a, nil
		case "h", "l", "left", "right", " ":
			if f.focus == fieldKind { // kind cycles in place, no edit state
				return a, f.update(msg)
			}
			return a, nil
		case "d", "q":
			a.requestDiscard()
			return a, nil
		}
		return a, nil
	}
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

// modeHelp picks the footer bindings for the current screen, so the bottom
// panel always shows what is actually pressable.
func (a *App) modeHelp() help.KeyMap {
	k := a.keys
	blur := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "to list"))
	apply := key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("enter/tab", "apply + to list"))

	switch a.mode {
	case modeDetail:
		if a.detail != nil && a.detail.find.Focused() {
			return helpSet{short: []key.Binding{apply, blur}}
		}
		return helpSet{
			short: []key.Binding{k.Back, k.Find, k.RateUp, k.Supersede, k.Delete, k.Help, k.Quit},
			full: [][]key.Binding{
				{k.Back, k.Find, k.Follow},
				{k.RateUp, k.RateDown, k.Supersede, k.Delete},
				{k.HalfDown, k.HalfUp, k.Quit},
			},
		}
	case modeForm:
		f := a.form
		edit := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit field"))
		move := key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "fields"))
		done := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "done editing"))
		discard := key.NewBinding(key.WithKeys("esc", "d"), key.WithHelp("esc/d", "discard"))
		switch {
		case f != nil && f.confirmAct != confirmNone:
			cancel := key.NewBinding(key.WithKeys("esc"), key.WithHelp("any key", "cancel"))
			return helpSet{short: []key.Binding{k.Confirm, cancel}}
		case f != nil && f.state == formEdit:
			return helpSet{short: []key.Binding{done, k.Submit}}
		case f != nil && f.state == formFind:
			return helpSet{short: []key.Binding{apply, blur}}
		default:
			return helpSet{short: []key.Binding{move, edit, k.Find, k.Submit, discard}}
		}
	case modeConfirm:
		cancel := key.NewBinding(key.WithKeys("esc"), key.WithHelp("any key", "cancel"))
		return helpSet{short: []key.Binding{k.Confirm, cancel}}
	case modeRemote:
		return helpSet{short: []key.Binding{k.Submit, k.Unset, k.Back}}
	default:
		if a.list.search.Focused() {
			return helpSet{short: []key.Binding{apply, blur}}
		}
		return k
	}
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
		"\n" + status + "\n" + a.help.View(a.modeHelp())
}
