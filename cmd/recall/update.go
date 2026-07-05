package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/cliout"
)

// Self-update lives in the composition root: it manages the binary itself,
// not memories. Update source is the repo's tags; the mechanism is
// `go install <module>@<tag>` (this is a developer's tool — a Go toolchain
// is assumed).
const (
	repoAPI     = "https://api.github.com/repos/KucherenkoIvan/go-agent-memory"
	installPath = "github.com/KucherenkoIvan/go-agent-memory/cmd/recall"
)

func updateCmd(version string) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check the repo for a newer release and install it",
		Long: `Check the repository's latest tag against this binary's version.

Interactive terminals get the changelog and a confirmation prompt;
non-interactive callers get JSON status only (pass --yes to install).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd, version, repoAPI, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install without prompting")
	return cmd
}

func runUpdate(cmd *cobra.Command, current, apiBase string, yes bool) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	latest, err := latestTag(ctx, apiBase)
	if err != nil {
		return err
	}

	mode := outputMode(cmd)
	newer := semverLess(current, latest)
	if !newer {
		return cliout.Emit(cmd.OutOrStdout(), mode,
			map[string]any{"current": current, "latest": latest, "updateAvailable": false},
			fmt.Sprintf("%s is up to date (latest: %s)", current, latest))
	}

	changes, err := changelog(ctx, apiBase, current, latest)
	if err != nil {
		changes = []string{"(changelog unavailable: " + err.Error() + ")"}
	}

	interactive := term.IsTerminal(int(os.Stdout.Fd()))
	if !yes {
		if !interactive {
			// agent contract: never prompt — report and let the caller decide
			return cliout.Emit(cmd.OutOrStdout(), mode, map[string]any{
				"current": current, "latest": latest,
				"updateAvailable": true, "changelog": changes,
				"hint": "run `recall update --yes` to install",
			}, "")
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "recall %s → %s\n\nchanges:\n", current, latest)
		for _, line := range changes {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  · "+line)
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "\nupdate now? [y/N] ")
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "not updating")
			return nil
		}
	}

	install := exec.CommandContext(cmd.Context(), "go", "install", installPath+"@"+latest)
	install.Env = append(os.Environ(), "CGO_ENABLED=0")
	install.Stdout = cmd.ErrOrStderr() // progress is diagnostics, not output
	install.Stderr = cmd.ErrOrStderr()
	if err := install.Run(); err != nil {
		return fmt.Errorf("go install %s@%s: %w", installPath, latest, err)
	}
	return cliout.Emit(cmd.OutOrStdout(), mode,
		map[string]any{"ok": true, "installed": latest},
		"updated to "+latest+" — restart long-running faces (mcp, serve, tui) to pick it up")
}

// outputMode reads the root --output persistent flag when present.
func outputMode(cmd *cobra.Command) string {
	if f := cmd.Flag("output"); f != nil {
		return f.Value.String()
	}
	return "auto"
}

// latestTag returns the highest semver v-tag of the repo.
func latestTag(ctx context.Context, apiBase string) (string, error) {
	var tags []struct {
		Name string `json:"name"`
	}
	if err := getJSON(ctx, apiBase+"/tags?per_page=100", &tags); err != nil {
		return "", fmt.Errorf("checking latest release: %w", err)
	}

	latest := ""
	for _, tag := range tags {
		if parseSemver(tag.Name) == nil {
			continue
		}
		if latest == "" || semverLess(latest, tag.Name) {
			latest = tag.Name
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no release tags found in the repository")
	}
	return latest, nil
}

// changelog lists commit subjects between the current version and the
// latest tag; when the current version is unknown (dev build), it shows
// the latest tag's recent history instead.
func changelog(ctx context.Context, apiBase, current, latest string) ([]string, error) {
	if parseSemver(current) != nil {
		var cmp struct {
			Commits []struct {
				Commit struct {
					Message string `json:"message"`
				} `json:"commit"`
			} `json:"commits"`
		}
		if err := getJSON(ctx, apiBase+"/compare/"+current+"..."+latest, &cmp); err != nil {
			return nil, err
		}
		lines := make([]string, 0, len(cmp.Commits))
		for _, c := range cmp.Commits {
			lines = append(lines, strings.SplitN(c.Commit.Message, "\n", 2)[0])
		}
		return lines, nil
	}

	var commits []struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := getJSON(ctx, apiBase+"/commits?sha="+latest+"&per_page=10", &commits); err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(commits)+1)
	lines = append(lines, "(current version unknown — recent changes up to "+latest+")")
	for _, c := range commits {
		lines = append(lines, strings.SplitN(c.Commit.Message, "\n", 2)[0])
	}
	return lines, nil
}

// githubToken makes private repositories reachable: explicit env first,
// then whatever the gh CLI is logged in with. Empty means anonymous.
func githubToken(ctx context.Context) string {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(key); token != "" {
			return token
		}
	}
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := githubToken(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// parseSemver reads v<major>.<minor>.<patch>; nil when it is not one
// (dev builds, pseudo-versions).
func parseSemver(v string) []int {
	raw, ok := strings.CutPrefix(v, "v")
	if !ok {
		return nil
	}
	raw, _, _ = strings.Cut(raw, "-") // ignore prerelease/pseudo suffix
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		nums[i] = n
	}
	return nums
}

// semverLess reports whether a < b. Unknown versions (dev builds) count as
// older than any release — an update is always on offer for them.
func semverLess(a, b string) bool {
	av, bv := parseSemver(a), parseSemver(b)
	if bv == nil {
		return false
	}
	if av == nil {
		return true
	}
	for i := range 3 {
		if av[i] != bv[i] {
			return av[i] < bv[i]
		}
	}
	return false
}
