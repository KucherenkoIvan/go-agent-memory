package tui

import "github.com/charmbracelet/x/ansi"

// confirmModel is the y/N modal for the one destructive action.
type confirmModel struct {
	st       *styles
	id       string
	summary  string
	returnTo mode
}

func (c *confirmModel) view(width int) string {
	w := min(64, max(30, width-8))
	body := "Delete " + c.st.title.Render(ansi.Truncate(c.summary, w-10, "…")) + "?\n\n" +
		c.st.errText.Render("This is permanent — agents correct with supersede instead.") + "\n\n" +
		"[y] delete    [any other key] cancel"
	return c.st.modal.Width(w).Render(body)
}
