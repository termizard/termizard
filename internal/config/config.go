package config

import (
	"flag"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

// EnvDebug define environment names.
const (
	EnvDebug = "debug"
)

// TerminalConfig holds terminal settings.
type TerminalConfig struct {
	Rows int `toml:"rows" env-default:"24"`
	Cols int `toml:"cols" env-default:"80"`
}

// Config holds application configuration.
type Config struct {
	Env      string         `toml:"env" env-default:""`
	Terminal TerminalConfig `toml:"terminal"`
}

// MustLoad loads config from the default path or panics.
func MustLoad() *Config {
	path := fetchConfigPath()

	if path == "" {
		panic("config file path is empty")
	}

	return MustLoadByPath(path)
}

func MustLoadByPath(configPath string) *Config {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		panic("config file does not exist: " + configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}
	return &cfg
}

// fetchConfigPath resolves the config path from flags or env.
func fetchConfigPath() string {
	var res string

	flag.StringVar(&res, "config", "", "path to config file")
	flag.Parse()

	if res == "" {
		res = os.Getenv("CONFIG_PATH")
	}

	return res
}
