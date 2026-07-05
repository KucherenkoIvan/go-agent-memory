package server

import (
	"github.com/KucherenkoIvan/go-kernel/config"

	"github.com/KucherenkoIvan/go-agent-memory/internal/shared/infra/storage"
)

// Config is hosted mode's knobs; env first, cobra flags override in cmd.
type Config struct {
	Addr string `env:"RECALL_ADDR" default:":7846"`
	// Dir holds keys.db and spaces/; empty resolves to the default below.
	Dir string `env:"RECALL_SERVER_DIR"`
}

func LoadConfig() (Config, error) {
	cfg, err := config.Load[Config]()
	if err != nil {
		return Config{}, err
	}
	if cfg.Dir == "" {
		if cfg.Dir, err = storage.DefaultServerDir(); err != nil {
			return Config{}, err
		}
	}
	return *cfg, nil
}
