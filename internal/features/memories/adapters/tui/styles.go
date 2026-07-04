package tui

import "github.com/charmbracelet/lipgloss"

// styles: one accent, one dim, adaptive to light/dark terminals. Kind
// badges get muted hues so the list scans by shape, not by rainbow.
type styles struct {
	accent   lipgloss.Style
	dim      lipgloss.Style
	title    lipgloss.Style
	selected lipgloss.Style
	errText  lipgloss.Style
	okText   lipgloss.Style
	badge    map[string]lipgloss.Style
	modal    lipgloss.Style
	statusBg lipgloss.Style
	focused  lipgloss.Style
	blurred  lipgloss.Style
}

func newStyles() *styles {
	accent := lipgloss.AdaptiveColor{Light: "63", Dark: "111"}
	dim := lipgloss.AdaptiveColor{Light: "244", Dark: "241"}

	badge := func(light, dark string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: light, Dark: dark})
	}

	return &styles{
		accent:   lipgloss.NewStyle().Foreground(accent),
		dim:      lipgloss.NewStyle().Foreground(dim),
		title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		selected: lipgloss.NewStyle().Bold(true).Foreground(accent),
		errText:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}),
		okText:   lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "78"}),
		badge: map[string]lipgloss.Style{
			"fact":       badge("28", "78"),
			"preference": badge("130", "215"),
			"research":   badge("63", "111"),
			"decision":   badge("160", "203"),
			"location":   badge("30", "80"),
			"reference":  badge("96", "140"),
		},
		modal:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2),
		statusBg: lipgloss.NewStyle().Foreground(dim),
		focused:  lipgloss.NewStyle().Foreground(accent),
		blurred:  lipgloss.NewStyle().Foreground(dim),
	}
}

func (s *styles) kindBadge(kind string) string {
	if style, ok := s.badge[kind]; ok {
		return style.Render("[" + kind + "]")
	}
	return s.dim.Render("[" + kind + "]")
}
