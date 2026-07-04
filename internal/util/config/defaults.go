// Package config provides the default configuration values.
// Types and loading live in github.com/termizard/termizard/internal/config.
package config

import icfg "github.com/termizard/termizard/internal/config"

// Defaults returns the out-of-the-box configuration.
// See internal/config/defaults.go for the full JetBrains Darcula theme.
func Defaults() *icfg.Config {
	return icfg.Defaults()
}
