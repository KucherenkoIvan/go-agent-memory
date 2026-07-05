package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys"
	apikeyscli "github.com/KucherenkoIvan/go-agent-memory/internal/features/apikeys/adapters/cli"
	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

func run(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--output", "json"))
	err := cmd.Execute()
	return out.String(), err
}

// withOutput mimics the root command's persistent --output flag.
func withOutput(cmd *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "recall", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "auto", "")
	root.AddCommand(cmd)
	return root
}

func testConnect(t *testing.T, dir string) apikeyscli.Connect {
	t.Helper()
	return func(ctx context.Context, resolvedDir string) (apikeys.Service, func(), error) {
		if resolvedDir != dir {
			t.Errorf("dir resolution: got %q want %q", resolvedDir, dir)
		}
		store, err := storage.OpenServer(ctx, filepath.Join(resolvedDir, "keys.db"))
		if err != nil {
			return nil, nil, err
		}
		return apikeys.New(store.DB), func() { _ = store.Close() }, nil
	}
}

func TestKeysCLI_CreateListRevoke(t *testing.T) {
	dir := t.TempDir()
	connect := testConnect(t, dir)

	// create prints the raw key exactly once
	out, err := run(t, withOutput(apikeyscli.NewKeysCmd(connect)),
		"keys", "create", "--name", "laptop", "--space", "team-x", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	var created struct{ ID, Key, Space, Prefix string }
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("create output not JSON: %v\n%s", err, out)
	}
	if !strings.HasPrefix(created.Key, "rcl_") || created.Space != "team-x" {
		t.Fatalf("create: %+v", created)
	}

	// list never leaks the raw key or a hash
	out, err = run(t, withOutput(apikeyscli.NewKeysCmd(connect)), "keys", "list", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, created.Key) {
		t.Fatal("keys list leaks the raw key")
	}
	if !strings.Contains(out, created.Prefix) || !strings.Contains(out, created.ID) {
		t.Fatalf("keys list missing view fields:\n%s", out)
	}

	if _, err = run(t, withOutput(apikeyscli.NewKeysCmd(connect)), "keys", "revoke", created.ID, "--dir", dir); err != nil {
		t.Fatal(err)
	}
	out, _ = run(t, withOutput(apikeyscli.NewKeysCmd(connect)), "keys", "list", "--dir", dir)
	if !strings.Contains(out, "revokedAt") {
		t.Fatalf("revoked key must show revokedAt:\n%s", out)
	}
}

func TestSpacesCLI_ListAndExport(t *testing.T) {
	dir := t.TempDir()
	connect := testConnect(t, dir)

	if _, err := run(t, withOutput(apikeyscli.NewKeysCmd(connect)),
		"keys", "create", "--name", "k", "--space", "demo", "--dir", dir); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, withOutput(apikeyscli.NewSpacesCmd(connect, testExport(t))), "spaces", "list", "--dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"demo"`) || !strings.Contains(out, `"keys": 1`) {
		t.Fatalf("spaces list:\n%s", out)
	}

	// export goes through the closure with validated space + resolved dir
	dest := filepath.Join(dir, "out.db")
	if _, err := run(t, withOutput(apikeyscli.NewSpacesCmd(connect, testExport(t))),
		"spaces", "export", "demo", dest, "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, withOutput(apikeyscli.NewSpacesCmd(connect, testExport(t))),
		"spaces", "export", "../evil", dest, "--dir", dir); err == nil {
		t.Fatal("unsafe space name must be rejected before the export closure")
	}
}

func testExport(t *testing.T) apikeyscli.Export {
	t.Helper()
	return func(ctx context.Context, dir, space, dest string) error {
		// stand in for storage.ExportSnapshot: create the space db, then copy
		src := filepath.Join(dir, "spaces", space+".db")
		store, err := storage.Open(ctx, src)
		if err != nil {
			return err
		}
		defer store.Close() //nolint:errcheck // test helper
		return storage.ExportSnapshot(ctx, src, dest)
	}
}
