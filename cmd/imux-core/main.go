// Command imux-core is the i-mux-core engine (D83), built from go.mod's pin of
// its module so that releases ship it beside imux (T88.4). It is the engine's
// own main, as its repository builds it.
package main

import (
	"os"

	"github.com/hamzayogurtcuoglu/i-mux-core/engine"
)

func main() {
	os.Exit(engine.Main(os.Args[1:], os.Stdout, os.Stderr))
}
