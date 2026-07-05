package remotecfg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/remotecfg"
)

func testPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "remote.json")
	t.Setenv("RECALL_REMOTE_CONFIG", path)
	t.Setenv("RECALL_REMOTE_ADDR", "")
	t.Setenv("RECALL_API_KEY", "")
	return path
}

func TestSaveLoadRemove(t *testing.T) {
	path := testPath(t)

	if cfg, err := remotecfg.Load(); err != nil || cfg != nil {
		t.Fatalf("absent config: %v %v", cfg, err)
	}

	want := remotecfg.Config{Addr: "memory.example:7846", APIKey: "rcl_secret"}
	if err := remotecfg.Save(want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config holds a secret — mode %o, want 600", info.Mode().Perm())
	}

	got, err := remotecfg.Load()
	if err != nil || got == nil || *got != want {
		t.Fatalf("load: %+v, %v", got, err)
	}

	if err := remotecfg.Remove(); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := remotecfg.Load(); cfg != nil {
		t.Fatalf("config survived remove: %+v", cfg)
	}
	if err := remotecfg.Remove(); err != nil {
		t.Fatalf("removing absent config must be fine: %v", err)
	}
}

func TestResolve_EnvOverridesFile(t *testing.T) {
	testPath(t)

	// nothing anywhere → local mode
	if cfg, err := remotecfg.Resolve(); err != nil || cfg != nil {
		t.Fatalf("empty resolve: %+v, %v", cfg, err)
	}

	_ = remotecfg.Save(remotecfg.Config{Addr: "file-addr:1", APIKey: "rcl_file"})

	cfg, err := remotecfg.Resolve()
	if err != nil || cfg.Addr != "file-addr:1" || cfg.APIKey != "rcl_file" {
		t.Fatalf("file resolve: %+v, %v", cfg, err)
	}

	// env wins per-field
	t.Setenv("RECALL_REMOTE_ADDR", "env-addr:2")
	cfg, err = remotecfg.Resolve()
	if err != nil || cfg.Addr != "env-addr:2" || cfg.APIKey != "rcl_file" {
		t.Fatalf("env-addr resolve: %+v, %v", cfg, err)
	}
	t.Setenv("RECALL_API_KEY", "rcl_env")
	if cfg, _ = remotecfg.Resolve(); cfg.APIKey != "rcl_env" {
		t.Fatalf("env-key resolve: %+v", cfg)
	}
}

func TestResolve_AddrWithoutKeyErrors(t *testing.T) {
	testPath(t)
	t.Setenv("RECALL_REMOTE_ADDR", "somewhere:7846")

	if _, err := remotecfg.Resolve(); err == nil {
		t.Fatal("addr without key must error, not silently fall back to local")
	}
}
