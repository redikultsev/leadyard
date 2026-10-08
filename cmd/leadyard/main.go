// Command leadyard runs the developer loop with evidence levels computed from
// artifacts. See docs/design.md.
package main

import (
	"os"

	"github.com/zireaelq/leadyard/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
