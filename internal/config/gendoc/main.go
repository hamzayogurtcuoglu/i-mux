// Command gendoc writes docs/CONFIG.md from the built-in default
// configuration (run by `go generate ./internal/config`).
package main

import (
	"fmt"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gendoc <output file>")
		os.Exit(2)
	}
	if err := os.WriteFile(os.Args[1], []byte(config.Doc()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
