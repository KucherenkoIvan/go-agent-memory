package main

import (
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/cliout"
)

// versionCmd reports what binary this is: the resolved version plus the
// VCS revision Go embedded, when available. (`recall --version` prints the
// short form; this one is the machine-readable face.)
func versionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			revision, modified := buildRevision()

			text := "recall " + version
			payload := map[string]any{"version": version}
			if revision != "" {
				payload["revision"] = revision
				payload["modified"] = modified
				text += " (" + revision
				if modified {
					text += ", modified"
				}
				text += ")"
			}
			return cliout.Emit(cmd.OutOrStdout(), outputMode(cmd), payload, text)
		},
	}
}

// buildRevision digs the VCS commit out of the embedded build info.
func buildRevision() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) >= 12 {
				revision = setting.Value[:12]
			} else {
				revision = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "", false
	}
	return revision, modified
}
