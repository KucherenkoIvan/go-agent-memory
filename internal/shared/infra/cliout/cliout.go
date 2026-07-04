// Package cliout single-sources the CLI output contract every command obeys:
// stable JSON on non-TTY stdout (or --output json), human text otherwise.
// Changing this shape breaks other agents — treat like a public API.
package cliout

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// JSONOutput resolves an output mode flag: "json", "text", or "auto"
// (machines get JSON, humans get text).
func JSONOutput(mode string) bool {
	switch mode {
	case "json":
		return true
	case "text":
		return false
	default:
		return !term.IsTerminal(int(os.Stdout.Fd()))
	}
}

// Emit writes jsonValue as indented JSON or the text line, per mode.
func Emit(w io.Writer, mode string, jsonValue any, text string) error {
	if JSONOutput(mode) {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonValue)
	}
	_, err := fmt.Fprintln(w, text)
	return err
}
