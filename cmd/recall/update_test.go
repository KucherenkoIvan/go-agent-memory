package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.1.0", false},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.9", "v0.1.10", true},
		{"v1.0.0", "v0.9.9", false},
		{"dev", "v0.1.0", true},         // unknown local — always offer
		{"(devel)", "v0.1.0", true},     //
		{"v0.1.0", "dev", false},        // unknown remote — never offer
		{"v0.1.0-rc1", "v0.1.0", false}, // prerelease suffix ignored
		{"v0.1.0", "not-a-version", false},
	}
	for _, tc := range cases {
		if got := semverLess(tc.a, tc.b); got != tc.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// fakeGitHub serves the three endpoints update touches.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/tags", func(w http.ResponseWriter, _ *http.Request) {
		// deliberately unsorted + one junk tag
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"name": "v0.2.0"}, {"name": "milestone-alpha"}, {"name": "v0.10.0"}, {"name": "v0.9.1"},
		})
	})
	mux.HandleFunc("/compare/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "v0.2.0...v0.10.0") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commits": []map[string]any{
				{"commit": map[string]string{"message": "feat: one\n\nbody"}},
				{"commit": map[string]string{"message": "fix: two"}},
			},
		})
	})
	mux.HandleFunc("/commits", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"commit": map[string]string{"message": "feat: recent"}},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestLatestTag_PicksSemverMax(t *testing.T) {
	gh := fakeGitHub(t)
	latest, err := latestTag(context.Background(), gh.URL)
	if err != nil {
		t.Fatal(err)
	}
	if latest != "v0.10.0" {
		t.Fatalf("latest = %q, want v0.10.0 (numeric, not lexicographic)", latest)
	}
}

func TestChangelog(t *testing.T) {
	gh := fakeGitHub(t)

	lines, err := changelog(context.Background(), gh.URL, "v0.2.0", "v0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "feat: one" || lines[1] != "fix: two" {
		t.Fatalf("compare changelog: %v", lines)
	}

	lines, err = changelog(context.Background(), gh.URL, "dev", "v0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || !strings.Contains(lines[0], "unknown") || lines[1] != "feat: recent" {
		t.Fatalf("dev changelog: %v", lines)
	}
}

// runForTest wires a bare command with the --output persistent flag.
func runForTest(t *testing.T, version, apiBase string, stdin string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "update", RunE: func(cmd *cobra.Command, _ []string) error {
		return runUpdate(cmd, version, apiBase, false)
	}}
	cmd.Flags().String("output", "json", "")
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err := cmd.Execute()
	return out.String(), err
}

func TestRunUpdate_UpToDate(t *testing.T) {
	gh := fakeGitHub(t)
	out, err := runForTest(t, "v0.10.0", gh.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"updateAvailable": false`) {
		t.Fatalf("output: %s", out)
	}
}

func TestRunUpdate_NonInteractiveReportsWithoutInstalling(t *testing.T) {
	gh := fakeGitHub(t)
	// test stdout is not a TTY — the command must report, prompt nothing,
	// install nothing
	out, err := runForTest(t, "v0.2.0", gh.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"updateAvailable": true`) || !strings.Contains(out, "feat: one") {
		t.Fatalf("output: %s", out)
	}
	if !strings.Contains(out, "--yes") {
		t.Fatalf("hint missing: %s", out)
	}
}
