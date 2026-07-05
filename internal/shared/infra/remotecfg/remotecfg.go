// Package remotecfg persists the client's remote endpoint: where a hosted
// recall lives and the API key that unlocks it. When a config resolves, the
// composition root swaps the local Service for the gRPC client — every face
// follows automatically.
package remotecfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the persisted shape (remote.json, 0600 — it holds a secret).
type Config struct {
	Addr   string `json:"addr"`
	APIKey string `json:"apiKey"`
}

// Path resolves the config file location: RECALL_REMOTE_CONFIG overrides,
// else <user-config-dir>/recall/remote.json.
func Path() (string, error) {
	if override := os.Getenv("RECALL_REMOTE_CONFIG"); override != "" {
		return override, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("remotecfg: resolving config dir: %w", err)
	}
	return filepath.Join(dir, "recall", "remote.json"), nil
}

// Load reads the persisted config; (nil, nil) when none exists.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("remotecfg: reading %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("remotecfg: parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// Save persists the config with owner-only permissions.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("remotecfg: creating config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("remotecfg: encoding: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("remotecfg: writing %s: %w", path, err)
	}
	return nil
}

// Remove deletes the persisted config; removing a non-existent one is fine.
func Remove() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remotecfg: removing %s: %w", path, err)
	}
	return nil
}

// Resolve merges environment over the persisted file per field
// (RECALL_REMOTE_ADDR, RECALL_API_KEY — real env wins, kernel config
// philosophy). nil means local mode; an addr without a key is an error.
func Resolve() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{}
	}
	if addr := os.Getenv("RECALL_REMOTE_ADDR"); addr != "" {
		cfg.Addr = addr
	}
	if key := os.Getenv("RECALL_API_KEY"); key != "" {
		cfg.APIKey = key
	}

	if cfg.Addr == "" {
		return nil, nil
	}
	if cfg.APIKey == "" {
		return nil, errors.New("remotecfg: remote addr configured without an API key — run `recall remote set <addr> <key>` or set RECALL_API_KEY")
	}
	return cfg, nil
}
