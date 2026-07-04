package tui

import (
	"reflect"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/memories/application/ports"
)

func TestParseQuery(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want ports.SearchFilters
	}{
		"empty":     {"", ports.SearchFilters{}},
		"free text": {"sqlite locking", ports.SearchFilters{Query: "sqlite locking"}},
		"keywords": {"k:project:kernel k:lint", ports.SearchFilters{
			Keywords: []string{"project:kernel", "lint"},
		}},
		"kind": {"kind:research", ports.SearchFilters{Kind: "research"}},
		"mixed": {"config k:GO kind:fact parsing", ports.SearchFilters{
			Query: "config parsing", Keywords: []string{"go"}, Kind: "fact",
		}},
		"bare prefixes stay text": {"k: kind:", ports.SearchFilters{Query: "k: kind:"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ParseQuery(tc.raw)
			if got.Query != tc.want.Query || got.Kind != tc.want.Kind ||
				!reflect.DeepEqual(got.Keywords, tc.want.Keywords) {
				t.Fatalf("ParseQuery(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}
