package config

import (
	_ "embed"
	"errors"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Template is the commented default configuration written by `imux config init`.
//
//go:embed default.toml
var Template string

// ErrExists is returned by WriteTemplate when the file exists and force is false.
var ErrExists = errors.New("config file already exists")

// WriteTemplate writes Template to path (0600). It refuses to overwrite an
// existing file unless force is set.
func WriteTemplate(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return ErrExists
		}
	}
	return paths.WriteFile(path, []byte(Template))
}
