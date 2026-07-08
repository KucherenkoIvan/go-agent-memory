package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// highlightFuzzy re-renders plain text with the characters any term
// fuzzy-matches in the hit style and everything else in base. plain must be
// unstyled — byte offsets from the matcher index into it directly. Empty
// terms (or no match) render base only, which is how highlights clear when
// the search text clears.
func highlightFuzzy(plain string, terms []string, base, hit lipgloss.Style) string {
	matched := matchedBytes(plain, terms)
	if len(matched) == 0 {
		return base.Render(plain)
	}

	var b strings.Builder
	var seg strings.Builder
	segHit := false
	flush := func() {
		if seg.Len() == 0 {
			return
		}
		if segHit {
			b.WriteString(hit.Render(seg.String()))
		} else {
			b.WriteString(base.Render(seg.String()))
		}
		seg.Reset()
	}
	for i := 0; i < len(plain); {
		_, size := utf8.DecodeRuneInString(plain[i:])
		if matched[i] != segHit {
			flush()
			segHit = matched[i]
		}
		seg.WriteString(plain[i : i+size])
		i += size
	}
	flush()
	return b.String()
}

// matchedBytes unions the byte offsets each term fuzzy-matches in plain.
func matchedBytes(plain string, terms []string) map[int]bool {
	matched := map[int]bool{}
	for _, term := range terms {
		if term == "" {
			continue
		}
		for _, m := range fuzzy.Find(term, []string{plain}) {
			for _, idx := range m.MatchedIndexes {
				matched[idx] = true
			}
		}
	}
	return matched
}
