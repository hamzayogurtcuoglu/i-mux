package cli

import (
	"errors"
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

func init() {
	register(&command{
		Name:    "config",
		Args:    "[init [--force] | path | show]",
		Summary: "cmd.config",
		Run:     runConfig,
	})
}

// loadConfig loads the user's config and prints warnings to stderr.
func loadConfig(e *Env) (*config.Config, error) {
	cfg, warns, err := config.Load(paths.Config())
	for _, w := range warns {
		fmt.Fprintf(e.Stderr, "ajan: %s\n", i18n.T("cli.warning", w))
	}
	if cfg != nil {
		i18n.Init(cfg.Locale)
	}
	return cfg, err
}

func runConfig(e *Env, args []string) error {
	sub := "show"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "path":
		fmt.Fprintln(e.Stdout, paths.Config())
		return nil
	case "init":
		fs := newFlags(e, "config")
		force := fs.Bool("force", false, "overwrite an existing config file")
		if _, err := parseFlags(fs, args); err != nil {
			return err
		}
		if err := paths.Ensure(); err != nil {
			return err
		}
		err := config.WriteTemplate(paths.Config(), *force)
		if errors.Is(err, config.ErrExists) {
			return errors.New(i18n.T("config.exists", paths.Config()))
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, i18n.T("config.wrote", paths.Config()))
		return nil
	case "show":
		cfg, err := loadConfig(e)
		if err != nil {
			return err
		}
		b, err := cfg.Encode()
		if err != nil {
			return err
		}
		_, err = e.Stdout.Write(b)
		return err
	default:
		return &UsageError{Msg: i18n.T("config.unknown_sub", sub)}
	}
}
